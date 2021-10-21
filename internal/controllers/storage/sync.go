package storage

import (
	"context"
	"fmt"
	"time"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/exec"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/healthcheck"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	DefaultRequeueDelay               = 10 * time.Second
	HealthcheckRequeueDelay           = 30 * time.Second
	StorageInitializationRequeueDelay = 30 * time.Second

	StorageInitializedCondition        = "StorageInitialized"
	StorageInitializedReasonInProgress = "InProgres"
	StorageInitializedReasonCompleted  = "Completed"

	InitStorageStepCondition        = "InitStorageStep"
	InitStorageStepReasonInProgress = "InProgres"
	InitStorageStepReasonCompleted  = "Completed"

	InitRootStorageStepCondition         = "InitRootStorageStep"
	InitRootStorageStepReasonInProgress  = "InProgres"
	InitRootStorageStepReasonNotRequired = "NotRequired"
	InitRootStorageStepReasonCompleted   = "Completed"

	InitCMSStepCondition        = "InitCMSStep"
	InitCMSStepReasonInProgress = "InProgres"
	InitCMSStepReasonCompleted  = "Completed"
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

	result, err = r.waitForHealthCheck(ctx, &storage)
	if err != nil || !result.IsZero() {
		return result, err
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, StorageInitializedCondition) {
		result, err = r.setInitialStatus(ctx, &storage)
		if err != nil || !result.IsZero() {
			return result, err
		}
		result, err = r.runInitScripts(ctx, &storage)
		if err != nil || !result.IsZero() {
			return result, err
		}
	}

	return controllers.Ok()
}

func (r *StorageReconciler) runInitScripts(ctx context.Context, storage *resources.StorageClusterBuilder) (ctrl.Result, error) {
	podName := fmt.Sprintf("%s-0", storage.Name)

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, InitStorageStepCondition) {
		cmd := []string{"/bin/bash", "/opt/kikimr/cfg/init_storage.bash"}
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
		cmd := []string{"/bin/bash", "/opt/kikimr/cfg/init_root_storage.bash"}
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
		cmd := []string{"/bin/bash", "/opt/kikimr/cfg/init_cms.bash"}
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

func (r *StorageReconciler) setInitialStatus(ctx context.Context, storage *resources.StorageClusterBuilder) (ctrl.Result, error) {
	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:    StorageInitializedCondition,
		Status:  "False",
		Reason:  StorageInitializedReasonInProgress,
		Message: "Storage is not initialized",
	})
	if _, err := r.setState(ctx, storage); err != nil {
		return controllers.NoRequeue(err)
	}

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

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:    InitStorageStepCondition,
		Status:  "False",
		Reason:  InitStorageStepReasonInProgress,
		Message: "InitStorageStep is required",
	})
	if _, err := r.setState(ctx, storage); err != nil {
		return controllers.NoRequeue(err)
	}

	if _, ok := configMap.Data["init_root_storage.bash"]; ok {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    InitRootStorageStepCondition,
			Status:  "False",
			Reason:  InitRootStorageStepReasonInProgress,
			Message: fmt.Sprintf("InitRootStorageStep (init_root_storage.bash script) is specified in ConfigMap: %s", configMapName),
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

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:    InitCMSStepCondition,
		Status:  "False",
		Reason:  InitCMSStepReasonInProgress,
		Message: "InitCMSStep is required",
	})
	if _, err := r.setState(ctx, storage); err != nil {
		return controllers.NoRequeue(err)
	}

	return controllers.Ok()
}

func (r *StorageReconciler) waitForHealthCheck(ctx context.Context, storage *resources.StorageClusterBuilder) (ctrl.Result, error) {
	err := healthcheck.CheckBootstrapHealth(ctx, storage)

	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			"HealthcheckInProgress",
			fmt.Sprintf("Waiting for healthcheck, current status: %s", err),
		)
		return controllers.RequeueAfter(HealthcheckRequeueDelay, err)
	}

	r.Recorder.Event(
		storage,
		corev1.EventTypeNormal,
		"HealthcheckOK",
		"Bootstrap healthcheck is green",
	)
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
