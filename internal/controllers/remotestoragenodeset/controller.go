package remotestoragenodeset

import (
	"context"

	"github.com/go-logr/logr"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
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

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	remoteStorageNodeSet := &api.RemoteStorageNodeSet{}

	if err := r.RemoteClient.Get(ctx, req.NamespacedName, remoteStorageNodeSet); err != nil {
		if apierrs.IsNotFound(err) {
			logger.Info("RemoteStorageNodeSet has been deleted")
			return r.handleRemoteResourceDeleted(ctx, req)
		}
		logger.Error(err, "unable to get RemoteStorageNodeSet")
		return ctrl.Result{}, err
	}

	result, err := r.Sync(ctx, remoteStorageNodeSet)
	if err != nil {
		r.Log.Error(err, "unexpected Sync error")
	}

	return result, err
}

func (r *Reconciler) handleRemoteResourceDeleted(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	storageNodeSet := &api.StorageNodeSet{}

	if err := r.Client.Get(ctx, req.NamespacedName, storageNodeSet); err != nil {
		if apierrs.IsNotFound(err) {
			logger.Info("StorageNodeSet has been deleted")
			return ctrl.Result{Requeue: false}, nil
		}
		logger.Error(err, "unable to get StorageNodeSet")
		return ctrl.Result{}, err
	}

	if err := r.Client.Delete(ctx, storageNodeSet); err != nil {
		logger.Error(err, "unable to delete StorageNodeSet")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: false}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, remoteCluster *cluster.Cluster) error {
	cluster := *remoteCluster

	r.RemoteRecorder = cluster.GetEventRecorderFor("RemoteStorageNodeSet")

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
		Named("RemoteStorageNodeSet").
		Watches(source.NewKindWithCache(&api.RemoteStorageNodeSet{}, cluster.GetCache()), &handler.EnqueueRequestForObject{}).
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
