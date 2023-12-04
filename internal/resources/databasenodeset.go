package resources

import (
	"errors"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DatabaseNodeSetBuilder struct {
	*api.DatabaseNodeSet

	Name            string
	StorageEndpoint string

	Labels      map[string]string
	Annotations map[string]string

	DatabaseNodeSetSpec api.DatabaseNodeSetSpec
}

func NewDatabaseNodeSet(databaseNodeSet *api.DatabaseNodeSet) DatabaseNodeSetBuilder {
	crDatabaseNodeSet := databaseNodeSet.DeepCopy()

	return DatabaseNodeSetBuilder{
		Name:                crDatabaseNodeSet.Name,
		Labels:              crDatabaseNodeSet.Labels,
		Annotations:         crDatabaseNodeSet.Annotations,
		DatabaseNodeSetSpec: crDatabaseNodeSet.Spec,
	}
}

func (b *DatabaseNodeSetBuilder) SetStatusOnFirstReconcile() {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}
	}
}

func (b *DatabaseNodeSetBuilder) Unwrap() *api.DatabaseNodeSet {
	return b.DeepCopy()
}

func (b *DatabaseNodeSetBuilder) Build(obj client.Object) error {
	dns, ok := obj.(*api.DatabaseNodeSet)
	if !ok {
		return errors.New("failed to cast to DatabaseNodeSet object")
	}

	if dns.ObjectMeta.Name == "" {
		dns.ObjectMeta.Name = b.Name
	}
	dns.ObjectMeta.Namespace = b.GetNamespace()
	dns.ObjectMeta.Labels = b.Labels
	dns.ObjectMeta.Annotations = b.Annotations

	dns.Spec = b.DatabaseNodeSetSpec

	return nil

}

func (b *DatabaseNodeSetBuilder) Placeholder(cr client.Object) client.Object {
	return &api.DatabaseNodeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.Name,
			Namespace: cr.GetNamespace(),
		},
	}
}

func (b *DatabaseNodeSetBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {

	var resourceBuilders []ResourceBuilder

	resourceBuilders = append(resourceBuilders,
		&DatabaseStatefulSetBuilder{
			Database:   b.RecastDatabaseNodeSet().DeepCopy(),
			RestConfig: restConfig,

			Labels:          b.Labels,
			StorageEndpoint: b.StorageEndpoint,
		},
	)
	return resourceBuilders
}

func (b *DatabaseNodeSetBuilder) RecastDatabaseNodeSet() *api.Database {
	return &api.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.GetName(),
			Namespace: b.GetNamespace(),
			Labels:    b.Labels,
		},
		Spec: api.DatabaseSpec{
			AdditionalAnnotations: b.Annotations,
			Nodes:                 b.DatabaseNodeSetSpec.Nodes,
			Configuration:         b.DatabaseNodeSetSpec.Configuration, // TODO: migrate to configmapRef
			Service:               b.DatabaseNodeSetSpec.Service,       // TODO: export only public host bia externalHost
			Encryption:            b.DatabaseNodeSetSpec.Encryption,
			Volumes:               b.DatabaseNodeSetSpec.Volumes,
			Datastreams:           b.DatabaseNodeSetSpec.Datastreams,
			Resources:             b.DatabaseNodeSetSpec.Resources,
			SharedResources:       b.DatabaseNodeSetSpec.SharedResources,
			//Image:                     b.DatabaseNodeSetSpec.Image,
			InitContainers:            b.DatabaseNodeSetSpec.InitContainers,
			CABundle:                  b.DatabaseNodeSetSpec.CABundle, // TODO: migrate to trust-manager
			Secrets:                   b.DatabaseNodeSetSpec.Secrets,
			NodeSelector:              b.DatabaseNodeSetSpec.NodeSelector,
			Affinity:                  b.DatabaseNodeSetSpec.Affinity,
			Tolerations:               b.DatabaseNodeSetSpec.Tolerations,
			TopologySpreadConstraints: b.DatabaseNodeSetSpec.TopologySpreadConstraints,
			PriorityClassName:         b.DatabaseNodeSetSpec.PriorityClassName,
		},
	}
}
