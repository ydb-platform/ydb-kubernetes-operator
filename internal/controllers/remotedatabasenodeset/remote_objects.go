package remotedatabasenodeset

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
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step initRemoteObjectsStatus")

	for _, remoteObj := range remoteObjects {
		existInStatus := false
		for idx := range remoteDatabaseNodeSet.Status.RemoteResources {
			if resources.EqualRemoteResourceWithObject(
				&remoteDatabaseNodeSet.Status.RemoteResources[idx],
				remoteObj,
			) {
				existInStatus = true
				break
			}
		}
		if !existInStatus {
			remoteDatabaseNodeSet.CreateRemoteResourceStatus(remoteObj)
			return r.updateStatusRemoteObjects(ctx, remoteDatabaseNodeSet, StatusUpdateRequeueDelay)
		}
	}

	r.Log.Info("complete step initRemoteObjectsStatus")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) syncRemoteObjects(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step syncRemoteObjects")

	for _, remoteObj := range remoteObjects {
		remoteObjName := remoteObj.GetName()
		remoteObjKind := remoteObj.GetObjectKind().GroupVersionKind().Kind
		remoteObjRV := remoteObj.GetResourceVersion()
		var remoteResource *v1alpha1.RemoteResource
		for idx := range remoteDatabaseNodeSet.Status.RemoteResources {
			if resources.EqualRemoteResourceWithObject(&remoteDatabaseNodeSet.Status.RemoteResources[idx], remoteObj) {
				remoteResource = &remoteDatabaseNodeSet.Status.RemoteResources[idx]
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
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Resource %s with name %s was not found on remote cluster: %s", remoteObjKind, remoteObjName, remoteGetErr),
				)
				r.RemoteRecorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Resource %s with name %s was not found: %s", remoteObjKind, remoteObjName, remoteGetErr),
				)
			} else {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s on remote cluster: %s", remoteObjKind, remoteObjName, remoteGetErr),
				)
				r.RemoteRecorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteObjKind, remoteObjName, remoteGetErr),
				)
			}
			remoteDatabaseNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionFalse, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteDatabaseNodeSet, DefaultRequeueDelay)
		}

		// Check object existence in local cluster
		localObj := resources.CreateResource(remoteObj)
		getErr := r.Client.Get(ctx, types.NamespacedName{
			Name:      localObj.GetName(),
			Namespace: localObj.GetNamespace(),
		}, localObj)

		// Handler for kubernetes internal error
		if getErr != nil && !apierrors.IsNotFound(getErr) {
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteObjKind, remoteObjName, getErr),
			)
			remoteDatabaseNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionFalse, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteDatabaseNodeSet, DefaultRequeueDelay)
		}

		// Try to create non-existing remote object in local cluster
		if apierrors.IsNotFound(getErr) {
			createErr := r.Client.Create(ctx, localObj)
			if createErr != nil {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to create resource %s with name %s: %s", remoteObjKind, remoteObjName, getErr),
				)
				remoteDatabaseNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionFalse, remoteObjRV)
				return r.updateStatusRemoteObjects(ctx, remoteDatabaseNodeSet, DefaultRequeueDelay)
			}
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				fmt.Sprintf("RemoteSync CREATE resource %s with name %s", remoteObjKind, remoteObjName),
			)
			remoteDatabaseNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionFalse, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteDatabaseNodeSet, StatusUpdateRequeueDelay)
		}

		// Get patch diff between remote object and existing object
		remoteDatabaseNodeSet.SetPrimaryResourceAnnotations(remoteObj)
		patchResult, patchErr := resources.GetPatchResult(localObj, remoteObj)
		if patchErr != nil {
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to get diff for remote resource %s with name %s: %s", remoteObjKind, remoteObjName, patchErr),
			)
			remoteDatabaseNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionFalse, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteDatabaseNodeSet, DefaultRequeueDelay)
		}

		// Try to update existing object in local cluster by rawPatch
		if !patchResult.IsEmpty() {
			updateErr := r.Client.Patch(ctx, localObj, client.RawPatch(types.StrategicMergePatchType, patchResult.Patch))
			if updateErr != nil {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to update resource %s with name %s: %v", remoteObjKind, remoteObjName, updateErr),
				)
				remoteDatabaseNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionFalse, remoteObjRV)
				return r.updateStatusRemoteObjects(ctx, remoteDatabaseNodeSet, DefaultRequeueDelay)
			}
			r.Recorder.Event(
				remoteDatabaseNodeSet,
				corev1.EventTypeNormal,
				"Provisioning",
				fmt.Sprintf("RemoteSync UPDATE resource %s with name %s resourceVersion %s", remoteObjKind, remoteObjName, remoteObj.GetResourceVersion()),
			)
			remoteDatabaseNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionTrue, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteDatabaseNodeSet, StatusUpdateRequeueDelay)
		}

		if !meta.IsStatusConditionTrue(remoteResource.Conditions, RemoteResourceSyncedCondition) {
			remoteDatabaseNodeSet.UpdateRemoteResourceStatus(remoteResource, metav1.ConditionTrue, remoteObjRV)
			return r.updateStatusRemoteObjects(ctx, remoteDatabaseNodeSet, StatusUpdateRequeueDelay)
		}
	}

	r.Log.Info("complete step syncRemoteObjects")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) removeUnusedRemoteObjects(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
	remoteObjects []client.Object,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step removeUnusedRemoteResources")

	// We should check every remote resource to need existence in cluster
	// Get processed remote resources from object Status
	candidatesToDelete := []v1alpha1.RemoteResource{}
	for idx := range remoteDatabaseNodeSet.Status.RemoteResources {
		// Remove remote resource from candidates to delete if it declared
		// to using in current RemoteDatabaseNodeSet spec
		existInSpec := false
		for _, remoteObj := range remoteObjects {
			if resources.EqualRemoteResourceWithObject(
				&remoteDatabaseNodeSet.Status.RemoteResources[idx],
				remoteObj,
			) {
				existInSpec = true
				break
			}
		}
		if !existInSpec {
			candidatesToDelete = append(candidatesToDelete, remoteDatabaseNodeSet.Status.RemoteResources[idx])
		}
	}

	if len(candidatesToDelete) == 0 {
		r.Log.Info("complete step removeUnusedRemoteObjects")
		return Continue, ctrl.Result{}, nil
	}

	existInDatabase := false
	anotherDatabaseNodeSets, err := r.getAnotherDatabaseNodeSets(ctx, remoteDatabaseNodeSet)
	if err != nil {
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to check remote resource usage in another: %v", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	if len(anotherDatabaseNodeSets) > 0 {
		existInDatabase = true
	}

	// Check RemoteResource usage in DatabaseNodeSet
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

		localObj := resources.CreateResource(remoteObj)
		if err := r.Client.Get(ctx, types.NamespacedName{
			Name:      localObj.GetName(),
			Namespace: localObj.GetNamespace(),
		}, localObj); err != nil {
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

		// Remove annotation if no one another DatabaseNodeSet
		if !existInDatabase {
			// Try to update existing object in local cluster by rawPatch
			patch := []byte(fmt.Sprintf(`{"metadata": {"annotations": {"%s": null}}}`, ydbannotations.PrimaryResourceStorageAnnotation))
			updateErr := r.Client.Patch(ctx, localObj, client.RawPatch(types.StrategicMergePatchType, patch))
			if updateErr != nil {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to update resource %s with name %s: %s", remoteResource.Kind, remoteResource.Name, err),
				)
			} else {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeNormal,
					"Provisioning",
					fmt.Sprintf("RemoteSync UPDATE resource %s with name %s unset primaryResource annotation", remoteResource.Kind, remoteResource.Name),
				)
			}
		}

		// Delete resource if annotation `ydb.tech/primary-resource-storage` does not exist
		_, existInStorage := localObj.GetAnnotations()[ydbannotations.PrimaryResourceStorageAnnotation]
		if !existInStorage {
			// Try to delete unused resource from local cluster
			deleteErr := r.Client.Delete(ctx, localObj)
			if deleteErr != nil {
				if !apierrors.IsNotFound(deleteErr) {
					r.Recorder.Event(
						remoteDatabaseNodeSet,
						corev1.EventTypeWarning,
						"ControllerError",
						fmt.Sprintf("Failed to delete resource %s with name %s: %s", remoteResource.Kind, remoteResource.Name, err),
					)
				}
			} else {
				r.Recorder.Event(
					remoteDatabaseNodeSet,
					corev1.EventTypeNormal,
					"Provisioning",
					fmt.Sprintf("RemoteSync DELETE resource %s with name %s", remoteResource.Kind, remoteResource.Name),
				)
			}
		}
		remoteDatabaseNodeSet.RemoveRemoteResourceStatus(remoteObj)
	}

	return r.updateStatusRemoteObjects(ctx, remoteDatabaseNodeSet, StatusUpdateRequeueDelay)
}

