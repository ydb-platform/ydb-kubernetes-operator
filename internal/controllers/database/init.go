package database

import (
	"context"
	"fmt"

	ydb "github.com/ydb-platform/ydb-go-sdk/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/cms"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

func (r *Reconciler) setInitPipelineStatus(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	if database.Status.State == DatabasePreparing {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    DatabaseInitializedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonInProgress,
			Message: "Database has not been initialized yet",
		})
		database.Status.State = DatabaseInitializing
		return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
	}

	// This block is special internal logic that skips all Database initialization.
	if value, ok := database.Annotations[v1alpha1.AnnotationSkipInitialization]; ok && value == v1alpha1.AnnotationValueTrue {
		r.Log.Info("Database initialization disabled (with annotation), proceed with caution")
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"SkippingInit",
			"Skipping initialization due to skip annotation present, be careful!",
		)
		return r.setInitDatabaseCompleted(ctx, database, "Database initialization not performed because initialization is skipped")
	}

	if meta.IsStatusConditionTrue(database.Status.Conditions, OldDatabaseInitializedCondition) {
		return r.setInitDatabaseCompleted(ctx, database, "Database initialized successfully")
	}

	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) setInitDatabaseCompleted(
	ctx context.Context,
	database *resources.DatabaseBuilder,
	message string,
) (bool, ctrl.Result, error) {
	meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
		Type:    DatabaseInitializedCondition,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonCompleted,
		Message: message,
	})
	meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
		Type:    CreateDatabaseOperationCondition,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonCompleted,
		Message: "Tenant creation operation is completed",
	})
	return r.updateStatus(ctx, database, StatusUpdateRequeueDelay)
}

func (r *Reconciler) checkCreateDatabaseOperation(
	ctx context.Context,
	database *resources.DatabaseBuilder,
	tenant *cms.Tenant,
	ydbOptions ydb.Option,
) (bool, ctrl.Result, error) {
	condition := meta.FindStatusCondition(database.Status.Conditions, CreateDatabaseOperationCondition)
	if len(condition.Message) == 0 {
		// Something is wrong with the condition where we save operation id
		// retry create tenant
		errMessage := fmt.Sprintf("Something is wrong with the condition, retry creating tenant %s", tenant.Path)
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"InitializingFailed",
			errMessage,
		)
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    CreateDatabaseOperationCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonFailed,
			Message: errMessage,
		})
		return r.updateStatus(ctx, database, DatabaseInitializationRequeueDelay)
	}

	operation := &cms.Operation{
		StorageEndpoint: tenant.StorageEndpoint,
		Domain:          tenant.Domain,
		ID:              condition.Message,
	}
	response, err := operation.GetOperation(ctx, ydbOptions)
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"InitializingFailed",
			fmt.Sprintf("Failed to check creation operation, operationID %s: %s", operation.ID, err),
		)
		return Stop, ctrl.Result{RequeueAfter: DatabaseInitializationRequeueDelay}, err
	}

	finished, operationID, err := operation.CheckGetOperationResponse(ctx, response)
	if err != nil {
		errMessage := fmt.Sprintf("Error creating tenant %s: %s", tenant.Path, err)
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"InitializingFailed",
			errMessage,
		)
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    CreateDatabaseOperationCondition,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonCompleted,
			Message: errMessage,
		})
		return r.updateStatus(ctx, database, DatabaseInitializationRequeueDelay)
	}

	if !finished {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			string(DatabaseInitializing),
			fmt.Sprintf("Tenant creation operation is not completed, operationID: %s", operationID),
		)
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    CreateDatabaseOperationCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonInProgress,
			Message: operationID,
		})
		return r.updateStatus(ctx, database, DatabaseInitializationRequeueDelay)
	}

	r.Recorder.Event(
		database,
		corev1.EventTypeNormal,
		string(DatabaseInitializing),
		fmt.Sprintf("Tenant %s created", tenant.Path),
	)
	return r.setInitDatabaseCompleted(ctx, database, "Database initialized successfully")
}

