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

type DatabaseNodeSetBuilder struct {
	client.Object

	Name        string
	Labels      map[string]string
	Annotations map[string]string

	DatabaseNodeSetSpec api.DatabaseNodeSetSpec
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

type DatabaseNodeSetResource struct {
	*api.DatabaseNodeSet
}

func (b *DatabaseNodeSetResource) NewLabels() labels.Labels {
	l := labels.Common(b.Name, b.Labels)
	l.Merge(map[string]string{labels.ComponentKey: labels.DynamicComponent})

	databaseNodeSetName := b.Labels[labels.DatabaseNodeSetComponent]
	l.Merge(map[string]string{labels.DatabaseNodeSetComponent: databaseNodeSetName})

	if remoteCluster, exist := b.Labels[labels.RemoteClusterKey]; exist {
		l.Merge(map[string]string{labels.RemoteClusterKey: remoteCluster})
	}

	return l
}

func (b *DatabaseNodeSetResource) NewAnnotations() annotations.Annotations {
	an := annotations.Common(b.Annotations)

	return an
}

func (b *DatabaseNodeSetResource) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	nodeSetLabels := b.NewLabels()
	nodeSetAnnotations := b.NewAnnotations()

	statefulSetLabels := nodeSetLabels.Copy()
	statefulSetLabels.Merge(b.Spec.AdditionalLabels)
	statefulSetLabels.Merge(map[string]string{labels.StatefulsetComponent: b.Name})

	statefulSetAnnotations := nodeSetAnnotations.Copy()
	statefulSetAnnotations.Merge(b.Spec.AdditionalAnnotations)

	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(resourceBuilders,
		&DatabaseStatefulSetBuilder{
			Database:   api.RecastDatabaseNodeSet(b.Unwrap()),
			RestConfig: restConfig,

			Name:        b.Name,
			Labels:      statefulSetLabels,
			Annotations: statefulSetAnnotations,
		},
	)
	return resourceBuilders
}

func NewDatabaseNodeSet(databaseNodeSet *api.DatabaseNodeSet) DatabaseNodeSetResource {
	crDatabaseNodeSet := databaseNodeSet.DeepCopy()

	if crDatabaseNodeSet.Spec.Service.Status.TLSConfiguration == nil {
		crDatabaseNodeSet.Spec.Service.Status.TLSConfiguration = &api.TLSConfiguration{Enabled: false}
	}

	return DatabaseNodeSetResource{DatabaseNodeSet: crDatabaseNodeSet}
}

func (b *DatabaseNodeSetResource) Unwrap() *api.DatabaseNodeSet {
	return b.DeepCopy()
}
