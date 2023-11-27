package nodeset

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

// Reconciler reconciles a Storage object
type DatabaseNodeSetReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Config   *rest.Config
	Scheme   *runtime.Scheme
	Log      logr.Logger
}

//+kubebuilder:rbac:groups=ydb.tech,resources=databases,verbs=get;list;watch
//+kubebuilder:rbac:groups=ydb.tech,resources=databasesnodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=databasesnodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=databasesnodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseNodeSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	crDatabaseNodeSet := &api.DatabaseNodeSet{}
	err := r.Get(ctx, req.NamespacedName, crDatabaseNodeSet)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("StorageNodeSet has been deleted")
			return ctrl.Result{Requeue: false}, nil
		}
		logger.Error(err, "unable to get StorageNodeSet")
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	databaseNamespacedName := types.NamespacedName{
		Name:      crDatabaseNodeSet.Spec.DatabaseRef,
		Namespace: crDatabaseNodeSet.GetNamespace(),
	}

	crDatabase := &api.Database{}
	if err := r.Get(ctx, databaseNamespacedName, crDatabase); err != nil {
		if errors.IsNotFound(err) {
			r.Recorder.Eventf(crDatabaseNodeSet, corev1.EventTypeNormal, "Pending",
				"Unknown YDB Storage %s",
				databaseNamespacedName.String())
			return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		r.Recorder.Eventf(crDatabaseNodeSet, corev1.EventTypeWarning, "Error",
			"Unable to find YDB Storage %s: %s", databaseNamespacedName.String(), err.Error())
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	result, err := r.Sync(ctx, crDatabaseNodeSet, crDatabase)
	if err != nil {
		r.Log.Error(err, "unexpected Sync error")
	}

	return result, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseNodeSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("DatabaseNodeSet")

	return ctrl.NewControllerManagedBy(mgr).
		For(&api.DatabaseNodeSet{}).
		Owns(&appsv1.StatefulSet{}).
		WithEventFilter(ignoreDeletionPredicate()).
		Complete(r)
}
