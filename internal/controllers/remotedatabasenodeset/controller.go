package remotedatabasenodeset

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

// Reconciler reconciles a RemoteDatabaseNodeSet object
type Reconciler struct {
	Client         client.Client
	RemoteClient   client.Client
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

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	remoteDatabaseNodeSet := &api.RemoteDatabaseNodeSet{}

	if err := r.RemoteClient.Get(ctx, req.NamespacedName, remoteDatabaseNodeSet); err != nil {
		if apierrs.IsNotFound(err) {
			logger.Info("RemotedatabaseNodeSet has been deleted")
			return r.handleRemoteResourceDeleted(ctx, req)
		}
		logger.Error(err, "unable to get RemotedatabaseNodeSet")
		return ctrl.Result{}, err
	}

	result, err := r.Sync(ctx, remoteDatabaseNodeSet)
	if err != nil {
		r.Log.Error(err, "unexpected Sync error")
	}

	return result, err
}

func (r *Reconciler) handleRemoteResourceDeleted(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	databaseNodeSet := &api.DatabaseNodeSet{}

	if err := r.Client.Get(ctx, req.NamespacedName, databaseNodeSet); err != nil {
		if apierrs.IsNotFound(err) {
			logger.Info("databaseNodeSet has been deleted")
			return ctrl.Result{Requeue: false}, nil
		}
		logger.Error(err, "unable to get databaseNodeSet")
		return ctrl.Result{}, err
	}

	if err := r.Client.Delete(ctx, databaseNodeSet); err != nil {
		logger.Error(err, "unable to delete databaseNodeSet")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: false}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, remoteCluster *cluster.Cluster) error {
	cluster := *remoteCluster

	r.RemoteRecorder = cluster.GetEventRecorderFor("RemoteDatabaseNodeSet")

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
		Watches(source.NewKindWithCache(&api.RemoteDatabaseNodeSet{}, cluster.GetCache()), &handler.EnqueueRequestForObject{}).
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
