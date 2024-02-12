package remotestoragenodeset

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

// Reconciler reconciles a RemoteStorageNodeSet object
type Reconciler struct {
	Client         client.Client
	RemoteClient   client.Client
	Recorder       record.EventRecorder
	RemoteRecorder record.EventRecorder
	Log            logr.Logger
	Scheme         *runtime.Scheme
}

//+kubebuilder:rbac:groups=ydb.tech,resources=remotestoragenodesets,verbs=get;list;watch
//+kubebuilder:rbac:groups=ydb.tech,resources=remotestoragenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=remotestoragenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	remoteStorageNodeSet := &api.RemoteStorageNodeSet{}
	// we'll ignore not-found errors, since they can't be fixed by an immediate
	// requeue (we'll need to wait for a new notification), and we can get them
	// on deleted requests.
	if err := r.RemoteClient.Get(ctx, req.NamespacedName, remoteStorageNodeSet); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("StorageNodeSet has been deleted")
			return ctrl.Result{Requeue: false}, nil
		}
		logger.Error(err, "unable to get RemoteStorageNodeSet")
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, nil
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if remoteStorageNodeSet.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(remoteStorageNodeSet, RemoteFinalizerKey) {
			controllerutil.AddFinalizer(remoteStorageNodeSet, RemoteFinalizerKey)
			if err := r.RemoteClient.Update(ctx, remoteStorageNodeSet); err != nil {
				return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(remoteStorageNodeSet, RemoteFinalizerKey) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.deleteExternalResources(ctx, remoteStorageNodeSet); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried.
				return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(remoteStorageNodeSet, RemoteFinalizerKey)
			if err := r.RemoteClient.Update(ctx, remoteStorageNodeSet); err != nil {
				return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{Requeue: false}, nil
	}

	result, err := r.Sync(ctx, remoteStorageNodeSet)
	if err != nil {
		r.Log.Error(err, "unexpected Sync error")
	}

	return result, err
}

func (r *Reconciler) deleteExternalResources(ctx context.Context, remoteStorageNodeSet *api.RemoteStorageNodeSet) error {
	logger := log.FromContext(ctx)

	storageNodeSet := &api.StorageNodeSet{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      remoteStorageNodeSet.Name,
		Namespace: remoteStorageNodeSet.Namespace,
	}, storageNodeSet); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("StorageNodeSet not found")
			return nil
		}
		logger.Error(err, "unable to get StorageNodeSet")
		return err
	}

	if err := r.Client.Delete(ctx, storageNodeSet); err != nil {
		logger.Error(err, "unable to delete StorageNodeSet")
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, remoteCluster *cluster.Cluster) error {
	cluster := *remoteCluster
	resource := &api.RemoteStorageNodeSet{}
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
		Watches(&source.Kind{Type: &api.StorageNodeSet{}}, handler.EnqueueRequestsFromMapFunc(annotationFilter)).
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
