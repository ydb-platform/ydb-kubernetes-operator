package remotestoragenodeset

import (
	"context"
	"fmt"
	"reflect"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	ydblabels "github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

var (
	annotator  = patch.NewAnnotator(ydbannotations.LastAppliedAnnotation)
	patchMaker = patch.NewPatchMaker(annotator)
)

func (r *Reconciler) initRemoteResourcesStatus(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step initRemoteResourcesStatus")
	syncedResources := []v1alpha1.RemoteResource{}
	// copy actual slice to local variable
	if remoteStorageNodeSet.Status.RemoteResources != nil {
		syncedResources = append(syncedResources, remoteStorageNodeSet.Status.RemoteResources...)
	}

	for idx := range remoteObjects {
		remoteObj := remoteObjects[idx]
		remoteObjGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
		if err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to recognize GVK for remote object %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		existInStatus := false
		for i := range syncedResources {
			remoteResource := syncedResources[i]
			if resources.EqualRemoteResourceWithObject(
				&remoteResource,
				remoteStorageNodeSet.Namespace,
				remoteObj,
				remoteObjGVK,
			) {
				existInStatus = true
				break
			}
		}

		if !existInStatus {
			remoteStorageNodeSet.Status.RemoteResources = append(
				remoteStorageNodeSet.Status.RemoteResources,
				v1alpha1.RemoteResource{
					Group:      remoteObjGVK.Group,
					Version:    remoteObjGVK.Version,
					Kind:       remoteObjGVK.Kind,
					Name:       remoteObj.GetName(),
					State:      ResourceSyncPending,
					Conditions: []metav1.Condition{},
				},
			)
		}
	}

	return r.updateRemoteResourcesStatus(ctx, remoteStorageNodeSet)
}

func (r *Reconciler) updateRemoteResourcesStatus(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
) (bool, ctrl.Result, error) {
	crRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
	err := r.RemoteClient.Get(ctx, types.NamespacedName{
		Name:      remoteStorageNodeSet.Name,
		Namespace: remoteStorageNodeSet.Namespace,
	}, crRemoteStorageNodeSet)
	if err != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching RemoteStorageNodeSet on remote cluster before remote status update",
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching RemoteStorageNodeSet before remote status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldSyncResources := append([]v1alpha1.RemoteResource{}, crRemoteStorageNodeSet.Status.RemoteResources...)
	crRemoteStorageNodeSet.Status.RemoteResources = append([]v1alpha1.RemoteResource{}, remoteStorageNodeSet.Status.RemoteResources...)

	if !reflect.DeepEqual(oldSyncResources, remoteStorageNodeSet.Status.RemoteResources) {
		if err = r.RemoteClient.Status().Update(ctx, crRemoteStorageNodeSet); err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to update status for remote resources on remote cluster: %s", err),
			)
			r.RemoteRecorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to update status for remote resources: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			"Status updated for remote resources on remote cluster",
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			"Status updated for remote resources",
		)
		return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) syncRemoteObjects(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step syncRemoteObjects")

	for _, remoteObj := range remoteObjects {
		// Determine actual GVK for generic client.Object
		remoteObjGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
		if err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to recognize GVK for remote object %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		// Get object to sync from remote cluster
		err = r.RemoteClient.Get(ctx, types.NamespacedName{
			Name:      remoteObj.GetName(),
			Namespace: remoteObj.GetNamespace(),
		}, remoteObj)
		if err != nil {
			// Resource not found on remote cluster but we should retry
			if apierrors.IsNotFound(err) {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Resource %s with name %s was not found on remote cluster: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
				r.RemoteRecorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Resource %s with name %s was not found: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
			} else {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s on remote cluster: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
				r.RemoteRecorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
			}
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		// Create client.Object from api.RemoteResource struct
		localObj := resources.CreateResource(remoteObj)
		remoteStorageNodeSet.SetPrimaryResourceAnnotations(localObj)

		// Check object existence in local cluster
		objExist := false
		if err = r.Client.Get(ctx, types.NamespacedName{
			Name:      remoteObj.GetName(),
			Namespace: remoteObj.GetNamespace(),
		}, localObj); err != nil {
			if !apierrors.IsNotFound(err) {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			objExist = true
		}

		if objExist {
			// Update client.Object for local object with spec from remote object
			updatedObj := resources.UpdateResource(localObj, remoteObj)
			remoteStorageNodeSet.SetPrimaryResourceAnnotations(updatedObj)
			// Remote object existing in local cluster, сheck the need for an update
			// Get diff resources and compare bytes by k8s-objectmatcher PatchMaker
			patched, err := r.patchRemoteObject(ctx, localObj, updatedObj)
			if err != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to patch resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
			}
			if patched {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeNormal,
					"Provisioning",
					fmt.Sprintf("RemoteSync UPDATE resource %s with name %s resourceVersion %s", remoteObjGVK.Kind, remoteObj.GetName(), remoteObj.GetResourceVersion()),
				)
			}
		} else {
			// Object does not exist in local cluster
			// Try to create resource in remote cluster
			if err := r.Client.Create(ctx, localObj); err != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to create resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
			}
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				fmt.Sprintf("RemoteSync CREATE resource %s with name %s", remoteObjGVK.Kind, remoteObj.GetName()),
			)
		}

		// Update status for remote resource in RemoteStorageNodeSet object
		remoteStorageNodeSet.SetRemoteResourceStatus(localObj, remoteObjGVK)
	}

	return r.updateRemoteResourcesStatus(ctx, remoteStorageNodeSet)
}

