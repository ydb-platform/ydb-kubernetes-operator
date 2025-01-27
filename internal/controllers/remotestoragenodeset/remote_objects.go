package remotestoragenodeset

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	ydblabels "github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func (r *Reconciler) initRemoteObjectsStatus(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step initRemoteObjectsStatus")

	for _, remoteObj := range remoteObjects {
		existInStatus := false
		for idx := range remoteStorageNodeSet.Status.RemoteResources {
			if resources.EqualRemoteResourceWithObject(
				&remoteStorageNodeSet.Status.RemoteResources[idx],
				remoteObj,
			) {
				existInStatus = true
				break
			}
		}
		if !existInStatus {
			remoteStorageNodeSet.CreateRemoteResourceStatus(remoteObj)
			return r.updateStatusRemoteObjects(ctx, remoteStorageNodeSet, StatusUpdateRequeueDelay)
		}
	}

	r.Log.Info("complete step initRemoteObjectsStatus")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) syncRemoteObjects(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step syncRemoteObjects")

	for _, remoteObj := range remoteObjects {
		remoteObjName := remoteObj.GetName()
		remoteObjKind := remoteObj.GetObjectKind().GroupVersionKind().Kind
		var remoteResource *v1alpha1.RemoteResource
		for idx := range remoteStorageNodeSet.Status.RemoteResources {
			if resources.EqualRemoteResourceWithObject(&remoteStorageNodeSet.Status.RemoteResources[idx], remoteObj) {
				remoteResource = &remoteStorageNodeSet.Status.RemoteResources[idx]
				break
			}
		}

		// Get object to sync from remote cluster
		remoteGetErr := r.RemoteClient.Get(ctx, types.NamespacedName{
			Name:      remoteObj.GetName(),
			Namespace: remoteObj.GetNamespace(),
		}, remoteObj)
		// Resource not found on remote cluster or internal kubernetes error
		if remoteGetErr != nil {
			if apierrors.IsNotFound(remoteGetErr) {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Resource %s with name %s was not found on remote cluster: %s", remoteObjKind, remoteObjName, remoteGetErr),
				)
				r.RemoteRecorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Resource %s with name %s was not found: %s", remoteObjKind, remoteObjName, remoteGetErr),
				)
			} else {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s on remote cluster: %s", remoteObjKind, remoteObjName, remoteGetErr),
				)
				r.RemoteRecorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteObjKind, remoteObjName, remoteGetErr),
				)
			}
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, remoteGetErr
		}

		// Check object existence in local cluster
		remoteObjRV := remoteObj.GetResourceVersion()
		localObj := resources.CreateResource(remoteObj)
		getErr := r.Client.Get(ctx, types.NamespacedName{
			Name:      localObj.GetName(),
			Namespace: localObj.GetNamespace(),
		}, localObj)

		// Handler for kubernetes internal error
		if getErr != nil && !apierrors.IsNotFound(getErr) {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteObjKind, remoteObjName, getErr),
			)
			remoteStorageNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionFalse, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteStorageNodeSet, DefaultRequeueDelay)
		}

		// Try to create non-existing remote object in local cluster
		if apierrors.IsNotFound(getErr) {
			createErr := r.Client.Create(ctx, localObj)
			if createErr != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to create resource %s with name %s: %s", remoteObjKind, remoteObjName, getErr),
				)
				remoteStorageNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionFalse, remoteObjRV)
				return r.updateStatusRemoteObjects(ctx, remoteStorageNodeSet, DefaultRequeueDelay)
			}
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				fmt.Sprintf("RemoteSync CREATE resource %s with name %s", remoteObjKind, remoteObjName),
			)
			remoteStorageNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionTrue, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteStorageNodeSet, StatusUpdateRequeueDelay)
		}

		// Get patch diff between remote object and existing object
		remoteStorageNodeSet.SetPrimaryResourceAnnotations(remoteObj)
		patchResult, patchErr := resources.GetPatchResult(localObj, remoteObj)
		if patchErr != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to get diff for remote resource %s with name %s: %s", remoteObjKind, remoteObjName, patchErr),
			)
			remoteStorageNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionFalse, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteStorageNodeSet, DefaultRequeueDelay)
		}

		// Try to update existing object in local cluster by rawPatch
		if !patchResult.IsEmpty() {
			updateErr := r.Client.Patch(ctx, localObj, client.RawPatch(types.StrategicMergePatchType, patchResult.Patch))
			if updateErr != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to update resource %s with name %s: %v", remoteObjKind, remoteObjName, updateErr),
				)
				remoteStorageNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionFalse, remoteObjRV)
				return r.updateStatusRemoteObjects(ctx, remoteStorageNodeSet, DefaultRequeueDelay)
			}
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				fmt.Sprintf("RemoteSync UPDATE resource %s with name %s resourceVersion %s", remoteObjKind, remoteObjName, remoteObjRV),
			)
			remoteStorageNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionTrue, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteStorageNodeSet, StatusUpdateRequeueDelay)
		}

		if !meta.IsStatusConditionTrue(remoteResource.Conditions, RemoteResourceSyncedCondition) {
			remoteStorageNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionTrue, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteStorageNodeSet, StatusUpdateRequeueDelay)
		}
	}

	r.Log.Info("complete step syncRemoteObjects")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) removeUnusedRemoteObjects(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step removeUnusedRemoteResources")

	// We should check every remote resource to need existence in cluster
	// Get processed remote resources from object Status
	candidatesToDelete := []v1alpha1.RemoteResource{}
	for idx := range remoteStorageNodeSet.Status.RemoteResources {
		// Remove remote resource from candidates to delete if it declared
		// to using in current RemoteStorageNodeSet spec
		existInSpec := false
		for _, remoteObj := range remoteObjects {
			if resources.EqualRemoteResourceWithObject(
				&remoteStorageNodeSet.Status.RemoteResources[idx],
				remoteObj,
			) {
				existInSpec = true
				break
			}
		}
		if !existInSpec {
			candidatesToDelete = append(candidatesToDelete, remoteStorageNodeSet.Status.RemoteResources[idx])
		}
	}

	if len(candidatesToDelete) == 0 {
		r.Log.Info("complete step removeUnusedRemoteObjects")
		return Continue, ctrl.Result{}, nil
	}

	existInStorage := false
	anotherStorageNodeSets, err := r.getAnotherStorageNodeSets(ctx, remoteStorageNodeSet)
	if err != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to check remote resource usage in another: %v", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	if len(anotherStorageNodeSets) > 0 {
		existInStorage = true
	}

	// Check RemoteResource usage in StorageNodeSet
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

		localObj := resources.CreateResource(remoteObj)
		if err := r.Client.Get(ctx, types.NamespacedName{
			Name:      localObj.GetName(),
			Namespace: localObj.GetNamespace(),
		}, localObj); err != nil {
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

		// Remove annotation if no one another StorageNodeSet
		if !existInStorage {
			patch := []byte(fmt.Sprintf(`{"metadata": {"annotations": {"%s": null}}}`, ydbannotations.PrimaryResourceStorageAnnotation))
			updateErr := r.Client.Patch(ctx, localObj, client.RawPatch(types.StrategicMergePatchType, patch))
			if updateErr != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to update resource %s with name %s: %s", remoteResource.Kind, remoteResource.Name, err),
				)
			} else {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeNormal,
					"Provisioning",
					fmt.Sprintf("RemoteSync UPDATE resource %s with name %s unset primaryResource annotation", remoteResource.Kind, remoteResource.Name),
				)
			}
		}

		// Delete resource if annotation `ydb.tech/primary-resource-database` does not exist
		_, existInDatabase := localObj.GetAnnotations()[ydbannotations.PrimaryResourceDatabaseAnnotation]
		if !existInDatabase {
			// Try to delete unused resource from local cluster
			deleteErr := r.Client.Delete(ctx, localObj)
			if deleteErr != nil {
				if !apierrors.IsNotFound(deleteErr) {
					r.Recorder.Event(
						remoteStorageNodeSet,
						corev1.EventTypeWarning,
						"ControllerError",
						fmt.Sprintf("Failed to delete resource %s with name %s: %s", remoteResource.Kind, remoteResource.Name, err),
					)
				}
			} else {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeNormal,
					"Provisioning",
					fmt.Sprintf("RemoteSync DELETE resource %s with name %s", remoteResource.Kind, remoteResource.Name),
				)
			}
		}
		remoteStorageNodeSet.RemoveRemoteResourceStatus(remoteObj)
	}

	return r.updateStatusRemoteObjects(ctx, remoteStorageNodeSet, StatusUpdateRequeueDelay)
}

