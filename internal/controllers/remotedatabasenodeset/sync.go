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

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func (r *Reconciler) Sync(ctx context.Context, crRemoteDatabaseNodeSet *ydbv1alpha1.RemoteDatabaseNodeSet) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	remoteDatabaseNodeSet := resources.NewRemoteDatabaseNodeSet(crRemoteDatabaseNodeSet)
	stop, result, err = r.handleResourcesSync(ctx, &remoteDatabaseNodeSet)
	if stop {
		return result, err
	}

	stop, result, err = r.updateStatus(ctx, crRemoteDatabaseNodeSet)
	if stop {
		return result, err
	}

	return result, err
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

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
	r.Log.Info("resource sync complete")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	crRemoteDatabaseNodeSet *ydbv1alpha1.RemoteDatabaseNodeSet,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step updateStatus")

	databaseNodeSet := &ydbv1alpha1.DatabaseNodeSet{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      crRemoteDatabaseNodeSet.Name,
		Namespace: crRemoteDatabaseNodeSet.Namespace,
	}, databaseNodeSet)
	if err != nil {
		r.Recorder.Event(
			crRemoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching DatabaseNodeSet before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := crRemoteDatabaseNodeSet.Status.State
	crRemoteDatabaseNodeSet.Status.State = databaseNodeSet.Status.State
	crRemoteDatabaseNodeSet.Status.Conditions = databaseNodeSet.Status.Conditions

	err = r.RemoteClient.Status().Update(ctx, crRemoteDatabaseNodeSet)
	if err != nil {
		r.Recorder.Event(
			crRemoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status on remote cluster: %s", err),
		)
		r.RemoteRecorder.Event(
			crRemoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	} else if oldStatus != crRemoteDatabaseNodeSet.Status.State {
		r.Recorder.Event(
			crRemoteDatabaseNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("DatabaseNodeSet moved from %s to %s on remote cluster", oldStatus, crRemoteDatabaseNodeSet.Status.State),
		)
		r.RemoteRecorder.Event(
			crRemoteDatabaseNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("DatabaseNodeSet moved from %s to %s", oldStatus, crRemoteDatabaseNodeSet.Status.State),
		)
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}
