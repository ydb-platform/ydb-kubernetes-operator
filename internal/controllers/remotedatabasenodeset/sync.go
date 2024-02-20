package remotedatabasenodeset

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

func (r *Reconciler) Sync(ctx context.Context, crRemoteDatabaseNodeSet *v1alpha1.RemoteDatabaseNodeSet) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	remoteDatabaseNodeSet := resources.NewRemoteDatabaseNodeSet(crRemoteDatabaseNodeSet)
	remoteResources := remoteDatabaseNodeSet.GetRemoteResources()

	stop, result, err = r.initRemoteResourcesStatus(ctx, &remoteDatabaseNodeSet, remoteResources)
	if stop {
		return result, err
	}

	stop, result, err = r.syncRemoteResources(ctx, &remoteDatabaseNodeSet, remoteResources)
	if stop {
		return result, err
	}

	stop, result, err = r.handleResourcesSync(ctx, &remoteDatabaseNodeSet)
	if stop {
		return result, err
	}

	stop, result, err = r.removeUnusedRemoteResources(ctx, &remoteDatabaseNodeSet, remoteResources)
	if stop {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync for RemoteDatabaseNodeSet")

	for _, builder := range remoteDatabaseNodeSet.GetResourceBuilders() {
		newResource := builder.Placeholder(remoteDatabaseNodeSet)

		result, err := resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
			err := builder.Build(newResource)
			if err != nil {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}
			remoteDatabaseNodeSet.SetPrimaryResourceAnnotations(newResource)

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
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				eventMessage+fmt.Sprintf(", failed to sync, error: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}

	return r.updateRemoteStatus(ctx, remoteDatabaseNodeSet)
}

func (r *Reconciler) updateRemoteStatus(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step updateRemoteStatus for RemoteDatabaseNodeSet")

	crDatabaseNodeSet := &v1alpha1.DatabaseNodeSet{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      remoteDatabaseNodeSet.Name,
		Namespace: remoteDatabaseNodeSet.Namespace,
	}, crDatabaseNodeSet); err != nil {
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching DatabaseNodeSet before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	crRemoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}
	if err := r.RemoteClient.Get(ctx, types.NamespacedName{
		Name:      remoteDatabaseNodeSet.Name,
		Namespace: remoteDatabaseNodeSet.Namespace,
	}, crRemoteDatabaseNodeSet); err != nil {
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching RemoteDatabaseNodeSet on remote cluster before status update",
		)
		r.RemoteRecorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching RemoteDatabaseNodeSet before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := crRemoteDatabaseNodeSet.Status.State
	crRemoteDatabaseNodeSet.Status.State = crDatabaseNodeSet.Status.State
	crRemoteDatabaseNodeSet.Status.Conditions = crDatabaseNodeSet.Status.Conditions

	if oldStatus != crRemoteDatabaseNodeSet.Status.State {
		if err := r.RemoteClient.Status().Update(ctx, crRemoteDatabaseNodeSet); err != nil {
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to update status on remote cluster: %s", err),
			)
			r.RemoteRecorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to update status: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			"DatabaseNodeSet status updated on remote cluster",
		)
		r.RemoteRecorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			"RemoteDatabaseNodeSet status updated",
		)

		r.Log.Info("step updateRemoteStatus for RemoteDatabaseNodeSet requeue reconcile")
		return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
	}

	r.Log.Info("step updateRemoteStatus for RemoteDatabaseNodeSet completed")
	return Continue, ctrl.Result{Requeue: false}, nil
}
