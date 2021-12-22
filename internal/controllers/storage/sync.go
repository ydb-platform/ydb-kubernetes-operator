package storage

import (
	"context"
	"fmt"
	"time"
	"reflect"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/healthcheck"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	Provisioning ClusterState = "Provisioning"
	Initializing ClusterState = "Initializing"
	Ready        ClusterState = "Ready"

	DefaultRequeueDelay               = 10 * time.Second
	StatusUpdateRequeueDelay          =  1 * time.Second
	SelfCheckRequeueDelay             = 30 * time.Second
	StorageInitializationRequeueDelay =  5 * time.Second

	ReasonInProgress  = "InProgress"
	ReasonNotRequired = "NotRequired"
	ReasonCompleted   = "Completed"

	StorageInitializedCondition        = "StorageInitialized"
	StorageInitializedReasonInProgress = ReasonInProgress
	StorageInitializedReasonCompleted  = ReasonCompleted

	InitStorageStepCondition        = "InitStorageStep"
	InitStorageStepReasonInProgress = ReasonInProgress
	InitStorageStepReasonCompleted  = ReasonCompleted

	InitRootStorageStepCondition         = "InitRootStorageStep"
	InitRootStorageStepReasonInProgress  = ReasonInProgress
	InitRootStorageStepReasonNotRequired = ReasonNotRequired
	InitRootStorageStepReasonCompleted   = ReasonCompleted

	InitCMSStepCondition        = "InitCMSStep"
	InitCMSStepReasonInProgress = ReasonInProgress
	InitCMSStepReasonCompleted  = ReasonCompleted

	Stop     = true
	Continue = false
)

type ClusterState string

func (r *StorageReconciler) Sync(ctx context.Context, cr *ydbv1alpha1.Storage) (ctrl.Result, error) {
	var stop bool
	var result ctrl.Result
	var err error

	storage := resources.NewCluster(cr, r.Log)
	storage.SetStatusOnFirstReconcile()

	stop, result, err = r.handleResourcesSync(ctx, &storage)
	if stop {
		return result, err
	}
	stop, result, err = r.waitForStatefulSetToScale(ctx, &storage)
	if stop {
		return result, err
	}
	if !meta.IsStatusConditionTrue(storage.Status.Conditions, StorageInitializedCondition) {
		stop, result, err = r.setInitialStatus(ctx, &storage)
		if stop {
			return result, err
		}
		stop, result, err = r.runSelfCheck(ctx, &storage, true)
		if stop {
			return result, err
		}
		stop, result, err = r.runInitScripts(ctx, &storage)
		if stop {
			return result, err
		}
	}
	_, result, err = r.runSelfCheck(ctx, &storage, false)
	return result, err
}

func (r *StorageReconciler) waitForStatefulSetToScale(ctx context.Context, storage *resources.StorageClusterBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step waitForStatefulSetToScale")
	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      storage.Name,
		Namespace: storage.Namespace,
	}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
		}
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"Syncing",
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
			corev1.EventTypeNormal,
			"Syncing",
			fmt.Sprintf("Failed to list cluster pods: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	runningPods := 0
	for _, e := range podList.Items {
		if e.Status.Phase == "Running" {
			runningPods += 1
		}
	}

	if runningPods != int(storage.Spec.Nodes) {
		msg := fmt.Sprintf("Waiting for number of running pods to match expected: %d != %d", runningPods, storage.Spec.Nodes)
		r.Recorder.Event(storage, corev1.EventTypeNormal, string(Provisioning), msg)
		storage.Status.State = string(Provisioning)
		return r.setState(ctx, storage)
	}

	if storage.Status.State != string(Ready) && meta.IsStatusConditionTrue(storage.Status.Conditions, StorageInitializedCondition) {
		r.Recorder.Event(storage, corev1.EventTypeNormal, "ResourcesReady", "Everything should be in sync")
		storage.Status.State = string(Ready)
		return r.setState(ctx, storage)
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *StorageReconciler) handleResourcesSync(ctx context.Context, storage *resources.StorageClusterBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleResourcesSync")

	for _, builder := range storage.GetResourceBuilders() {
		new_resource := builder.Placeholder(storage)

		result, err := resources.CreateOrUpdateIgnoreStatus(ctx, r.Client, new_resource, func() error {
			var err error

			err = builder.Build(new_resource)
			if err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}
			err = ctrl.SetControllerReference(storage.Unwrap(), new_resource, r.Scheme)
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
		})

		var eventMessage string = fmt.Sprintf(
			"Resource: %s, Namespace: %s, Name: %s",
			reflect.TypeOf(new_resource),
			new_resource.GetNamespace(),
			new_resource.GetName(),
		)
		if err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				eventMessage + fmt.Sprintf(", failed to sync, error: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		} else if (result == controllerutil.OperationResultCreated || result == controllerutil.OperationResultUpdated) {
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				string(Provisioning),
				eventMessage + fmt.Sprintf(", changed, result: %s", result),
			)
		}
	}
	r.Log.Info("resource sync complete")
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *StorageReconciler) runSelfCheck(ctx context.Context, storage *resources.StorageClusterBuilder, waitForGoodResultWithoutIssues bool) (bool, ctrl.Result, error) {
	r.Log.Info("running step runSelfCheck")
	result, err := healthcheck.GetSelfCheckResult(ctx, storage)

	if err != nil {
		r.Log.Error(err, "GetSelfCheckResult error")
		return Stop, ctrl.Result{RequeueAfter: SelfCheckRequeueDelay}, err
	}

	eventType := corev1.EventTypeNormal
	if result.SelfCheckResult.String() != "GOOD" {
		eventType = corev1.EventTypeWarning
	}

	r.Recorder.Event(
		storage,
		eventType,
		"SelfCheck",
		fmt.Sprintf("SelfCheck result: %s, issues found: %d", result.SelfCheckResult.String(), len(result.IssueLog)),
	)

	if waitForGoodResultWithoutIssues && result.SelfCheckResult.String() != "GOOD" {
		return Stop, ctrl.Result{RequeueAfter: SelfCheckRequeueDelay}, err
	}
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *StorageReconciler) setState(ctx context.Context, storage *resources.StorageClusterBuilder) (bool, ctrl.Result, error) {
	storageCr := &ydbv1alpha1.Storage{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: storage.Namespace,
		Name:      storage.Name,
	}, storageCr)

	if err != nil {
		r.Recorder.Event(storageCr, corev1.EventTypeWarning, "ControllerError", "Failed fetching CR before status update")
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	storageCr.Status.State = storage.Status.State
	storageCr.Status.Conditions = storage.Status.Conditions

	err = r.Status().Update(ctx, storageCr)
	if err != nil {
		r.Recorder.Event(storageCr, corev1.EventTypeWarning, "ControllerError", fmt.Sprintf("Failed setting status: %s", err))
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
}
