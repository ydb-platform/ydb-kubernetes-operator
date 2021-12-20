package storage

import (
	"context"
	"fmt"
	"path"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/exec"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	InitStorageScript     = "init_storage.bash"
	InitRootStorageScript = "init_root_storage.bash"
	InitCMSScript         = "init_cms.bash"
)

func (r *StorageReconciler) setInitialStatus(ctx context.Context, storage *resources.StorageClusterBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")
	var changed bool = false
	if meta.FindStatusCondition(storage.Status.Conditions, StorageInitializedCondition) == nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageInitializedCondition,
			Status:  "False",
			Reason:  StorageInitializedReasonInProgress,
			Message: "Storage is not initialized",
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
	if meta.FindStatusCondition(storage.Status.Conditions, InitCMSStepCondition) == nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    InitCMSStepCondition,
			Status:  "False",
			Reason:  InitCMSStepReasonInProgress,
			Message: "InitCMSStep is required",
		})
		changed = true
	}
	if meta.FindStatusCondition(storage.Status.Conditions, InitRootStorageStepCondition) == nil {
		configMapName := storage.Name
		if storage.Spec.ClusterConfig != "" {
			configMapName = storage.Spec.ClusterConfig
		}
		configMap := &corev1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      configMapName,
			Namespace: storage.Namespace,
		}, configMap)

		if err != nil {
			if !errors.IsNotFound(err) {
				r.Recorder.Event(
					storage,
					corev1.EventTypeNormal,
					"Syncing",
					fmt.Sprintf("Failed to get ConfigMap: %s", err),
				)
			}
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		if _, ok := configMap.Data[InitRootStorageScript]; ok {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:    InitRootStorageStepCondition,
				Status:  "False",
				Reason:  InitRootStorageStepReasonInProgress,
				Message: fmt.Sprintf("InitRootStorageStep is required, %s script is specified in ConfigMap: %s", InitRootStorageScript, configMapName),
			})
			changed = true
		} else {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:    InitRootStorageStepCondition,
				Status:  "True",
				Reason:  InitRootStorageStepReasonNotRequired,
				Message: "InitRootStorageStep is not required",
			})
			changed = true
		}
	}
	if changed {
		return r.setState(ctx, storage)
	}
	return Continue, ctrl.Result{}, nil
}

func (r *StorageReconciler) runInitScripts(ctx context.Context, storage *resources.StorageClusterBuilder) (bool, ctrl.Result, error) {
	r.Log.Info("running step runInitScripts")
	podName := fmt.Sprintf("%s-0", storage.Name)

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, InitStorageStepCondition) {
		cmd := []string{"/bin/bash", path.Join(v1alpha1.ConfigDir, InitStorageScript)}
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

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, InitRootStorageStepCondition) {
		cmd := []string{"/bin/bash", path.Join(v1alpha1.ConfigDir, InitRootStorageScript)}
		_, _, err := exec.ExecInPod(r.Scheme, r.Config, storage.Namespace, podName, "ydb-storage", cmd)
		if err != nil {
			return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, err
		}
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    InitRootStorageStepCondition,
			Status:  "True",
			Reason:  InitRootStorageStepReasonCompleted,
			Message: "InitRootStorageStep completed successfully",
		})
		return r.setState(ctx, storage)
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, InitCMSStepCondition) {
		cmd := []string{"/bin/bash", path.Join(v1alpha1.ConfigDir, InitCMSScript)}
		_, _, err := exec.ExecInPod(r.Scheme, r.Config, storage.Namespace, podName, "ydb-storage", cmd)
		if err != nil {
			return Stop, ctrl.Result{RequeueAfter: StorageInitializationRequeueDelay}, err
		}
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    InitCMSStepCondition,
			Status:  "True",
			Reason:  InitCMSStepReasonCompleted,
			Message: "InitCMSStep completed successfully",
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
