package remotedatabasenodeset

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
	r.Log = log.FromContext(ctx)

	remoteDatabaseNodeSet := &v1alpha1.RemoteDatabaseNodeSet{}
	if err := r.RemoteClient.Get(ctx, req.NamespacedName, remoteDatabaseNodeSet); err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("RemoteDatabaseNodeSet resource not found on remote cluster")
			return ctrl.Result{Requeue: false}, nil
		}
		r.Log.Error(err, "unable to get RemoteDatabaseNodeSet on remote cluster")
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
			if err := r.deleteExternalResources(ctx, remoteDatabaseNodeSet); err != nil {
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

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, remoteCluster *cluster.Cluster) error {
	cluster := *remoteCluster

	r.Recorder = mgr.GetEventRecorderFor(RemoteDatabaseNodeSetKind)
	r.RemoteRecorder = cluster.GetEventRecorderFor(RemoteDatabaseNodeSetKind)
	r.RemoteClient = cluster.GetClient()

	annotationFilter := func(mapObj client.Object) []reconcile.Request {
		requests := make([]reconcile.Request, 0)

		annotations := mapObj.GetAnnotations()
		primaryResourceName, exist := annotations[ydbannotations.PrimaryResourceDatabaseAnnotation]
		if exist {
			databaseNodeSets := &v1alpha1.DatabaseNodeSetList{}
			if err := r.Client.List(
				context.Background(),
				databaseNodeSets,
				client.InNamespace(mapObj.GetNamespace()),
				client.MatchingFields{
					DatabaseRefField: primaryResourceName,
				},
			); err != nil {
				return requests
			}
			for _, databaseNodeSet := range databaseNodeSets.Items {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: databaseNodeSet.GetNamespace(),
						Name:      databaseNodeSet.GetName(),
					},
				})
			}
		}
		return requests
	}

	isNodeSetFromMgmt, err := buildLocalSelector()
	if err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1alpha1.DatabaseNodeSet{},
		DatabaseRefField,
		func(obj client.Object) []string {
			databaseNodeSet := obj.(*v1alpha1.DatabaseNodeSet)
			return []string{databaseNodeSet.Spec.DatabaseRef.Name}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named(RemoteDatabaseNodeSetKind).
		WatchesRawSource(
			source.Kind(cluster.GetCache(), &v1alpha1.RemoteDatabaseNodeSet{}, &handler.TypedEnqueueRequestForObject[*v1alpha1.RemoteDatabaseNodeSet]{}, predicate.TypedGenerationChangedPredicate[*v1alpha1.RemoteDatabaseNodeSet]{}),
		).
		Watches(
			&v1alpha1.DatabaseNodeSet{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(resources.LabelExistsPredicate(isNodeSetFromMgmt)),
		).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return annotationFilter(obj)
			}),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return annotationFilter(obj)
			}),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				return annotationFilter(obj)
			}),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithEventFilter(resources.IsRemoteDatabaseNodeSetCreatePredicate()).
		WithEventFilter(resources.IgnoreDeleteStateUnknownPredicate()).
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

func (r *Reconciler) deleteExternalResources(
	ctx context.Context,
	crRemoteDatabaseNodeSet *v1alpha1.RemoteDatabaseNodeSet,
) error {
	databaseNodeSet := &v1alpha1.DatabaseNodeSet{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      crRemoteDatabaseNodeSet.Name,
		Namespace: crRemoteDatabaseNodeSet.Namespace,
	}, databaseNodeSet); err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("DatabaseNodeSet not found")
		} else {
			r.Log.Error(err, "unable to get DatabaseNodeSet")
			return err
		}
	} else {
		if err := r.Client.Delete(ctx, databaseNodeSet); err != nil {
			r.Log.Error(err, "unable to delete DatabaseNodeSet")
			return err
		}
	}

	remoteDatabaseNodeSet := resources.NewRemoteDatabaseNodeSet(crRemoteDatabaseNodeSet)
	if _, _, err := r.removeUnusedRemoteObjects(ctx, &remoteDatabaseNodeSet, []client.Object{}); err != nil {
		r.Log.Error(err, "unable to delete unused remote resources")
		return err
	}

	return nil
}
