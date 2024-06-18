package storage

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/cms"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func (r *Reconciler) handleConfigurationSync(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleConfigurationSync")

	// DryRun ReplaceConfig
	condition := meta.FindStatusCondition(storage.Status.Conditions, ReplaceConfigDryRunOperationCondition)
	if condition != nil && condition.ObservedGeneration == storage.Generation && condition.Status == metav1.ConditionUnknown {
		return r.pollOperation(ctx, storage, condition)
	}
	if condition == nil || condition.ObservedGeneration < storage.Generation || condition.Status == metav1.ConditionFalse {
		return r.requestOperation(ctx, storage, ReplaceConfigDryRunOperationCondition, true)
	}

	// Exactly ReplaceConfig
	condition = meta.FindStatusCondition(storage.Status.Conditions, ReplaceConfigOperationCondition)
	if condition != nil && condition.ObservedGeneration == storage.Generation && condition.Status == metav1.ConditionUnknown {
		return r.pollOperation(ctx, storage, condition)
	}
	if condition == nil || condition.ObservedGeneration < storage.Generation || condition.Status == metav1.ConditionFalse {
		return r.requestOperation(ctx, storage, ReplaceConfigOperationCondition, false)
	}

	_, dynconfig, _ := v1alpha1.TryParseDynconfig(storage.Spec.Configuration)
	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               ConfigurationSyncedCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            fmt.Sprintf("Configuration synced successfully to version %d", dynconfig.Metadata.Version),
	})
	r.Log.Info("complete step handleConfigurationSync")
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}

func (r *Reconciler) requestOperation(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	operationType string,
	dryRun bool,
) (bool, ctrl.Result, error) {
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
			fmt.Sprintf("Failed to get YDB credentials: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	response, err := cms.ReplaceConfig(ctx, storage, dryRun, creds, tlsOptions)
	if err != nil {
		r.Log.Error(err, "failed to request CMS ReplaceConfig")
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               operationType,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonFailed,
			Message:            fmt.Sprintf("Failed to request CMS ReplaceConfig: %s", err),
		})
		return Stop, ctrl.Result{RequeueAfter: ReplaceConfigOperationRequeueDelay}, err
	}
	r.Log.Info("CMS ReplaceConfig", "response", response)

	operation := response.GetOperation()
	if operation.GetReady() {
		err := cms.CheckOperationSuccess(operation)
		if err != nil {
			r.Log.Error(err, "failed response CMS ReplaceConfig")
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:               operationType,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: storage.Generation,
				Reason:             ReasonFailed,
				Message:            fmt.Sprintf("Failed response CMS ReplaceConfig: %s", err),
			})
			return r.updateStatus(ctx, storage, DefaultRequeueDelay)
		}

		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               operationType,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonCompleted,
			Message:            "CMS ReplaceConfig operation is Success",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	operationID := operation.GetId()
	if len(operationID) == 0 {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               operationType,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonFailed,
			Message:            fmt.Sprintf("Failed to get operationID from response: %s", response),
		})
		return r.updateStatus(ctx, storage, DefaultRequeueDelay)
	}

	r.Log.Info("Waiting for CMS ReplaceConfig operation to be Ready", "operationID", operationID)
	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               operationType,
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: storage.Generation,
		Reason:             operationID,
		Message:            "Waiting for CMS ReplaceConfig operation to be Ready",
	})
	return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
}

func (r *Reconciler) pollOperation(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	condition *metav1.Condition,
) (bool, ctrl.Result, error) {
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

	operationID := condition.Reason
	response, err := cms.GetOperation(ctx, storage, operationID, creds, tlsOptions)
	if err != nil {
		r.Log.Error(err, "request CMS GetOperation error")
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to request CMS GetOperation: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	operation := response.GetOperation()
	if !operation.GetReady() {
		r.Log.Info("Waiting for CMS ReplaceConfig operation to be Ready", "operationID", operationID)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               condition.Type,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             operationID,
			Message:            "Waiting for CMS ReplaceConfig operation to be Ready",
		})
		return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
	}

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               condition.Type,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            "CMS ReplaceConfig operation is Success",
	})
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}

func (r *Reconciler) setConfigPipelineStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setConfigPipelineStatus")
	success, dynconfig, _ := v1alpha1.TryParseDynconfig(storage.Spec.Configuration)
	if !success {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonNotRequired,
			Message:            "Sync static configuration does not supported",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

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

	response, err := cms.GetConfig(ctx, storage, creds, tlsOptions)
	if err != nil {
		r.Log.Error(err, "request CMS GetConfig error")
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Failed to request CMS GetConfig: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	r.Log.Info("CMS GetConfig response", "message", response)
	cmsConfig, err := cms.GetConfigResult(response)
	if err != nil {
		r.Log.Error(err, "Failed to unmarshal GetConfigResponse")
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Failed to unmarshal GetConfigResponse: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	if dynconfig.Metadata.Version < cmsConfig.GetIdentity().GetVersion() {
		r.Log.Info("Configuration already synced", "metadata", cmsConfig.GetIdentity().String())
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonNotRequired,
			Message:            fmt.Sprintf("Configuration already synced to version %d", cmsConfig.GetIdentity().GetVersion()),
		})
	} else {
		r.Log.Info("Sync configuration", "metadata", dynconfig.Metadata)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonInProgress,
			Message:            fmt.Sprintf("Sync configuration to version %d in progress", dynconfig.Metadata.Version),
		})
	}

	r.Log.Info("complete step setConfigPipelineStatus")
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}
