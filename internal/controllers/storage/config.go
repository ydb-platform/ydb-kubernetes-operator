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

func (r *Reconciler) getConfig(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running handler getConfig")
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

	condition := meta.FindStatusCondition(storage.Status.Conditions, GetConfigOperationCondition)
	if condition == nil ||
		condition.ObservedGeneration < storage.Generation ||
		condition.Status == metav1.ConditionFalse {
		getConfig, err := cms.GetConfig(ctx, storage, creds, tlsOptions)
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
		r.Log.Info("CMS GetConfig response", getConfig)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               GetConfigOperationCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             getConfig.GetOperation().GetId(),
			Message:            fmt.Sprintf("Waiting for request CMS GetConfig: %s", err),
		})
		return r.updateStatus(ctx, storage, GetConfigOperationRequeueDelay)
	}

	operationId := condition.Reason
	cmsResponse, err := cms.GetOperation(ctx, storage, operationId, creds, tlsOptions)
	if err != nil {
		r.Log.Error(err, "request CMS GetOperation error")
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Failed to request CMS GetOperation: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	err = cms.CheckOperationReady(cmsResponse)
	if err != nil {
		r.Log.Info("Waiting for request CMS GetConfig", "status", err)
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			string(StorageProvisioning),
			fmt.Sprintf("Waiting for request CMS GetConfig: %s", err),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               GetConfigOperationCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             operationId,
			Message:            fmt.Sprintf("Waiting for request CMS GetConfig: %s", err),
		})
		return r.updateStatus(ctx, storage, GetConfigOperationRequeueDelay)
	}

	err = cms.CheckOperationSuccess(cmsResponse)
	if err != nil {
		r.Log.Error(err, "Failed status for CMS GetConfig")
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Failed status for CMS GetConfig: %s", err),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               GetConfigOperationCondition,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: storage.Generation,
			Reason:             operationId,
			Message:            fmt.Sprintf("Failed status for CMS GetConfig: %s", err),
		})
		return r.updateStatus(ctx, storage, GetConfigOperationRequeueDelay)
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, GetConfigOperationCondition) {
		r.Log.Info("Success status for CMS GetConfig operation", "operationId", operationId)
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			string(StorageProvisioning),
			fmt.Sprintf("Success CMS GetConfig operation %s", operationId),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               GetConfigOperationCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             operationId,
			Message:            fmt.Sprintf("Success CMS GetConfig operation %s", operationId),
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete handler getConfig")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) checkConfig(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step checkConfig")

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

	condition := meta.FindStatusCondition(storage.Status.Conditions, GetConfigOperationCondition)
	if condition == nil {
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	operationId := condition.Reason
	cmsResponse, err := cms.GetOperation(ctx, storage, operationId, creds, tlsOptions)
	if err != nil {
		r.Log.Error(err, "request CMS GetOperation error")
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Failed to request CMS GetOperation: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	cmsConfig, err := cms.GetConfigResult(cmsResponse)
	if err != nil {
		r.Log.Error(err, "Failed to unmarshal OperationResult to ConfigResult", "operationId", operationId)
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Failed to unmarshal OperationResult %s to ConfigResult: %s", operationId, err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	dynConfig, _ := v1alpha1.TryParseDynconfig(storage.Spec.Configuration)
	cmsConfigIdentity := cmsConfig.GetIdentity()
	if dynConfig.Metadata.Cluster != cmsConfigIdentity.Cluster {
		r.Log.Error(err, "Configuration metadata.cluster does not equal with current CMS value", "Storage Config metadata", dynConfig.Metadata, "CMS Config metadata", cmsConfigIdentity)
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Configuration metadata.cluster %s does not equal with current CMS value: %s", dynConfig.Metadata.Cluster, cmsConfigIdentity.Cluster),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    ConfigurationSyncedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNotRequired,
			Message: fmt.Sprintf("Configuration metadata.cluster %s does not equal with current CMS value: %s", dynConfig.Metadata.Cluster, cmsConfigIdentity.Cluster),
		})
		return r.updateStatus(ctx, storage, 0)
	}

	if dynConfig.Metadata.Version < cmsConfigIdentity.Version {
		r.Log.Error(err, "Configuration metadata.version less than current CMS value", "Storage Config metadata", dynConfig.Metadata, "CMS Config metadata", cmsConfigIdentity.String())
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			string(StorageProvisioning),
			fmt.Sprintf("Configuration metadata.version %s less than current CMS value: %s", dynConfig.Metadata.Cluster, cmsConfigIdentity.Cluster),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:    ConfigurationSyncedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonNotRequired,
			Message: fmt.Sprintf("Configuration metadata.version %s less than current CMS value: %s", dynConfig.Metadata.Cluster, cmsConfigIdentity.Cluster),
		})
		return r.updateStatus(ctx, storage, 0)
	}

	r.Log.Info("complete step checkConfig")
	return Continue, ctrl.Result{}, nil
}

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
		replaceConfig, err := cms.ReplaceConfig(ctx, storage, creds, tlsOptions)
		if err != nil {
			r.Log.Error(err, "request CMS ReplaceConfig error")
			r.Recorder.Event(
				storage,
				corev1.EventTypeWarning,
				string(StorageProvisioning),
				fmt.Sprintf("Failed to request CMS ReplaceConfig: %s", err),
			)
			return Continue, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		r.Log.Info("CMS ReplaceConfig response", replaceConfig)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigOperationCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             replaceConfig.GetOperation().GetId(),
			Message:            fmt.Sprintf("Waiting for request CMS ReplaceConfig: %s", err),
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

	err = cms.CheckOperationReady(response)
	if err != nil {
		r.Log.Info("Waiting for request CMS ReplaceConfig", "message", err)
		r.Recorder.Event(
			storage,
			corev1.EventTypeNormal,
			string(StorageProvisioning),
			fmt.Sprintf("Waiting for request CMS ReplaceConfig: %s", err),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigOperationCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             operationId,
			Message:            fmt.Sprintf("Waiting for request CMS ReplaceConfig: %s", err),
		})
		return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
	}

	err = cms.CheckOperationSuccess(response)
	if err != nil {
		r.Log.Error(err, "Failed status for CMS ReplaceConfig")
		r.Recorder.Event(
			storage,
			corev1.EventTypeWarning,
			"ReplaceConfig",
			fmt.Sprintf("Failed status for CMS ReplaceConfig: %s", err),
		)
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigOperationCondition,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: storage.Generation,
			Reason:             operationId,
			Message:            fmt.Sprintf("Failed status for CMS ReplaceConfig: %s", err),
		})
		return r.updateStatus(ctx, storage, ReplaceConfigOperationRequeueDelay)
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, ReplaceConfigOperationCondition) {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ReplaceConfigOperationCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             operationId,
			Message:            fmt.Sprintf("Success CMS ReplaceConfig operation %s", operationId),
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	if !meta.IsStatusConditionTrue(storage.Status.Conditions, ConfigurationSyncedCondition) {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonCompleted,
			Message:            "Configuration synced with CMS successfully",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	r.Log.Info("complete step replaceConfig")
	return Continue, ctrl.Result{}, nil
}

func (r *Reconciler) setConfigPipelineStatus(
	ctx context.Context,
	storage *resources.StorageClusterBuilder,
) (bool, ctrl.Result, error) {

	dynConfig, err := v1alpha1.TryParseDynconfig(storage.Spec.Configuration)
	if err != nil {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonCompleted,
			Message:            "Sync static configuration does not support, skip...",
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	configSyncCondition := meta.FindStatusCondition(storage.Status.Conditions, ConfigurationSyncedCondition)
	if configSyncCondition == nil || configSyncCondition.ObservedGeneration < storage.Generation {
		meta.SetStatusCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               ConfigurationSyncedCondition,
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: storage.Generation,
			Reason:             ReasonInProgress,
			Message:            fmt.Sprintf("Sync configuration version %d with CMS", dynConfig.Metadata.Version),
		})
		return r.updateStatus(ctx, storage, StatusUpdateRequeueDelay)
	}

	return Continue, ctrl.Result{}, nil
}
