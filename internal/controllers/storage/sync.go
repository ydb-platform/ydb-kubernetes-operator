package storage

import (
	"context"
	"fmt"
	"time"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers"
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
	DefaultRequeueDelay               = 10 * time.Second
	SelfCheckRequeueDelay             = 30 * time.Second
	StorageInitializationRequeueDelay = 30 * time.Second

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
)

func (r *StorageReconciler) Sync(ctx context.Context, cr *ydbv1alpha1.Storage) (ctrl.Result, error) {
	var err error
	var result ctrl.Result

	storage := resources.NewCluster(cr, r.Log)
	storage.SetStatusOnFirstReconcile()

	result, err = r.waitForStatefulSetToScale(ctx, &storage)
	if err != nil || !result.IsZero() {
		return result, err
	}

	result, err = r.handleResourcesSync(ctx, &storage)
	if err != nil || !result.IsZero() {
		return result, err
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, StorageInitializedCondition) {
		result, err = r.setInitialStatus(ctx, &storage)
		if err != nil || !result.IsZero() {
			return result, err
		}
		result, err = r.runSelfCheck(ctx, &storage, true)
		if err != nil || !result.IsZero() {
			return result, err
		}
		result, err = r.runInitScripts(ctx, &storage)
		if err != nil || !result.IsZero() {
			return result, err
		}
	}

	result, err = r.runSelfCheck(ctx, &storage, false)
	if err != nil || !result.IsZero() {
		return result, err
	}

	return controllers.Ok()
}

func (r *StorageReconciler) waitForStatefulSetToScale(ctx context.Context, storage *resources.StorageClusterBuilder) (ctrl.Result, error) {
	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      storage.Name,
		Namespace: storage.Namespace,
	}, found)

	if err != nil && errors.IsNotFound(err) {
		return controllers.Ok()
	} else if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"Syncing",
			fmt.Sprintf("Failed to get StatefulSets: %s", err),
		)
		return controllers.NoRequeue(err)
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
		return controllers.NoRequeue(err)
	}

	runningPods := 0
	for _, e := range podList.Items {
		if e.Status.Phase == "Running" {
			runningPods += 1
		}
	}

	if runningPods != int(storage.Spec.Nodes) {
		storage.Status.State = "Provisioning"
		if _, err := r.setState(ctx, storage); err != nil {
			return controllers.NoRequeue(err)
		}

		msg := fmt.Sprintf("Waiting for number of running pods to match expected: %d != %d", runningPods, storage.Spec.Nodes)
		r.Recorder.Event(storage, corev1.EventTypeNormal, "Provisioning", msg)

		return controllers.RequeueAfter(DefaultRequeueDelay, nil)
	}

	if storage.Status.State != "Ready" && meta.IsStatusConditionTrue(storage.Status.Conditions, StorageInitializedCondition) {
		storage.Status.State = "Ready"
		if _, err = r.setState(ctx, storage); err != nil {
			return controllers.NoRequeue(err)
		}
		r.Recorder.Event(storage, corev1.EventTypeNormal, "ResourcesReady", "Everything should be in sync")
	}

	return controllers.Ok()
}

func (r *StorageReconciler) handleResourcesSync(ctx context.Context, storage *resources.StorageClusterBuilder) (ctrl.Result, error) {
	r.Recorder.Event(storage, corev1.EventTypeNormal, "Provisioning", "Resource sync is in progress")

	areResourcesCreated := false

	for _, builder := range storage.GetResourceBuilders() {
		rr := builder.Placeholder(storage)

		result, err := ctrl.CreateOrUpdate(ctx, r.Client, rr, func() error {
			err := builder.Build(rr)

			if err != nil {
				r.Recorder.Event(
					storage,
					corev1.EventTypeWarning,
					"ProvisioningFailed",
					fmt.Sprintf("Failed building resources: %s", err),
				)
				return err
			}

			err = ctrl.SetControllerReference(storage.Unwrap(), rr, r.Scheme)
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

		if err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				fmt.Sprintf("Failed syncing resources: %s", err),
			)
			return controllers.NoRequeue(err)
		}

		areResourcesCreated = areResourcesCreated || (result == controllerutil.OperationResultCreated)
	}

	r.Recorder.Event(storage, corev1.EventTypeNormal, "Provisioning", "Resource sync complete")

	if areResourcesCreated {
		return controllers.RequeueImmediately()
	}

	return controllers.Ok()
}

func (r *StorageReconciler) runSelfCheck(ctx context.Context, storage *resources.StorageClusterBuilder, waitForGoodResultWithoutIssues bool) (ctrl.Result, error) {
	result, err := healthcheck.GetSelfCheckResult(ctx, storage)

	if err != nil {
		r.Log.Error(err, "GetSelfCheckResult error")
		return controllers.RequeueAfter(SelfCheckRequeueDelay, err)
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

	if waitForGoodResultWithoutIssues && (result.SelfCheckResult.String() != "GOOD" || len(result.IssueLog) > 0) {
		return controllers.RequeueAfter(SelfCheckRequeueDelay, err)
	}
	return controllers.Ok()
}

func (r *StorageReconciler) setState(ctx context.Context, storage *resources.StorageClusterBuilder) (ctrl.Result, error) {
	storageCr := &ydbv1alpha1.Storage{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: storage.Namespace,
		Name:      storage.Name,
	}, storageCr)

	if err != nil {
		r.Recorder.Event(storageCr, corev1.EventTypeWarning, "ControllerError", "Failed fetching CR before status update")
		return controllers.NoRequeue(err)
	}

	storageCr.Status.State = storage.Status.State
	storageCr.Status.Conditions = storage.Status.Conditions

	err = r.Status().Update(ctx, storageCr)
	if err != nil {
		r.Recorder.Event(storageCr, corev1.EventTypeWarning, "ControllerError", fmt.Sprintf("Failed setting status: %s", err))
		return controllers.NoRequeue(err)
	}

	return controllers.Ok()
}
