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
		err = r.Client.Get(ctx, types.NamespacedName{
			Name:      remoteObj.GetName(),
			Namespace: remoteObj.GetNamespace(),
		}, localObj)
		//nolint:nestif
		if err != nil {
			if !apierrors.IsNotFound(err) {
				r.Recorder.Event(
					remoteStorageNodeSet,
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
		} else {
			// Update client.Object for local object with spec from remote object
			updatedObj := resources.UpdateResource(localObj, remoteObj)
			remoteStorageNodeSet.SetPrimaryResourceAnnotations(updatedObj)
			// Remote object existing in local cluster, сheck the need for an update
			// Get diff resources and compare bytes by k8s-objectmatcher PatchMaker
			patchResult, err := patchMaker.Calculate(localObj, updatedObj,
				[]patch.CalculateOption{
					patch.IgnoreStatusFields(),
				}...,
			)
			if err != nil {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get diff for remote resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
			}
			// We need to check patchResult by k8s-objectmatcherand and resourceVersion from annotation
			// And update if localObj does not match updatedObj from remote cluster
			if !patchResult.IsEmpty() ||
				remoteObj.GetResourceVersion() != localObj.GetAnnotations()[ydbannotations.RemoteResourceVersionAnnotation] {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeNormal,
					"Provisioning",
					fmt.Sprintf("Patch for resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), string(patchResult.Patch)),
				)
				// Try to update resource in local cluster
				if err := r.Client.Update(ctx, updatedObj); err != nil {
					r.Recorder.Event(
						remoteStorageNodeSet,
						corev1.EventTypeWarning,
						"ControllerError",
						fmt.Sprintf("Failed to update resource %s with name %s: %s", remoteObjGVK.Kind, remoteObj.GetName(), err),
					)
					return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
				}
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeNormal,
					"Provisioning",
					fmt.Sprintf("RemoteSync UPDATE resource %s with name %s resourceVersion %s", remoteObjGVK.Kind, remoteObj.GetName(), remoteObj.GetResourceVersion()),
				)
			}
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

	// Remove remote resource from candidates to delete if it declared
	// to using in current RemoteStorageNodeSet spec
	for idx := range remoteStorageNodeSet.Status.RemoteResources {
		remoteResource := remoteStorageNodeSet.Status.RemoteResources[idx]
		existInSpec := false
		for i := range remoteObjects {
			declaredObj := remoteObjects[i]
			declaredObjGVK, err := apiutil.GVKForObject(declaredObj, r.Scheme)
			if err != nil {
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			if resources.EqualRemoteResourceWithObject(
				&remoteResource,
				remoteStorageNodeSet.Namespace,
				declaredObj,
				declaredObjGVK,
			) {
				existInSpec = true
				break
			}
		}
		if !existInSpec {
			candidatesToDelete = append(candidatesToDelete, remoteResource)
		}
	}

	// Check resources usage in another StorageNodeSet and make List request
	// only if we have candidates to Delete
	resourcesToDelete := []v1alpha1.RemoteResource{}
	if len(candidatesToDelete) > 0 {
		remoteObjectsUsed, err := r.getRemoteObjectsUsedInNamespace(ctx, remoteStorageNodeSet, remoteObjects)
		if err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("Failed to get resources used in another object: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		for idx := range candidatesToDelete {
			remoteResource := candidatesToDelete[idx]
			isCandidateExistInANotherObject := false
			// Remove resource from cadidates to Delete if another object using it now
			for i := range remoteObjectsUsed {
				usedObj := remoteObjectsUsed[i]
				usedObjGVK, err := apiutil.GVKForObject(usedObj, r.Scheme)
				if err != nil {
					return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
				}
				if resources.EqualRemoteResourceWithObject(
					&remoteResource,
					remoteStorageNodeSet.Namespace,
					usedObj,
					usedObjGVK,
				) {
					isCandidateExistInANotherObject = true
					break
				}
			}
			if !isCandidateExistInANotherObject {
				resourcesToDelete = append(resourcesToDelete, remoteResource)
			}
		}
	}

	// Remove unused remote resource from cluster and make API call DELETE
	// for every candidate to Delete
	for _, recourceToDelete := range resourcesToDelete {
		// Convert RemoteResource struct from Status to client.Object
		remoteObj, err := resources.ConvertRemoteResourceToObject(
			recourceToDelete,
			remoteStorageNodeSet.Namespace,
		)
		if err != nil {
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		// Determine actual GVK for generic client.Object
		remoteResourceGVK, err := apiutil.GVKForObject(remoteObj, r.Scheme)
		if err != nil {
			r.Recorder.Event(
				remoteStorageNodeSet,
				corev1.EventTypeWarning,
				"ControllerError",
				fmt.Sprintf("Failed to recognize GVK for remote object %v: %s", remoteObj, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		// Try to get resource in local cluster
		if err := r.Client.Get(ctx, types.NamespacedName{
			Name:      remoteObj.GetName(),
			Namespace: remoteObj.GetNamespace(),
		}, remoteObj); err != nil {
			if !apierrors.IsNotFound(err) {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to get resource %s with name %s: %s", remoteResourceGVK.Kind, remoteObj.GetName(), err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
		}

		// Skip resource deletion because it using in some Database
		// check by existence of annotation `ydb.tech/primary-resource-database`
		if _, exist := remoteObj.GetAnnotations()[ydbannotations.PrimaryResourceDatabaseAnnotation]; exist {
			continue
		}

		// Try to delete unused resource from local cluster
		if err := r.Client.Delete(ctx, remoteObj); err != nil {
			if !apierrors.IsNotFound(err) {
				r.Recorder.Event(
					remoteStorageNodeSet,
					corev1.EventTypeWarning,
					"ControllerError",
					fmt.Sprintf("Failed to delete resource %s with name %s: %s", remoteResourceGVK.Kind, remoteObj.GetName(), err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
		}
		r.Recorder.Event(
			remoteStorageNodeSet,
			corev1.EventTypeNormal,
			"Provisioning",
			fmt.Sprintf("RemoteSync DELETE resource %s with name %s", remoteResourceGVK.Kind, remoteObj.GetName()),
		)
		// Remove status for remote resource from RemoteStorageNodeSet object
		remoteStorageNodeSet.RemoveRemoteResourceStatus(remoteObj, remoteResourceGVK)
	}

	return r.updateRemoteResourcesStatus(ctx, remoteStorageNodeSet)
}

func (r *Reconciler) getRemoteObjectsUsedInNamespace(
	ctx context.Context,
	remoteStorageNodeSet *resources.RemoteStorageNodeSetResource,
	remoteObjects []client.Object,
) ([]client.Object, error) {
	remoteObjectsUsedInNamespace := []client.Object{}

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
	storageNodeSets := &v1alpha1.StorageNodeSetList{}
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

	// We found some StorageNodeSet and should check objects usage
	if len(storageNodeSets.Items) > 0 {
		for _, remoteObj := range remoteObjects {
			switch obj := remoteObj.(type) {
			// If client.Object typed by Secret search existence
			// in another StorageNodeSet spec.secrets
			case *corev1.Secret:
				for _, storageNodeSet := range storageNodeSets.Items {
					for _, secret := range storageNodeSet.Spec.Secrets {
						if obj.GetName() == secret.Name {
							remoteObjectsUsedInNamespace = append(
								remoteObjectsUsedInNamespace,
								obj,
							)
						}
					}
				}
			// Else client.Object typed by ConfigMap or Service
			// which always used in another StorageNodeSet
			default:
				remoteObjectsUsedInNamespace = append(
					remoteObjectsUsedInNamespace,
					obj,
				)
			}
		}
	}

	return remoteObjectsUsedInNamespace, nil
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
			"Failed fetching RemoteStorageNodeSet before status update",
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
