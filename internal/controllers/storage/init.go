package storage

import (
	"context"
	"fmt"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/exec"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

var mismatchItemConfigGenerationRegexp = regexp.MustCompile(".*mismatch.*ItemConfigGenerationProvided# " +
	"0.*ItemConfigGenerationExpected# 1.*")

func (r *Reconciler) processSkipInitPipeline(storage *resources.StorageClusterBuilder) {
	r.Log.Info("running step processSkipInitPipeline")
	r.Log.Info("Storage initialization disabled (with annotation), proceed with caution")

	r.Recorder.Event(
		storage,
		corev1.EventTypeWarning,
		"SkippingInit",
		"Skipping initialization due to skip annotation present, be careful!",
	)

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:    InitStorageStepCondition,
		Status:  metav1.ConditionTrue,
		Reason:  InitStorageStepReasonCompleted,
		Message: "InitStorageStep not performed because initialization is skipped",
	})

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:    StorageInitializedCondition,
		Status:  metav1.ConditionTrue,
		Reason:  StorageInitializedReasonCompleted,
		Message: "Storage initialization skipped",
	})

	r.Recorder.Event(
		storage,
		corev1.EventTypeNormal,
		"ResourcesReady",
		"Everything should be in sync",
	)

	storage.Status.State = string(Ready)
}

func (r *Reconciler) setInitialStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")

	// This block is special internal logic that skips all Storage initialization.
	// It is needed when large clusters are migrated where `waitForStatefulSetToScale`
	// does not make sense, since some nodes can be down for a long time (and it is okay, since
	// database is healthy even with partial outage).
	if value, ok := storage.Annotations[annotationSkipInitialization]; ok && value == "true" {
		if meta.FindStatusCondition(storage.Status.Conditions, StorageInitializedCondition) == nil ||
			meta.IsStatusConditionFalse(storage.Status.Conditions, StorageInitializedCondition) {
			r.processSkipInitPipeline(storage)
			return r.setState(ctx, storage)
		}
		return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, nil
	}

	changed := false
	if meta.FindStatusCondition(storage.Status.Conditions, StorageInitializedCondition) == nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageInitializedCondition,
			Status:  "False",
			Reason:  StorageInitializedReasonInProgress,
			Message: "Storage initialization in progress",
		})
		changed = true
	}
	if meta.FindStatusCondition(storage.Status.Conditions, InitStorageStepCondition) == nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    InitStorageStepCondition,
			Status:  "False",
			Reason:  InitStorageStepReasonInProgress,
			Message: "InitStorageStep is required",
		})
		changed = true
	}
	if storage.Status.State != string(Initializing) {
		storage.Status.State = string(Initializing)
		changed = true
	}
	if changed {
		return r.setState(ctx, storage)
	}
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) runInitScripts(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step runInitScripts")
	podName := fmt.Sprintf("%s-0", storage.Name)

	if meta.IsStatusConditionTrue(storage.Status.Conditions, InitStorageStepCondition) {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageInitializedCondition,
			Status:  "True",
			Reason:  StorageInitializedReasonCompleted,
			Message: "Storage initialized successfully",
		})

		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"ResourcesReady",
			"Everything should be in sync",
		)
		storage.Status.State = string(Ready)

		return r.setState(ctx, storage)
	}

	cmd := []string{
		fmt.Sprintf("%s/%s", v1alpha1.BinariesDir, v1alpha1.DaemonBinaryName),
	}
	if storage.Spec.Service.GRPC.TLSConfiguration.Enabled {
		cmd = append(
			cmd,
			"-s", storage.GetGRPCEndpointWithProto(),
		)
	}
	cmd = append(
		cmd,
		"admin", "blobstorage", "config", "init",
		"--yaml-file",
		fmt.Sprintf("%s/%s", v1alpha1.ConfigDir, v1alpha1.ConfigFileName),
	)

	stdout, _, err := exec.InPod(r.Scheme, r.Config, storage.Namespace, podName, "ydb-storage", cmd)
	if err != nil {
		if mismatchItemConfigGenerationRegexp.MatchString(stdout) {
			r.Log.Info("Storage is already initialized, continuing...")

			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:    InitStorageStepCondition,
				Status:  "True",
				Reason:  InitStorageStepReasonCompleted,
				Message: "InitStorageStep counted as completed, Storage already initialized",
			})
			return r.setState(ctx, storage)
		}

		return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, err
	}

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:    InitStorageStepCondition,
		Status:  "True",
		Reason:  InitStorageStepReasonCompleted,
		Message: "InitStorageStep completed successfully",
	})

	return r.setState(ctx, storage)
}