func (r *Reconciler) removeUnusedRemoteObjects(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step removeUnusedRemoteObjects")
	// We should check every remote resource to need existence in cluster
	// Get processed remote resources from object Status
	candidatesToDelete := []v1alpha1.RemoteResource{}

	// Check RemoteResource usage in local StorageNodeSet object
	for _, remoteResource := range remoteStorageNodeSet.Status.RemoteResources {
		exist, err := resources.CheckRemoteResourceUsage(
			remoteStorageNodeSet.Namespace, r.Scheme, remoteResource, remoteObjects,
		)
		if err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to check usage in current StorageNodeSet: %v", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		if !exist {
			candidatesToDelete = append(candidatesToDelete, remoteResource)
		}
	}

	// Сhecking to avoid unnecessary List request
	//nolint:nestif
	if len(candidatesToDelete) > 0 {
		// Get remote objects from another StorageNodeSet spec
		remoteObjectsFromAnother, err := r.getUsedRemoteObjects(ctx, remoteStorageNodeSet)
		if err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to get remote objects from another StorageNodeSets: %v", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		// Check RemoteResource usage in another objects
		for _, remoteResource := range candidatesToDelete {
			remoteObj, err := resources.ConvertRemoteResourceToObject(remoteResource, remoteStorageNodeSet.Namespace)
			if err != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to convert RemoteResource %s with name %s to object: %v", remoteResource.Kind, remoteResource.Name, err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}

			if err := r.Client.Get(ctx, types.NamespacedName{
				Name:      remoteObj.GetName(),
				Namespace: remoteObj.GetNamespace(),
			}, remoteObj); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get RemoteResource %s with name %s as object: %v", remoteResource.Kind, remoteResource.Name, err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}

			existInStorage, err := resources.CheckRemoteResourceUsage(
				remoteStorageNodeSet.Namespace, r.Scheme, remoteResource, remoteObjectsFromAnother,
			)
			if err != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to check RemoteResource usage in another StorageNodeSets: %v", err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}

			// Skip resource deletion because it using in some Database
			// check by existence of annotation `ydb.tech/primary-resource-database`
			_, existInDatabase := remoteObj.GetAnnotations()[ydbannotations.PrimaryResourceDatabaseAnnotation]

			if existInStorage || existInDatabase {
				// Only remove annotation to unbind remote objects from Storage
				// Another StorageNodeSet receive an event and reattach it
				if err := r.unbindRemoteObject(ctx, remoteStorageNodeSet, remoteObj); err != nil {
					r.Recorder.Event(
						remoteStorageNodeSet,
						corev1.EventTypeWarning,
						"ControllerError",
						fmt.Sprintf("Failed to unbind remote object from StorageNodeSet: %v", err),
					)
					return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
				}
			} else {
				// Delete unused remote object from namespace
				if err := r.deleteRemoteObject(ctx, remoteStorageNodeSet, remoteObj); err != nil {
					r.Recorder.Event(
						remoteStorageNodeSet,
						corev1.EventTypeWarning,
						"ControllerError",
						fmt.Sprintf("Failed to delete remote object from namespace: %v", err),
					)
				}
			}
		}
	}

	return r.updateRemoteResourcesStatus(ctx, remoteStorageNodeSet)
}

