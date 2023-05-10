package monitoring

import (
	"context"
	storagectrl "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/storage"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/metrics"
	core "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

// StorageMonitoringReconciler reconciles a StorageMonitoring object
type StorageMonitoringReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
}

//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.ydb.tech,resources=storagemonitorings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.ydb.tech,resources=storagemonitorings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.ydb.tech,resources=storagemonitorings/finalizers,verbs=update

func (r *StorageMonitoringReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	cr := &api.StorageMonitoring{}

	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		if apierrs.IsNotFound(err) {
			logger.Info("StorageMonitoring has been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to get StorageMonitoring")
		return ctrl.Result{}, err
	}

	storage, err := r.waitForStorage(ctx, cr)

	if storage == nil {
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	syncer := &Syncer{
		Client:          r.Client,
		Recorder:        r.Recorder,
		Scheme:          r.Scheme,
		metricsServices: metrics.GetStorageMetricsServices(),

		Object: cr,
	}
	return syncer.Sync(ctx, storage)

}

func (r *StorageMonitoringReconciler) waitForStorage(ctx context.Context, cr *api.StorageMonitoring) (*api.Storage, error) {
	ns := cr.Spec.StorageRef.Namespace

	if ns == "" {
		ns = cr.GetNamespace()
	}

	nsName := types.NamespacedName{
		Name:      cr.Spec.StorageRef.Name,
		Namespace: ns,
	}

	found := &api.Storage{}

	if err := r.Get(ctx, nsName, found); err != nil {
		if apierrs.IsNotFound(err) {
			r.Recorder.Eventf(cr, core.EventTypeNormal, "Pending",
				"Unknown YDB Storage %s",
				nsName.String())
			return nil, nil
		}
		r.Recorder.Eventf(cr, core.EventTypeWarning, "Error",
			"Unable to find YDB Storage %s: %s", nsName.String(), err.Error())
		return nil, err
	} else {
		if found.Status.State != string(storagectrl.Ready) {
			r.Recorder.Eventf(cr, core.EventTypeNormal, "Pending",
				"YDB Storage %s state %s is not ready",
				nsName.String(), found.Status.State)
			return nil, nil

		}
	}
	return found, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StorageMonitoringReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("DatabaseMonitoring")

	return ctrl.NewControllerManagedBy(mgr).
		For(&api.StorageMonitoring{}).
		Complete(r)
}
