package storage

import (
	"context"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

// StorageReconciler reconciles a Storage object
type StorageReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
	Log      logr.Logger
}

//+kubebuilder:rbac:groups=ydb.tech,resources=storages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=storages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=storages/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=get;list;watch
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *StorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)

	storage := &ydbv1alpha1.Storage{}
	err := r.Get(ctx, req.NamespacedName, storage)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("storage resources not found")
			return controllers.Ok()
		} else {
			r.Log.Error(err, "unexpected error")
		}
		return controllers.NoRequeue(err)
	}

	return r.Sync(ctx, storage)
}

func ignoreDeletionPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates to CR status in which case metadata.Generation does not change
			_, isService := e.ObjectOld.(*corev1.Service)

			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() || isService
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Evaluates to false if the object has been confirmed deleted.
			return !e.DeleteStateUnknown
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *StorageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ydbv1alpha1.Storage{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&monitoringv1.ServiceMonitor{}).
		WithEventFilter(ignoreDeletionPredicate()).
		Complete(r)
}
