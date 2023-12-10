package database

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/cms"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

var ErrIncorrectDatabaseResourcesConfiguration = errors.New("incorrect database resources configuration, " +
	"must be one of: Resources, SharedResources, ServerlessResources")

func (r *Reconciler) Sync(ctx context.Context, ydbCr *v1alpha1.Database) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	database := resources.NewDatabase(ydbCr)
	stop, result, err = database.SetStatusOnFirstReconcile()
	if stop {
		return result, err
	}

	stop, result, err = r.waitForClusterResources(ctx, &database)
	if stop {
		return result, err
	}
	stop, result, err = r.handleResourcesSync(ctx, &database)
	if stop {
		return result, err
	}
	stop, result, err = r.letStorageKnowAboutThisDatabase(ctx, &database)
	if stop {
		return result, err
	}
	auth, result, err := r.getYDBCredentials(ctx, &database)
	if auth == nil {
		return result, err
	}

	if !meta.IsStatusConditionTrue(database.Status.Conditions, DatabaseTenantInitializedCondition) {
		stop, result, err = r.setInitialStatus(ctx, &database)
		if stop {
			return result, err
		}
		stop, result, err = r.waitForStatefulSetToScale(ctx, &database)
		if stop {
			return result, err
		}
		stop, result, err = r.handleTenantCreation(ctx, &database, auth)
		if stop {
			return result, err
		}
	}

	stop, result, err = r.handlePauseResume(ctx, &database, auth)
	if stop {
		return result, err
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

	if storage.Status.State != string(DatabaseReady) {
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
		database.Status.State = string(DatabaseProvisioning)
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
		r.Recorder.Event(database, corev1.EventTypeNormal, string(DatabaseProvisioning), msg)
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

		result, err := resources.CreateOrUpdateWithIgnoreCheck(ctx, r.Client, newResource, func() error {
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
		}, func(oldObj, newObj runtime.Object) bool {
			if _, ok := newObj.(*appsv1.StatefulSet); ok {
				if database.Spec.Pause == PausePaused && oldObj == nil {
					return true
				}
			}
			return false
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
		if meta.FindStatusCondition(database.Status.Conditions, DatabaseTenantInitializedCondition) == nil ||
			meta.IsStatusConditionFalse(database.Status.Conditions, DatabaseTenantInitializedCondition) {
			return r.processSkipInitPipeline(ctx, database)
		}
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	changed := false
	if meta.FindStatusCondition(database.Status.Conditions, DatabaseTenantInitializedCondition) == nil {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    DatabaseTenantInitializedCondition,
			Status:  "False",
			Reason:  DatabaseTenantInitializedReasonInProgress,
			Message: "Tenant creation in progress",
		})
		changed = true
	}
	if database.Status.State == string(DatabasePending) {
		database.Status.State = string(DatabaseInitializing)
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
		Type:    DatabaseTenantInitializedCondition,
		Status:  "True",
		Reason:  DatabaseTenantInitializedReasonCompleted,
		Message: message,
	})

	database.Status.State = string(DatabaseReady)
	return r.setState(ctx, database)
}

