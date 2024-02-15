package remotestoragenodeset

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func (r *Reconciler) Sync(ctx context.Context, crRemoteStorageNodeSet *ydbv1alpha1.RemoteStorageNodeSet) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	remoteStorageNodeSet := resources.NewRemoteStorageNodeSet(crRemoteStorageNodeSet)

	for _, object := range remoteStorageNodeSet.GetRemoteResources() {
		stop, result, err = r.syncRemoteObject(ctx, &remoteStorageNodeSet, object)
		if stop {
			return result, err
		}
	}

	stop, result, err = r.handleResourcesSync(ctx, &remoteStorageNodeSet)
	if stop {
		return result, err
	}

	stop, result, err = r.updateRemoteStatus(ctx, &remoteStorageNodeSet)
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

			resources.SetPrimaryResourceAnnotations(remoteStorageNodeSet, newResource)

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
	r.Log.Info("resource sync complete")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) updateRemoteStatus(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step updateStatus")

	storageNodeSet := &ydbv1alpha1.StorageNodeSet{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      remoteStorageNodeSet.Name,
		Namespace: remoteStorageNodeSet.Namespace,
	}, storageNodeSet)
	if err != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching StorageNodeSet before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := remoteStorageNodeSet.Status.State
	remoteStorageNodeSet.Status.State = storageNodeSet.Status.State
	remoteStorageNodeSet.Status.Conditions = storageNodeSet.Status.Conditions

	err = r.RemoteClient.Status().Update(ctx, remoteStorageNodeSet.RemoteStorageNodeSet)
	if err != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status on remote cluster: %s", err),
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	} else if oldStatus != remoteStorageNodeSet.Status.State {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("StorageNodeSet moved from %s to %s on remote cluster", oldStatus, remoteStorageNodeSet.Status.State),
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("StorageNodeSet moved from %s to %s", oldStatus, remoteStorageNodeSet.Status.State),
		)
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) syncRemoteObject(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObj client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleRemoteObjectSync")

	remoteObjName := remoteObj.GetName()
	remoteObjNamespace := remoteObj.GetNamespace()
	remoteObjGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
	if err != nil {
		r.Log.Error(err, "does not recognize GVK for resource %s", remoteObjName)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}
	remoteObjKind := remoteObjGVK.Kind

	err = r.RemoteClient.Get(ctx, types.NamespacedName{
		Name:      remoteObjName,
		Namespace: remoteObjNamespace,
	}, remoteObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("Resource %s with name %s was not found on remote cluster: %s", remoteObjKind, remoteObjName, err),
			)
			r.RemoteRecorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("Resource %s with name %s was not found: %s", remoteObjKind, remoteObjName, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get resource %s with name %s on remote cluster: %s", remoteObjKind, remoteObjName, err),
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteObjKind, remoteObjName, err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	// use unstrsuctured objects for client to use generic
	localObj := &unstructured.Unstructured{}
	localObj.SetGroupVersionKind(remoteObjGVK)
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      remoteObjName,
		Namespace: remoteObjNamespace,
	}, localObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			newResource := resources.CopyResource(remoteObj)
			resources.SetPrimaryResourceAnnotations(remoteStorageNodeSet, newResource)
			resources.SetRemoteResourceVersionAnnotation(remoteObj, newResource)
			if err := r.Client.Create(ctx, newResource); err != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to create resource %s with name %s: %s", remoteObjKind, remoteObjName, err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
			}
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				fmt.Sprintf("RemoteSync CREATE resource %s with name %s", remoteObjKind, remoteObjName),
			)
			return Continue, ctrl.Result{Requeue: false}, nil
		}
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteObjKind, remoteObjName, err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	// get current resource version from remoteObj
	remoteObjResourceVersion := remoteObj.GetResourceVersion()
	observedResourceVersion := localObj.GetAnnotations()[ydbannotations.RemoteResourceVersionAnnotation]
	if remoteObjResourceVersion != observedResourceVersion {
		updatedResource := resources.UpdateResource(localObj, remoteObj)
		resources.SetPrimaryResourceAnnotations(remoteStorageNodeSet, updatedResource)
		resources.SetRemoteResourceVersionAnnotation(remoteObj, updatedResource)
		if err := r.Client.Update(ctx, updatedResource); err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to update resource %s with name %s: %s", remoteObjKind, remoteObjName, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"Provisioning",
			fmt.Sprintf("RemoteSync UPDATE resource %s with name %s resourceVersion %s", remoteObjKind, remoteObjName, remoteObjResourceVersion),
		)
	}
	return Continue, ctrl.Result{Requeue: false}, nil
}
