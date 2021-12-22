package database

import (
	"context"
	"fmt"
	"time"
	"reflect"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/cms"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

	DefaultRequeueDelay        = 10 * time.Second
	StatusUpdateRequeueDelay   =  1 * time.Second
	TenantCreationRequeueDelay = 30 * time.Second
	StorageAwaitRequeueDelay   = 60 * time.Second

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

	stop, result, err = r.waitForClusterResource(ctx, &database)
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

func (r *DatabaseReconciler) waitForClusterResource(ctx context.Context, database *resources.DatabaseBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForClusterResource")
	found := &ydbv1alpha1.Storage{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      database.Spec.StorageClusterRef.Name,
		Namespace: database.Spec.StorageClusterRef.Namespace,
	}, found)

	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"Pending",
			fmt.Sprintf(
				"Failed to get (%s, %s) resource of type cluster.ydb.tech: %s",
				database.Spec.StorageClusterRef.Name,
				database.Spec.StorageClusterRef.Namespace,
				err,
			),
		)
		return Stop, ctrl.Result{RequeueAfter: StorageAwaitRequeueDelay}, err
	}

	if found.Status.State != "Ready" {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"Pending",
			fmt.Sprintf(
				"Referenced storage cluster (%s, %s) in a bad state: %s != Ready",
				database.Spec.StorageClusterRef.Name,
				database.Spec.StorageClusterRef.Namespace,
				found.Status.State,
			),
		)
		return Stop, ctrl.Result{RequeueAfter: StorageAwaitRequeueDelay}, err
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *DatabaseReconciler) waitForStatefulSetToScale(ctx context.Context, database *resources.DatabaseBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale")
	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      database.Name,
		Namespace: database.Namespace,
	}, found)

	if err != nil {
		if errors.IsNotFound(err) {
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
		new_resource := builder.Placeholder(database)

		result, err := resources.CreateOrUpdateIgnoreStatus(ctx, r.Client, new_resource, func() error {
			var err error

			err = builder.Build(new_resource)
			if err != nil {
				r.Recorder.Event(
					database,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}

			err = ctrl.SetControllerReference(database.Unwrap(), new_resource, r.Scheme)
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
			reflect.TypeOf(new_resource),
			new_resource.GetNamespace(),
			new_resource.GetName(),
		)
		if err != nil {
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				eventMessage + fmt.Sprintf(", failed to sync, error: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		} else if (result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated) {
			r.Recorder.Event(
				database,
				corev1.EventTypeNormal,
				"Provisioning",
				eventMessage + fmt.Sprintf(", changed, result: %s", result),
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

	tenant := cms.NewTenant(database.GetTenantName())
	err := tenant.Create(ctx, database)                                 // <------!
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"InitializingFailed",
			fmt.Sprintf("Error creating tenant %s: %s", tenant.Name, err),
		)
		return Stop, ctrl.Result{RequeueAfter: TenantCreationRequeueDelay}, err
	}
	r.Recorder.Event(
		database,
		corev1.EventTypeNormal,
		"Initialized",
		fmt.Sprintf("Tenant %s created", tenant.Name),
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
