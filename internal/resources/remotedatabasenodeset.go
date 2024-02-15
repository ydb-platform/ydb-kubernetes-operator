package resources

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
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

	database := b.recastRemoteDatabaseNodeSet()
	databaseLabels := labels.DatabaseLabels(&database)

	grpcServiceLabels := databaseLabels.Copy()
	grpcServiceLabels.Merge(b.Spec.Service.GRPC.AdditionalLabels)
	grpcServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.GRPCComponent})

	interconnectServiceLabels := databaseLabels.Copy()
	interconnectServiceLabels.Merge(b.Spec.Service.Interconnect.AdditionalLabels)
	interconnectServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.InterconnectComponent})

	statusServiceLabels := databaseLabels.Copy()
	statusServiceLabels.Merge(b.Spec.Service.Status.AdditionalLabels)
	statusServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.StatusComponent})

	datastreamsServiceLabels := databaseLabels.Copy()
	datastreamsServiceLabels.Merge(b.Spec.Service.Datastreams.AdditionalLabels)
	datastreamsServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.DatastreamsComponent})

	resourceBuilders = append(resourceBuilders,
		&DatabaseNodeSetBuilder{
			Object: b,

			Name:   b.Name,
			Labels: b.Labels,

			DatabaseNodeSetSpec: b.Spec,
		},
		&ServiceBuilder{
			Object:         &database,
			NameFormat:     GRPCServiceNameFormat,
			Labels:         grpcServiceLabels,
			SelectorLabels: databaseLabels,
			Annotations:    b.Spec.Service.GRPC.AdditionalAnnotations,
			Ports: []corev1.ServicePort{{
				Name: api.GRPCServicePortName,
				Port: api.GRPCPort,
			}},
			IPFamilies:     b.Spec.Service.GRPC.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.GRPC.IPFamilyPolicy,
		},
		&ServiceBuilder{
			Object:         &database,
			NameFormat:     InterconnectServiceNameFormat,
			Labels:         interconnectServiceLabels,
			SelectorLabels: databaseLabels,
			Annotations:    b.Spec.Service.Interconnect.AdditionalAnnotations,
			Headless:       true,
			Ports: []corev1.ServicePort{{
				Name: api.InterconnectServicePortName,
				Port: api.InterconnectPort,
			}},
			IPFamilies:     b.Spec.Service.Interconnect.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.Interconnect.IPFamilyPolicy,
		},
		&ServiceBuilder{
			Object:         &database,
			NameFormat:     StatusServiceNameFormat,
			Labels:         statusServiceLabels,
			SelectorLabels: databaseLabels,
			Annotations:    b.Spec.Service.Status.AdditionalAnnotations,
			Ports: []corev1.ServicePort{{
				Name: api.StatusServicePortName,
				Port: api.StatusPort,
			}},
			IPFamilies:     b.Spec.Service.Status.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.Status.IPFamilyPolicy,
		},
	)

	if b.Spec.Datastreams != nil && b.Spec.Datastreams.Enabled {
		resourceBuilders = append(
			resourceBuilders,
			&ServiceBuilder{
				Object:         &database,
				NameFormat:     DatastreamsServiceNameFormat,
				Labels:         datastreamsServiceLabels,
				SelectorLabels: databaseLabels,
				Annotations:    b.Spec.Service.Datastreams.AdditionalAnnotations,
				Ports: []corev1.ServicePort{{
					Name: api.DatastreamsServicePortName,
					Port: api.DatastreamsPort,
				}},
				IPFamilies:     b.Spec.Service.Datastreams.IPFamilies,
				IPFamilyPolicy: b.Spec.Service.Datastreams.IPFamilyPolicy,
			},
		)
	}

	return resourceBuilders
}

func NewRemoteDatabaseNodeSet(remoteDatabaseNodeSet *api.RemoteDatabaseNodeSet) RemoteDatabaseNodeSetResource {
	crRemoteDatabaseNodeSet := remoteDatabaseNodeSet.DeepCopy()

	return RemoteDatabaseNodeSetResource{RemoteDatabaseNodeSet: crRemoteDatabaseNodeSet}
}

func (b *RemoteDatabaseNodeSetResource) GetRemoteResources() []client.Object {
	objects := []client.Object{}
	for _, secret := range b.Spec.Secrets {
		objects = append(objects,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret.Name,
					Namespace: b.Namespace,
				},
			})
	}
	return objects
}

func (b *RemoteDatabaseNodeSetResource) recastRemoteDatabaseNodeSet() api.Database {
	return api.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.RemoteDatabaseNodeSet.Spec.DatabaseRef.Name,
			Namespace: b.RemoteDatabaseNodeSet.Spec.DatabaseRef.Namespace,
			Labels:    b.RemoteDatabaseNodeSet.Labels,
		},
		Spec: api.DatabaseSpec{
			DatabaseClusterSpec: b.RemoteDatabaseNodeSet.Spec.DatabaseClusterSpec,
			DatabaseNodeSpec:    b.RemoteDatabaseNodeSet.Spec.DatabaseNodeSpec,
		},
	}
}
