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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	ydblabels "github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
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

//+kubebuilder:rbac:groups=ydb.tech,resources=remotedatabasenodesets,verbs=get;list;watch
//+kubebuilder:rbac:groups=ydb.tech,resources=remotedatabasenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=remotedatabasenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=ydb.tech,resources=databasenodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=databasenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=databasenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	remoteDatabaseNodeSet := &api.RemoteDatabaseNodeSet{}
	// we'll ignore not-found errors, since they can't be fixed by an immediate
	// requeue (we'll need to wait for a new notification), and we can get them
	// on deleted requests.
	if err := r.RemoteClient.Get(ctx, req.NamespacedName, remoteDatabaseNodeSet); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DatabaseNodeSet has been deleted")
			return ctrl.Result{Requeue: false}, nil
		}
		logger.Error(err, "unable to get RemoteDatabaseNodeSet")
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	//nolint:nestif
	// examine DeletionTimestamp to determine if object is under deletion
	if remoteDatabaseNodeSet.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(remoteDatabaseNodeSet, RemoteFinalizerKey) {
			controllerutil.AddFinalizer(remoteDatabaseNodeSet, RemoteFinalizerKey)
			if err := r.RemoteClient.Update(ctx, remoteDatabaseNodeSet); err != nil {
				return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(remoteDatabaseNodeSet, RemoteFinalizerKey) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.deleteExternalResources(ctx, remoteDatabaseNodeSet); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried.
				return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(remoteDatabaseNodeSet, RemoteFinalizerKey)
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

func (r *Reconciler) deleteExternalResources(ctx context.Context, remoteDatabaseNodeSet *api.RemoteDatabaseNodeSet) error {
	logger := log.FromContext(ctx)

	storageNodeSet := &api.StorageNodeSet{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      remoteDatabaseNodeSet.Name,
		Namespace: remoteDatabaseNodeSet.Namespace,
	}, storageNodeSet); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DatabaseNodeSet not found")
			return nil
		}
		logger.Error(err, "unable to get DatabaseNodeSet")
		return err
	}

	if err := r.Client.Delete(ctx, storageNodeSet); err != nil {
		logger.Error(err, "unable to delete DatabaseNodeSet")
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, remoteCluster *cluster.Cluster) error {
	cluster := *remoteCluster
	resource := &api.RemoteDatabaseNodeSet{}
	resourceGVK, err := apiutil.GVKForObject(resource, r.Scheme)
	if err != nil {
		r.Log.Error(err, "does not recognize GVK for resource")
		return err
	}

	r.Recorder = mgr.GetEventRecorderFor(resourceGVK.Kind)
	r.RemoteRecorder = cluster.GetEventRecorderFor(resourceGVK.Kind)

	annotationFilter := func(mapObj client.Object) []reconcile.Request {
		requests := make([]reconcile.Request, 0)

		annotations := mapObj.GetAnnotations()
		primaryResourceName := annotations[PrimaryResourceNameAnnotation]
		primaryResourceNamespace := annotations[PrimaryResourceNamespaceAnnotation]

		if primaryResourceName != "" && primaryResourceNamespace != "" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: primaryResourceNamespace,
					Name:      primaryResourceName,
				},
			})
		}
		return requests
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named(resourceGVK.Kind).
		Watches(source.NewKindWithCache(resource, cluster.GetCache()), &handler.EnqueueRequestForObject{}).
		Watches(&source.Kind{Type: &api.DatabaseNodeSet{}}, handler.EnqueueRequestsFromMapFunc(annotationFilter)).
		Complete(r)
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
