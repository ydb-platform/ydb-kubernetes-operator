package storage

import (
	"context"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
)

// Reconciler reconciles a Storage object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
	Log      logr.Logger

	WithServiceMonitors bool
}

//+kubebuilder:rbac:groups=ydb.tech,resources=storages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=storages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=storages/finalizers,verbs=update
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=get;list;watch
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)

	storage := &ydbv1alpha1.Storage{}
	err := r.Get(ctx, req.NamespacedName, storage)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("storage resources not found")
			return ctrl.Result{Requeue: false}, nil
		}
		r.Log.Error(err, "unexpected Get error")
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	result, err := r.Sync(ctx, storage)
	if err != nil {
		r.Log.Error(err, "unexpected Sync error")
	}
	return result, err
}

func ignoreDeletionPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			generationChanged := e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			annotationsChanged := !annotations.CompareYdbTechAnnotations(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())
			_, isService := e.ObjectOld.(*corev1.Service)

			return generationChanged || annotationsChanged || isService
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Evaluates to false if the object has been confirmed deleted.
			return !e.DeleteStateUnknown
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	controller := ctrl.NewControllerManagedBy(mgr).For(&ydbv1alpha1.Storage{})

	r.Recorder = mgr.GetEventRecorderFor("Storage")

	if r.WithServiceMonitors {
		controller = controller.
			Owns(&monitoringv1.ServiceMonitor{})
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&ydbv1alpha1.RemoteStorageNodeSet{},
		OwnerControllerKey,
		func(obj client.Object) []string {
			// grab the RemoteStorageNodeSet object, extract the owner...
			remoteStorageNodeSet := obj.(*ydbv1alpha1.RemoteStorageNodeSet)
			owner := metav1.GetControllerOf(remoteStorageNodeSet)
			if owner == nil {
				return nil
			}
			// ...make sure it's a Storage...
			if owner.APIVersion != ydbv1alpha1.GroupVersion.String() || owner.Kind != "Storage" {
				return nil
			}

			// ...and if so, return it
			return []string{owner.Name}
		}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&ydbv1alpha1.StorageNodeSet{},
		OwnerControllerKey,
		func(obj client.Object) []string {
			// grab the StorageNodeSet object, extract the owner...
			storageNodeSet := obj.(*ydbv1alpha1.StorageNodeSet)
			owner := metav1.GetControllerOf(storageNodeSet)
			if owner == nil {
				return nil
			}
			// ...make sure it's a Storage...
			if owner.APIVersion != ydbv1alpha1.GroupVersion.String() || owner.Kind != "Storage" {
				return nil
			}

			// ...and if so, return it
			return []string{owner.Name}
		}); err != nil {
		return err
	}

	return controller.
		Owns(&ydbv1alpha1.StorageNodeSet{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		WithEventFilter(ignoreDeletionPredicate()).
		Complete(r)
}
