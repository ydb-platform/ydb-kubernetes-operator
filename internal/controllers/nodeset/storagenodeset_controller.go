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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
)

// Reconciler reconciles a Storage object
type StorageNodeSetReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Config   *rest.Config
	Scheme   *runtime.Scheme
	Log      logr.Logger
}

//+kubebuilder:rbac:groups=ydb.tech,resources=storages,verbs=get;list;watch
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *StorageNodeSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	crStorageNodeSet := &api.StorageNodeSet{}
	err := r.Get(ctx, req.NamespacedName, crStorageNodeSet)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("StorageNodeSet has been deleted")
			return ctrl.Result{Requeue: false}, nil
		}
		logger.Error(err, "unable to get StorageNodeSet")
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	storageNamespacedName := types.NamespacedName{
		Name:      crStorageNodeSet.Spec.StorageRef,
		Namespace: crStorageNodeSet.GetNamespace(),
	}

	crStorage := &api.Storage{}
	if err := r.Get(ctx, storageNamespacedName, crStorage); err != nil {
		if errors.IsNotFound(err) {
			r.Recorder.Eventf(crStorageNodeSet, corev1.EventTypeNormal, "Pending",
				"Unknown YDB Storage %s",
				storageNamespacedName.String())
			return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
		}
		r.Recorder.Eventf(crStorageNodeSet, corev1.EventTypeWarning, "Error",
			"Unable to find YDB Storage %s: %s", storageNamespacedName.String(), err.Error())
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	result, err := r.Sync(ctx, crStorageNodeSet, crStorage)
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

			return generationChanged || annotationsChanged
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Evaluates to false if the object has been confirmed deleted.
			return !e.DeleteStateUnknown
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *StorageNodeSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("StorageNodeSet")

	return ctrl.NewControllerManagedBy(mgr).
		For(&api.StorageNodeSet{}).
		Owns(&appsv1.StatefulSet{}).
		WithEventFilter(ignoreDeletionPredicate()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}
