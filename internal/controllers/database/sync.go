package database

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/cms"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

const (
	Pending      ClusterState = "Pending"
	Provisioning ClusterState = "Provisioning"
	Initializing ClusterState = "Initializing"
	Ready        ClusterState = "Ready"

	DefaultRequeueDelay             = 10 * time.Second
	StatusUpdateRequeueDelay        = 1 * time.Second
	TenantCreationRequeueDelay      = 30 * time.Second
	StorageAwaitRequeueDelay        = 60 * time.Second
	SharedDatabaseAwaitRequeueDelay = 60 * time.Second

	TenantInitializedCondition        = "TenantInitialized"
	TenantInitializedReasonInProgress = "InProgres"
	TenantInitializedReasonCompleted  = "Completed"

	Stop     = true
	Continue = false
)

var ErrIncorrectDatabaseResourcesConfiguration = errors.New("incorrect database resources configuration, " +
	"must be one of: Resources, SharedResources, ServerlessResources")

type ClusterState string

func (r *Reconciler) Sync(ctx context.Context, ydbCr *v1alpha1.Database) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	database := resources.NewDatabase(ydbCr)
	database.SetStatusOnFirstReconcile()

	stop, result, err = r.waitForClusterResources(ctx, &database)
	if stop {
		return result, err
	}
	stop, result, err = r.handleResourcesSync(ctx, &database)
	if stop {
		return result, err
	}
	stop, result, err = r.waitForStatefulSetToScale(ctx, &database)
	if stop {
		return result, err
	}
	if !meta.IsStatusConditionTrue(database.Status.Conditions, TenantInitializedCondition) {
		stop, result, err = r.setInitialStatus(ctx, &database)
		if stop {
			return result, err
		}
		stop, result, err = r.handleTenantCreation(ctx, &database)
		if stop {
			return result, err
		}
	}
	return ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) waitForClusterResources(ctx context.Context, database *resources.DatabaseBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForClusterResources")
	storage := &v1alpha1.Storage{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      database.Spec.StorageClusterRef.Name,
		Namespace: database.Spec.StorageClusterRef.Namespace,
	}, storage)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"Pending",
				fmt.Sprintf(
					"Storage (%s/%s) not found.",
					database.Spec.StorageClusterRef.Name,
					database.Spec.StorageClusterRef.Namespace,
				),
			)
			return Stop, ctrl.Result{RequeueAfter: StorageAwaitRequeueDelay}, nil
		}
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"Pending",
			fmt.Sprintf(
				"Failed to get Database (%s, %s) resource, error: %s",
				database.Spec.StorageClusterRef.Name,
				database.Spec.StorageClusterRef.Namespace,
				err,
			),
		)
		return Stop, ctrl.Result{RequeueAfter: StorageAwaitRequeueDelay}, err
	}

	if storage.Status.State != string(Ready) {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"Pending",
			fmt.Sprintf(
				"Referenced storage cluster (%s, %s) in a bad state: %s != Ready",
				database.Spec.StorageClusterRef.Name,
				database.Spec.StorageClusterRef.Namespace,
				storage.Status.State,
			),
		)
		return Stop, ctrl.Result{RequeueAfter: StorageAwaitRequeueDelay}, err
	}

	database.Storage = storage

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) waitForStatefulSetToScale(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale for Database")

	if database.Spec.ServerlessResources != nil {
		return Continue, ctrl.Result{Requeue: false}, nil
	}

	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      database.Name,
		Namespace: database.Namespace,
	}, found)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			database,
			corev1.EventTypeNormal,
			"Syncing",
			fmt.Sprintf("Failed to get StatefulSets: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	podLabels := labels.Common(database.Name, make(map[string]string))
	podLabels.Merge(map[string]string{
		labels.ComponentKey: labels.DynamicComponent,
	})

	matchingLabels := client.MatchingLabels{}
	for k, v := range podLabels {
		matchingLabels[k] = v
	}

	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(database.Namespace),
		matchingLabels,
	}

	err = r.List(ctx, podList, opts...)
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeNormal,
			"Syncing",
			fmt.Sprintf("Failed to list cluster pods: %s", err),
		)
		database.Status.State = string(Provisioning)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	runningPods := 0
	for _, e := range podList.Items {
		if e.Status.Phase == "Running" {
			runningPods++
		}
	}

	if runningPods != int(database.Spec.Nodes) {
		msg := fmt.Sprintf("Waiting for number of running dynamic pods to match expected: %d != %d", runningPods, database.Spec.Nodes)
		r.Recorder.Event(database, corev1.EventTypeNormal, string(Provisioning), msg)
		return r.setState(ctx, database)
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	for _, builder := range database.GetResourceBuilders(r.Config) {
		newResource := builder.Placeholder(database)

		result, err := resources.CreateOrUpdateIgnoreStatus(ctx, r.Client, newResource, func() error {
			var err error

			err = builder.Build(newResource)
			if err != nil {
				r.Recorder.Event(
					database,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}

			err = ctrl.SetControllerReference(database.Unwrap(), newResource, r.Scheme)
			if err != nil {
				r.Recorder.Event(
					database,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Error setting controller reference for resource: %s", err),
				)
				return err
			}

			return nil
		})

		eventMessage := fmt.Sprintf(
			"Resource: %s, Namespace: %s, Name: %s",
			reflect.TypeOf(newResource),
			newResource.GetNamespace(),
			newResource.GetName(),
		)
		if err != nil {
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				eventMessage+fmt.Sprintf(", failed to sync, error: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				database,
				corev1.EventTypeNormal,
				"Provisioning",
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}
	r.Log.Info("resource sync complete")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) processSkipInitPipeline(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step processSkipInitPipeline")
	r.Log.Info("Database initialization disabled (with annotation), proceed with caution")

	r.Recorder.Event(
		database,
		corev1.EventTypeWarning,
		"SkippingInit",
		"Skipping database creation due to skip annotation present, be careful!",
	)

	return r.setInitDatabaseCompleted(
		ctx,
		database,
		"Database creation not performed because initialization is skipped",
	)
}

func (r *Reconciler) setInitialStatus(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")

	if value, ok := database.Annotations[v1alpha1.AnnotationSkipInitialization]; ok && value == v1alpha1.AnnotationValueTrue {
		if meta.FindStatusCondition(database.Status.Conditions, TenantInitializedCondition) == nil ||
			meta.IsStatusConditionFalse(database.Status.Conditions, TenantInitializedCondition) {
			return r.processSkipInitPipeline(ctx, database)
		}
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	changed := false
	if meta.FindStatusCondition(database.Status.Conditions, TenantInitializedCondition) == nil {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    TenantInitializedCondition,
			Status:  "False",
			Reason:  TenantInitializedReasonInProgress,
			Message: "Tenant creation in progress",
		})
		changed = true
	}
	if database.Status.State == string(Pending) {
		database.Status.State = string(Initializing)
		changed = true
	}
	if changed {
		return r.setState(ctx, database)
	}
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) setInitDatabaseCompleted(
	ctx context.Context,
	database *resources.DatabaseBuilder,
	message string,
) (bool, ctrl.Result, error) {
	meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
		Type:    TenantInitializedCondition,
		Status:  "True",
		Reason:  TenantInitializedReasonCompleted,
		Message: message,
	})

	database.Status.State = string(Ready)
	return r.setState(ctx, database)
}

