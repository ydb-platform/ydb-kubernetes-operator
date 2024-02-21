package remotestoragenodeset

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func (r *Reconciler) Sync(ctx context.Context, crRemoteStorageNodeSet *v1alpha1.RemoteStorageNodeSet) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	remoteStorageNodeSet := resources.NewRemoteStorageNodeSet(crRemoteStorageNodeSet)
	remoteResources := remoteStorageNodeSet.GetRemoteResources()

	stop, result, err = r.initRemoteResourcesStatus(ctx, &remoteStorageNodeSet, remoteResources)
	if stop {
		return result, err
	}

	stop, result, err = r.syncRemoteResources(ctx, &remoteStorageNodeSet, remoteResources)
	if stop {
		return result, err
	}

	stop, result, err = r.handleResourcesSync(ctx, &remoteStorageNodeSet)
	if stop {
		return result, err
	}

	stop, result, err = r.removeUnusedRemoteResources(ctx, &remoteStorageNodeSet, remoteResources)
	if stop {
		return result, err
	}

	return result, err
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	for _, builder := range remoteStorageNodeSet.GetResourceBuilders() {
		newResource := builder.Placeholder(remoteStorageNodeSet)

		result, err := resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
			err := builder.Build(newResource)
			if err != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}
			remoteStorageNodeSet.SetPrimaryResourceAnnotations(newResource)

			return nil
		}, func(oldObj, newObj runtime.Object) bool {
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
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				eventMessage+fmt.Sprintf(", failed to sync, error: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}

	return r.updateRemoteStatus(ctx, remoteStorageNodeSet)
}

func (r *Reconciler) updateRemoteStatus(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step updateRemoteStatus")

	crStorageNodeSet := &v1alpha1.StorageNodeSet{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      remoteStorageNodeSet.Name,
		Namespace: remoteStorageNodeSet.Namespace,
	}, crStorageNodeSet); err != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching StorageNodeSet before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	crRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
	if err := r.RemoteClient.Get(ctx, types.NamespacedName{
		Name:      remoteStorageNodeSet.Name,
		Namespace: remoteStorageNodeSet.Namespace,
	}, crRemoteStorageNodeSet); err != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching RemoteStorageNodeSet on remote cluster before status update",
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching RemoteStorageNodeSet before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := crRemoteStorageNodeSet.Status.State
	if oldStatus != crStorageNodeSet.Status.State {
		crRemoteStorageNodeSet.Status.State = crStorageNodeSet.Status.State
		crRemoteStorageNodeSet.Status.Conditions = crStorageNodeSet.Status.Conditions
		if err := r.RemoteClient.Status().Update(ctx, crRemoteStorageNodeSet); err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to update status on remote cluster: %s", err),
			)
			r.RemoteRecorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to update status: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			"StorageNodeSet status updated on remote cluster",
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			"RemoteStorageNodeSet status updated",
		)
		r.Log.Info("step updateRemoteStatus for RemoteStorageNodeSet requeue reconcile")
		return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
	}

	r.Log.Info("step updateRemoteStatus for RemoteStorageNodeSet completed")
	return Continue, ctrl.Result{Requeue: false}, nil
}
