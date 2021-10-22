package storage

import (
	"context"
	"fmt"
	"path"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers"
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

func (r *StorageReconciler) setInitialStatus(ctx context.Context, storage *resources.StorageClusterBuilder) (ctrl.Result, error) {
	if meta.FindStatusCondition(storage.Status.Conditions, StorageInitializedCondition) == nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    StorageInitializedCondition,
			Status:  "False",
			Reason:  StorageInitializedReasonInProgress,
			Message: "Storage is not initialized",
		})
		if _, err := r.setState(ctx, storage); err != nil {
			return controllers.NoRequeue(err)
		}
	}

	if meta.FindStatusCondition(storage.Status.Conditions, InitStorageStepCondition) == nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    InitStorageStepCondition,
			Status:  "False",
			Reason:  InitStorageStepReasonInProgress,
			Message: "InitStorageStep is required",
		})
		if _, err := r.setState(ctx, storage); err != nil {
			return controllers.NoRequeue(err)
		}
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

		if err != nil && errors.IsNotFound(err) {
			return controllers.Ok()
		} else if err != nil {
			r.Recorder.Event(
				storage,
				corev1.EventTypeNormal,
				"Syncing",
				fmt.Sprintf("Failed to get ConfigMap: %s", err),
			)
			return controllers.NoRequeue(err)
		}

		if _, ok := configMap.Data[InitRootStorageScript]; ok {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:    InitRootStorageStepCondition,
				Status:  "False",
				Reason:  InitRootStorageStepReasonInProgress,
				Message: fmt.Sprintf("InitRootStorageStep is required, %s script is specified in ConfigMap: %s", InitRootStorageScript, configMapName),
			})
			if _, err := r.setState(ctx, storage); err != nil {
				return controllers.NoRequeue(err)
			}
		} else {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:    InitRootStorageStepCondition,
				Status:  "True",
				Reason:  InitRootStorageStepReasonNotRequired,
				Message: "InitRootStorageStep is not required",
			})
			if _, err := r.setState(ctx, storage); err != nil {
				return controllers.NoRequeue(err)
			}
		}
	}

	if meta.FindStatusCondition(storage.Status.Conditions, InitCMSStepCondition) == nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    InitCMSStepCondition,
			Status:  "False",
			Reason:  InitCMSStepReasonInProgress,
			Message: "InitCMSStep is required",
		})
		if _, err := r.setState(ctx, storage); err != nil {
			return controllers.NoRequeue(err)
		}
	}

	return controllers.Ok()
}

func (r *StorageReconciler) runInitScripts(ctx context.Context, storage *resources.StorageClusterBuilder) (ctrl.Result, error) {
	podName := fmt.Sprintf("%s-0", storage.Name)

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, InitStorageStepCondition) {
		cmd := []string{"/bin/bash", path.Join(v1alpha1.ConfigDir, InitStorageScript)}
		_, _, err := exec.ExecInPod(r.Scheme, r.Config, storage.Namespace, podName, "ydb-storage", cmd)
		if err != nil {
			return controllers.RequeueAfter(StorageInitializationRequeueDelay, err)
		}
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    InitStorageStepCondition,
			Status:  "True",
			Reason:  InitStorageStepReasonCompleted,
			Message: "InitStorageStep completed successfully",
		})
		if _, err := r.setState(ctx, storage); err != nil {
			return controllers.NoRequeue(err)
		}
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, InitRootStorageStepCondition) {
		cmd := []string{"/bin/bash", path.Join(v1alpha1.ConfigDir, InitRootStorageScript)}
		_, _, err := exec.ExecInPod(r.Scheme, r.Config, storage.Namespace, podName, "ydb-storage", cmd)
		if err != nil {
			return controllers.RequeueAfter(StorageInitializationRequeueDelay, err)
		}
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    InitRootStorageStepCondition,
			Status:  "True",
			Reason:  InitRootStorageStepReasonCompleted,
			Message: "InitRootStorageStep completed successfully",
		})
		if _, err := r.setState(ctx, storage); err != nil {
			return controllers.NoRequeue(err)
		}
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, InitCMSStepCondition) {
		cmd := []string{"/bin/bash", path.Join(v1alpha1.ConfigDir, InitCMSScript)}
		_, _, err := exec.ExecInPod(r.Scheme, r.Config, storage.Namespace, podName, "ydb-storage", cmd)
		if err != nil {
			return controllers.RequeueAfter(StorageInitializationRequeueDelay, err)
		}
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    InitCMSStepCondition,
			Status:  "True",
			Reason:  InitCMSStepReasonCompleted,
			Message: "InitCMSStep completed successfully",
		})
		if _, err := r.setState(ctx, storage); err != nil {
			return controllers.NoRequeue(err)
		}
	}

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:    StorageInitializedCondition,
		Status:  "True",
		Reason:  StorageInitializedReasonCompleted,
		Message: "Storage initialized successfully",
	})
	if _, err := r.setState(ctx, storage); err != nil {
		return controllers.NoRequeue(err)
	}

	return controllers.RequeueImmediately()
}
