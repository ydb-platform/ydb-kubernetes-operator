package remotedatabasenodeset

import (
	"context"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	ydblabels "github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

// Reconciler reconciles a RemoteDatabaseNodeSet object
type Reconciler struct {
	Client         client.Client
	RemoteClient   client.Client
	Recorder       record.EventRecorder
	RemoteRecorder record.EventRecorder
	Log            logr.Logger
	Scheme         *runtime.Scheme
}

//+kubebuilder:rbac:groups=ydb.tech,resources=remotedatabasenodesets,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=ydb.tech,resources=remotedatabasenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=remotedatabasenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=ydb.tech,resources=databasenodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=databasenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=databasenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	remoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}
	// we'll ignore not-found errors, since they can't be fixed by an immediate
	// requeue (we'll need to wait for a new notification), and we can get them
	// on deleted requests.
	if err := r.RemoteClient.Get(ctx, req.NamespacedName, remoteDatabaseNodeSet); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("RemoteDatabaseNodeSet resource not found on remote cluster")
			return ctrl.Result{Requeue: false}, nil
		}
		logger.Error(err, "unable to get RemoteDatabaseNodeSet on remote cluster")
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	//nolint:nestif
	// examine DeletionTimestamp to determine if object is under deletion
	if remoteDatabaseNodeSet.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(remoteDatabaseNodeSet, ydbannotations.RemoteFinalizerKey) {
			controllerutil.AddFinalizer(remoteDatabaseNodeSet, ydbannotations.RemoteFinalizerKey)
			if err := r.RemoteClient.Update(ctx, remoteDatabaseNodeSet); err != nil {
				return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(remoteDatabaseNodeSet, ydbannotations.RemoteFinalizerKey) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.deleteExternalResources(ctx, req.NamespacedName); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried.
				return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(remoteDatabaseNodeSet, ydbannotations.RemoteFinalizerKey)
			if err := r.RemoteClient.Update(ctx, remoteDatabaseNodeSet); err != nil {
				return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{Requeue: false}, nil
	}

	result, err := r.Sync(ctx, remoteDatabaseNodeSet)
	if err != nil {
		r.Log.Error(err, "unexpected Sync error")
	}

	return result, err
}

func (r *Reconciler) deleteExternalResources(ctx context.Context, key types.NamespacedName) error {
	logger := log.FromContext(ctx)

	databaseNodeSet := &v1alpha1.DatabaseNodeSet{}
	if err := r.Client.Get(ctx, key, databaseNodeSet); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DatabaseNodeSet not found")
			return nil
		}
		logger.Error(err, "unable to get DatabaseNodeSet")
		return err
	}

	if err := r.Client.Delete(ctx, databaseNodeSet); err != nil {
		logger.Error(err, "unable to delete DatabaseNodeSet")
		return err
	}

	return nil
}

func ignoreDeletionPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			generationChanged := e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			annotationsChanged := !ydbannotations.CompareYdbTechAnnotations(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())

			return generationChanged || annotationsChanged
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Evaluates to false if the object has been confirmed deleted.
			return !e.DeleteStateUnknown
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, remoteCluster *cluster.Cluster) error {
	cluster := *remoteCluster
	remoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}

	r.Recorder = mgr.GetEventRecorderFor(RemoteDatabaseNodeSetKind)
	r.RemoteRecorder = cluster.GetEventRecorderFor(RemoteDatabaseNodeSetKind)
	r.RemoteClient = cluster.GetClient()

	isNodeSetFromMgmt, err := buildLocalSelector()
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named(RemoteDatabaseNodeSetKind).
		Watches(
			source.NewKindWithCache(remoteDatabaseNodeSet, cluster.GetCache()),
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(ignoreDeletionPredicate()),
		).
		Watches(
			&source.Kind{Type: &v1alpha1.DatabaseNodeSet{}},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(
				resources.LabelExistsPredicate(isNodeSetFromMgmt),
			),
		).
		Complete(r)
}

func buildLocalSelector() (labels.Selector, error) {
	labelRequirements := []labels.Requirement{}
	localClusterRequirement, err := labels.NewRequirement(
		ydblabels.RemoteClusterKey,
		selection.Exists,
		[]string{},
	)
	if err != nil {
		return nil, err
	}
	labelRequirements = append(labelRequirements, *localClusterRequirement)
	return labels.NewSelector().Add(labelRequirements...), nil
}

func BuildRemoteSelector(remoteCluster string) (labels.Selector, error) {
	labelRequirements := []labels.Requirement{}
	remoteClusterRequirement, err := labels.NewRequirement(
		ydblabels.RemoteClusterKey,
		selection.Equals,
		[]string{remoteCluster},
	)
	if err != nil {
		return nil, err
	}
	labelRequirements = append(labelRequirements, *remoteClusterRequirement)
	return labels.NewSelector().Add(labelRequirements...), nil
}
