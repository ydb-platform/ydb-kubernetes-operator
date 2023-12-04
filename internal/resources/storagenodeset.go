package resources

import (
	"errors"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StorageNodeSetBuilder struct {
	*api.StorageNodeSet

	Name string

	Labels      map[string]string
	Annotations map[string]string

	StorageNodeSetSpec api.StorageNodeSetSpec
}

func NewStorageNodeSet(storageNodeSet *api.StorageNodeSet) StorageNodeSetBuilder {
	crStorageNodeSet := storageNodeSet.DeepCopy()

	return StorageNodeSetBuilder{
		Name:               crStorageNodeSet.Name,
		Labels:             crStorageNodeSet.Labels,
		Annotations:        crStorageNodeSet.Annotations,
		StorageNodeSetSpec: crStorageNodeSet.Spec,
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

func (b *StorageNodeSetBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	var resourceBuilders []ResourceBuilder

	resourceBuilders = append(resourceBuilders,
		&StorageStatefulSetBuilder{
			Storage:    b.RecastStorageNodeSet().DeepCopy(),
			RestConfig: restConfig,
			Labels:     b.Labels,
		},
	)
	return resourceBuilders
}

func (b *StorageNodeSetBuilder) RecastStorageNodeSet() *api.Storage {
	return &api.Storage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.GetName(),
			Namespace: b.GetNamespace(),
			Labels:    b.Labels,
		},
		Spec: api.StorageSpec{
			AdditionalAnnotations:     b.Annotations,
			Nodes:                     b.StorageNodeSetSpec.Nodes,
			Secrets:                   b.StorageNodeSetSpec.Secrets,
			Volumes:                   b.StorageNodeSetSpec.Volumes,
			DataStore:                 b.StorageNodeSetSpec.DataStore,
			HostNetwork:               b.StorageNodeSetSpec.HostNetwork,
			Resources:                 b.StorageNodeSetSpec.Resources,
			NodeSelector:              b.StorageNodeSetSpec.NodeSelector,
			Affinity:                  b.StorageNodeSetSpec.Affinity,
			Tolerations:               b.StorageNodeSetSpec.Tolerations,
			TopologySpreadConstraints: b.StorageNodeSetSpec.TopologySpreadConstraints,
			InitContainers:            b.StorageNodeSetSpec.InitContainers,
			Configuration:             b.StorageNodeSetSpec.Configuration, // TODO: migrate to configmapRef
			Service:                   b.StorageNodeSetSpec.Service,       // TODO: export only externalHost
			CABundle:                  b.StorageNodeSetSpec.CABundle,      // TODO: migrate to trust-manager
			Erasure:                   b.StorageNodeSetSpec.Erasure,       // TODO: get from configuration
		},
	}
}
