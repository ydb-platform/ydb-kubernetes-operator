package resources

import (
	"errors"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StorageNodeSetBuilder struct {
	*api.StorageNodeSet
	Storage *api.Storage

	Labels            labels.Labels
	NodeSetSpecInline *api.NodeSetSpecInline
}

func NewStorageNodeSet(storageNodeSet *api.StorageNodeSet, storage *api.Storage) StorageNodeSetBuilder {
	crStorage := storage.DeepCopy()
	crStorageNodeSet := storageNodeSet.DeepCopy()

	return StorageNodeSetBuilder{
		StorageNodeSet: crStorageNodeSet,
		Storage:        crStorage,
		Labels:         labels.StorageLabels(storage),
	}
}

func (b *StorageNodeSetBuilder) SetStatusOnFirstReconcile() {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}
	}
}

func (b *StorageNodeSetBuilder) Unwrap() *api.StorageNodeSet {
	return b.DeepCopy()
}

func (b *StorageNodeSetBuilder) Build(obj client.Object) error {
	sns, ok := obj.(*api.StorageNodeSet)
	if !ok {
		return errors.New("failed to cast to StorageNodeSet object")
	}

	if sns.ObjectMeta.Name == "" {
		sns.ObjectMeta.Name = b.NodeSetSpecInline.Name
	}
	sns.ObjectMeta.Namespace = b.Storage.GetNamespace()

	storageLabels := labels.Labels{}
	storageLabels = storageLabels.MergeInPlace(b.Storage.Labels)

	sns.ObjectMeta.Labels = storageLabels.Merge(b.NodeSetSpecInline.AdditionalLabels)

	sns.Spec = api.StorageNodeSetSpec{
		StorageRef:  b.Storage.GetName(),
		NodeSetSpec: b.NodeSetSpecInline.NodeSetSpec,
	}

	return nil

}

func (b *StorageNodeSetBuilder) Placeholder(cr client.Object) client.Object {
	return &api.StorageNodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.NodeSetSpecInline.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *StorageNodeSetBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {

	var optionalBuilders []ResourceBuilder

	stsBuilder := &StorageStatefulSetBuilder{
		Storage:    b.Storage.DeepCopy(),
		RestConfig: restConfig,
		Labels:     b.Labels.AsMap(),
	}

	return append(optionalBuilders,
		&StorageNodeSetStatefulSetBuilder{
			StorageStatefulSetBuilder: stsBuilder,
			StorageNodeSet:            b.StorageNodeSet,
		},
	)
}
