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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

var ErrIncorrectDatabaseResourcesConfiguration = errors.New("incorrect database resources configuration, " +
	"must be one of: Resources, SharedResources, ServerlessResources")

func (r *Reconciler) Sync(ctx context.Context, ydbCr *v1alpha1.Database) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	database := resources.NewDatabase(ydbCr)

	stop, result, err = r.setInitialStatus(ctx, &database)
	if stop {
		return result, err
	}

	stop, result, err = r.waitForClusterResources(ctx, &database)
	if stop {
		return result, err
	}

	stop, result, err = r.syncNodeSetSpecInline(ctx, &database)
	if stop {
		return result, err
	}

	stop, result, err = r.handleResourcesSync(ctx, &database)
	if stop {
		return result, err
	}

	if !meta.IsStatusConditionTrue(database.Status.Conditions, DatabaseInitializedCondition) {
		return r.handleTenantCreation(ctx, &database)
	}

	if database.Spec.NodeSets != nil {
		stop, result, err = r.waitForNodeSetsToProvisioned(ctx, &database)
		if stop {
			return result, err
		}
	} else {
		stop, result, err = r.waitForStatefulSetToScale(ctx, &database)
		if stop {
			return result, err
		}
	}

	stop, result, err = r.handlePauseResume(ctx, &database)
	if stop {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) setInitialStatus(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")
	if database.Status.Conditions == nil {
		database.Status.Conditions = []metav1.Condition{}

		if database.Spec.Pause {
			meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
				Type:               DatabasePausedCondition,
				Status:             metav1.ConditionUnknown,
				Reason:             ReasonInProgress,
				ObservedGeneration: database.Generation,
				Message:            "Transitioning to state Paused",
			})
		} else {
			meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
				Type:               DatabaseReadyCondition,
				Status:             metav1.ConditionUnknown,
				Reason:             ReasonInProgress,
				ObservedGeneration: database.Generation,
				Message:            "Transitioning to state Ready",
			})
		}

		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step setInitialStatus")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) waitForClusterResources(ctx context.Context, database *resources.DatabaseBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForClusterResources")

	if database.Status.State == DatabasePending {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    DatabasePreparedCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonInProgress,
			Message: "Waiting for sync resources",
		})
		database.Status.State = DatabasePreparing
		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

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
				"Failed to get Storage (%s, %s) resource, error: %s",
				database.Spec.StorageClusterRef.Name,
				database.Spec.StorageClusterRef.Namespace,
				err,
			),
		)
		return Stop, ctrl.Result{RequeueAfter: StorageAwaitRequeueDelay}, err
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, StorageInitializedCondition) {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"Pending",
			fmt.Sprintf(
				"Referenced storage cluster (%s, %s) is not initialized",
				database.Spec.StorageClusterRef.Name,
				database.Spec.StorageClusterRef.Namespace,
			),
		)
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:   DatabasePreparedCondition,
			Status: metav1.ConditionFalse,
			Reason: ReasonInProgress,
			Message: fmt.Sprintf(
				"Referenced storage cluster (%s, %s) is not initialized",
				database.Spec.StorageClusterRef.Name,
				database.Spec.StorageClusterRef.Namespace,
			),
		})
		return r.updateStatus(ctx, database, StorageAwaitRequeueDelay)
	}

	database.Storage = storage

	r.Log.Info("complete step waitForClusterResources")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) waitForNodeSetsToProvisioned(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForNodeSetsToProvisioned")

	if database.Status.State == DatabaseInitializing {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    DatabaseProvisionedCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonInProgress,
			Message: "Waiting for NodeSets conditions to be Provisioned",
		})
		database.Status.State = DatabaseProvisioning
		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

	for _, nodeSetSpec := range database.Spec.NodeSets {
		var nodeSetObject client.Object
		var nodeSetKind string
		var nodeSetConditions []metav1.Condition
		if nodeSetSpec.Remote != nil {
			nodeSetObject = &v1alpha1.RemoteDatabaseNodeSet{}
			nodeSetKind = RemoteDatabaseNodeSetKind
		} else {
			nodeSetObject = &v1alpha1.DatabaseNodeSet{}
			nodeSetKind = DatabaseNodeSetKind
		}

		nodeSetName := database.Name + "-" + nodeSetSpec.Name
		if err := r.Get(ctx, types.NamespacedName{
			Name:      nodeSetName,
			Namespace: database.Namespace,
		}, nodeSetObject); err != nil {
			if apierrors.IsNotFound(err) {
				r.Recorder.Event(
					database,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("%s with name %s was not found: %s", nodeSetKind, nodeSetName, err),
				)
			}
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("Failed to get %s with name %s: %s", nodeSetKind, nodeSetName, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		if nodeSetSpec.Remote != nil {
			nodeSetConditions = nodeSetObject.(*v1alpha1.RemoteDatabaseNodeSet).Status.Conditions
		} else {
			nodeSetConditions = nodeSetObject.(*v1alpha1.DatabaseNodeSet).Status.Conditions
		}

		condition := meta.FindStatusCondition(nodeSetConditions, NodeSetProvisionedCondition)
		if condition == nil || condition.ObservedGeneration != nodeSetObject.GetGeneration() || condition.Status != metav1.ConditionTrue {
			r.Recorder.Event(
				database,
				corev1.EventTypeNormal,
				string(DatabaseProvisioning),
				fmt.Sprintf(
					"Waiting %s with name %s for condition NodeSetProvisioned to be True",
					nodeSetKind,
					nodeSetName,
				),
			)
			meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
				Type:               DatabaseProvisionedCondition,
				Status:             metav1.ConditionFalse,
				Reason:             ReasonInProgress,
				ObservedGeneration: database.Generation,
				Message: fmt.Sprintf(
					"Waiting %s with name %s for condition NodeSetProvisioned to be True",
					nodeSetKind,
					nodeSetName,
				),
			})
			return r.updateStatus(ctx, database, DefaultRequeueDelay)
		}
	}

	if !meta.IsStatusConditionTrue(database.Status.Conditions, DatabaseProvisionedCondition) {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:               DatabaseProvisionedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonCompleted,
			ObservedGeneration: database.Generation,
			Message:            fmt.Sprintf("Successfully scaled to desired number of nodes: %d", database.Spec.Nodes),
		})
		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step waitForNodeSetsToProvisioned")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) waitForStatefulSetToScale(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale")

	if database.Status.State == DatabaseInitializing {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    DatabaseProvisionedCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Waiting for scale to desired nodes: %d", database.Spec.Nodes),
		})
		database.Status.State = DatabaseProvisioning
		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

	if database.Spec.ServerlessResources != nil {
		return Continue, ctrl.Result{Requeue: false}, nil
	}

	foundStatefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      database.Name,
		Namespace: database.Namespace,
	}, foundStatefulSet)
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
			"ControllerError",
			fmt.Sprintf("Failed to get StatefulSet: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	if foundStatefulSet.Status.ReadyReplicas != database.Spec.Nodes {
		r.Recorder.Event(
			database,
			corev1.EventTypeNormal,
			string(DatabaseProvisioning),
			fmt.Sprintf("Waiting for number of running pods to match expected: %d != %d", foundStatefulSet.Status.ReadyReplicas, database.Spec.Nodes),
		)
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:               DatabaseProvisionedCondition,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonInProgress,
			ObservedGeneration: database.Generation,
			Message:            fmt.Sprintf("Number of running nodes does not match expected: %d != %d", foundStatefulSet.Status.ReadyReplicas, database.Spec.Nodes),
		})
		return r.updateStatus(ctx, database, DefaultRequeueDelay)
	}

	if !meta.IsStatusConditionTrue(database.Status.Conditions, DatabaseProvisionedCondition) {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:               DatabaseProvisionedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonCompleted,
			ObservedGeneration: database.Generation,
			Message:            fmt.Sprintf("Successfully scaled to desired number of nodes: %d", database.Spec.Nodes),
		})
		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step waitForStatefulSetToScale")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func shouldIgnoreDatabaseChange(database *resources.DatabaseBuilder) resources.IgnoreChangesFunction {
	return func(oldObj, newObj runtime.Object) bool {
		if statefulSet, ok := oldObj.(*appsv1.StatefulSet); ok {
			if database.Spec.Pause && *statefulSet.Spec.Replicas == 0 {
				return true
			}
		}
		return false
	}
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	if !database.Spec.OperatorSync {
		r.Log.Info("`operatorSync: false` is set, no further steps will be run")
		r.Recorder.Event(
			database,
			corev1.EventTypeNormal,
			string(DatabasePreparing),
			fmt.Sprintf("Found .spec.operatorSync set to %t, skip further steps", database.Spec.OperatorSync),
		)
		return Stop, ctrl.Result{}, nil
	}

	for _, builder := range database.GetResourceBuilders(r.Config) {
		newResource := builder.Placeholder(database)

		result, err := resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
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
		}, shouldIgnoreDatabaseChange(database))

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
			meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
				Type:               DatabasePreparedCondition,
				Status:             metav1.ConditionFalse,
				Reason:             ReasonInProgress,
				ObservedGeneration: database.Generation,
				Message:            "Failed to sync resources",
			})
			return r.updateStatus(ctx, database, DefaultRequeueDelay)
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				database,
				corev1.EventTypeNormal,
				"Provisioning",
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}

	if !meta.IsStatusConditionTrue(database.Status.Conditions, DatabasePreparedCondition) {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:               DatabasePreparedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             ReasonCompleted,
			ObservedGeneration: database.Generation,
			Message:            "Successfully synced resources",
		})
		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step handleResourcesSync")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	database *resources.DatabaseBuilder,
	requeueAfter time.Duration,
) (bool, ctrl.Result, error) {
	r.Log.Info("running updateStatus handler")

	if meta.IsStatusConditionFalse(database.Status.Conditions, DatabasePreparedCondition) ||
		meta.IsStatusConditionFalse(database.Status.Conditions, DatabaseInitializedCondition) ||
		meta.IsStatusConditionFalse(database.Status.Conditions, DatabaseProvisionedCondition) {
		if database.Spec.Pause {
			meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
				Type:               DatabasePausedCondition,
				Status:             metav1.ConditionFalse,
				Reason:             ReasonInProgress,
				ObservedGeneration: database.Generation,
			})
		} else {
			meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
				Type:               DatabaseReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             ReasonInProgress,
				ObservedGeneration: database.Generation,
			})
		}
	}

	databaseCr := &v1alpha1.Database{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: database.Namespace,
		Name:      database.Name,
	}, databaseCr)
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching CR before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := databaseCr.Status.State
	databaseCr.Status.State = database.Status.State
	databaseCr.Status.Conditions = database.Status.Conditions
	err = r.Status().Update(ctx, databaseCr)
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("failed setting status: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	if oldStatus != database.Status.State {
		r.Recorder.Event(
			database,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("Database moved from %s to %s", oldStatus, databaseCr.Status.State),
		)
	}

	r.Log.Info("complete updateStatus handler")
	return Stop, ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *Reconciler) syncNodeSetSpecInline(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step syncNodeSetSpecInline")

	databaseNodeSets := &v1alpha1.DatabaseNodeSetList{}
	if err := r.List(ctx, databaseNodeSets,
		client.InNamespace(database.Namespace),
		client.MatchingFields{
			OwnerControllerField: database.Name,
		},
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
		for _, nodeSetSpecInline := range database.Spec.NodeSets {
			if nodeSetSpecInline.Remote == nil {
				nodeSetName := database.Name + "-" + nodeSetSpecInline.Name
				if databaseNodeSet.Name == nodeSetName {
					isFoundDatabaseNodeSetSpecInline = true
					break
				}
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
					DatabaseNodeSetKind,
					databaseNodeSet.Namespace,
					databaseNodeSet.Name),
			)
		}
	}

	remoteDatabaseNodeSets := &v1alpha1.RemoteDatabaseNodeSetList{}
	if err := r.List(ctx, remoteDatabaseNodeSets,
		client.InNamespace(database.Namespace),
		client.MatchingFields{
			OwnerControllerField: database.Name,
		},
	); err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"ProvisioningFailed",
			fmt.Sprintf("Failed to list RemoteDatabaseNodeSets: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	for _, remoteDatabaseNodeSet := range remoteDatabaseNodeSets.Items {
		remoteDatabaseNodeSet := remoteDatabaseNodeSet.DeepCopy()
		isFoundRemoteDatabaseNodeSetSpecInline := false
		for _, nodeSetSpecInline := range database.Spec.NodeSets {
			if nodeSetSpecInline.Remote != nil {
				nodeSetName := database.Name + "-" + nodeSetSpecInline.Name
				if remoteDatabaseNodeSet.Name == nodeSetName {
					isFoundRemoteDatabaseNodeSetSpecInline = true
					break
				}
			}
		}

		if !isFoundRemoteDatabaseNodeSetSpecInline {
			if err := r.Delete(ctx, remoteDatabaseNodeSet); err != nil {
				r.Recorder.Event(
					database,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed to delete RemoteDatabaseNodeSet: %s", err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			r.Recorder.Event(
				database,
				corev1.EventTypeNormal,
				"Syncing",
				fmt.Sprintf("Resource: %s, Namespace: %s, Name: %s, deleted",
					RemoteDatabaseNodeSetKind,
					remoteDatabaseNodeSet.Namespace,
					remoteDatabaseNodeSet.Name),
			)
		}
	}

	r.Log.Info("complete step syncNodeSetSpecInline")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) handlePauseResume(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handlePauseResume")

	if database.Status.State == DatabaseProvisioning {
		if database.Spec.Pause {
			meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
				Type:               DatabasePausedCondition,
				Status:             metav1.ConditionTrue,
				Reason:             ReasonCompleted,
				ObservedGeneration: database.Generation,
			})
			database.Status.State = DatabasePaused
		} else {
			meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
				Type:               DatabaseReadyCondition,
				Status:             metav1.ConditionTrue,
				Reason:             ReasonCompleted,
				ObservedGeneration: database.Generation,
			})
			database.Status.State = DatabaseReady
		}
		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

	if database.Status.State == DatabaseReady && database.Spec.Pause {
		r.Log.Info("`pause: true` was noticed, moving Database to state `Paused`")
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:               DatabaseReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonNotRequired,
			ObservedGeneration: database.Generation,
			Message:            "Transitioning to state Paused",
		})
		database.Status.State = DatabasePaused
		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

	if database.Status.State == DatabasePaused && !database.Spec.Pause {
		r.Log.Info("`pause: false` was noticed, moving Database to state `Ready`")
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:               DatabasePausedCondition,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonNotRequired,
			ObservedGeneration: database.Generation,
			Message:            "Transitioning to state Ready",
		})
		database.Status.State = DatabaseReady
		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

	if database.Spec.Pause {
		if !meta.IsStatusConditionTrue(database.Status.Conditions, DatabasePausedCondition) {
			meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
				Type:               DatabasePausedCondition,
				Status:             metav1.ConditionTrue,
				Reason:             ReasonCompleted,
				ObservedGeneration: database.Generation,
			})
			return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
		}
	} else {
		if !meta.IsStatusConditionTrue(database.Status.Conditions, DatabaseReadyCondition) {
			meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
				Type:               DatabaseReadyCondition,
				Status:             metav1.ConditionTrue,
				Reason:             ReasonCompleted,
				ObservedGeneration: database.Generation,
			})
			return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
		}
	}

	r.Log.Info("complete step handlePauseResume")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) handleTenantCreation(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (ctrl.Result, error) {
	r.Log.Info("running step handleTenantCreation")

	stop, result, err := r.setInitPipelineStatus(ctx, database)
	if stop {
		return result, err
	}

	stop, result, err = r.initializeTenant(ctx, database)
	if stop {
		return result, err
	}

	r.Log.Info("complete step handleTenantCreation")
	return ctrl.Result{}, nil
}
