package database

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

const (
	Pending      TenantState = "Pending"
	Preparing    TenantState = "Preparing"
	Provisioning TenantState = "Provisioning"
	Initializing TenantState = "Initializing"
	Ready        TenantState = "Ready"

	DefaultRequeueDelay             = 10 * time.Second
	StatusUpdateRequeueDelay        = 1 * time.Second
	TenantCreationRequeueDelay      = 30 * time.Second
	StorageAwaitRequeueDelay        = 30 * time.Second
	SharedDatabaseAwaitRequeueDelay = 30 * time.Second

	TenantInitializedCondition        = "TenantInitialized"
	TenantInitializedReasonInProgress = "InProgres"
	TenantInitializedReasonCompleted  = "Completed"

	Stop     = true
	Continue = false

	ownerControllerKey = ".metadata.controller"
)

var ErrIncorrectDatabaseResourcesConfiguration = errors.New("incorrect database resources configuration, " +
	"must be one of: Resources, SharedResources, ServerlessResources")

type TenantState string

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

	stop, result, err = r.syncNodeSetSpecInline(ctx, &database)
	if stop {
		return result, err
	}

	auth, result, err := r.getYDBCredentials(ctx, &database)
	if auth == nil {
		return result, err
	}

	if !meta.IsStatusConditionTrue(database.Status.Conditions, TenantInitializedCondition) { //nolint:nestif
		stop, result, err = r.setInitialStatus(ctx, &database)
		if stop {
			return result, err
		}

		if database.Spec.NodeSet != nil {
			stop, result, err = r.waitForDatabaseNodeSetsToReady(ctx, &database)
			if stop {
				return result, err
			}
		} else {
			stop, result, err = r.waitForStatefulSetToScale(ctx, &database)
			if stop {
				return result, err
			}
		}

		stop, result, err = r.handleTenantCreation(ctx, &database, auth)
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

func (r *Reconciler) waitForDatabaseNodeSetsToReady(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForDatabaseNodeSetToReady for Database")

	if database.Status.State == string(Preparing) {
		msg := fmt.Sprintf("Starting to track readiness of running nodeSets objects, expected: %d", len(database.Spec.NodeSet))
		r.Recorder.Event(database, corev1.EventTypeNormal, string(Provisioning), msg)
		database.Status.State = string(Provisioning)
		return r.setState(ctx, database)
	}

	for _, nodeSetSpec := range database.Spec.NodeSet {
		foundDatabaseNodeSet := v1alpha1.DatabaseNodeSet{}
		databaseNodeSetName := database.Name + "-" + nodeSetSpec.Name
		err := r.Get(ctx, types.NamespacedName{
			Name:      databaseNodeSetName,
			Namespace: database.Namespace,
		}, &foundDatabaseNodeSet)
		if err != nil {
			if apierrors.IsNotFound(err) {
				r.Recorder.Event(
					database,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("DatabaseNodeSet with name %s was not found: %s", databaseNodeSetName, err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
			}
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("Failed to get DatabaseNodeSet: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		if foundDatabaseNodeSet.Status.State != string(Ready) {
			msg := fmt.Sprintf("Waiting %s state for DatabaseNodeSet object %s, current: %s",
				string(Ready),
				foundDatabaseNodeSet.Name,
				foundDatabaseNodeSet.Status.State,
			)
			r.Recorder.Event(
				database,
				corev1.EventTypeNormal,
				string(Provisioning),
				msg,
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) waitForStatefulSetToScale(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale for Database")

	if database.Status.State == string(Preparing) {
		msg := fmt.Sprintf("Starting to track number of running database pods, expected: %d", database.Spec.Nodes)
		r.Recorder.Event(database, corev1.EventTypeNormal, string(Provisioning), msg)
		database.Status.State = string(Provisioning)
		return r.setState(ctx, database)
	}

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
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("StatefulSet with name %s was not found: %s", database.Name, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"ProvisioningFailed",
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
			corev1.EventTypeWarning,
			"ProvisioningFailed",
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

func (r *Reconciler) syncNodeSetSpecInline(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step syncNodeSetSpecInline")

	databaseNodeSets := &v1alpha1.DatabaseNodeSetList{}
	matchingFields := client.MatchingFields{
		ownerControllerKey: database.Name,
	}
	if err := r.List(ctx, databaseNodeSets,
		client.InNamespace(database.Namespace),
		matchingFields,
	); err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"ProvisioningFailed",
			fmt.Sprintf("Failed to list DatabaseNodeSets: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	for _, databaseNodeSet := range databaseNodeSets.Items {
		databaseNodeSet := databaseNodeSet.DeepCopy()
		isFoundDatabaseNodeSetSpecInline := false
		for _, nodeSetSpecInline := range database.Spec.NodeSet {
			databaseNodeSetName := database.Name + "-" + nodeSetSpecInline.Name
			if databaseNodeSet.Name == databaseNodeSetName {
				isFoundDatabaseNodeSetSpecInline = true
				break
			}
		}
		if !isFoundDatabaseNodeSetSpecInline {
			if err := r.Delete(ctx, databaseNodeSet); err != nil {
				r.Recorder.Event(
					database,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed to delete DatabaseNodeSet: %s", err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			r.Recorder.Event(
				database,
				corev1.EventTypeNormal,
				"Syncing",
				fmt.Sprintf("Resource: %s, Namespace: %s, Name: %s, deleted",
					reflect.TypeOf(databaseNodeSet),
					databaseNodeSet.Namespace,
					databaseNodeSet.Name),
			)
		}

		oldGeneration := databaseNodeSet.Status.ObservedDatabaseGeneration
		if oldGeneration != database.Generation {
			databaseNodeSet.Status.ObservedDatabaseGeneration = database.Generation
			if err := r.Status().Update(ctx, databaseNodeSet); err != nil {
				r.Recorder.Event(
					databaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed setting status: %s", err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			r.Recorder.Event(
				databaseNodeSet,
				corev1.EventTypeNormal,
				"StatusChanged",
				fmt.Sprintf(
					"DatabaseNodeSet updated observedStorageGeneration from %d to %d",
					oldGeneration,
					database.Generation),
			)
		}
	}

	r.Log.Info("syncNodeSetSpecInline complete")
	return Continue, ctrl.Result{Requeue: false}, nil
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
			secure := connection.LoadTLSCredentials(database.IsStorageEndpointSecure())
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
