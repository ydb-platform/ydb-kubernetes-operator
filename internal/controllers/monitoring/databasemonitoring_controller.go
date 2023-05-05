package monitoring

import (
	"context"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

// DatabaseMonitoringReconciler reconciles a DatabaseMonitoring object
type DatabaseMonitoringReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
}

//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=databasemonitorings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=databasemonitorings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=databasemonitorings/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseMonitoringReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var dbMon ydbv1alpha1.DatabaseMonitoring

	if err := r.Get(ctx, req.NamespacedName, &dbMon); err != nil {
		if apierrs.IsNotFound(err) {
			logger.Info("DatabaseMonitoring has been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to get DatabaseMonitoring")
		return ctrl.Result{}, err
	}

	result, err := r.Sync(ctx, &dbMon)
	if err != nil {
		logger.Error(err, "unexpected Sync error")
	}
	return result, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseMonitoringReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("DatabaseMonitoring")

	return ctrl.NewControllerManagedBy(mgr).
		For(&ydbv1alpha1.DatabaseMonitoring{}).
		Owns(&monitoringv1.ServiceMonitor{}).
		Complete(r)
}
