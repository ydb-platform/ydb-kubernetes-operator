package storagenodeset

import (
	"context"
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

func (r *Reconciler) Sync(ctx context.Context, crStorageNodeSet *v1alpha1.StorageNodeSet) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	storageNodeSet := resources.NewStorageNodeSet(crStorageNodeSet)

	stop, result, err = r.setInitialStatus(ctx, &storageNodeSet)
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

	stop, result, err = r.handlePauseResume(ctx, &storageNodeSet)
	if stop {
		return result, err
	}

	return result, err
}

func (r *Reconciler) setInitialStatus(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")

	if storageNodeSet.Status.Conditions == nil {
		storageNodeSet.Status.Conditions = []metav1.Condition{}

		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:   NodeSetReadyCondition,
			Status: "Unknown",
			Reason: ReasonInProgress,
		})

		if storageNodeSet.Spec.Pause {
			meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
				Type:    StoragePausedCondition,
				Status:  "Unknown",
				Reason:  ReasonInProgress,
				Message: "Transitioning to state Paused",
			})
		}

		return r.updateStatus(ctx, storageNodeSet, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step setInitialStatus")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	if !storageNodeSet.Spec.OperatorSync {
		r.Log.Info("`operatorSync: false` is set, no further steps will be run")
		r.Recorder.Event(
			storageNodeSet,
			corev1.EventTypeNormal,
			string(StorageNodeSetPreparing),
			fmt.Sprintf("Found .spec.operatorSync set to %t, skip further steps", storageNodeSet.Spec.OperatorSync),
		)
		return Stop, ctrl.Result{Requeue: false}, nil
	}

	if storageNodeSet.Status.State == StorageNodeSetPending {
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetPreparingCondition,
			Status:  "Unknown",
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Waiting for sync resources for generation %d", storageNodeSet.Generation),
		})
		storageNodeSet.Status.State = StorageNodeSetPreparing
		return r.updateStatus(ctx, storageNodeSet, StatusUpdateRequeueDelay)
	}

	for _, builder := range storageNodeSet.GetResourceBuilders(r.Config) {
		newResource := builder.Placeholder(storageNodeSet)

		result, err := resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
			var err error

			err = builder.Build(newResource)
			if err != nil {
				r.Recorder.Event(
					storageNodeSet,
					corev1.EventTypeWarning,
					string(StorageNodeSetPreparing),
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}
			err = ctrl.SetControllerReference(storageNodeSet.Unwrap(), newResource, r.Scheme)
			if err != nil {
				r.Recorder.Event(
					storageNodeSet,
					corev1.EventTypeWarning,
					string(StorageNodeSetPreparing),
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
				string(StorageNodeSetPreparing),
				eventMessage+fmt.Sprintf(", failed to sync, error: %s", err),
			)
			meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
				Type:    NodeSetPreparingCondition,
				Status:  "False",
				Reason:  ReasonInProgress,
				Message: fmt.Sprintf("Failed to sync resources for generation %d", storageNodeSet.Generation),
			})
			return r.updateStatus(ctx, storageNodeSet, DefaultRequeueDelay)
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				storageNodeSet,
				corev1.EventTypeNormal,
				string(StorageNodeSetPreparing),
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}

	meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
		Type:    NodeSetPreparingCondition,
		Status:  "True",
		Reason:  ReasonCompleted,
		Message: fmt.Sprintf("Successfully synced resources for generation %d", storageNodeSet.Generation),
	})
	r.Log.Info("complete step handleResourcesSync")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) waitForStatefulSetToScale(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale")

	if storageNodeSet.Status.State == StorageNodeSetPreparing {
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetProvisioningCondition,
			Status:  "Unknown",
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Waiting for scale to desired nodes: %d", storageNodeSet.Spec.Nodes),
		})
		storageNodeSet.Status.State = StorageNodeSetProvisioning
		return r.updateStatus(ctx, storageNodeSet, StatusUpdateRequeueDelay)
	}

	foundStatefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      storageNodeSet.Name,
		Namespace: storageNodeSet.Namespace,
	}, foundStatefulSet)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Recorder.Event(
				storageNodeSet,
				corev1.EventTypeWarning,
				string(StorageNodeSetProvisioning),
				fmt.Sprintf("Failed to find StatefulSet: %s", err),
			)
		} else {
			r.Recorder.Event(
				storageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to get StatefulSet: %s", err),
			)
		}
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetProvisioningCondition,
			Status:  "False",
			Reason:  ReasonInProgress,
			Message: "Failed to check StatefulSet .spec.replicas",
		})
		return r.updateStatus(ctx, storageNodeSet, DefaultRequeueDelay)
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
			"ControllerError",
			fmt.Sprintf("Failed to list pods: %s", err),
		)
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetProvisioningCondition,
			Status:  "False",
			Reason:  ReasonInProgress,
			Message: "Failed to check Pods .status.phase",
		})
		return r.updateStatus(ctx, storageNodeSet, DefaultRequeueDelay)
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
			fmt.Sprintf("Waiting for number of running pods to match expected: %d != %d", runningPods, storageNodeSet.Spec.Nodes),
		)
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetProvisioningCondition,
			Status:  "False",
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Number of running nodes does not match expected: %d != %d", runningPods, storageNodeSet.Spec.Nodes),
		})
		return r.updateStatus(ctx, storageNodeSet, DefaultRequeueDelay)
	}

	meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
		Type:    NodeSetProvisioningCondition,
		Status:  "True",
		Reason:  ReasonCompleted,
		Message: fmt.Sprintf("Successfully scaled to desired number of nodes: %d", storageNodeSet.Spec.Nodes),
	})
	r.Log.Info("complete step waitForStatefulSetToScale")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) handlePauseResume(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handlePauseResume")

	if storageNodeSet.Status.State == StorageNodeSetProvisioning {
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:   NodeSetReadyCondition,
			Status: "True",
			Reason: ReasonCompleted,
		})
		storageNodeSet.Status.State = StorageNodeSetReady
		return r.updateStatus(ctx, storageNodeSet, StatusUpdateRequeueDelay)
	}

	if storageNodeSet.Status.State == StorageNodeSetReady && storageNodeSet.Spec.Pause {
		r.Log.Info("`pause: true` was noticed, moving StorageNodeSet to state `Paused`")
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetReadyCondition,
			Status:  "False",
			Reason:  ReasonNotRequired,
			Message: "Transitioning to state Paused",
		})
		storageNodeSet.Status.State = StorageNodeSetPaused
		return r.updateStatus(ctx, storageNodeSet, StatusUpdateRequeueDelay)
	}

	if storageNodeSet.Status.State == StorageNodeSetPaused && !storageNodeSet.Spec.Pause {
		r.Log.Info("`pause: false` was noticed, moving StorageNodeSet to state `Ready`")
		meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetPausedCondition,
			Status:  "False",
			Reason:  ReasonNotRequired,
			Message: "Transitioning to state Ready",
		})
		storageNodeSet.Status.State = StorageNodeSetReady
		return r.updateStatus(ctx, storageNodeSet, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step handlePauseResume")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetResource,
	requeueAfter time.Duration,
) (bool, ctrl.Result, error) {
	r.Log.Info("running updateStatus handler")

	if meta.IsStatusConditionTrue(storageNodeSet.Status.Conditions, NodeSetPreparingCondition) &&
		meta.IsStatusConditionTrue(storageNodeSet.Status.Conditions, NodeSetProvisioningCondition) {
		if storageNodeSet.Spec.Pause {
			meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
				Type:   NodeSetPausedCondition,
				Status: "True",
				Reason: ReasonCompleted,
			})
		} else {
			meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
				Type:   NodeSetReadyCondition,
				Status: "True",
				Reason: ReasonCompleted,
			})
		}
	} else {
		if storageNodeSet.Spec.Pause {
			meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
				Type:   NodeSetPausedCondition,
				Status: "False",
				Reason: ReasonInProgress,
			})
		} else {
			meta.SetStatusCondition(&storageNodeSet.Status.Conditions, metav1.Condition{
				Type:   NodeSetReadyCondition,
				Status: "False",
				Reason: ReasonInProgress,
			})
		}
	}

	crStorageNodeSet := &v1alpha1.StorageNodeSet{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: storageNodeSet.Namespace,
		Name:      storageNodeSet.Name,
	}, crStorageNodeSet)
	if err != nil {
		r.Recorder.Event(
			storageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching CR before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := crStorageNodeSet.Status.State
	crStorageNodeSet.Status.State = storageNodeSet.Status.State
	crStorageNodeSet.Status.Conditions = storageNodeSet.Status.Conditions
	if err = r.Status().Update(ctx, crStorageNodeSet); err != nil {
		r.Recorder.Event(
			storageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	if oldStatus != storageNodeSet.Status.State {
		r.Recorder.Event(
			storageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("State moved from %s to %s", oldStatus, storageNodeSet.Status.State),
		)
	}

	r.Log.Info("complete updateStatus handler")
	return Stop, ctrl.Result{RequeueAfter: requeueAfter}, nil
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
