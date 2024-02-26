package resources

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
)

type RemoteDatabaseNodeSetBuilder struct {
	client.Object

	Name   string
	Labels map[string]string

	DatabaseNodeSetSpec api.DatabaseNodeSetSpec
}

type RemoteDatabaseNodeSetResource struct {
	*api.RemoteDatabaseNodeSet
}

func (b *RemoteDatabaseNodeSetBuilder) Build(obj client.Object) error {
	dns, ok := obj.(*api.RemoteDatabaseNodeSet)
	if !ok {
		return errors.New("failed to cast to RemoteDatabaseNodeSet object")
	}

	if dns.ObjectMeta.Name == "" {
		dns.ObjectMeta.Name = b.Name
	}
	dns.ObjectMeta.Namespace = b.GetNamespace()

	dns.ObjectMeta.Labels = b.Labels
	dns.Spec = b.DatabaseNodeSetSpec

	return nil
}

func (b *RemoteDatabaseNodeSetBuilder) Placeholder(cr client.Object) client.Object {
	return &api.RemoteDatabaseNodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *RemoteDatabaseNodeSetResource) GetResourceBuilders() []ResourceBuilder {
	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(resourceBuilders,
		&DatabaseNodeSetBuilder{
			Object: b,

			Name:   b.Name,
			Labels: b.Labels,

			DatabaseNodeSetSpec: b.Spec,
		},
	)
	return resourceBuilders
}

func NewRemoteDatabaseNodeSet(remoteDatabaseNodeSet *api.RemoteDatabaseNodeSet) RemoteDatabaseNodeSetResource {
	crRemoteDatabaseNodeSet := remoteDatabaseNodeSet.DeepCopy()

	return RemoteDatabaseNodeSetResource{RemoteDatabaseNodeSet: crRemoteDatabaseNodeSet}
}

func (b *RemoteDatabaseNodeSetResource) SetPrimaryResourceAnnotations(obj client.Object) {
	annotations := make(map[string]string)
	for key, value := range obj.GetAnnotations() {
		annotations[key] = value
	}

	if _, exist := annotations[ydbannotations.PrimaryResourceStorageAnnotation]; !exist {
		annotations[ydbannotations.PrimaryResourceStorageAnnotation] = b.Spec.DatabaseRef.Name
	}

	obj.SetAnnotations(annotations)
}
