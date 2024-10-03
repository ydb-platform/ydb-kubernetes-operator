package storage

import (
	"context"
	"fmt"

	"github.com/ydb-platform/ydb-go-sdk/v3"
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
	cmsConfig *cms.Config,
	ydbOptions ydb.Option,
) (bool, ctrl.Result, error) {
	response, err := cmsConfig.ReplaceConfig(ctx, ydbOptions)
	if err != nil {
		r.Log.Error(err, "failed to request CMS ReplaceConfig")
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigOperationCondition,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonFailed,
			Message:            fmt.Sprintf("Failed to request CMS ReplaceConfig: %s", err),
		})
		return Stop, ctrl.Result{RequeueAfter: ReplaceConfigOperationRequeueDelay}, err
	}

	finished, operationID, err := cmsConfig.CheckReplaceConfigResponse(ctx, response)
	if err != nil {
		r.Log.Error(err, "failed response CMS ReplaceConfig")
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigOperationCondition,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonFailed,
			Message:            fmt.Sprintf("Failed response CMS ReplaceConfig: %s", err),
		})
		return r.updateStatus(ctx, storage, DefaultRequeueDelay)
	}

	if !finished {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StoragePreparing),
			fmt.Sprintf("CMS ReplaceConfig operation in progress, operationID: %s", operationID),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigOperationCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonInProgress,
			Message:            operationID,
		})
		return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
	}

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               ReplaceConfigOperationCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            fmt.Sprintf("Config replaced to version: %d", cmsConfig.Version),
	})
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}

func (r *Reconciler) checkReplaceConfigOperation(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	cmsConfig *cms.Config,
	ydbOptions ydb.Option,
) (bool, ctrl.Result, error) {
	condition := meta.FindStatusCondition(storage.Status.Conditions, ReplaceConfigOperationCondition)
	if len(condition.Message) == 0 {
		// retry replace config
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    ReplaceConfigOperationCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonFailed,
			Message: "Something is wrong with the condition",
		})
		return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
	}

	operation := &cms.Operation{
		StorageEndpoint: cmsConfig.StorageEndpoint,
		Domain:          cmsConfig.Domain,
		ID:              condition.Message,
	}
	response, err := operation.GetOperation(ctx, ydbOptions)
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

	finished, operationID, err := operation.CheckGetOperationResponse(ctx, response)
	if err != nil {
		errMessage := fmt.Sprintf("Error replacing config to version %d: %s", cmsConfig.Version, err)
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"PreparingFailed",
			errMessage,
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    ReplaceConfigOperationCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonFailed,
			Message: errMessage,
		})
		return Stop, ctrl.Result{RequeueAfter: ReplaceConfigOperationRequeueDelay}, err
	}

	if !finished {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StoragePreparing),
			fmt.Sprintf("Config replacing operation is not completed, operationID: %s", operationID),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigOperationCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonInProgress,
			Message:            operationID,
		})
		return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
	}

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               ReplaceConfigOperationCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            fmt.Sprintf("Config replaced to version: %d", cmsConfig.Version),
	})
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}

func (r *Reconciler) setConfigPipelineStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setConfigPipelineStatus")
	isDynConfig, dynConfig, _ := v1alpha1.ParseDynConfig(storage.Spec.Configuration)
	if !isDynConfig {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonNotRequired,
			Message:            "Sync static configuration does not supported",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	yamlConfig, err := v1alpha1.GetConfigForCMS(dynConfig)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get configuration for CMS: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	cmsConfig := &cms.Config{
		StorageEndpoint:    storage.GetStorageEndpointWithProto(),
		Domain:             storage.Spec.Domain,
		Config:             string(yamlConfig),
		DryRun:             true,
		AllowUnknownFields: true,
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
	ydbOpts := ydb.MergeOptions(ydb.WithCredentials(creds), tlsOptions)

	response, err := cmsConfig.GetConfig(ctx, ydbOpts)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Failed to request CMS GetConfig: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	err = cmsConfig.ProcessConfigResponse(ctx, response)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Failed to process config response: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	if cmsConfig.Version > dynConfig.Metadata.Version {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonNotRequired,
			Message:            fmt.Sprintf("Configuration already synced to version %d", cmsConfig.Version),
		})
		r.Log.Info("complete step setConfigPipelineStatus")
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               ConfigurationSyncedCondition,
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonInProgress,
		Message:            fmt.Sprintf("Sync configuration in progress to version %d", dynConfig.Metadata.Version),
	})
	r.Log.Info("complete step setConfigPipelineStatus")
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}

func (r *Reconciler) handleConfigurationSync(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleConfigurationSync")

	_, dynConfig, err := v1alpha1.ParseDynConfig(storage.Spec.Configuration)
	if err != nil {
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	yamlConfig, err := v1alpha1.GetConfigForCMS(dynConfig)
	if err != nil {
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
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
			fmt.Sprintf("Failed to get YDB credentials: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	ydbOpts := ydb.MergeOptions(ydb.WithCredentials(creds), tlsOptions)

	cmsConfig := &cms.Config{
		StorageEndpoint:    storage.GetStorageEndpointWithProto(),
		Domain:             storage.Spec.Domain,
		Config:             string(yamlConfig),
		DryRun:             false,
		AllowUnknownFields: true,
	}

	condition := meta.FindStatusCondition(storage.Status.Conditions, ReplaceConfigOperationCondition)
	if condition != nil && condition.ObservedGeneration == storage.Generation && condition.Status == metav1.ConditionUnknown {
		return r.checkReplaceConfigOperation(ctx, storage, cmsConfig, ydbOpts)
	}

	if condition == nil || condition.ObservedGeneration < storage.Generation || condition.Status == metav1.ConditionFalse {
		return r.replaceConfig(ctx, storage, cmsConfig, ydbOpts)
	}

	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               ConfigurationSyncedCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            fmt.Sprintf("Configuration synced successfully to version %d", cmsConfig.Version),
	})
	r.Log.Info("complete step handleConfigurationSync")
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}
