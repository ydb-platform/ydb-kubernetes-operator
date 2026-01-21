package databasenodeset

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

// Reconciler reconciles a DatabaseNodeSet object
type Reconciler struct {
	client.Client
	Recorder record.EventRecorder
	Config   *rest.Config
	Scheme   *runtime.Scheme
	Log      logr.Logger
}

//+kubebuilder:rbac:groups=ydb.tech,resources=databases,verbs=get;list;watch
//+kubebuilder:rbac:groups=ydb.tech,resources=databasenodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=databasenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=databasenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)

	crDatabaseNodeSet := &v1alpha1.DatabaseNodeSet{}
	err := r.Get(ctx, req.NamespacedName, crDatabaseNodeSet)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("DatabaseNodeSet resource not found")
			return ctrl.Result{Requeue: false}, nil
		}
		r.Log.Error(err, "unable to get DatabaseNodeSet")
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	result, err := r.Sync(ctx, crDatabaseNodeSet)
	if err != nil {
		r.Log.Error(err, "unexpected Sync error")
	}

	return result, err
}

// NewReconciler creates a new Reconciler instance
func NewReconciler(client client.Client, scheme *runtime.Scheme, config *rest.Config) *Reconciler {
	return &Reconciler{
		Client: client,
		Scheme: scheme,
		Config: config,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.SetupWithManagerAndName(mgr, DatabaseNodeSetKind)
}

// SetupWithManagerAndName sets up the controller with the Manager using a custom name.
func (r *Reconciler) SetupWithManagerAndName(mgr ctrl.Manager, name string) error {
	r.Recorder = mgr.GetEventRecorderFor(name)
	controller := ctrl.NewControllerManagedBy(mgr)

	return controller.
		Named(name).
		For(&v1alpha1.DatabaseNodeSet{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Owns(&appsv1.StatefulSet{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		WithEventFilter(resources.IsDatabaseNodeSetCreatePredicate()).
		WithEventFilter(resources.IgnoreDeleteStateUnknownPredicate()).
		Complete(r)
}
