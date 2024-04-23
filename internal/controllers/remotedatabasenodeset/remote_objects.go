package remotedatabasenodeset

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
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step initRemoteResourcesStatus")

	syncedResources := []v1alpha1.RemoteResource{}
	// copy actual slice to local variable
	if remoteDatabaseNodeSet.Status.RemoteResources != nil {
		syncedResources = append(syncedResources, remoteDatabaseNodeSet.Status.RemoteResources...)
	}

	for idx := range remoteObjects {
		remoteObj := remoteObjects[idx]
		remoteObjGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
		if err != nil {
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to recognize GVK for remote object %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		existInStatus := false
		for i := range syncedResources {
			syncedResource := syncedResources[i]
			if resources.EqualRemoteResourceWithObject(
				&syncedResource,
				remoteDatabaseNodeSet.Namespace,
				remoteObj,
				remoteObjGVK,
			) {
				existInStatus = true
				break
			}
		}

		if !existInStatus {
			remoteDatabaseNodeSet.Status.RemoteResources = append(
				remoteDatabaseNodeSet.Status.RemoteResources,
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

	return r.updateRemoteResourcesStatus(ctx, remoteDatabaseNodeSet)
}

func (r *Reconciler) syncRemoteObjects(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step syncRemoteObjects")
	for _, remoteObj := range remoteObjects {
		// Determine actual GVK for generic client.Object
		remoteObjGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
		if err != nil {
			r.Recorder.Event(
				remoteDatabaseNodeSet,
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
			if apierrors.IsNotFound(err) {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Resource %s with name %s was not found on remote cluster: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
				r.RemoteRecorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Resource %s with name %s was not found: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
			} else {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s on remote cluster: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
				r.RemoteRecorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
			}
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		// Create client.Object from api.RemoteResource struct
		localObj := resources.CreateResource(remoteObj)
		remoteDatabaseNodeSet.SetPrimaryResourceAnnotations(localObj)
		// Check object existence in local cluster
		err = r.Client.Get(ctx, types.NamespacedName{
			Name:      remoteObj.GetName(),
			Namespace: remoteObj.GetNamespace(),
		}, localObj)
		//nolint:nestif
		if err != nil {
			if !apierrors.IsNotFound(err) {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			// Object does not exist in local cluster
			// Try to create resource in remote cluster
			if err := r.Client.Create(ctx, localObj); err != nil {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to create resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
			}
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				fmt.Sprintf("RemoteSync CREATE resource %s with name %s", remoteObjGVK.Kind, remoteObj.GetName()),
			)
		} else {
			// Update client.Object for local object with spec from remote object
			updatedObj := resources.UpdateResource(localObj, remoteObj)
			remoteDatabaseNodeSet.SetPrimaryResourceAnnotations(updatedObj)
			// Remote object existing in local cluster, сheck the need for an update
			// Get diff resources and compare bytes by k8s-objectmatcher PatchMaker
			updated, err := r.patchObject(ctx, localObj, updatedObj)
			if err != nil {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to patch resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
			}
			// Send event with information about updated resource
			if updated {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeNormal,
					"Provisioning",
					fmt.Sprintf("RemoteSync UPDATE resource %s with name %s resourceVersion %s", remoteObjGVK.Kind, remoteObj.GetName(), remoteObj.GetResourceVersion()),
				)
			}
		}
		// Set status for remote resource in RemoteDatabaseNodeSet object
		remoteDatabaseNodeSet.SetRemoteResourceStatus(localObj, remoteObjGVK)
	}
	return r.updateRemoteResourcesStatus(ctx, remoteDatabaseNodeSet)
}

func (r *Reconciler) removeUnusedRemoteObjects(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step removeUnusedRemoteObjects")

	// We should check every remote resource to need existence in cluster
	candidatesToDelete := []v1alpha1.RemoteResource{}

	// Check RemoteResource usage in local DatabaseNodeSet object
	for _, remoteResource := range remoteDatabaseNodeSet.Status.RemoteResources {
		exist, err := r.checkRemoteResourceUsage(remoteDatabaseNodeSet, &remoteResource, remoteObjects)
		if err != nil {
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to check usage in current DatabaseNodeSet: %v", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		if !exist {
			candidatesToDelete = append(candidatesToDelete, remoteResource)
		}
	}

	// Сhecking to avoid unnecessary List request
	if len(candidatesToDelete) > 0 {
		// Get remote objects from another DatabaseNodeSet spec
		remoteObjectsFromAnother, err := r.getRemoteObjectsFromAnother(ctx, remoteDatabaseNodeSet)
		if err != nil {
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to get remote objects from another DatabaseNodeSets: %v", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		// Check RemoteResource usage in another objects
		for _, remoteResource := range candidatesToDelete {
			remoteObj, err := resources.ConvertRemoteResourceToObject(remoteResource, remoteDatabaseNodeSet.Namespace)
			if err != nil {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
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
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get RemoteResource %s with name %s as object: %v", remoteResource.Kind, remoteResource.Name, err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}

			existInDatabase, err := r.checkRemoteResourceUsage(remoteDatabaseNodeSet, &remoteResource, remoteObjectsFromAnother)
			if err != nil {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to check RemoteResource usage in another DatabaseNodeSets: %v", err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}

			// Skip resource deletion because it using in some Storage
			// check by existence of annotation `ydb.tech/primary-resource-storage`
			_, existInStorage := remoteObj.GetAnnotations()[ydbannotations.PrimaryResourceStorageAnnotation]

			if existInDatabase || existInStorage {
				// Only remove annotation to unbind remote objects from Database
				// Another DatabaseNodeSet receive an event and reattach it
				if err := r.unbindRemoteObject(ctx, remoteDatabaseNodeSet, remoteObj); err != nil {
					r.Recorder.Event(
						remoteDatabaseNodeSet,
						corev1.EventTypeWarning,
						"ControllerError",
						fmt.Sprintf("Failed to unbind remote object from DatabaseNodeSet: %v", err),
					)
					return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
				}
			} else {
				// Delete unused remote object from namespace
				if err := r.deleteRemoteObject(ctx, remoteDatabaseNodeSet, remoteObj); err != nil {
					r.Recorder.Event(
						remoteDatabaseNodeSet,
						corev1.EventTypeWarning,
						"ControllerError",
						fmt.Sprintf("Failed to delete remote object from namespace: %v", err),
					)
				}
			}
		}
	}

	return r.updateRemoteResourcesStatus(ctx, remoteDatabaseNodeSet)
}

func (r *Reconciler) updateRemoteResourcesStatus(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
) (bool, ctrl.Result, error) {
	crRemoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}
	err := r.RemoteClient.Get(ctx, types.NamespacedName{
		Name:      remoteDatabaseNodeSet.Name,
		Namespace: remoteDatabaseNodeSet.Namespace,
	}, crRemoteDatabaseNodeSet)
	if err != nil {
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching RemoteDatabaseNodeSet on remote cluster before remote status update",
		)
		r.RemoteRecorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching RemoteDatabaseNodeSet before remote status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldSyncResources := append([]v1alpha1.RemoteResource{}, crRemoteDatabaseNodeSet.Status.RemoteResources...)
	crRemoteDatabaseNodeSet.Status.RemoteResources = append([]v1alpha1.RemoteResource{}, remoteDatabaseNodeSet.Status.RemoteResources...)

	if !reflect.DeepEqual(oldSyncResources, remoteDatabaseNodeSet.Status.RemoteResources) {
		if err = r.RemoteClient.Status().Update(ctx, crRemoteDatabaseNodeSet); err != nil {
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to update status for remote resources on remote cluster: %s", err),
			)
			r.RemoteRecorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed fetching RemoteDatabaseNodeSet before status update: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			"Status updated for remote resources on remote cluster",
		)
		r.RemoteRecorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeNormal,
			"StatusChanged",
			"Status updated for remote resources",
		)
		return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) checkRemoteResourceUsage(
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
	remoteResource *v1alpha1.RemoteResource,
	remoteObjects []client.Object,
) (bool, error) {
	for _, remoteObj := range remoteObjects {
		remoteObjGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
		if err != nil {
			return false, err
		}
		if resources.EqualRemoteResourceWithObject(
			remoteResource,
			remoteDatabaseNodeSet.Namespace,
			remoteObj,
			remoteObjGVK,
		) {
			return true, nil
		}
	}
	return false, nil
}

func (r *Reconciler) getRemoteObjectsFromAnother(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
) ([]client.Object, error) {
	databaseNodeSets := &v1alpha1.DatabaseNodeSetList{}

	// Create label requirement that label `ydb.tech/database-nodeset` which not equal
	// to current DatabaseNodeSet object for exclude current nodeSet from List result
	labelRequirement, err := labels.NewRequirement(
		ydblabels.DatabaseNodeSetComponent,
		selection.NotEquals,
		[]string{remoteDatabaseNodeSet.Labels[ydblabels.DatabaseNodeSetComponent]},
	)
	if err != nil {
		return nil, err
	}

	// Search another DatabaseNodeSets in current namespace with the same DatabaseRef
	// but exclude current nodeSet from result
	if err := r.Client.List(
		ctx,
		databaseNodeSets,
		client.InNamespace(remoteDatabaseNodeSet.Namespace),
		client.MatchingLabelsSelector{
			Selector: labels.NewSelector().Add(*labelRequirement),
		},
		client.MatchingFields{
			DatabaseRefField: remoteDatabaseNodeSet.Spec.DatabaseRef.Name,
		},
	); err != nil {
		return nil, err
	}

	remoteObjects := []client.Object{}
	for _, databaseNodeSet := range databaseNodeSets.Items {
		remoteDatabaseNodeSet := resources.NewRemoteDatabaseNodeSet(
			&v1alpha1.RemoteDatabaseNodeSet{Spec: databaseNodeSet.Spec},
		)
		remoteObjects = append(remoteObjects, remoteDatabaseNodeSet.GetRemoteObjects()...)
	}

	return remoteObjects, nil
}

func (r *Reconciler) patchObject(
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
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
	remoteObj client.Object,
) error {
	remoteObjGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
	if err != nil {
		return err
	}

	patch := []byte(fmt.Sprintf(`{"metadata": {"annotations": {"%s": null}}}`, ydbannotations.PrimaryResourceDatabaseAnnotation))
	if err := r.Client.Patch(ctx, remoteObj, client.RawPatch(types.MergePatchType, patch)); err != nil {
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to patch resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
		)
	}

	// Send event with information about unbinded resource
	r.Recorder.Event(
		remoteDatabaseNodeSet,
		corev1.EventTypeNormal,
		"Provisioning",
		fmt.Sprintf("RemoteSync UPDATE resource %s with name %s unbind from DatabaseNodeSet %s", remoteObjGVK.Kind, remoteObj.GetName(), remoteDatabaseNodeSet.Name),
	)

	// Remove status for remote resource from RemoteDatabaseNodeSet object
	remoteDatabaseNodeSet.RemoveRemoteResourceStatus(remoteObj, remoteObjGVK)

	return nil
}

func (r *Reconciler) deleteRemoteObject(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
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
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to delete resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
			)
			return err
		}
	}

	// Remove status for remote resource from RemoteDatabaseNodeSet object
	remoteDatabaseNodeSet.RemoveRemoteResourceStatus(remoteObj, remoteObjGVK)

	// Send event with information about deleted resource
	r.Recorder.Event(
		remoteDatabaseNodeSet,
		corev1.EventTypeNormal,
		"Provisioning",
		fmt.Sprintf("RemoteSync DELETE resource %s with name %s", remoteObjGVK.Kind, remoteObj.GetName()),
	)
	return nil
}
