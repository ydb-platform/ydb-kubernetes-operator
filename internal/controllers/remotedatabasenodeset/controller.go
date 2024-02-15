package remotedatabasenodeset

import (
	"context"
	"fmt"

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
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	ydbfinalizers "github.com/ydb-platform/ydb-kubernetes-operator/internal/finalizers"
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
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	remoteDatabaseNodeSet := &ydbv1alpha1.RemoteDatabaseNodeSet{}
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
		if !controllerutil.ContainsFinalizer(remoteDatabaseNodeSet, ydbfinalizers.RemoteFinalizerKey) {
			controllerutil.AddFinalizer(remoteDatabaseNodeSet, ydbfinalizers.RemoteFinalizerKey)
			if err := r.RemoteClient.Update(ctx, remoteDatabaseNodeSet); err != nil {
				return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(remoteDatabaseNodeSet, ydbfinalizers.RemoteFinalizerKey) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.deleteExternalResources(ctx, remoteDatabaseNodeSet); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried.
				return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(remoteDatabaseNodeSet, ydbfinalizers.RemoteFinalizerKey)
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

func ignoreDeletionPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			generationChanged := e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			annotationsChanged := !ydbannotations.CompareYdbTechAnnotations(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())

			_, isService := e.ObjectOld.(*corev1.Service)

			return generationChanged || annotationsChanged || isService
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
	resource := &ydbv1alpha1.RemoteDatabaseNodeSet{}
	resourceGVK, err := apiutil.GVKForObject(resource, r.Scheme)
	if err != nil {
		r.Log.Error(err, "does not recognize GVK for resource")
		return err
	}

	r.Recorder = mgr.GetEventRecorderFor(resourceGVK.Kind)
	r.RemoteRecorder = cluster.GetEventRecorderFor(resourceGVK.Kind)
	r.RemoteClient = cluster.GetClient()

	annotationFilter := func(mapObj client.Object) []reconcile.Request {
		requests := make([]reconcile.Request, 0)

		annotations := mapObj.GetAnnotations()
		primaryResourceName := annotations[ydbannotations.PrimaryResourceNameAnnotation]
		primaryResourceNamespace := annotations[ydbannotations.PrimaryResourceNamespaceAnnotation]

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
		Watches(source.NewKindWithCache(resource, cluster.GetCache()), &handler.EnqueueRequestForObject{}, builder.WithPredicates(ignoreDeletionPredicate())).
		Watches(&source.Kind{Type: &ydbv1alpha1.DatabaseNodeSet{}}, handler.EnqueueRequestsFromMapFunc(annotationFilter)).
		Watches(&source.Kind{Type: &corev1.Service{}}, handler.EnqueueRequestsFromMapFunc(annotationFilter)).
		Watches(&source.Kind{Type: &corev1.Secret{}}, handler.EnqueueRequestsFromMapFunc(annotationFilter)).
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

func (r *Reconciler) deleteExternalResources(
	ctx context.Context,
	crRemoteDatabaseNodeSet *ydbv1alpha1.RemoteDatabaseNodeSet,
) error {
	logger := log.FromContext(ctx)

	databaseNodeSet := &ydbv1alpha1.DatabaseNodeSet{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      crRemoteDatabaseNodeSet.Name,
		Namespace: crRemoteDatabaseNodeSet.Namespace,
	}, databaseNodeSet); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("DatabaseNodeSet not found")
		} else {
			logger.Error(err, "unable to get DatabaseNodeSet")
			return err
		}
	} else {
		if err := r.Client.Delete(ctx, databaseNodeSet); err != nil {
			logger.Error(err, "unable to delete DatabaseNodeSet")
			return err
		}
	}

	grpcService := &corev1.Service{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf(resources.GRPCServiceNameFormat, crRemoteDatabaseNodeSet.Spec.DatabaseRef.Name),
		Namespace: crRemoteDatabaseNodeSet.Namespace,
	}, grpcService); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Service GRPC not found")
		} else {
			logger.Error(err, "unable to get GRPC Service")
			return err
		}
	} else {
		if err := r.Client.Delete(ctx, grpcService); err != nil {
			logger.Error(err, "unable to delete GRPC Service")
			return err
		}
	}

	interconnectService := &corev1.Service{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf(resources.InterconnectServiceNameFormat, crRemoteDatabaseNodeSet.Spec.DatabaseRef.Name),
		Namespace: crRemoteDatabaseNodeSet.Namespace,
	}, interconnectService); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Service Interconnect not found")
		} else {
			logger.Error(err, "unable to get Interconnect Service")
			return err
		}
	} else {
		if err := r.Client.Delete(ctx, interconnectService); err != nil {
			logger.Error(err, "unable to delete Interconnect Service")
			return err
		}
	}

	statusService := &corev1.Service{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf(resources.StatusServiceNameFormat, crRemoteDatabaseNodeSet.Spec.DatabaseRef.Name),
		Namespace: crRemoteDatabaseNodeSet.Namespace,
	}, statusService); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Service Status not found")
		} else {
			logger.Error(err, "unable to get Status Service")
			return err
		}
	} else {
		if err := r.Client.Delete(ctx, statusService); err != nil {
			logger.Error(err, "unable to delete Status Service")
			return err
		}
	}

	remoteDatabaseNodeSet := resources.NewRemoteDatabaseNodeSet(crRemoteDatabaseNodeSet)
	remoteResources := remoteDatabaseNodeSet.GetRemoteResources()
	for _, object := range remoteResources {
		objectName := object.GetName()
		objectNamespace := object.GetNamespace()
		objectKind := object.GetObjectKind().GroupVersionKind().Kind

		if err := r.Client.Get(ctx, types.NamespacedName{
			Name:      objectName,
			Namespace: objectNamespace,
		}, object); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("Resource %s with name %s not found", objectKind, objectName)
			} else {
				logger.Error(err, "unable to get resource %s with name %s", objectKind, objectName)
				return err
			}
		} else {
			if err := r.Client.Delete(ctx, object); err != nil {
				logger.Error(err, "unable to delete resource %s with name %s", objectKind, objectName)
				return err
			}
		}
	}

	return nil
}
