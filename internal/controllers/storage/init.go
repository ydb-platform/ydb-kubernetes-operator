package storage

import (
	"context"
	"fmt"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/exec"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *StorageReconciler) setInitialStatus(ctx context.Context, storage *resources.StorageClusterBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")
	var changed bool = false
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

func (r *StorageReconciler) runInitScripts(ctx context.Context, storage *resources.StorageClusterBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step runInitScripts")
	podName := fmt.Sprintf("%s-0", storage.Name)

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, InitStorageStepCondition) {
		cmd := []string{
			fmt.Sprintf("%s/%s", v1alpha1.BinariesDir, v1alpha1.DaemonBinaryName),
			"admin", "blobstorage", "config", "init",
			"--yaml-file",
			fmt.Sprintf("%s/%s", v1alpha1.ConfigDir, v1alpha1.ConfigFileName),
		}
		_, _, err := exec.ExecInPod(r.Scheme, r.Config, storage.Namespace, podName, "ydb-storage", cmd)
		if err != nil {
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

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:    StorageInitializedCondition,
		Status:  "True",
		Reason:  StorageInitializedReasonCompleted,
		Message: "Storage initialized successfully",
	})
	return r.setState(ctx, storage)
}
