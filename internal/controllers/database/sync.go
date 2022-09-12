package database

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/cms"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
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

type ClusterState string

func (r *DatabaseReconciler) Sync(ctx context.Context, ydbCr *ydbv1alpha1.Database) (ctrl.Result, error) {
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

func (r *DatabaseReconciler) waitForClusterResources(ctx context.Context, database *resources.DatabaseBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForClusterResources")
	storage := &ydbv1alpha1.Storage{}
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

	if storage.Status.State != "Ready" {
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

	database.StorageRef = storage

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *DatabaseReconciler) waitForStatefulSetToScale(ctx context.Context, database *resources.DatabaseBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale")

	if database.Spec.ServerlessResources == nil {
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

		if found.Status.Replicas != database.Spec.Nodes {
			msg := fmt.Sprintf("Waiting for number of running pods to match expected: %d != %d", found.Status.Replicas, database.Spec.Nodes)
			r.Recorder.Event(database, corev1.EventTypeNormal, "Provisioning", msg)
			database.Status.State = string(Provisioning)
			return r.setState(ctx, database)
		}
	}

	if database.Status.State != string(Ready) && meta.IsStatusConditionTrue(database.Status.Conditions, TenantInitializedCondition) {
		r.Recorder.Event(database, corev1.EventTypeNormal, "ResourcesReady", "Resource are ready and DB is initialized")
		database.Status.State = string(Ready)
		return r.setState(ctx, database)
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *DatabaseReconciler) handleResourcesSync(ctx context.Context, database *resources.DatabaseBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	for _, builder := range database.GetResourceBuilders() {
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

		var eventMessage string = fmt.Sprintf(
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

func (r *DatabaseReconciler) setInitialStatus(ctx context.Context, database *resources.DatabaseBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")
	var changed bool = false
	if meta.FindStatusCondition(database.Status.Conditions, TenantInitializedCondition) == nil {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    TenantInitializedCondition,
			Status:  "False",
			Reason:  TenantInitializedReasonInProgress,
			Message: "Tenant creation in progress",
		})
		changed = true
	}
	if database.Status.State != string(Initializing) {
		database.Status.State = string(Initializing)
		changed = true
	}
	if changed {
		return r.setState(ctx, database)
	}
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *DatabaseReconciler) handleTenantCreation(ctx context.Context, database *resources.DatabaseBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleTenantCreation")

	var Path string = database.GetPath()
	var StorageUnits []ydbv1alpha1.StorageUnit
	var Shared bool
	var SharedDatabasePath string
	if database.Spec.Resources != nil {
		StorageUnits = (*database.Spec.Resources).StorageUnits
		Shared = false
	} else if database.Spec.SharedResources != nil {
		StorageUnits = (*database.Spec.SharedResources).StorageUnits
		Shared = true
	} else if database.Spec.ServerlessResources != nil {
		sharedDatabaseCr := &ydbv1alpha1.Database{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      (*database.Spec.ServerlessResources).SharedDatabaseRef.Name,
			Namespace: (*database.Spec.ServerlessResources).SharedDatabaseRef.Namespace,
		}, sharedDatabaseCr)
		if err != nil {
			if apierrors.IsNotFound(err) {
				r.Recorder.Event(
					database,
					corev1.EventTypeWarning,
					"Pending",
					fmt.Sprintf(
						"Database (%s/%s) not found.",
						(*database.Spec.ServerlessResources).SharedDatabaseRef.Name,
						(*database.Spec.ServerlessResources).SharedDatabaseRef.Namespace,
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
					(*database.Spec.ServerlessResources).SharedDatabaseRef.Name,
					(*database.Spec.ServerlessResources).SharedDatabaseRef.Namespace,
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
					(*database.Spec.ServerlessResources).SharedDatabaseRef.Name,
					(*database.Spec.ServerlessResources).SharedDatabaseRef.Namespace,
					sharedDatabaseCr.Status.State,
				),
			)
			return Stop, ctrl.Result{RequeueAfter: SharedDatabaseAwaitRequeueDelay}, err
		}

		SharedDatabasePath = fmt.Sprintf(ydbv1alpha1.TenantNameFormat, sharedDatabaseCr.Spec.Domain, sharedDatabaseCr.Name)
	} else {
		// TODO: move this logic to webhook
		msg := "Incorrect database resources configuration, must be one of: Resources, SharedResources, ServerlessResources"
		r.Recorder.Event(database, corev1.EventTypeWarning, "ControllerError", msg)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, errors.New(msg)
	}
	tenant := cms.Tenant{
		StorageEndpoint:      database.GetStorageEndpoint(),
		Path:                 Path,
		StorageUnits:         StorageUnits,
		Shared:               Shared,
		SharedDatabasePath:   SharedDatabasePath,
		UseGrpcSecureChannel: database.StorageRef.Spec.Service.GRPC.TLSConfiguration.Enabled,
	}
	err := tenant.Create(ctx)
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
	meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
		Type:    TenantInitializedCondition,
		Status:  "True",
		Reason:  TenantInitializedReasonCompleted,
		Message: "Tenant creation is complete",
	})
	return r.setState(ctx, database)
}

func (r *DatabaseReconciler) setState(ctx context.Context, database *resources.DatabaseBuilder) (bool, ctrl.Result, error) {
	databaseCr := &ydbv1alpha1.Database{}
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
		r.Recorder.Event(databaseCr, corev1.EventTypeWarning, "ControllerError", fmt.Sprintf("Failed setting status: %s", err))
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
}
