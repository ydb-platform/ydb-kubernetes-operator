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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
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
	logger := log.FromContext(ctx)

	crDatabaseNodeSet := &api.DatabaseNodeSet{}
	err := r.Get(ctx, req.NamespacedName, crDatabaseNodeSet)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DatabaseNodeSet resource not found")
			return ctrl.Result{Requeue: false}, nil
		}
		logger.Error(err, "unable to get DatabaseNodeSet")
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	result, err := r.Sync(ctx, crDatabaseNodeSet)
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
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("DatabaseNodeSet")
	controller := ctrl.NewControllerManagedBy(mgr)

	return controller.
		For(&api.DatabaseNodeSet{}).
		Owns(&appsv1.StatefulSet{}).
		WithEventFilter(ignoreDeletionPredicate()).
		Complete(r)
}
