package resources

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
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
	ydbCr := api.RecastStorageNodeSet(b.Unwrap())
	clusterBuilder := NewCluster(ydbCr)

	statefulSetName := b.Name
	statefulSetLabels := clusterBuilder.buildLabels()
	statefulSetAnnotations := CopyDict(b.Spec.AdditionalAnnotations)
	statefulSetAnnotations[annotations.ConfigurationChecksum] = SHAChecksum(b.Spec.Configuration)

	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(
		resourceBuilders,
		&StorageStatefulSetBuilder{
			Storage:    ydbCr,
			RestConfig: restConfig,

			Name:        statefulSetName,
			Labels:      statefulSetLabels,
			Annotations: statefulSetAnnotations,
		},
	)

	return resourceBuilders
}

func NewStorageNodeSet(storageNodeSet *api.StorageNodeSet) StorageNodeSetResource {
	crStorageNodeSet := storageNodeSet.DeepCopy()

	if crStorageNodeSet.Spec.Service.Status.TLSConfiguration == nil {
		crStorageNodeSet.Spec.Service.Status.TLSConfiguration = &api.TLSConfiguration{Enabled: false}
	}

	return StorageNodeSetResource{StorageNodeSet: crStorageNodeSet}
}

func (b *StorageNodeSetResource) Unwrap() *api.StorageNodeSet {
	return b.DeepCopy()
}