func (r *Reconciler) handleTenantCreation(
	ctx context.Context,
	database *resources.DatabaseBuilder,
	creds ydbCredentials.Credentials,
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

	err := tenant.Create(ctx, database, creds)
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

func (r *Reconciler) letStorageKnowAboutThisDatabase(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	storageNamespace := database.Namespace
	if database.Spec.StorageClusterRef.Namespace != "" {
		storageNamespace = database.Spec.StorageClusterRef.Namespace
	}

	storage := &v1alpha1.Storage{}
	_ = r.Client.Get(context.TODO(),
		types.NamespacedName{
			Name:      database.Spec.StorageClusterRef.Name,
			Namespace: storageNamespace,
		},
		storage,
	)

	var newConnectedDatabases []v1alpha1.ConnectedDatabase
	for _, connectedDatabase := range storage.Status.ConnectedDatabases {
		if connectedDatabase.Name == database.Name {
			continue
		}
		newConnectedDatabases = append(newConnectedDatabases, connectedDatabase)
	}

	newConnectedDatabases = append(newConnectedDatabases,
		v1alpha1.ConnectedDatabase{
			Name:  database.Name,
			State: database.Status.State,
		})

	storage.Status.ConnectedDatabases = newConnectedDatabases

	if err := r.Client.Status().Update(context.TODO(), storage); err != nil {
		r.Log.Error(err, "failed to update the ConnectedDatabase list of parent Storage")
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	return Continue, ctrl.Result{}, nil
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

func (r *Reconciler) getYDBCredentials(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (ydbCredentials.Credentials, ctrl.Result, error) {
	r.Log.Info("running step getYDBCredentials")

	if auth := database.Storage.Spec.OperatorConnection; auth != nil {
		switch {
		case auth.AccessToken != nil:
			token, err := r.getSecretKey(
				ctx,
				database.Storage.Namespace,
				auth.AccessToken.SecretKeyRef,
			)
			if err != nil {
				return nil, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			return ydbCredentials.NewAccessTokenCredentials(token), ctrl.Result{Requeue: false}, nil
		case auth.StaticCredentials != nil:
			username := auth.StaticCredentials.Username
			password := v1alpha1.DefaultRootPassword
			if auth.StaticCredentials.Password != nil {
				var err error
				password, err = r.getSecretKey(
					ctx,
					database.Storage.Namespace,
					auth.StaticCredentials.Password.SecretKeyRef,
				)
				if err != nil {
					return nil, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
				}
			}
			endpoint := database.GetStorageEndpoint()
			secure := connection.LoadTLSCredentials(resources.IsGrpcSecure(database.Storage))
			return ydbCredentials.NewStaticCredentials(username, password, endpoint, secure), ctrl.Result{Requeue: false}, nil
		}
	}
	return ydbCredentials.NewAnonymousCredentials(), ctrl.Result{Requeue: false}, nil
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

func (r *Reconciler) handlePauseResume(
	ctx context.Context,
	database *resources.DatabaseBuilder,
	creds ydbCredentials.Credentials,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handlePauseResume for Database")
	if database.Status.State == string(DatabaseReady) && database.Spec.Pause == PausePaused {
		r.Log.Info("`pause: Paused` was noticed, attempting to delete Database StatefulSet")
		database.Status.State = string(StoragePaused)

		statefulSet := &appsv1.StatefulSet{}
		err := r.Client.Get(context.TODO(),
			types.NamespacedName{
				Name:      database.Name, // TODOPAUSE assuming implicitly storageName and statefulSetName are the same
				Namespace: database.Namespace,
			},
			statefulSet,
		)
		if err != nil {
			r.Log.Error(err, "failed to get the Database StatefulSet object before deletion")
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		err = r.Client.Delete(context.TODO(), statefulSet)
		if err != nil {
			r.Log.Error(err, "failed to delete the Database StatefulSet object when moving from Ready -> Paused")
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		for _, condition := range database.Status.Conditions {
			if condition.Type != DatabaseTenantInitializedCondition {
				meta.RemoveStatusCondition(&database.Status.Conditions, condition.Type)
			}
		}

		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    DatabasePausedCondition,
			Status:  "True",
			Reason:  DatabasePausedReason,
			Message: "pause: Paused is set on Database",
		})

		return r.setState(ctx, database)
	}

	if database.Status.State == string(DatabasePaused) && database.Spec.Pause == PauseRunning {
		r.Log.Info("`pause: Running` was noticed, moving Database to `Pending`")
		meta.RemoveStatusCondition(&database.Status.Conditions, DatabasePausedCondition)

		database.Status.State = string(DatabaseReady)
		return r.setState(ctx, database)
	}

	return Continue, ctrl.Result{}, nil
}
