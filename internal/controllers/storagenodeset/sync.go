package storagenodeset

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

func (r *Reconciler) Sync(ctx context.Context, crStorageNodeSet *v1alpha1.StorageNodeSet) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	storageNodeSet := resources.NewStorageNodeSet(crStorageNodeSet)
	stop, result, err = storageNodeSet.SetStatusOnFirstReconcile()
	if stop {
		return result, err
	}

	stop, result = r.checkStorageFrozen(&storageNodeSet)
	if stop {
		return result, nil
	}

	stop, result, err = r.handlePauseResume(ctx, &storageNodeSet)
	if stop {
		return result, err
	}

	stop, result, err = r.handleResourcesSync(ctx, &storageNodeSet)
	if stop {
		return result, err
	}

	stop, result, err = r.waitForStatefulSetToScale(ctx, &storageNodeSet)
	if stop {
		return result, err
	}

	return result, err
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	for _, builder := range storageNodeSet.GetResourceBuilders(r.Config) {
		newResource := builder.Placeholder(storageNodeSet)

		result, err := resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
			var err error

			err = builder.Build(newResource)
			if err != nil {
				r.Recorder.Event(
					storageNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}
			err = ctrl.SetControllerReference(storageNodeSet.Unwrap(), newResource, r.Scheme)
			if err != nil {
				r.Recorder.Event(
					storageNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Error setting controller reference for resource: %s", err),
				)
				return err
			}

			return nil
		}, shouldIgnoreStorageNodeSetChange(storageNodeSet))

		eventMessage := fmt.Sprintf(
			"Resource: %s, Namespace: %s, Name: %s",
			reflect.TypeOf(newResource),
			newResource.GetNamespace(),
			newResource.GetName(),
		)
		if err != nil {
			r.Recorder.Event(
				storageNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				eventMessage+fmt.Sprintf(", failed to sync, error: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				storageNodeSet,
				corev1.EventTypeNormal,
				string(StorageNodeSetProvisioning),
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) waitForStatefulSetToScale(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale for StorageNodeSet")

	if storageNodeSet.Status.State == StorageNodeSetPending {
		r.Recorder.Event(
			storageNodeSet,
			corev1.EventTypeNormal,
			string(StorageNodeSetProvisioning),
			fmt.Sprintf("Starting to track number of running storageNodeSet pods, expected: %d", storageNodeSet.Spec.Nodes))
		storageNodeSet.Status.State = StorageNodeSetProvisioning
		return r.updateStatus(ctx, storageNodeSet)
	}

	foundStatefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      storageNodeSet.Name,
		Namespace: storageNodeSet.Namespace,
	}, foundStatefulSet)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Recorder.Event(
				storageNodeSet,
				corev1.EventTypeWarning,
				"Syncing",
				fmt.Sprintf("Failed to found StatefulSet: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			storageNodeSet,
			corev1.EventTypeWarning,
			"Syncing",
			fmt.Sprintf("Failed to get StatefulSets: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	matchingLabels := client.MatchingLabels{}
	for k, v := range storageNodeSet.Labels {
		matchingLabels[k] = v
	}

	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(storageNodeSet.Namespace),
		matchingLabels,
	}
	if err = r.List(ctx, podList, opts...); err != nil {
		r.Recorder.Event(
			storageNodeSet,
			corev1.EventTypeWarning,
			"Syncing",
			fmt.Sprintf("Failed to list storageNodeSet pods: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	runningPods := 0
	for _, e := range podList.Items {
		if e.Status.Phase == "Running" {
			runningPods++
		}
	}

	if runningPods != int(storageNodeSet.Spec.Nodes) {
		r.Recorder.Event(
			storageNodeSet,
			corev1.EventTypeNormal,
			string(StorageNodeSetProvisioning),
			fmt.Sprintf("Waiting for number of running storageNodeSet pods to match expected: %d != %d", runningPods, storageNodeSet.Spec.Nodes),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	if storageNodeSet.Spec.Pause {
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    StoragePausedCondition,
			Status:  "True",
			Reason:  ReasonCompleted,
			Message: "Scaled StorageNodeSet to 0 successfully",
		})
		storageNodeSet.Status.State = StorageNodeSetPaused
	} else {
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    StorageNodeSetReadyCondition,
			Status:  "True",
			Reason:  ReasonCompleted,
			Message: fmt.Sprintf("Scaled StorageNodeSet to %d successfully", storageNodeSet.Spec.Nodes),
		})
		storageNodeSet.Status.State = StorageNodeSetReady
	}

	return r.updateStatus(ctx, storageNodeSet)
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step updateStatus for StorageNodeSet")

	crStorageNodeSet := &v1alpha1.StorageNodeSet{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: storageNodeSet.Namespace,
		Name:      storageNodeSet.Name,
	}, crStorageNodeSet)
	if err != nil {
		r.Recorder.Event(
			crStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching CR before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := crStorageNodeSet.Status.State
	crStorageNodeSet.Status.State = storageNodeSet.Status.State
	crStorageNodeSet.Status.Conditions = storageNodeSet.Status.Conditions

	if oldStatus != storageNodeSet.Status.State {
		if err = r.Status().Update(ctx, crStorageNodeSet); err != nil {
			r.Recorder.Event(
				crStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed setting status: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		r.Recorder.Event(
			crStorageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("StorageNodeSet moved from %s to %s", oldStatus, storageNodeSet.Status.State),
		)

		r.Log.Info("step updateStatus for StorageNodeSet requeue reconcile")
		return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func shouldIgnoreStorageNodeSetChange(storageNodeSet *resources.StorageNodeSetResource) resources.IgnoreChangesFunction {
	return func(oldObj, newObj runtime.Object) bool {
		if _, ok := newObj.(*appsv1.StatefulSet); ok {
			if storageNodeSet.Spec.Pause && *oldObj.(*appsv1.StatefulSet).Spec.Replicas == 0 {
				return true
			}
		}
		return false
	}
}

func (r *Reconciler) handlePauseResume(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handlePauseResume for Storage")
	if storageNodeSet.Status.State == StorageReady && storageNodeSet.Spec.Pause {
		r.Log.Info("`pause: true` was noticed, moving StorageNodeSet to state `Paused`")
		meta.RemoveStatusCondition(&storageNodeSet.Status.Conditions, StorageNodeSetReadyCondition)
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    StoragePausedCondition,
			Status:  "False",
			Reason:  ReasonInProgress,
			Message: "Transitioning StorageNodeSet to Paused state",
		})
		storageNodeSet.Status.State = StorageNodeSetPaused
		return r.updateStatus(ctx, storageNodeSet)
	}

	if storageNodeSet.Status.State == StoragePaused && !storageNodeSet.Spec.Pause {
		r.Log.Info("`pause: false` was noticed, moving Storage to state `Ready`")
		meta.RemoveStatusCondition(&storageNodeSet.Status.Conditions, StoragePausedCondition)
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    StorageNodeSetReadyCondition,
			Status:  "False",
			Reason:  ReasonInProgress,
			Message: "Recovering StorageNodeSet from Paused state",
		})
		storageNodeSet.Status.State = StorageNodeSetReady
		return r.updateStatus(ctx, storageNodeSet)
	}

	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) checkStorageFrozen(storageNodeSet *resources.StorageNodeSetResource) (bool, ctrl.Result) {
	r.Log.Info("running step checkStorageFrozen for StorageNodeSet parent object")
	if !storageNodeSet.Spec.OperatorSync {
		r.Log.Info("`operatorSync: false` is set, no further steps will be run")
		return Stop, ctrl.Result{}
	}

	return Continue, ctrl.Result{}
}
