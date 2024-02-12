package remotedatabasenodeset

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

	stop, result, err = r.updateStatus(ctx, &remoteDatabaseNodeSet)
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

			// Set primary resource annotation
			newResource.SetAnnotations(map[string]string{
				PrimaryResourceNameAnnotation:      remoteDatabaseNodeSet.GetName(),
				PrimaryResourceNamespaceAnnotation: remoteDatabaseNodeSet.GetNamespace(),
				PrimaryResourceTypeAnnotation:      remoteDatabaseNodeSet.GetObjectKind().GroupVersionKind().Kind,
			})
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
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step updateStatus")

	databaseNodeSet := ydbv1alpha1.DatabaseNodeSet{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      remoteDatabaseNodeSet.Name,
		Namespace: remoteDatabaseNodeSet.Namespace,
	}, &databaseNodeSet)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("DatabaseNodeSet with name %s was not found: %s", remoteDatabaseNodeSet.Name, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get DatabaseNodeSet: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := remoteDatabaseNodeSet.Status.State
	remoteDatabaseNodeSet.Status.State = databaseNodeSet.Status.State
	remoteDatabaseNodeSet.Status.Conditions = databaseNodeSet.Status.Conditions

	err = r.RemoteClient.Status().Update(ctx, remoteDatabaseNodeSet)
	if err != nil {
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status on remote cluster: %s", err),
		)
		r.RemoteRecorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	} else if oldStatus != databaseNodeSet.Status.State {
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("RemoteDatabaseNodeSet moved from %s to %s on remote cluster", oldStatus, remoteDatabaseNodeSet.Status.State),
		)
		r.RemoteRecorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("RemoteDatabaseNodeSet moved from %s to %s", oldStatus, remoteDatabaseNodeSet.Status.State),
		)
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}
