package resources

import (
	"errors"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
)

type StorageNodeSetBuilder struct {
	client.Object

	Name        string
	Labels      map[string]string
	Annotations map[string]string

	StorageNodeSetSpec api.StorageNodeSetSpec
}

type StorageNodeSetResource struct {
	*api.StorageNodeSet
}

func (b *StorageNodeSetBuilder) Build(obj client.Object) error {
	sns, ok := obj.(*api.StorageNodeSet)
	if !ok {
		return errors.New("failed to cast to StorageNodeSet object")
	}

	if sns.ObjectMeta.Name == "" {
		sns.ObjectMeta.Name = b.Name
	}
	sns.ObjectMeta.Namespace = b.GetNamespace()

	sns.ObjectMeta.Labels = b.Labels
	sns.ObjectMeta.Annotations = b.Annotations

	sns.Spec = b.StorageNodeSetSpec

	return nil
}

func (b *StorageNodeSetBuilder) Placeholder(cr client.Object) client.Object {
	return &api.StorageNodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *StorageNodeSetResource) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(
		resourceBuilders,
		&StorageStatefulSetBuilder{
			Storage:    api.RecastStorageNodeSet(b.StorageNodeSet),
			RestConfig: restConfig,

			Name:   b.Name,
			Labels: b.Labels,
		},
	)

	return resourceBuilders
}

func NewStorageNodeSet(storageNodeSet *api.StorageNodeSet) StorageNodeSetResource {
	crStorageNodeSet := storageNodeSet.DeepCopy()

	return StorageNodeSetResource{
		StorageNodeSet: crStorageNodeSet,
	}
}

func (b *StorageNodeSetResource) SetStatusOnFirstReconcile() (bool, ctrl.Result, error) {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}

		if b.Spec.Pause {
			meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
				Type:    StoragePausedCondition,
				Status:  "False",
				Reason:  ReasonInProgress,
				Message: "Transitioning StorageNodeSet to Paused state",
			})

			return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
		}
	}

	return Continue, ctrl.Result{}, nil
}

func (b *StorageNodeSetResource) Unwrap() *api.StorageNodeSet {
	return b.DeepCopy()
}
