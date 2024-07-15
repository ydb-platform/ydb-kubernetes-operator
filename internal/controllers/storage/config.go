package storage

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/ydb-platform/ydb-go-sdk/v3"
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
	// condition := meta.FindStatusCondition(storage.Status.Conditions, ReplaceConfigDryRunOperationCondition)
	// if condition != nil && condition.ObservedGeneration == storage.Generation && condition.Status == metav1.ConditionUnknown {
	// 	return r.pollOperation(ctx, storage, condition)
	// }
	// if condition == nil || condition.ObservedGeneration < storage.Generation || condition.Status == metav1.ConditionFalse {
	// 	return r.requestOperation(ctx, storage, ReplaceConfigDryRunOperationCondition, true)
	// }

	config, err := v1alpha1.GetConfigForCMS(storage.Spec.Configuration)
	if err != nil {
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	cmsConfig := &cms.Config{
		StorageEndpoint:    storage.GetStorageEndpointWithProto(),
		Domain:             storage.Spec.Domain,
		Config:             string(config),
		DryRun:             false,
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
			fmt.Sprintf("Failed to get YDB credentials: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	ydbOpts := ydb.MergeOptions(ydb.WithCredentials(creds), tlsOptions)

	condition := meta.FindStatusCondition(storage.Status.Conditions, ReplaceConfigOperationCondition)
	if condition != nil && condition.ObservedGeneration == storage.Generation && condition.Status == metav1.ConditionUnknown {
		return r.checkReplaceConfigOperation(ctx, storage, cmsConfig, ydbOpts)
	}
	if condition == nil || condition.ObservedGeneration < storage.Generation || condition.Status == metav1.ConditionFalse {
		return r.replaceConfig(ctx, storage, cmsConfig, ydbOpts)
	}

	// meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
	// 	Type:               ConfigurationSyncedCondition,
	// 	Status:             metav1.ConditionTrue,
	// 	ObservedGeneration: storage.Generation,
	// 	Reason:             ReasonCompleted,
	// 	Message:            fmt.Sprintf("Configuration synced successfully to version %d", cmsConfig.Version),
	// })
	r.Log.Info("complete step handleConfigurationSync")
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}

func (r *Reconciler) replaceConfig(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	config *cms.Config,
	ydbOptions ydb.Option,
) (bool, ctrl.Result, error) {
	response, err := config.ReplaceConfig(ctx, ydbOptions)
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
	r.Log.Info("CMS ReplaceConfig", "response", response)

	finished, operationID, err := config.CheckReplaceConfigResponse(ctx, response)
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
			fmt.Sprintf("Tenant creation operation in progress, operationID: %s", operationID),
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

	// meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
	// 	Type:               operationType,
	// 	Status:             metav1.ConditionTrue,
	// 	ObservedGeneration: storage.Generation,
	// 	Reason:             ReasonCompleted,
	// 	Message:            "CMS ReplaceConfig operation is Success",
	// })
	r.Recorder.Event(
		storage,
		corev1.EventTypeNormal,
		string(DatabaseInitializing),
		fmt.Sprintf("Tenant %s created", tenant.Path),
	)
	return r.setReplaceConfigCompleted(ctx, storage, ReplaceConfigOperationCondition)
}

func (r *Reconciler) checkReplaceConfigOperation(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	config *cms.Config,
	ydbOptions ydb.Option,
) (bool, ctrl.Result, error) {
	condition := meta.FindStatusCondition(storage.Status.Conditions, ReplaceConfigOperationCondition)
	if len(condition.Message) == 0 {
		// Something is wrong with the condition where we save operation id
		// retry create tenant
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:   ReplaceConfigOperationCondition,
			Status: metav1.ConditionFalse,
			Reason: ReasonFailed,
		})
		return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
	}

	operation := &cms.Operation{
		StorageEndpoint: config.StorageEndpoint,
		Domain:          config.Domain,
		Id:              condition.Message,
	}
	response, err := operation.GetOperationResponse(ctx, ydbOptions)
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

	finished, operationID, err := operation.CheckOperationResponse(ctx, response)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"PreparingFailed",
			TODO,
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    ReplaceConfigOperationCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonFailed,
			Message: TODO,
		})
		return Stop, ctrl.Result{RequeueAfter: ReplaceConfigOperationRequeueDelay}, err
	}

	if !finished {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StoragePreparing),
			TODO,
		)
		return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
	}

	r.Recorder.Event(
		storage,
		corev1.EventTypeNormal,
		string(DatabaseInitializing),
		fmt.Sprintf("Tenant %s created", tenant.Path),
	)
	return r.setReplaceConfigCompleted(ctx, storage, message)
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

	config, err := v1alpha1.GetConfigForCMS(dynconfig)
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
		Config:             string(config),
		DryRun:             false,
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
			fmt.Sprintf("Failed to request GetConfig: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	err = cmsConfig.ProcessConfigResponse(response)
	if err != nil {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Failed to process config response: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	if dynconfig.Metadata.Version > cmsConfig.Version {
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Sync configuration in progress", "version", dynconfig.Metadata.Version),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonInProgress,
			Message:            fmt.Sprintf("Sync configuration to version %d in progress", dynconfig.Metadata.Version),
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	message := fmt.Sprintf("Configuration already synced to version %d", cmsConfig.Version)
	r.Recorder.Event(
		storage,
		corev1.EventTypeWarning,
		string(StorageProvisioning),
		message,
	)
	r.Log.Info("complete step setConfigPipelineStatus")
	return r.setReplaceConfigCompleted(ctx, storage, message)
}

func (r *Reconciler) setReplaceConfigCompleted(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
	message string,
) (bool, ctrl.Result, error) {
	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               ReplaceConfigOperationCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            "Replace config operation is completed",
	})
	meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               ReplaceConfigOperationCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: storage.Generation,
		Reason:             ReasonCompleted,
		Message:            message,
	})
	return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
}
