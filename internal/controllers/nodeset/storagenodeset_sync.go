package nodeset

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func (r *StorageNodeSetReconciler) Sync(ctx context.Context, crStorageNodeSet *ydbv1alpha1.StorageNodeSet, crStorage *ydbv1alpha1.Storage) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	storageNodeSet := resources.NewStorageNodeSet(crStorageNodeSet, crStorage)
	storageNodeSet.SetStatusOnFirstReconcile()

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

func (r *StorageNodeSetReconciler) handleResourcesSync(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	for _, builder := range storageNodeSet.GetResourceBuilders(r.Config) {
		newResource := builder.Placeholder(storageNodeSet)

		result, err := resources.CreateOrUpdateIgnoreStatus(ctx, r.Client, newResource, func() error {
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
		})

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
				string(Provisioning),
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}
	r.Log.Info("resource sync complete")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *StorageNodeSetReconciler) waitForStatefulSetToScale(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale for Storage")

	if storageNodeSet.Status.State == string(Pending) {
		msg := fmt.Sprintf("Starting to track number of running storage pods, expected: %d", storageNodeSet.Spec.Nodes)
		r.Recorder.Event(storageNodeSet, corev1.EventTypeNormal, string(Provisioning), msg)
		storageNodeSet.Status.State = string(Provisioning)
		return r.setState(ctx, storageNodeSet)
	}

	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      storageNodeSet.Name,
		Namespace: storageNodeSet.Namespace,
	}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			storageNodeSet,
			corev1.EventTypeNormal,
			"Syncing",
			fmt.Sprintf("Failed to get StatefulSets: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	podLabels := labels.Common(storageNodeSet.Name, make(map[string]string))
	podLabels.Merge(map[string]string{
		labels.ComponentKey: labels.StorageComponent,
	})

	matchingLabels := client.MatchingLabels{}
	for k, v := range podLabels {
		matchingLabels[k] = v
	}

	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(storageNodeSet.Namespace),
		matchingLabels,
	}

	err = r.List(ctx, podList, opts...)
	if err != nil {
		r.Recorder.Event(
			storageNodeSet,
			corev1.EventTypeNormal,
			"Syncing",
			fmt.Sprintf("Failed to list cluster pods: %s", err),
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
		msg := fmt.Sprintf("Waiting for number of running storageNodeSet pods to match expected: %d != %d", runningPods, storageNodeSet.Spec.Nodes)
		r.Recorder.Event(storageNodeSet, corev1.EventTypeNormal, string(Provisioning), msg)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *StorageNodeSetReconciler) setState(
	ctx context.Context,
	storageNodeSet *resources.StorageNodeSetBuilder,
) (bool, ctrl.Result, error) {
	crStorageNodeSet := &ydbv1alpha1.StorageNodeSet{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: storageNodeSet.Namespace,
		Name:      storageNodeSet.Name,
	}, crStorageNodeSet)
	if err != nil {
		r.Recorder.Event(crStorageNodeSet, corev1.EventTypeWarning, "ControllerError", "Failed fetching CR before status update")
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := crStorageNodeSet.Status.State
	crStorageNodeSet.Status.State = storageNodeSet.Status.State
	crStorageNodeSet.Status.Conditions = storageNodeSet.Status.Conditions

	err = r.Status().Update(ctx, crStorageNodeSet)
	if err != nil {
		r.Recorder.Event(crStorageNodeSet, corev1.EventTypeWarning, "ControllerError", fmt.Sprintf("Failed setting status: %s", err))
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	} else if oldStatus != storageNodeSet.Status.State {
		r.Recorder.Event(
			crStorageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("StorageNodeSet moved from %s to %s", oldStatus, storageNodeSet.Status.State),
		)
	}

	return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
}
