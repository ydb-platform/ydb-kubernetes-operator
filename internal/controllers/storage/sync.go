package storage

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Monitoring"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/healthcheck"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func (r *Reconciler) Sync(ctx context.Context, cr *v1alpha1.Storage) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	storage := resources.NewCluster(cr)

	stop, result, err = r.setInitialStatus(ctx, &storage)
	if stop {
		return result, err
	}

	stop, result, err = r.handleResourcesSync(ctx, &storage)
	if stop {
		return result, err
	}

	stop, result, err = r.syncNodeSetSpecInline(ctx, &storage)
	if stop {
		return result, err
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, StorageInitializedCondition) {
		return r.handleBlobstorageInit(ctx, &storage)
	}

	stop, result, err = r.setConfigPipelineStatus(ctx, &storage)
	if stop {
		return result, err
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, ConfigurationSyncedCondition) {
		return r.handleConfigurationSync(ctx, &storage)
	}

	if storage.Spec.NodeSets != nil {
		stop, result, err = r.waitForNodeSetsToProvisioned(ctx, &storage)
		if stop {
			return result, err
		}
	} else {
		stop, result, err = r.waitForStatefulSetToScale(ctx, &storage)
		if stop {
			return result, err
		}
	}

	stop, result, err = r.handlePauseResume(ctx, &storage)
	if stop {
		return result, err
	}

	stop, result, err = r.runSelfCheck(ctx, &storage, false)
	if stop {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) setInitialStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")
	if storage.Status.Conditions == nil {
		storage.Status.Conditions = []metav1.Condition{}

		if storage.Spec.Pause {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:    StoragePausedCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  ReasonInProgress,
				Message: "Transitioning to state Paused",
			})
		} else {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:    StorageReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  ReasonInProgress,
				Message: "Transitioning to state Ready",
			})
		}

		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step setInitialStatus")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) waitForStatefulSetToScale(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale")

	if storage.Status.State == StorageInitializing {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageProvisionedCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Waiting for scale to desired nodes: %d", storage.Spec.Nodes),
		})
		storage.Status.State = StorageProvisioning
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      storage.Name,
		Namespace: storage.Namespace,
	}, found)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("StatefulSet with name %s was not found: %s", storage.Name, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ProvisioningFailed",
			fmt.Sprintf("Failed to get StatefulSets: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	podLabels := labels.Common(storage.Name, make(map[string]string))
	podLabels.Merge(map[string]string{
		labels.ComponentKey: labels.StorageComponent,
	})

	matchingLabels := client.MatchingLabels{}
	for k, v := range podLabels {
		matchingLabels[k] = v
	}

	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(storage.Namespace),
		matchingLabels,
	}

	err = r.List(ctx, podList, opts...)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ProvisioningFailed",
			fmt.Sprintf("Failed to list cluster pods: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	runningPods := 0
	for _, e := range podList.Items {
		if e.Status.Phase == "Running" {
			runningPods++
		}
	}

	if runningPods != int(storage.Spec.Nodes) {
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			string(StorageProvisioning),
			fmt.Sprintf("Waiting for number of running storage pods to match expected: %d != %d", runningPods, storage.Spec.Nodes),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageProvisionedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Number of running nodes does not match expected: %d != %d", runningPods, storage.Spec.Nodes),
		})
		return r.updateStatus(ctx, storage, DefaultRequeueDelay)
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, StorageProvisionedCondition) {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageProvisionedCondition,
			Status:  metav1.ConditionTrue,
			Reason:  ReasonCompleted,
			Message: "Successfully scaled to desired number of nodes",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step waitForStatefulSetToScale")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) waitForNodeSetsToProvisioned(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForNodeSetsToProvisioned")

	if storage.Status.State == StorageInitializing {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageProvisionedCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonInProgress,
			Message: "Waiting for NodeSets conditions to be Provisioned",
		})
		storage.Status.State = StorageProvisioning
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	for _, nodeSetSpec := range storage.Spec.NodeSets {
		var nodeSetObject client.Object
		var nodeSetKind string
		var nodeSetConditions []metav1.Condition
		if nodeSetSpec.Remote != nil {
			nodeSetObject = &v1alpha1.RemoteStorageNodeSet{}
			nodeSetKind = RemoteStorageNodeSetKind
		} else {
			nodeSetObject = &v1alpha1.StorageNodeSet{}
			nodeSetKind = StorageNodeSetKind
		}

		nodeSetName := storage.Name + "-" + nodeSetSpec.Name
		if err := r.Get(ctx, types.NamespacedName{
			Name:      nodeSetName,
			Namespace: storage.Namespace,
		}, nodeSetObject); err != nil {
			if apierrors.IsNotFound(err) {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("%s with name %s was not found: %s", nodeSetKind, nodeSetName, err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("Failed to get %s with name %s: %s", nodeSetKind, nodeSetName, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		if nodeSetSpec.Remote != nil {
			nodeSetConditions = nodeSetObject.(*v1alpha1.RemoteStorageNodeSet).Status.Conditions
		} else {
			nodeSetConditions = nodeSetObject.(*v1alpha1.StorageNodeSet).Status.Conditions
		}

		// TODO: also check observedGeneration to guarantee that compare with updated object
		if !meta.IsStatusConditionTrue(nodeSetConditions, NodeSetProvisionedCondition) {
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				string(StorageProvisioning),
				fmt.Sprintf(
					"Waiting %s with name %s for condition NodeSetProvisioned to be True",
					nodeSetKind,
					nodeSetName,
				),
			)
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:   StorageProvisionedCondition,
				Status: metav1.ConditionFalse,
				Reason: ReasonInProgress,
				Message: fmt.Sprintf(
					"Waiting %s with name %s for condition NodeSetProvisioned to be True",
					nodeSetKind,
					nodeSetName,
				),
			})
			return r.updateStatus(ctx, storage, DefaultRequeueDelay)
		}
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, StorageProvisionedCondition) {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageProvisionedCondition,
			Status:  metav1.ConditionTrue,
			Reason:  ReasonCompleted,
			Message: "Successfully scaled to desired number of nodes",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step waitForNodeSetsToProvisioned")
	return Continue, ctrl.Result{}, nil
}

func shouldIgnoreStorageChange(storage *resources.StorageClusterBuilder) resources.IgnoreChangesFunction {
	return func(oldObj, newObj runtime.Object) bool {
		if _, ok := newObj.(*appsv1.StatefulSet); ok {
			if storage.Spec.Pause && *oldObj.(*appsv1.StatefulSet).Spec.Replicas == 0 {
				return true
			}
		}
		return false
	}
}

func (r *Reconciler) handleResourcesSync(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	if storage.Status.State == StoragePending {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StoragePreparedCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonInProgress,
			Message: fmt.Sprintf("Waiting for sync resources for generation %d", storage.Generation),
		})
		storage.Status.State = StoragePreparing
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	if !storage.Spec.OperatorSync {
		r.Log.Info("`operatorSync: false` is set, no further steps will be run")
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			string(StoragePreparing),
			fmt.Sprintf("Found .spec.operatorSync set to %t, skip further steps", storage.Spec.OperatorSync),
		)
		return Stop, ctrl.Result{}, nil
	}

	for _, builder := range storage.GetResourceBuilders(r.Config) {
		newResource := builder.Placeholder(storage)

		result, err := resources.CreateOrUpdateOrMaybeIgnore(ctx, r.Client, newResource, func() error {
			var err error

			err = builder.Build(newResource)
			if err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}

			err = ctrl.SetControllerReference(storage.Unwrap(), newResource, r.Scheme)
			if err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Error setting controller reference for resource: %s", err),
				)
				return err
			}

			return nil
		}, shouldIgnoreStorageChange(storage))

		eventMessage := fmt.Sprintf(
			"Resource: %s, Namespace: %s, Name: %s",
			reflect.TypeOf(newResource),
			newResource.GetNamespace(),
			newResource.GetName(),
		)
		if err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				eventMessage+fmt.Sprintf(", failed to sync, error: %s", err),
			)
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:    StoragePreparedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  ReasonInProgress,
				Message: fmt.Sprintf("Failed to sync resources for generation %d", storage.Generation),
			})
			return r.updateStatus(ctx, storage, DefaultRequeueDelay)
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				string(StorageProvisioning),
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, StoragePreparedCondition) {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StoragePreparedCondition,
			Status:  metav1.ConditionTrue,
			Reason:  ReasonCompleted,
			Message: "Successfully synced resources",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step handleResourcesSync")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) syncNodeSetSpecInline(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step syncNodeSetSpecInline")
	matchingFields := client.MatchingFields{
		OwnerControllerField: storage.Name,
	}

	storageNodeSets := &v1alpha1.StorageNodeSetList{}
	if err := r.List(ctx, storageNodeSets,
		client.InNamespace(storage.Namespace),
		matchingFields,
	); err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ProvisioningFailed",
			fmt.Sprintf("Failed to list StorageNodeSets: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	for _, storageNodeSet := range storageNodeSets.Items {
		storageNodeSet := storageNodeSet.DeepCopy()
		isFoundStorageNodeSetSpecInline := false
		for _, nodeSetSpecInline := range storage.Spec.NodeSets {
			if nodeSetSpecInline.Remote == nil {
				nodeSetName := storage.Name + "-" + nodeSetSpecInline.Name
				if storageNodeSet.Name == nodeSetName {
					isFoundStorageNodeSetSpecInline = true
					break
				}
			}
		}

		if !isFoundStorageNodeSetSpecInline {
			if err := r.Delete(ctx, storageNodeSet); err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed to delete StorageNodeSet: %s", err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				"Syncing",
				fmt.Sprintf("Resource: %s, Namespace: %s, Name: %s, deleted",
					reflect.TypeOf(storageNodeSet),
					storageNodeSet.Namespace,
					storageNodeSet.Name),
			)
		}
	}

	remoteStorageNodeSets := &v1alpha1.RemoteStorageNodeSetList{}
	if err := r.List(ctx, remoteStorageNodeSets,
		client.InNamespace(storage.Namespace),
		matchingFields,
	); err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ProvisioningFailed",
			fmt.Sprintf("Failed to list RemoteStorageNodeSets: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	for _, remoteStorageNodeSet := range remoteStorageNodeSets.Items {
		remoteStorageNodeSet := remoteStorageNodeSet.DeepCopy()
		isFoundRemoteStorageNodeSetSpecInline := false
		for _, nodeSetSpecInline := range storage.Spec.NodeSets {
			if nodeSetSpecInline.Remote != nil {
				nodeSetName := storage.Name + "-" + nodeSetSpecInline.Name
				if remoteStorageNodeSet.Name == nodeSetName {
					isFoundRemoteStorageNodeSetSpecInline = true
					break
				}
			}
		}

		if !isFoundRemoteStorageNodeSetSpecInline {
			if err := r.Delete(ctx, remoteStorageNodeSet); err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed to delete RemoteStorageNodeSet: %s", err),
				)
				return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				"Syncing",
				fmt.Sprintf("Resource: %s, Namespace: %s, Name: %s, deleted",
					reflect.TypeOf(remoteStorageNodeSet),
					remoteStorageNodeSet.Namespace,
					remoteStorageNodeSet.Name),
			)
		}
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, StoragePreparedCondition) {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StoragePreparedCondition,
			Status:  metav1.ConditionTrue,
			Reason:  ReasonCompleted,
			Message: "Successfully synced resources",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step syncNodeSetSpecInline")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) runSelfCheck(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	waitForGoodResultWithoutIssues bool,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step runSelfCheck")

	creds, err := resources.GetYDBCredentials(ctx, storage.Unwrap(), r.Config)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get YDB credentials: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	tlsOptions, err := resources.GetYDBTLSOption(ctx, storage.Unwrap(), r.Config)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get YDB TLS options: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	result, err := healthcheck.GetSelfCheckResult(ctx, storage, creds, tlsOptions)
	if err != nil {
		r.Log.Error(err, "GetSelfCheckResult error")
		return Stop, ctrl.Result{RequeueAfter: SelfCheckRequeueDelay}, err
	}

	eventType := corev1.EventTypeNormal
	if result.SelfCheckResult != Ydb_Monitoring.SelfCheck_GOOD {
		eventType = corev1.EventTypeWarning
	}

	r.Recorder.Event(
		storage,
		eventType,
		"SelfCheck",
		fmt.Sprintf(
			"SelfCheck result: %s, issues found: %d",
			result.SelfCheckResult.String(),
			len(result.IssueLog),
		),
	)

	if waitForGoodResultWithoutIssues && result.SelfCheckResult.String() != "GOOD" {
		return Stop, ctrl.Result{RequeueAfter: SelfCheckRequeueDelay}, err
	}

	r.Log.Info("complete step runSelfCheck")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	requeueAfter time.Duration,
) (bool, ctrl.Result, error) {
	r.Log.Info("running updateStatus handler")

	if meta.IsStatusConditionFalse(storage.Status.Conditions, StoragePreparedCondition) ||
		meta.IsStatusConditionFalse(storage.Status.Conditions, StorageInitializedCondition) ||
		meta.IsStatusConditionFalse(storage.Status.Conditions, StorageProvisionedCondition) {
		if storage.Spec.Pause {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:   StoragePausedCondition,
				Status: metav1.ConditionFalse,
				Reason: ReasonInProgress,
			})
		} else {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:   StorageReadyCondition,
				Status: metav1.ConditionFalse,
				Reason: ReasonInProgress,
			})
		}
	}

	storageCr := &v1alpha1.Storage{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: storage.Namespace,
		Name:      storage.Name,
	}, storageCr)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			"Failed fetching CR before status update",
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	oldStatus := storageCr.Status.State
	storageCr.Status.State = storage.Status.State
	storageCr.Status.Conditions = storage.Status.Conditions
	if err = r.Status().Update(ctx, storageCr); err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed setting status: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	if oldStatus != storage.Status.State {
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("Storage moved from %s to %s", oldStatus, storageCr.Status.State),
		)
	}

	r.Log.Info("complete updateStatus handler")
	return Stop, ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *Reconciler) handlePauseResume(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handlePauseResume")

	if storage.Status.State == StorageProvisioning {
		if storage.Spec.Pause {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:   StoragePausedCondition,
				Status: metav1.ConditionTrue,
				Reason: ReasonCompleted,
			})
			storage.Status.State = StoragePaused
		} else {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:   StorageReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: ReasonCompleted,
			})
			storage.Status.State = StorageReady
		}
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	if storage.Status.State == StorageReady && storage.Spec.Pause {
		r.Log.Info("`pause: true` was noticed, moving Storage to state `Paused`")
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNotRequired,
			Message: "Transitioning to state Paused",
		})
		storage.Status.State = StoragePaused
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	if storage.Status.State == StoragePaused && !storage.Spec.Pause {
		r.Log.Info("`pause: false` was noticed, moving Storage to state `Ready`")
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StoragePausedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNotRequired,
			Message: "Transitioning to state Ready",
		})
		storage.Status.State = StorageReady
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	if storage.Spec.Pause {
		if !meta.IsStatusConditionTrue(storage.Status.Conditions, StoragePausedCondition) {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:   StoragePausedCondition,
				Status: metav1.ConditionTrue,
				Reason: ReasonCompleted,
			})
			return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
		}
	} else {
		if !meta.IsStatusConditionTrue(storage.Status.Conditions, StorageReadyCondition) {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:   StorageReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: ReasonCompleted,
			})
			return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
		}
	}

	r.Log.Info("complete step handlePauseResume")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) handleBlobstorageInit(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (ctrl.Result, error) {
	r.Log.Info("running step handleBlobstorageInit")

	stop, result, err := r.setInitPipelineStatus(ctx, storage)
	if stop {
		return result, err
	}

	stop, result, err = r.initializeBlobstorage(ctx, storage)
	if stop {
		return result, err
	}

	r.Log.Info("complete step handleBlobstorageInit")
	return ctrl.Result{}, nil
}

func (r *Reconciler) handleConfigurationSync(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (ctrl.Result, error) {
	r.Log.Info("running step handleConfigurationSync")

	stop, result, err := r.checkConfig(ctx, storage)
	if stop {
		return result, err
	}

	stop, result, err = r.replaceConfig(ctx, storage)
	if stop {
		return result, err
	}

	r.Log.Info("complete step handleConfigurationSync")
	return ctrl.Result{}, nil
}