func (r *Reconciler) updateStatusRemoteObjects(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	requeueAfter time.Duration,
) (bool, ctrl.Result, error) {
	crRemoteStorageNodeSet := &v1alpha1.RemoteStorageNodeSet{}
	getErr := r.RemoteClient.Get(ctx, types.NamespacedName{
		Name:      remoteStorageNodeSet.Name,
		Namespace: remoteStorageNodeSet.Namespace,
	}, crRemoteStorageNodeSet)
	if getErr != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed fetching CR before status update for remote resources on remote cluster: %v", getErr),
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed fetching CR before status update for remote resources: %v", getErr),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, getErr
	}

	crRemoteStorageNodeSet.Status.RemoteResources = append([]v1alpha1.RemoteResource{}, remoteStorageNodeSet.Status.RemoteResources...)
	updateErr := r.RemoteClient.Status().Update(ctx, crRemoteStorageNodeSet)
	if updateErr != nil {
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to update status for remote resources on remote cluster: %s", updateErr),
		)
		r.RemoteRecorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to update status for remote resources: %s", updateErr),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, updateErr
	}

	r.Recorder.Event(
		remoteStorageNodeSet,
		corev1.EventTypeNormal,
		"StatusChanged",
		"Status for remote resources updated on remote cluster",
	)
	r.RemoteRecorder.Event(
		remoteStorageNodeSet,
		corev1.EventTypeNormal,
		"StatusChanged",
		"Status for remote resources updated",
	)

	return Stop, ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *Reconciler) getAnotherStorageNodeSets(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
) ([]v1alpha1.StorageNodeSet, error) {
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
	storageNodeSets := v1alpha1.StorageNodeSetList{}
	if err := r.Client.List(
		ctx,
		&storageNodeSets,
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

	return storageNodeSets.Items, nil
}