func (r *Reconciler) getUsedRemoteObjects(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
) ([]client.Object, error) {
	storageNodeSets := &v1alpha1.StorageNodeSetList{}

	// Create label requirement that label `ydb.tech/storage-nodeset` which not equal
	// to current StorageNodeSet object for exclude current nodeSet from List result
	labelRequirement, err := labels.NewRequirement(
		ydblabels.StorageNodeSetComponent,
		selection.NotEquals,
		[]string{remoteStorageNodeSet.Labels[ydblabels.StorageNodeSetComponent]},
	)
	if err != nil {
		return nil, err
	}

	// Search another StorageNodeSets in current namespace with the same StorageRef
	// but exclude current nodeSet from result
	if err := r.Client.List(
		ctx,
		storageNodeSets,
		client.InNamespace(remoteStorageNodeSet.Namespace),
		client.MatchingLabelsSelector{
			Selector: labels.NewSelector().Add(*labelRequirement),
		},
		client.MatchingFields{
			StorageRefField: remoteStorageNodeSet.Spec.StorageRef.Name,
		},
	); err != nil {
		return nil, err
	}

	remoteObjects := []client.Object{}
	for _, storageNodeSet := range storageNodeSets.Items {
		remoteStorageNodeSet := resources.NewRemoteStorageNodeSet(
			&v1alpha1.RemoteStorageNodeSet{Spec: storageNodeSet.Spec},
		)
		remoteObjects = append(remoteObjects, remoteStorageNodeSet.GetRemoteObjects()...)
	}

	return remoteObjects, nil
}

func (r *Reconciler) patchRemoteObject(
	ctx context.Context,
	localObj, remoteObj client.Object,
) (bool, error) {
	// Get diff resources and compare bytes by k8s-objectmatcher PatchMaker
	patchResult, err := patchMaker.Calculate(localObj, remoteObj,
		[]patch.CalculateOption{
			patch.IgnoreStatusFields(),
		}...,
	)
	if err != nil {
		return false, err
	}

	if !patchResult.IsEmpty() {
		r.Log.Info("Got a patch for resource", "name", remoteObj.GetName(), "result", patchResult.Patch)
		if err := r.Client.Patch(ctx, remoteObj, client.RawPatch(types.StrategicMergePatchType, patchResult.Patch)); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

func (r *Reconciler) unbindRemoteObject(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObj client.Object,
) error {
	remoteObjGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
	if err != nil {
		return err
	}

	patch := []byte(fmt.Sprintf(`{"metadata": {"annotations": {"%s": null}}}`, ydbannotations.PrimaryResourceStorageAnnotation))
	if err := r.Client.Patch(ctx, remoteObj, client.RawPatch(types.MergePatchType, patch)); err != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to patch resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
		)
	}

	// Send event with information about unbinded resource
	r.Recorder.Event(
		remoteStorageNodeSet,
		corev1.EventTypeNormal,
		"Provisioning",
		fmt.Sprintf("RemoteSync UPDATE resource %s with name %s unbind from StorageNodeSet %s", remoteObjGVK.Kind, remoteObj.GetName(), remoteStorageNodeSet.Name),
	)

	// Remove status for remote resource from RemoteStorageNodeSet object
	remoteStorageNodeSet.RemoveRemoteResourceStatus(remoteObj, remoteObjGVK)

	return nil
}

func (r *Reconciler) deleteRemoteObject(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObj client.Object,
) error {
	remoteObjGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
	if err != nil {
		return err
	}
	// Try to delete unused resource from local cluster
	if err := r.Client.Delete(ctx, remoteObj); err != nil {
		if !apierrors.IsNotFound(err) {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to delete resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
			)
			return err
		}
	}

	// Remove status for remote resource from RemoteStorageNodeSet object
	remoteStorageNodeSet.RemoveRemoteResourceStatus(remoteObj, remoteObjGVK)

	// Send event with information about deleted resource
	r.Recorder.Event(
		remoteStorageNodeSet,
		corev1.EventTypeNormal,
		"Provisioning",
		fmt.Sprintf("RemoteSync DELETE resource %s with name %s", remoteObjGVK.Kind, remoteObj.GetName()),
	)
	return nil
}
