package storage

import (
	"context"
	"errors"
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
	if condition != nil && condition.Status == metav1.ConditionUnknown {
		return r.pollOperation(ctx, storage, condition)
	}
	if condition == nil || condition.ObservedGeneration < storage.Generation || condition.Status == metav1.ConditionFalse {
		operationID, err := r.replaceConfig(ctx, storage, true)
		if err != nil {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:               ReplaceConfigDryRunOperationCondition,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: storage.Generation,
				Reason:             ReasonFailed,
				Message:            err.Error(),
			})
			return r.updateStatus(ctx, storage, DefaultRequeueDelay)
		}
		if operationID != "" {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:               ReplaceConfigDryRunOperationCondition,
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: storage.Generation,
				Reason:             operationID,
				Message:            "Waiting for CMS ReplaceConfig operation is Ready",
			})
			return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
		}
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigDryRunOperationCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonCompleted,
			Message:            "CMS ReplaceConfig operation status is Success",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	// Exactly ReplaceConfig
	condition = meta.FindStatusCondition(storage.Status.Conditions, ReplaceConfigOperationCondition)
	if condition != nil && condition.Status == metav1.ConditionUnknown {
		return r.pollOperation(ctx, storage, condition)
	}
	if condition == nil || condition.ObservedGeneration < storage.Generation || condition.Status == metav1.ConditionFalse {
		operationID, err := r.replaceConfig(ctx, storage, false)
		if err != nil {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:               ReplaceConfigOperationCondition,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: storage.Generation,
				Reason:             ReasonFailed,
				Message:            err.Error(),
			})
			return r.updateStatus(ctx, storage, DefaultRequeueDelay)
		}
		if operationID != "" {
			meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
				Type:               ReplaceConfigOperationCondition,
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: storage.Generation,
				Reason:             operationID,
				Message:            "Waiting for CMS ReplaceConfig operation is Ready",
			})
			return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
		}
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigOperationCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonCompleted,
			Message:            "CMS ReplaceConfig operation status is Success",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step handleConfigurationSync")
	return r.setConfigurationSyncCompleted(ctx, storage)
}

func (r *Reconciler) replaceConfig(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	dryRun bool,
) (string, error) {
	creds, err := resources.GetYDBCredentials(ctx, storage.Unwrap(), r.Config)
	if err != nil {
		errMessage := fmt.Sprintf("Failed to get YDB credentials: %s", err)
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			errMessage,
		)
		return "", errors.New(errMessage)
	}

	tlsOptions, err := resources.GetYDBTLSOption(ctx, storage.Unwrap(), r.Config)
	if err != nil {
		errMessage := fmt.Sprintf("Failed to get YDB TLS options: %s", err)
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			errMessage,
		)
		return "", errors.New(errMessage)
	}

	response, err := cms.ReplaceConfig(ctx, storage, creds, tlsOptions)
	if err != nil {
		r.Log.Error(err, "failed to request CMS ReplaceConfig")
		errMessage := fmt.Sprintf("Failed to request CMS ReplaceConfig: %s", err)
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			errMessage,
		)
		return "", errors.New(errMessage)
	}
	r.Log.Info("CMS ReplaceConfig", "response", response)

	operation := response.GetOperation()
	if operation.GetReady() {
		err := cms.CheckOperationSuccess(operation)
		if err != nil {
			r.Log.Error(err, "failed response CMS ReplaceConfig")
			errMessage := fmt.Sprintf("Failed response CMS ReplaceConfig: %s", err)
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				string(StorageProvisioning),
				errMessage,
			)
			return "", errors.New(errMessage)
		}

		return "", nil
	}

	operationId := operation.GetId()
	if len(operationId) == 0 {
		errMessage := fmt.Sprintf("Failed to get operationId from response: %s", response)
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			errMessage,
		)
		return "", errors.New(errMessage)
	}

	return operationId, nil
}

func (r *Reconciler) pollOperation(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	condition *metav1.Condition,
) (bool, ctrl.Result, error) {
	// poll response with ready == false
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

	operationId := condition.Reason
	response, err := cms.GetOperation(ctx, storage, operationId, creds, tlsOptions)
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
		r.Log.Info("Waiting for CMS ReplaceConfig operation is Ready", "operationId", operationId)
		return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
	}

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               condition.Type,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            fmt.Sprintf("CMS ReplaceConfig operation %s status is Success", operationId),
	})
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}

func (r *Reconciler) setConfigPipelineStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setConfigPipelineStatus")
	dynConfig, err := v1alpha1.ParseDynconfig(storage.Spec.Configuration)
	if err != nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonNotRequired,
			Message:            "Sync configuration does not support",
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

	if dynConfig.Metadata.Version < cmsConfig.GetIdentity().GetVersion() {
		r.Log.Info("Configuration version %s already synced", "Storage Config metadata", dynConfig.Metadata, "Current Config metadata", cmsConfig.GetIdentity().String())
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonNotRequired,
			Message:            fmt.Sprintf("Configuration already synced to version %d", cmsConfig.GetIdentity().GetVersion()),
		})
	} else {
		r.Log.Info("Sync configuration", "version", dynConfig.Metadata.Version)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonInProgress,
			Message:            fmt.Sprintf("Sync configuration with version %d", dynConfig.Metadata.Version),
		})
	}

	r.Log.Info("complete step setConfigPipelineStatus")
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}

func (r *Reconciler) setConfigurationSyncCompleted(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	dynConfig, _ := v1alpha1.ParseDynconfig(storage.Spec.Configuration)
	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               ConfigurationSyncedCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            fmt.Sprintf("Configuration with version %d synced successfully", dynConfig.Metadata.Version),
	})
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}