func (r *Reconciler) updateStatusRemoteObjects(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
	requeueAfter time.Duration,
) (bool, ctrl.Result, error) {
	crRemoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}
	getErr := r.RemoteClient.Get(ctx, types.NamespacedName{
		Name:      remoteDatabaseNodeSet.Name,
		Namespace: remoteDatabaseNodeSet.Namespace,
	}, crRemoteDatabaseNodeSet)
	if getErr != nil {
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed fetching CR before status update for remote resources on remote cluster: %v", getErr),
		)
		r.RemoteRecorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed fetching CR before status update for remote resources: %v", getErr),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, getErr
	}

	crRemoteDatabaseNodeSet.Status.RemoteResources = append([]v1alpha1.RemoteResource{}, remoteDatabaseNodeSet.Status.RemoteResources...)
	updateErr := r.RemoteClient.Status().Update(ctx, crRemoteDatabaseNodeSet)
	if updateErr != nil {
		r.Recorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to update status for remote resources on remote cluster: %s", updateErr),
		)
		r.RemoteRecorder.Event(
			remoteDatabaseNodeSet,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to update status for remote resources: %s", updateErr),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, updateErr
	}

	r.Recorder.Event(
		remoteDatabaseNodeSet,
		corev1.EventTypeNormal,
		"StatusChanged",
		"Status for remote resources updated on remote cluster",
	)
	r.RemoteRecorder.Event(
		remoteDatabaseNodeSet,
		corev1.EventTypeNormal,
		"StatusChanged",
		"Status for remote resources updated",
	)

	return Stop, ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *Reconciler) getAnotherDatabaseNodeSets(
	ctx context.Context,
	remoteDatabaseNodeSet *resources.RemoteDatabaseNodeSetResource,
) ([]v1alpha1.DatabaseNodeSet, error) {
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
	databaseNodeSets := v1alpha1.DatabaseNodeSetList{}
	if err := r.Client.List(
		ctx,
		&databaseNodeSets,
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

	return databaseNodeSets.Items, nil
}
