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

func (r *Reconciler) replaceConfig(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step replaceConfig")

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

	condition := meta.FindStatusCondition(storage.Status.Conditions, ReplaceConfigOperationCondition)
	if condition == nil ||
		condition.ObservedGeneration < storage.Generation ||
		condition.Status == metav1.ConditionFalse {
		response, err := cms.ReplaceConfig(ctx, storage, creds, tlsOptions)
		if err != nil {
			r.Log.Error(err, "request CMS ReplaceConfig error")
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				string(StorageProvisioning),
				fmt.Sprintf("Failed to request CMS ReplaceConfig: %s", err),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}

		r.Log.Info("CMS ReplaceConfig", "response", response)
		operation := response.GetOperation()
		if operation.GetReady() {
			err := cms.CheckOperationSuccess(operation)
			if err != nil {
				r.Log.Error(err, "Failed status for CMS ReplaceConfig")
				meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
					Type:               ReplaceConfigOperationCondition,
					Status:             metav1.ConditionFalse,
					ObservedGeneration: storage.Generation,
					Reason:             ReasonFailed,
					Message:            fmt.Sprintf("Failed status for CMS ReplaceConfig: %s", err),
				})
				return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
			}

			r.Log.Info("complete step replaceConfig")
			return r.setConfigurationSyncCompleted(ctx, storage)
		}

		// poll response with ready == false
		operationId := operation.GetId()
		if len(operationId) == 0 {
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				string(StorageProvisioning),
				fmt.Sprintf("Failed to get operationId from response: %s", response),
			)
			return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigOperationCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             operationId,
			Message:            "Waiting for CMS ReplaceConfig operation is Ready",
		})
		return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
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

	r.Log.Info("complete step replaceConfig")
	return r.setConfigurationSyncCompleted(ctx, storage)
}

func (r *Reconciler) setConfigPipelineStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
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
			Message:            fmt.Sprintf("Configuration version %d already synced", dynConfig.Metadata.Version),
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

	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}

func (r *Reconciler) setConfigurationSyncCompleted(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	dynConfig, _ := v1alpha1.ParseDynconfig(storage.Spec.Configuration)
	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               ReplaceConfigOperationCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            fmt.Sprintf("CMS ReplaceConfig operation status is Success"),
	})
	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               ConfigurationSyncedCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            fmt.Sprintf("Configuration with version %d synced successfully", dynConfig.Metadata.Version),
	})
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}
