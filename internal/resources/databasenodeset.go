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

type DatabaseNodeSetResource struct {
	*api.DatabaseNodeSet
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

func (b *DatabaseNodeSetResource) buildLabels() labels.Labels {
	l := labels.Common(b.Name, b.Labels)
	l.Merge(b.Spec.AdditionalLabels)
	l.Merge(map[string]string{labels.ComponentKey: labels.DynamicComponent})
	if nodeSetName, exist := b.Labels[labels.DatabaseNodeSetComponent]; exist {
		l.Merge(map[string]string{labels.DatabaseNodeSetComponent: nodeSetName})
	}
	if remoteCluster, exist := b.Labels[labels.RemoteClusterKey]; exist {
		l.Merge(map[string]string{labels.RemoteClusterKey: remoteCluster})
	}

	return l
}

func (b *DatabaseNodeSetResource) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	ydbCr := api.RecastDatabaseNodeSet(b.Unwrap())
	databaseNodeSetLabels := b.buildLabels()

	statefulSetName := b.Name
	statefulSetLabels := databaseNodeSetLabels.Copy()
	statefulSetLabels.Merge(map[string]string{labels.StatefulsetComponent: statefulSetName})

	statefulSetAnnotations := CopyDict(b.Spec.AdditionalAnnotations)
	statefulSetAnnotations[annotations.ConfigurationChecksum] = GetConfigurationChecksum(b.Spec.Configuration)

	var resourceBuilders []ResourceBuilder
	resourceBuilders = append(resourceBuilders,
		&DatabaseStatefulSetBuilder{
			Database:   ydbCr,
			RestConfig: restConfig,

			Name:        statefulSetName,
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