func (r *Reconciler) initializeTenant(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	path := database.GetDatabasePath()
	var storageUnits []v1alpha1.StorageUnit
	var shared bool
	var sharedDatabasePath string
	switch {
	case database.Spec.Resources != nil:
		storageUnits = database.Spec.Resources.StorageUnits
		shared = false
	case database.Spec.SharedResources != nil:
		storageUnits = database.Spec.SharedResources.StorageUnits
		shared = true
	case database.Spec.ServerlessResources != nil:
		sharedDatabaseCr := &v1alpha1.Database{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      database.Spec.ServerlessResources.SharedDatabaseRef.Name,
			Namespace: database.Spec.ServerlessResources.SharedDatabaseRef.Namespace,
		}, sharedDatabaseCr)
		if err != nil {
			if apierrors.IsNotFound(err) {
				r.Recorder.Event(
					database,
					corev1.EventTypeWarning,
					"Pending",
					fmt.Sprintf(
						"Database (%s/%s) not found.",
						database.Spec.ServerlessResources.SharedDatabaseRef.Name,
						database.Spec.ServerlessResources.SharedDatabaseRef.Namespace,
					),
				)
				return Stop, ctrl.Result{RequeueAfter: SharedDatabaseAwaitRequeueDelay}, nil
			}
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"Pending",
				fmt.Sprintf(
					"Failed to get Database (%s, %s) resource, error: %s",
					database.Spec.ServerlessResources.SharedDatabaseRef.Name,
					database.Spec.ServerlessResources.SharedDatabaseRef.Namespace,
					err,
				),
			)
			return Stop, ctrl.Result{RequeueAfter: SharedDatabaseAwaitRequeueDelay}, err
		}

		if !meta.IsStatusConditionTrue(sharedDatabaseCr.Status.Conditions, DatabaseProvisionedCondition) {
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"Pending",
				fmt.Sprintf(
					"Referenced shared Database (%s, %s) is not Provisioned",
					database.Spec.ServerlessResources.SharedDatabaseRef.Name,
					database.Spec.ServerlessResources.SharedDatabaseRef.Namespace,
				),
			)
			return Stop, ctrl.Result{RequeueAfter: SharedDatabaseAwaitRequeueDelay}, err
		}
		sharedDatabasePath = sharedDatabaseCr.GetDatabasePath()
	default:
		// TODO: move this logic to webhook
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"ControllerError",
			ErrIncorrectDatabaseResourcesConfiguration.Error(),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, ErrIncorrectDatabaseResourcesConfiguration
	}

	tenant := &cms.Tenant{
		StorageEndpoint:    database.Spec.StorageEndpoint,
		Domain:             database.Spec.Domain,
		Path:               path,
		StorageUnits:       storageUnits,
		Shared:             shared,
		SharedDatabasePath: sharedDatabasePath,
	}

	creds, err := resources.GetYDBCredentials(ctx, database.Storage, r.Config)
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get YDB credentials: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	tlsOptions, err := resources.GetYDBTLSOption(ctx, database.Storage, r.Config)
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"ControllerError",
			fmt.Sprintf("Failed to get YDB TLS options: %s", err),
		)
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	ydbOpts := ydb.MergeOptions(ydb.WithCredentials(creds), tlsOptions)

	if meta.IsStatusConditionPresentAndEqual(database.Status.Conditions, CreateDatabaseOperationCondition, metav1.ConditionUnknown) {
		return r.checkCreateDatabaseOperation(ctx, database, tenant, ydbOpts)
	}

	response, err := tenant.CreateDatabase(ctx, ydbOpts)
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"InitializingFailed",
			fmt.Sprintf("Error creating tenant %s: %s", tenant.Path, err),
		)
		return Stop, ctrl.Result{RequeueAfter: DatabaseInitializationRequeueDelay}, err
	}

	finished, operationID, err := tenant.CheckCreateDatabaseResponse(ctx, response)
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"InitializingFailed",
			fmt.Sprintf("Error checking operation for tenant %s: %s", tenant.Path, err),
		)
		return Stop, ctrl.Result{RequeueAfter: DatabaseInitializationRequeueDelay}, err
	}

	if !finished {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			string(DatabaseInitializing),
			fmt.Sprintf("Tenant creation operation in progress, operationID: %s", operationID),
		)
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    CreateDatabaseOperationCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  ReasonInProgress,
			Message: operationID,
		})
		return r.updateStatus(ctx, database, DatabaseInitializationRequeueDelay)
	}
	r.Recorder.Event(
		database,
		corev1.EventTypeNormal,
		"Initialized",
		fmt.Sprintf("Tenant %s created", tenant.Path),
	)

	return r.setInitDatabaseCompleted(ctx, database, "Database initialized successfully")
}
