package resources

import (
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
)

func LastAppliedAnnotationPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !annotations.CompareLastAppliedAnnotation(
				e.ObjectOld.GetAnnotations(),
				e.ObjectNew.GetAnnotations(),
			)
		},
	}
}

func IsStorageCreatePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, isStorage := e.Object.(*api.Storage)
			return isStorage
		},
	}
}

func IsStorageNodeSetCreatePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, isStorageNodeSet := e.Object.(*api.StorageNodeSet)
			return isStorageNodeSet
		},
	}
}

func IsRemoteStorageNodeSetCreatePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, isRemoteStorageNodeSet := e.Object.(*api.RemoteStorageNodeSet)
			return isRemoteStorageNodeSet
		},
	}
}

func IsDatabaseCreatePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, isDatabase := e.Object.(*api.Database)
			return isDatabase
		},
	}
}

func IsDatabaseNodeSetCreatePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, isDatabaseNodeSet := e.Object.(*api.DatabaseNodeSet)
			return isDatabaseNodeSet
		},
	}
}

func IsRemoteDatabaseNodeSetCreatePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			_, isRemoteDatabaseNodeSet := e.Object.(*api.RemoteDatabaseNodeSet)
			return isRemoteDatabaseNodeSet
		},
	}
}

func IgnoreDeleteStateUnknownPredicate() predicate.Predicate {
	return predicate.Funcs{
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Evaluates to false if the object has been confirmed deleted.
			return !e.DeleteStateUnknown
		},
	}
}

func LabelExistsPredicate(selector labels.Selector) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(o client.Object) bool {
		return selector.Matches(labels.Set(o.GetLabels()))
	})
}
