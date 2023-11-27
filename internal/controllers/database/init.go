package database

import (
	"context"
	"fmt"

	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/cms"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) processSkipInitPipeline(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step processSkipInitPipeline")
	r.Log.Info("Database initialization disabled (with annotation), proceed with caution")

	r.Recorder.Event(
		database,
		corev1.EventTypeWarning,
		"SkippingInit",
		"Skipping database creation due to skip annotation present, be careful!",
	)

	return r.setInitDatabaseCompleted(
		ctx,
		database,
		"Database creation not performed because initialization is skipped",
	)
}

func (r *Reconciler) setInitialStatus(
	ctx context.Context,
	database *resources.DatabaseBuilder,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step setInitialStatus")

	// This block is special internal logic that skips all Datbase initialization.
	// It is needed when large clusters are migrated where `waitForStatefulSetToScale`
	// does not make sense, since some nodes can be down for a long time (and it is okay, since
	// database is healthy even with partial outage).
	if value, ok := database.Annotations[v1alpha1.AnnotationSkipInitialization]; ok && value == v1alpha1.AnnotationValueTrue {
		if meta.FindStatusCondition(database.Status.Conditions, TenantInitializedCondition) == nil ||
			meta.IsStatusConditionFalse(database.Status.Conditions, TenantInitializedCondition) {
			return r.processSkipInitPipeline(ctx, database)
		}
		return Stop, ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	changed := false
	if meta.FindStatusCondition(database.Status.Conditions, TenantInitializedCondition) == nil {
		meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
			Type:    TenantInitializedCondition,
			Status:  "False",
			Reason:  TenantInitializedReasonInProgress,
			Message: "Tenant creation in progress",
		})
		changed = true
	}
	if database.Status.State == string(Pending) {
		database.Status.State = string(Preparing)
		changed = true
	}
	if changed {
		return r.setState(ctx, database)
	}
	return Continue, ctrl.Result{Requeue: false}, nil
}

func (r *Reconciler) setInitDatabaseCompleted(
	ctx context.Context,
	database *resources.DatabaseBuilder,
	message string,
) (bool, ctrl.Result, error) {
	meta.SetStatusCondition(&database.Status.Conditions, metav1.Condition{
		Type:    TenantInitializedCondition,
		Status:  "True",
		Reason:  TenantInitializedReasonCompleted,
		Message: message,
	})

	database.Status.State = string(Ready)
	return r.setState(ctx, database)
}

func (r *Reconciler) handleTenantCreation(
	ctx context.Context,
	database *resources.DatabaseBuilder,
	creds ydbCredentials.Credentials,
) (bool, ctrl.Result, error) {
	r.Log.Info("running step handleTenantCreation")

	path := v1alpha1.GetDatabasePath(database.Database)
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

		if sharedDatabaseCr.Status.State != "Ready" {
			r.Recorder.Event(
				database,
				corev1.EventTypeWarning,
				"Pending",
				fmt.Sprintf(
					"Referenced shared Database (%s, %s) in a bad state: %s != Ready",
					database.Spec.ServerlessResources.SharedDatabaseRef.Name,
					database.Spec.ServerlessResources.SharedDatabaseRef.Namespace,
					sharedDatabaseCr.Status.State,
				),
			)
			return Stop, ctrl.Result{RequeueAfter: SharedDatabaseAwaitRequeueDelay}, err
		}
		sharedDatabasePath = v1alpha1.GetDatabasePath(sharedDatabaseCr)
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

	tenant := cms.Tenant{
		StorageEndpoint:    database.GetStorageEndpointWithProto(),
		Path:               path,
		StorageUnits:       storageUnits,
		Shared:             shared,
		SharedDatabasePath: sharedDatabasePath,
	}

	err := tenant.Create(ctx, database, creds)
	if err != nil {
		r.Recorder.Event(
			database,
			corev1.EventTypeWarning,
			"InitializingFailed",
			fmt.Sprintf("Error creating tenant %s: %s", tenant.Path, err),
		)
		return Stop, ctrl.Result{RequeueAfter: TenantCreationRequeueDelay}, err
	}
	r.Recorder.Event(
		database,
		corev1.EventTypeNormal,
		"Initialized",
		fmt.Sprintf("Tenant %s created", tenant.Path),
	)

	r.Recorder.Event(
		database,
		corev1.EventTypeNormal,
		"DatabaseReady",
		"Database is initialized",
	)

	return r.setInitDatabaseCompleted(
		ctx,
		database,
		"Database initialization is completed",
	)
}
