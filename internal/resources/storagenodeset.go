package resources

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
)

type StorageNodeSetBuilder struct {
	client.Object

	Name        string
	Labels      map[string]string
	Annotations map[string]string

	StorageNodeSetSpec api.StorageNodeSetSpec
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

type StorageNodeSetResource struct {
	*api.StorageNodeSet
}

func (b *StorageNodeSetResource) NewLabels() labels.Labels {
	l := labels.Common(b.Name, b.Labels)

	l.Merge(b.Spec.AdditionalLabels)
	l.Merge(map[string]string{labels.ComponentKey: labels.StorageComponent})

	storageNodeSetName := b.Labels[labels.StorageNodeSetComponent]
	l.Merge(map[string]string{labels.StorageNodeSetComponent: storageNodeSetName})

	if remoteCluster, exist := b.Labels[labels.RemoteClusterKey]; exist {
		l.Merge(map[string]string{labels.RemoteClusterKey: remoteCluster})
	}

	return l
}

func (b *StorageNodeSetResource) NewAnnotations() map[string]string {
	an := annotations.Common(b.Annotations)

	an.Merge(b.Spec.AdditionalAnnotations)

	return an
}

func (b *StorageNodeSetResource) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	nodeSetLabels := b.NewLabels()
	nodeSetAnnotations := b.NewAnnotations()

	statefulSetLabels := nodeSetLabels.Copy()
	statefulSetLabels.Merge(map[string]string{labels.StatefulsetComponent: b.Name})

	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(
		resourceBuilders,
		&StorageStatefulSetBuilder{
			Storage:    api.RecastStorageNodeSet(b.StorageNodeSet),
			RestConfig: restConfig,

			Name:        b.Name,
			Labels:      statefulSetLabels,
			Annotations: nodeSetAnnotations,
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
