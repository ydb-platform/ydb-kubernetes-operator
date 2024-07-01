package storagenodeset

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

// Reconciler reconciles a Storage object
type Reconciler struct {
	client.Client
	Recorder record.EventRecorder
	Config   *rest.Config
	Scheme   *runtime.Scheme
	Log      logr.Logger
}

//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)

	crStorageNodeSet := &v1alpha1.StorageNodeSet{}
	err := r.Get(ctx, req.NamespacedName, crStorageNodeSet)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("StorageNodeSet resource not found")
			return ctrl.Result{Requeue: false}, nil
		}
		r.Log.Error(err, "unable to get StorageNodeSet")
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	result, err := r.Sync(ctx, crStorageNodeSet)
	if err != nil {
		r.Log.Error(err, "unexpected Sync error")
	}

	return result, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor(StorageNodeSetKind)
	controller := ctrl.NewControllerManagedBy(mgr)

	return controller.
		For(&v1alpha1.StorageNodeSet{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Owns(&appsv1.StatefulSet{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		WithEventFilter(resources.IsStorageNodeSetCreatePredicate()).
		WithEventFilter(resources.IgnoreDeleteStateUnknownPredicate()).
		Complete(r)
}
