package monitoring

import (
	"context"

	monitoring "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	core "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/metrics"
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

func (r *DatabaseMonitoringReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cr := &api.DatabaseMonitoring{}

	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		if apierrs.IsNotFound(err) {
			logger.Info("DatabaseMonitoring has been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to get DatabaseMonitoring")
		return ctrl.Result{}, err
	}

	db, err := r.waitForDatabase(ctx, cr)

	if db == nil {
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	syncer := &Syncer{
		Client:          r.Client,
		Recorder:        r.Recorder,
		Scheme:          r.Scheme,
		metricsServices: metrics.GetDatabaseMetricsServices(),
		Object:          cr,
	}
	return syncer.Sync(ctx, db)
}

func (r *DatabaseMonitoringReconciler) waitForDatabase(ctx context.Context, cr *api.DatabaseMonitoring) (*api.Database, error) {
	ns := cr.Spec.DatabaseClusterRef.Namespace

	if ns == "" {
		ns = cr.GetNamespace()
	}

	nsName := types.NamespacedName{
		Name:      cr.Spec.DatabaseClusterRef.Name,
		Namespace: ns,
	}

	found := &api.Database{}

	if err := r.Get(ctx, nsName, found); err != nil {
		if apierrs.IsNotFound(err) {
			r.Recorder.Eventf(cr, core.EventTypeNormal, "Pending",
				"Unknown YDB Database cluster %s",
				nsName.String())
			return nil, nil
		}
		r.Recorder.Eventf(cr, core.EventTypeWarning, "Error",
			"Unable to find YDB Database %s: %s", nsName.String(), err.Error())
		return nil, err
	} else if found.Status.State != DatabaseReady {
		r.Recorder.Eventf(cr, core.EventTypeNormal, "Pending",
			"YDB Database %s state %s is not ready",
			nsName.String(), found.Status.State)
		return nil, nil
	}

	return found, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseMonitoringReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("DatabaseMonitoring")

	return ctrl.NewControllerManagedBy(mgr).
		For(&api.DatabaseMonitoring{}).
		Owns(&monitoring.ServiceMonitor{}).
		Complete(r)
}