func (r *Reconciler) handleTenantCreation(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleTenantCreation")

	path := v1alpha1.GetDatabasePath(database.Database)
	var storageUnits []v1alpha1.StorageUnit
	var shared bool
	var sharedDatabasePath string
	switch {
	case database.Spec.Resources != nil:
		storageUnits = database.Spec.Resources.StorageUnits
		shared = false
	case database.Spec.SharedResources != nil:
		storageUnits = database.Spec.SharedResources.StorageUnits
		shared = true
	case database.Spec.ServerlessResources != nil:
		sharedDatabaseCr := &v1alpha1.Database{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      database.Spec.ServerlessResources.SharedDatabaseRef.Name,
			Namespace: database.Spec.ServerlessResources.SharedDatabaseRef.Namespace,
		}, sharedDatabaseCr)
		if err != nil {
			if apierrors.IsNotFound(err) {
				r.Recorder.Event(
					database,
					corev1.EventTypeWarning,
					"Pending",
					fmt.Sprintf(
						"Database (%s/%s) not found.",
						database.Spec.ServerlessResources.SharedDatabaseRef.Name,
						database.Spec.ServerlessResources.SharedDatabaseRef.Namespace,
					),
				)
				return Stop, ctrl.Result{RequeueAfter: SharedDatabaseAwaitRequeueDelay}, nil
			}
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"Pending",
				fmt.Sprintf(
					"Failed to get Database (%s, %s) resource, error: %s",
					database.Spec.ServerlessResources.SharedDatabaseRef.Name,
					database.Spec.ServerlessResources.SharedDatabaseRef.Namespace,
					err,
				),
			)
			return Stop, ctrl.Result{RequeueAfter: SharedDatabaseAwaitRequeueDelay}, err
		}

		if sharedDatabaseCr.Status.State != "Ready" {
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"Pending",
				fmt.Sprintf(
					"Referenced shared Database (%s, %s) in a bad state: %s != Ready",
					database.Spec.ServerlessResources.SharedDatabaseRef.Name,
					database.Spec.ServerlessResources.SharedDatabaseRef.Namespace,
					sharedDatabaseCr.Status.State,
				),
			)
			return Stop, ctrl.Result{RequeueAfter: SharedDatabaseAwaitRequeueDelay}, err
		}
		sharedDatabasePath = v1alpha1.GetDatabasePath(sharedDatabaseCr)
	default:
		// TODO: move this logic to webhook
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"ControllerError",
			ErrIncorrectDatabaseResourcesConfiguration.Error(),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, ErrIncorrectDatabaseResourcesConfiguration
	}

	tenant := cms.Tenant{
		StorageEndpoint:    database.GetStorageEndpointWithProto(),
		Path:               path,
		StorageUnits:       storageUnits,
		Shared:             shared,
		SharedDatabasePath: sharedDatabasePath,
	}

	credentials, err := r.getAuthCredentials(ctx, database)
	if err != nil {
		r.Log.Error(err, "Error connecting to YDB storage %s", database.Storage.Name)
		return Stop, ctrl.Result{RequeueAfter: TenantCreationRequeueDelay}, err
	}

	err = tenant.Create(ctx, database, credentials)
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"InitializingFailed",
			fmt.Sprintf("Error creating tenant %s: %s", tenant.Path, err),
		)
		return Stop, ctrl.Result{RequeueAfter: TenantCreationRequeueDelay}, err
	}
	r.Recorder.Event(
		database,
		corev1.EventTypeNormal,
		"Initialized",
		fmt.Sprintf("Tenant %s created", tenant.Path),
	)

	r.Recorder.Event(
		database,
		corev1.EventTypeNormal,
		"DatabaseReady",
		"Database is initialized",
	)

	return r.setInitDatabaseCompleted(
		ctx,
		database,
		"Database initialization is completed",
	)
}

