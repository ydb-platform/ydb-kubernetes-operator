package databasenodeset

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

func (r *Reconciler) Sync(ctx context.Context, crDatabaseNodeSet *v1alpha1.DatabaseNodeSet) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	databaseNodeSet := resources.NewDatabaseNodeSet(crDatabaseNodeSet)

	stop, result, err = r.setInitialStatus(ctx, &databaseNodeSet)
	if stop {
		return result, err
	}

	stop, result, err = r.handleResourcesSync(ctx, &databaseNodeSet)
	if stop {
		return result, err
	}

	stop, result, err = r.waitForStatefulSetToScale(ctx, &databaseNodeSet)
	if stop {
		return result, err
	}

	stop, result, err = r.handlePauseResume(ctx, &databaseNodeSet)
	if stop {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) setInitialStatus(
	ctx context.Context,
	databaseNodeSet *resources.DatabaseNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")

	if databaseNodeSet.Status.Conditions == nil {
		databaseNodeSet.Status.Conditions = []metav1.Condition{}

		meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
			Type:   NodeSetReadyCondition,
			Status: "Unknown",
			Reason: ReasonInProgress,
		})

		if databaseNodeSet.Spec.Pause {
			meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
				Type:    DatabasePausedCondition,
				Status:  "Unknown",
				Reason:  ReasonInProgress,
				Message: "Transitioning to state Paused",
			})
		}

		return r.updateStatus(ctx, databaseNodeSet, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step setInitialStatus")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	databaseNodeSet *resources.DatabaseNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	if !databaseNodeSet.Spec.OperatorSync {
		r.Log.Info("`operatorSync: false` is set, no further steps will be run")
		r.Recorder.Event(
			databaseNodeSet,
			corev1.EventTypeNormal,
			string(DatabaseNodeSetPreparing),
			fmt.Sprintf("Found .spec.operatorSync set to %t, skip further steps", databaseNodeSet.Spec.OperatorSync),
		)
		return Stop, ctrl.Result{Requeue: false}, nil
	}

	if databaseNodeSet.Status.State == DatabaseNodeSetPending {
		meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetPreparingCondition,
			Status:  "Unknown",
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Waiting for sync resources for generation %d", databaseNodeSet.Generation),
		})
		databaseNodeSet.Status.State = DatabaseNodeSetPreparing
		return r.updateStatus(ctx, databaseNodeSet, StatusUpdateRequeueDelay)
	}

	for _, builder := range databaseNodeSet.GetResourceBuilders(r.Config) {
		newResource := builder.Placeholder(databaseNodeSet)

		result, err := resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
			var err error

			err = builder.Build(newResource)
			if err != nil {
				r.Recorder.Event(
					databaseNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}
			err = ctrl.SetControllerReference(databaseNodeSet.Unwrap(), newResource, r.Scheme)
			if err != nil {
				r.Recorder.Event(
					databaseNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Error setting controller reference for resource: %s", err),
				)
				return err
			}

			return nil
		}, shouldIgnoreDatabaseNodeSetChange(databaseNodeSet))

		eventMessage := fmt.Sprintf(
			"Resource: %s, Namespace: %s, Name: %s",
			reflect.TypeOf(newResource),
			newResource.GetNamespace(),
			newResource.GetName(),
		)
		if err != nil {
			r.Recorder.Event(
				databaseNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				eventMessage+fmt.Sprintf(", failed to sync, error: %s", err),
			)
			meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
				Type:    NodeSetPreparingCondition,
				Status:  "False",
				Reason:  ReasonInProgress,
				Message: fmt.Sprintf("Failed to sync resources for generation %d", databaseNodeSet.Generation),
			})
			return r.updateStatus(ctx, databaseNodeSet, DefaultRequeueDelay)
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				databaseNodeSet,
				corev1.EventTypeNormal,
				string(DatabaseNodeSetProvisioning),
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}

	meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
		Type:    NodeSetPreparingCondition,
		Status:  "True",
		Reason:  ReasonCompleted,
		Message: fmt.Sprintf("Successfully synced resources for generation %d", databaseNodeSet.Generation),
	})
	r.Log.Info("complete step handleResourcesSync")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) waitForStatefulSetToScale(
	ctx context.Context,
	databaseNodeSet *resources.DatabaseNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale")

	if databaseNodeSet.Status.State == DatabaseNodeSetPreparing {
		meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetProvisioningCondition,
			Status:  "Unknown",
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Waiting for scale to desired nodes: %d", databaseNodeSet.Spec.Nodes),
		})
		databaseNodeSet.Status.State = DatabaseNodeSetProvisioning
		return r.updateStatus(ctx, databaseNodeSet, StatusUpdateRequeueDelay)
	}

	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      databaseNodeSet.Name,
		Namespace: databaseNodeSet.Namespace,
	}, found)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Recorder.Event(
				databaseNodeSet,
				corev1.EventTypeWarning,
				string(DatabaseNodeSetProvisioning),
				fmt.Sprintf("Failed to find StatefulSet: %s", err),
			)
		} else {
			r.Recorder.Event(
				databaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to get StatefulSet: %s", err),
			)
		}
		meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetProvisioningCondition,
			Status:  "False",
			Reason:  ReasonInProgress,
			Message: "Failed to check StatefulSet .spec.replicas",
		})
		return r.updateStatus(ctx, databaseNodeSet, DefaultRequeueDelay)
	}

	matchingLabels := client.MatchingLabels{}
	for k, v := range databaseNodeSet.Labels {
		matchingLabels[k] = v
	}

	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(databaseNodeSet.Namespace),
		matchingLabels,
	}
	if err = r.List(ctx, podList, opts...); err != nil {
		r.Recorder.Event(
			databaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to list pods: %s", err),
		)
		meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetProvisioningCondition,
			Status:  "False",
			Reason:  ReasonInProgress,
			Message: "Failed to check Pods .status.phase",
		})
		return r.updateStatus(ctx, databaseNodeSet, DefaultRequeueDelay)
	}

	runningPods := 0
	for _, e := range podList.Items {
		if e.Status.Phase == "Running" {
			runningPods++
		}
	}

	if runningPods != int(databaseNodeSet.Spec.Nodes) {
		r.Recorder.Event(
			databaseNodeSet,
			corev1.EventTypeNormal,
			string(DatabaseNodeSetProvisioning),
			fmt.Sprintf("Waiting for number of running pods to match expected: %d != %d", runningPods, databaseNodeSet.Spec.Nodes),
		)
		meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetProvisioningCondition,
			Status:  "False",
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Number of running nodes does not match expected: %d != %d", runningPods, databaseNodeSet.Spec.Nodes),
		})
		return r.updateStatus(ctx, databaseNodeSet, DefaultRequeueDelay)
	}

	meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
		Type:    NodeSetProvisioningCondition,
		Status:  "True",
		Reason:  ReasonCompleted,
		Message: fmt.Sprintf("Successfully scaled to desired number of nodes: %d", databaseNodeSet.Spec.Nodes),
	})
	r.Log.Info("complete step waitForStatefulSetToScale")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) handlePauseResume(
	ctx context.Context,
	databaseNodeSet *resources.DatabaseNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handlePauseResume")

	if databaseNodeSet.Status.State == DatabaseNodeSetProvisioning {
		meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
			Type:   NodeSetReadyCondition,
			Status: "True",
			Reason: ReasonCompleted,
		})
		databaseNodeSet.Status.State = DatabaseNodeSetReady
		return r.updateStatus(ctx, databaseNodeSet, StatusUpdateRequeueDelay)
	}

	if databaseNodeSet.Status.State == DatabaseNodeSetReady && databaseNodeSet.Spec.Pause {
		r.Log.Info("`pause: true` was noticed, moving DatabaseNodeSet to state `Paused`")
		meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetReadyCondition,
			Status:  "False",
			Reason:  ReasonNotRequired,
			Message: "Transitioning to state Paused",
		})
		databaseNodeSet.Status.State = DatabaseNodeSetPaused
		return r.updateStatus(ctx, databaseNodeSet, StatusUpdateRequeueDelay)
	}

	if databaseNodeSet.Status.State == DatabaseNodeSetPaused && !databaseNodeSet.Spec.Pause {
		r.Log.Info("`pause: false` was noticed, moving DatabaseNodeSet to state `Ready`")
		meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
			Type:    NodeSetPausedCondition,
			Status:  "False",
			Reason:  ReasonNotRequired,
			Message: "Transitioning to state Ready",
		})
		databaseNodeSet.Status.State = DatabaseNodeSetReady
		return r.updateStatus(ctx, databaseNodeSet, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step handlePauseResume")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	databaseNodeSet *resources.DatabaseNodeSetResource,
	requeueAfter time.Duration,
) (bool, ctrl.Result, error) {
	r.Log.Info("running updateStatus handler")

	if meta.IsStatusConditionTrue(databaseNodeSet.Status.Conditions, NodeSetPreparingCondition) &&
		meta.IsStatusConditionTrue(databaseNodeSet.Status.Conditions, NodeSetProvisioningCondition) {
		if databaseNodeSet.Spec.Pause {
			meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
				Type:   NodeSetPausedCondition,
				Status: "True",
				Reason: ReasonCompleted,
			})
		} else {
			meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
				Type:   NodeSetReadyCondition,
				Status: "True",
				Reason: ReasonCompleted,
			})
		}
	} else {
		if databaseNodeSet.Spec.Pause {
			meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
				Type:   NodeSetPausedCondition,
				Status: "False",
				Reason: ReasonInProgress,
			})
		} else {
			meta.SetStatusCondition(&databaseNodeSet.Status.Conditions, metav1.Condition{
				Type:   NodeSetReadyCondition,
				Status: "False",
				Reason: ReasonInProgress,
			})
		}
	}

	crDatabaseNodeSet := &v1alpha1.DatabaseNodeSet{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: databaseNodeSet.Namespace,
		Name:      databaseNodeSet.Name,
	}, crDatabaseNodeSet)
	if err != nil {
		r.Recorder.Event(
			databaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching CR before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := crDatabaseNodeSet.Status.State
	crDatabaseNodeSet.Status.State = databaseNodeSet.Status.State
	crDatabaseNodeSet.Status.Conditions = databaseNodeSet.Status.Conditions
	if err = r.Status().Update(ctx, crDatabaseNodeSet); err != nil {
		r.Recorder.Event(
			databaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	if oldStatus != databaseNodeSet.Status.State {
		r.Recorder.Event(
			databaseNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("State moved from %s to %s", oldStatus, databaseNodeSet.Status.State),
		)
	}

	r.Log.Info("complete updateStatus handler")
	return Stop, ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func shouldIgnoreDatabaseNodeSetChange(databaseNodeSet *resources.DatabaseNodeSetResource) resources.IgnoreChangesFunction {
	return func(oldObj, newObj runtime.Object) bool {
		if _, ok := newObj.(*appsv1.StatefulSet); ok {
			if databaseNodeSet.Spec.Pause && *oldObj.(*appsv1.StatefulSet).Spec.Replicas == 0 {
				return true
			}
		}
		return false
	}
}
