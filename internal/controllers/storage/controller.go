package storage

import (
	"context"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

// Reconciler reconciles a Storage object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
	Log      logr.Logger

	WithServiceMonitors bool
}

//+kubebuilder:rbac:groups=ydb.tech,resources=storages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=storages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=storages/finalizers,verbs=update
//+kubebuilder:rbac:groups=ydb.tech,resources=remotestoragenodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=remotestoragenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=remotestoragenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=storagenodesets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=get;list;watch
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)

	resource := &v1alpha1.Storage{}
	err := r.Get(ctx, req.NamespacedName, resource)
	if err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("Storage resource not found")
			return ctrl.Result{Requeue: false}, nil
		}
		r.Log.Error(err, "unexpected Get error")
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}
	result, err := r.Sync(ctx, resource)
	if err != nil {
		r.Log.Error(err, "unexpected Sync error")
	}

	return result, err
}

func createFieldIndexers(mgr ctrl.Manager, resourceKind string) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1alpha1.RemoteStorageNodeSet{},
		OwnerControllerField,
		func(obj client.Object) []string {
			// grab the RemoteStorageNodeSet object, extract the owner...
			remoteStorageNodeSet := obj.(*v1alpha1.RemoteStorageNodeSet)
			owner := metav1.GetControllerOf(remoteStorageNodeSet)
			if owner == nil {
				return nil
			}
			// ...make sure it's a Storage...
			if owner.APIVersion != v1alpha1.GroupVersion.String() || owner.Kind != resourceKind {
				return nil
			}

			// ...and if so, return it
			return []string{owner.Name}
		}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1alpha1.StorageNodeSet{},
		OwnerControllerField,
		func(obj client.Object) []string {
			// grab the StorageNodeSet object, extract the owner...
			storageNodeSet := obj.(*v1alpha1.StorageNodeSet)
			owner := metav1.GetControllerOf(storageNodeSet)
			if owner == nil {
				return nil
			}
			// ...make sure it's a Storage...
			if owner.APIVersion != v1alpha1.GroupVersion.String() || owner.Kind != resourceKind {
				return nil
			}

			// ...and if so, return it
			return []string{owner.Name}
		}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1alpha1.Storage{},
		SecretField,
		func(obj client.Object) []string {
			secrets := []string{}
			storage := obj.(*v1alpha1.Storage)
			for _, secret := range storage.Spec.Secrets {
				secrets = append(secrets, secret.Name)
			}

			return secrets
		}); err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	resource := &v1alpha1.Storage{}
	resourceGVK, err := apiutil.GVKForObject(resource, r.Scheme)
	if err != nil {
		r.Log.Error(err, "does not recognize GVK for resource")
		return err
	}

	r.Recorder = mgr.GetEventRecorderFor(resourceGVK.Kind)
	controller := ctrl.NewControllerManagedBy(mgr)
	if err := createFieldIndexers(mgr, resourceGVK.Kind); err != nil {
		r.Log.Error(err, "unexpected FieldIndexer error")
		return err
	}

	if r.WithServiceMonitors {
		controller = controller.
			Owns(&monitoringv1.ServiceMonitor{})
	}

	return controller.
		For(resource).
		Owns(&appsv1.StatefulSet{}).
		Owns(&v1alpha1.RemoteStorageNodeSet{}).
		Owns(&v1alpha1.StorageNodeSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.findStoragesForSecret),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithEventFilter(predicate.Or(
			predicate.GenerationChangedPredicate{},
			resources.IgnoreDeletetionPredicate(),
			resources.LastAppliedAnnotationPredicate(),
			resources.ResourcesPredicate()),
		).
		Complete(r)
}

func (r *Reconciler) findStoragesForSecret(secret client.Object) []reconcile.Request {
	attachedStorages := &v1alpha1.StorageList{}
	err := r.List(
		context.Background(),
		attachedStorages,
		client.InNamespace(secret.GetNamespace()),
		client.MatchingFields{SecretField: secret.GetName()},
	)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(attachedStorages.Items))
	for i, item := range attachedStorages.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}