func (r *Reconciler) setState(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	databaseCr := &v1alpha1.Database{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: database.Namespace,
		Name:      database.Name,
	}, databaseCr)
	if err != nil {
		r.Recorder.Event(databaseCr, corev1.EventTypeWarning, "ControllerError", "Failed fetching CR before status update")
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	databaseCr.Status.State = database.Status.State
	databaseCr.Status.Conditions = database.Status.Conditions

	err = r.Status().Update(ctx, databaseCr)
	if err != nil {
		r.Recorder.Event(
			databaseCr,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("failed setting status: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
}

func (r *Reconciler) getAuthCredentials(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (ydbCredentials.Credentials, error) {
	switch auth := database.Storage.Spec.Auth; {
	case auth.AccessToken != nil:
		token, err := r.getSecretKey(
			ctx,
			database.Storage.Namespace,
			auth.AccessToken.SecretKeyRef,
		)
		if err != nil {
			return nil, err
		}
		return ydbCredentials.NewAccessTokenCredentials(token), nil
	case auth.StaticCredentials != nil:
		endpoint := database.GetStorageEndpointWithProto()
		opts := connection.GetGRPCDialOptions(resources.IsGrpcSecure(database.Storage))
		username := auth.StaticCredentials.Username
		password := v1alpha1.DefaultRootPassword
		if auth.StaticCredentials.SecretKeyRef != nil {
			var err error
			password, err = r.getSecretKey(
				ctx,
				database.Storage.Namespace,
				auth.StaticCredentials.SecretKeyRef,
			)
			if err != nil {
				return nil, err
			}
		}
		return ydbCredentials.NewStaticCredentials(username, password, endpoint, opts...), nil
	default:
		return ydbCredentials.NewAnonymousCredentials(), nil
	}
}

func (r *Reconciler) getSecretKey(
	ctx context.Context,
	namespace string,
	secretKeyRef *corev1.SecretKeySelector,
) (string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      secretKeyRef.Name,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return "", err
	}
	secretVal, exist := secret.Data[secretKeyRef.Key]
	if !exist {
		return "", fmt.Errorf(
			"key %s does not exist in secretData %s",
			secretKeyRef.Key,
			secretKeyRef.Name,
		)
	}
	return string(secretVal), nil
}
