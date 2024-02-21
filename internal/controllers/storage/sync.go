package storage

import (
	"context"
	"fmt"
	"reflect"

	"github.com/ydb-platform/ydb-go-genproto/protos/Ydb_Monitoring"
	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
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
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
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
	stop, result, err = storage.SetStatusOnFirstReconcile()
	if stop {
		return result, err
	}

	stop, result = r.checkStorageFrozen(&storage)
	if stop {
		return result, nil
	}

	stop, result, err = r.handlePauseResume(ctx, &storage)
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
		return r.handleFirstStart(ctx, &storage)
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) waitForStatefulSetToScale(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale for Storage")

	if storage.Status.State == StoragePreparing {
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			string(StorageProvisioning),
			fmt.Sprintf("Starting to track number of running storage pods, expected: %d", storage.Spec.Nodes),
		)
		storage.Status.State = StorageProvisioning
		return r.updateStatus(ctx, storage)
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
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
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
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) waitForStorageNodeSetsToReady(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStorageNodeSetToReady for Storage")

	if storage.Status.State == StoragePreparing {
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			string(StorageProvisioning),
			fmt.Sprintf("Starting to track readiness of running nodeSets objects, expected: %d", storage.Spec.Nodes),
		)
		storage.Status.State = StorageProvisioning
		return r.updateStatus(ctx, storage)
	}

	for _, nodeSetSpec := range storage.Spec.NodeSets {
		var nodeSetKind string

		nodeSetKind = StorageNodeSetKind
		if nodeSetSpec.Remote != nil {
			nodeSetKind = RemoteStorageNodeSetKind
		}

		nodeSetName := storage.Name + "-" + nodeSetSpec.Name
		nodeSetStatus, err := r.getNodeSetStatus(ctx, storage, nodeSetName, nodeSetKind)
		if err != nil {
			if apierrors.IsNotFound(err) {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("%s with name %s was not found: %s", nodeSetKind, nodeSetName, err),
				)
			}
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("Failed to get %s with name %s: %s", nodeSetKind, nodeSetName, err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		if nodeSetStatus != StorageNodeSetReady {
			eventMessage := fmt.Sprintf("Waiting %s with name %s for Ready state , current: %s",
				nodeSetKind,
				nodeSetName,
				nodeSetStatus,
			)
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				string(StorageProvisioning),
				eventMessage,
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
	}

	return Continue, ctrl.Result{Requeue: false}, nil
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
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		} else if result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated {
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				string(StorageProvisioning),
				eventMessage+fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}

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

	r.Log.Info("syncNodeSetSpecInline complete")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) runSelfCheck(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	creds ydbCredentials.Credentials,
	waitForGoodResultWithoutIssues bool,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step runSelfCheck")

	result, err := healthcheck.GetSelfCheckResult(ctx, storage, creds)
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

	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) updateStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step updateStatus for Storage")

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
	if oldStatus != storage.Status.State {
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
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"StatusChanged",
			fmt.Sprintf("Storage moved from %s to %s", oldStatus, storageCr.Status.State),
		)

		r.Log.Info("step updateStatus for Storage requeue reconcile")
		return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
	}

	r.Log.Info("step updateStatus for Storage completed")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) getYDBCredentials(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (ydbCredentials.Credentials, ctrl.Result, error) {
	r.Log.Info("running step getYDBCredentials")

	if auth := storage.Spec.OperatorConnection; auth != nil {
		switch {
		case auth.AccessToken != nil:
			token, err := r.getSecretKey(
				ctx,
				storage.Storage.Namespace,
				auth.AccessToken.SecretKeyRef,
			)
			if err != nil {
				return nil, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
			return ydbCredentials.NewAccessTokenCredentials(token), ctrl.Result{Requeue: false}, nil
		case auth.StaticCredentials != nil:
			username := auth.StaticCredentials.Username
			password := v1alpha1.DefaultRootPassword
			if auth.StaticCredentials.Password != nil {
				var err error
				password, err = r.getSecretKey(
					ctx,
					storage.Storage.Namespace,
					auth.StaticCredentials.Password.SecretKeyRef,
				)
				if err != nil {
					return nil, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
				}
			}
			endpoint := storage.GetStorageEndpoint()
			secure := connection.LoadTLSCredentials(storage.IsStorageEndpointSecure())
			return ydbCredentials.NewStaticCredentials(username, password, endpoint, secure), ctrl.Result{Requeue: false}, nil
		}
	}
	return ydbCredentials.NewAnonymousCredentials(), ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) getSecretKey(
	ctx context.Context,
	namespace string,
	secretKeyRef *corev1.SecretKeySelector,
) (string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      secretKeyRef.Name,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return "", err
	}
	secretVal, exist := secret.Data[secretKeyRef.Key]
	if !exist {
		return "", fmt.Errorf(
			"key %s does not exist in secretData %s",
			secretKeyRef.Key,
			secretKeyRef.Name,
		)
	}

	return string(secretVal), nil
}

func (r *Reconciler) handlePauseResume(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handlePauseResume for Storage")
	if storage.Status.State == StorageReady && storage.Spec.Pause {
		r.Log.Info("`pause: true` was noticed, moving Storage to state `Paused`")
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StoragePausedCondition,
			Status:  "True",
			Reason:  ReasonCompleted,
			Message: "State Storage set to Paused",
		})
		storage.Status.State = StoragePaused
		return r.updateStatus(ctx, storage)
	}

	if storage.Status.State == StoragePaused && !storage.Spec.Pause {
		r.Log.Info("`pause: false` was noticed, moving Storage to state `Ready`")
		meta.RemoveStatusCondition(&storage.Status.Conditions, StoragePausedCondition)
		storage.Status.State = StorageReady
		return r.updateStatus(ctx, storage)
	}

	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) handleFirstStart(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (ctrl.Result, error) {
	stop, result, err := r.setInitialStatus(ctx, storage)
	if stop {
		return result, err
	}

	if storage.Spec.NodeSets != nil {
		stop, result, err = r.waitForStorageNodeSetsToReady(ctx, storage)
		if stop {
			return result, err
		}
	} else {
		stop, result, err = r.waitForStatefulSetToScale(ctx, storage)
		if stop {
			return result, err
		}
	}

	auth, result, err := r.getYDBCredentials(ctx, storage)
	if auth == nil {
		return result, err
	}

	stop, result, err = r.runSelfCheck(ctx, storage, auth, false)
	if stop {
		return result, err
	}

	stop, result, err = r.initializeStorage(ctx, storage, auth)
	if stop {
		return result, err
	}

	return result, err
}

func (r *Reconciler) checkStorageFrozen(
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result) {
	r.Log.Info("running step checkStorageFrozen for Storage")
	if !storage.Spec.OperatorSync {
		r.Log.Info("`operatorSync: false` is set, no further steps will be run")
		return Stop, ctrl.Result{}
	}

	return Continue, ctrl.Result{}
}

func (r *Reconciler) getNodeSetStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	nodeSetName string,
	nodeSetKind string,
) (ClusterState, error) {
	var nodeSetObject client.Object

	nodeSetObject = &v1alpha1.StorageNodeSet{}
	if nodeSetKind == RemoteStorageNodeSetKind {
		nodeSetObject = &v1alpha1.RemoteStorageNodeSet{}
	}

	if err := r.Get(ctx, types.NamespacedName{
		Name:      nodeSetName,
		Namespace: storage.Namespace,
	}, nodeSetObject); err != nil {
		return "", err
	}

	if nodeSetKind == RemoteStorageNodeSetKind {
		return nodeSetObject.(*v1alpha1.RemoteStorageNodeSet).Status.State, nil
	}

	return nodeSetObject.(*v1alpha1.StorageNodeSet).Status.State, nil
}
