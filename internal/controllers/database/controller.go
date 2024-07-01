package database

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
)

// Reconciler reconciles a Database object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
	Log      logr.Logger
}

//+kubebuilder:rbac:groups=ydb.tech,resources=databases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ydb.tech,resources=databases/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ydb.tech,resources=databases/finalizers,verbs=update
//+kubebuilder:rbac:groups=ydb.tech,resources=remotedatabasenodesets,verbs=get;list;watch;create;update;patch;delete
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
//+kubebuilder:rbac:groups=apps,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=secrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)

	resource := &v1alpha1.Database{}
	err := r.Get(ctx, req.NamespacedName, resource)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Database resource not found")
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

// Create FieldIndexer to usage for List requests in Reconcile
func createFieldIndexers(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1alpha1.RemoteDatabaseNodeSet{},
		OwnerControllerField,
		func(obj client.Object) []string {
			// grab the RemoteDatabaseNodeSet object, extract the owner...
			remoteDatabaseNodeSet := obj.(*v1alpha1.RemoteDatabaseNodeSet)
			owner := metav1.GetControllerOf(remoteDatabaseNodeSet)
			if owner == nil {
				return nil
			}
			// ...make sure it's a Database...
			if owner.APIVersion != v1alpha1.GroupVersion.String() || owner.Kind != DatabaseKind {
				return nil
			}

			// ...and if so, return it
			return []string{owner.Name}
		}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1alpha1.DatabaseNodeSet{},
		OwnerControllerField,
		func(obj client.Object) []string {
			// grab the DatabaseNodeSet object, extract the owner...
			databaseNodeSet := obj.(*v1alpha1.DatabaseNodeSet)
			owner := metav1.GetControllerOf(databaseNodeSet)
			if owner == nil {
				return nil
			}
			// ...make sure it's a Database...
			if owner.APIVersion != v1alpha1.GroupVersion.String() || owner.Kind != DatabaseKind {
				return nil
			}

			// ...and if so, return it
			return []string{owner.Name}
		}); err != nil {
		return err
	}

	return mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1alpha1.Database{},
		SecretField,
		func(obj client.Object) []string {
			secrets := []string{}
			// grab the Database object, extract secrets from spec...
			database := obj.(*v1alpha1.Database)

			// ...append declared Secret to index...
			for _, secret := range database.Spec.Secrets {
				secrets = append(secrets, secret.Name)
			}

			// ...and if so, return it
			return secrets
		})
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor(DatabaseKind)
	controller := ctrl.NewControllerManagedBy(mgr)
	if err := createFieldIndexers(mgr); err != nil {
		r.Log.Error(err, "unexpected FieldIndexer error")
		return err
	}

	return controller.
		For(&v1alpha1.Database{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Owns(&v1alpha1.RemoteDatabaseNodeSet{},
			builder.WithPredicates(resources.LastAppliedAnnotationPredicate()), // TODO: YDBOPS-9194
		).
		Owns(&v1alpha1.DatabaseNodeSet{},
			builder.WithPredicates(resources.LastAppliedAnnotationPredicate()), // TODO: YDBOPS-9194
		).
		Owns(&appsv1.StatefulSet{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Owns(&corev1.ConfigMap{},
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Owns(&corev1.Service{},
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.findDatabasesForSecret),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithEventFilter(resources.IsDatabaseCreatePredicate()).
		WithEventFilter(resources.IgnoreDeleteStateUnknownPredicate()).
		Complete(r)
}

// Find all Databases which using Secret and make request for Reconcile
func (r *Reconciler) findDatabasesForSecret(secret client.Object) []reconcile.Request {
	attachedDatabases := &v1alpha1.DatabaseList{}
	err := r.List(
		context.Background(),
		attachedDatabases,
		client.InNamespace(secret.GetNamespace()),
		client.MatchingFields{SecretField: secret.GetName()},
	)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(attachedDatabases.Items))
	for i, item := range attachedDatabases.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}
