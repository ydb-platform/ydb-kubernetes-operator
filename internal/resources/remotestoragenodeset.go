package resources

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
)

type RemoteStorageNodeSetBuilder struct {
	client.Object

	Name   string
	Labels map[string]string

	StorageNodeSetSpec api.StorageNodeSetSpec
}

type RemoteStorageNodeSetResource struct {
	*api.RemoteStorageNodeSet
}

func (b *RemoteStorageNodeSetBuilder) Build(obj client.Object) error {
	dns, ok := obj.(*api.RemoteStorageNodeSet)
	if !ok {
		return errors.New("failed to cast to RemoteStorageNodeSet object")
	}

	if dns.ObjectMeta.Name == "" {
		dns.ObjectMeta.Name = b.Name
	}
	dns.ObjectMeta.Namespace = b.GetNamespace()

	dns.ObjectMeta.Labels = b.Labels
	dns.Spec = b.StorageNodeSetSpec

	return nil
}

func (b *RemoteStorageNodeSetBuilder) Placeholder(cr client.Object) client.Object {
	return &api.RemoteStorageNodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *RemoteStorageNodeSetResource) GetResourceBuilders() []ResourceBuilder {
	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(resourceBuilders,
		&StorageNodeSetBuilder{
			Object: b,

			Name:               b.Name,
			Labels:             b.Labels,
			StorageNodeSetSpec: b.Spec,
		},
	)
	return resourceBuilders
}

func NewRemoteStorageNodeSet(remoteStorageNodeSet *api.RemoteStorageNodeSet) RemoteStorageNodeSetResource {
	crRemoteStorageNodeSet := remoteStorageNodeSet.DeepCopy()

	return RemoteStorageNodeSetResource{RemoteStorageNodeSet: crRemoteStorageNodeSet}
}

func (b *RemoteStorageNodeSetResource) SetPrimaryResourceAnnotations(obj client.Object) {
	annotations := make(map[string]string)
	for key, value := range obj.GetAnnotations() {
		annotations[key] = value
	}

	if _, exist := annotations[ydbannotations.PrimaryResourceStorageAnnotation]; !exist {
		annotations[ydbannotations.PrimaryResourceStorageAnnotation] = b.Spec.StorageRef.Name
	}

	obj.SetAnnotations(annotations)
}
