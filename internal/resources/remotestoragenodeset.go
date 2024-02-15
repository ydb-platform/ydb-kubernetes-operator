package resources

import (
	"errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
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

	storage := b.recastRemoteStorageNodeSet()
	storageLabels := labels.StorageLabels(&storage)

	grpcServiceLabels := storageLabels.Copy()
	grpcServiceLabels.Merge(b.Spec.Service.GRPC.AdditionalLabels)
	grpcServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.GRPCComponent})

	interconnectServiceLabels := storageLabels.Copy()
	interconnectServiceLabels.Merge(b.Spec.Service.Interconnect.AdditionalLabels)
	interconnectServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.InterconnectComponent})

	statusServiceLabels := storageLabels.Copy()
	statusServiceLabels.Merge(b.Spec.Service.Status.AdditionalLabels)
	statusServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.StatusComponent})

	resourceBuilders = append(resourceBuilders,
		&StorageNodeSetBuilder{
			Object: b,

			Name:   b.Name,
			Labels: b.Labels,

			StorageNodeSetSpec: b.Spec,
		},
		&ServiceBuilder{
			Object:         &storage,
			NameFormat:     GRPCServiceNameFormat,
			Labels:         grpcServiceLabels,
			SelectorLabels: storageLabels,
			Annotations:    b.Spec.Service.GRPC.AdditionalAnnotations,
			Ports: []corev1.ServicePort{{
				Name: api.GRPCServicePortName,
				Port: api.GRPCPort,
			}},
			IPFamilies:     b.Spec.Service.GRPC.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.GRPC.IPFamilyPolicy,
		},
		&ServiceBuilder{
			Object:         &storage,
			NameFormat:     InterconnectServiceNameFormat,
			Labels:         interconnectServiceLabels,
			SelectorLabels: storageLabels,
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
			Object:         &storage,
			NameFormat:     StatusServiceNameFormat,
			Labels:         statusServiceLabels,
			SelectorLabels: storageLabels,
			Annotations:    b.Spec.Service.GRPC.AdditionalAnnotations,
			Ports: []corev1.ServicePort{{
				Name: api.StatusServicePortName,
				Port: api.StatusPort,
			}},
			IPFamilies:     b.Spec.Service.Status.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.Status.IPFamilyPolicy,
		},
	)
	return resourceBuilders
}

func NewRemoteStorageNodeSet(remoteStorageNodeSet *api.RemoteStorageNodeSet) RemoteStorageNodeSetResource {
	crRemoteStorageNodeSet := remoteStorageNodeSet.DeepCopy()

	return RemoteStorageNodeSetResource{crRemoteStorageNodeSet}
}

func (b *RemoteStorageNodeSetResource) GetRemoteResources() []client.Object {
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

func (b *RemoteStorageNodeSetResource) recastRemoteStorageNodeSet() api.Storage {
	return api.Storage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.RemoteStorageNodeSet.Spec.StorageRef.Name,
			Namespace: b.RemoteStorageNodeSet.Spec.StorageRef.Namespace,
			Labels:    b.RemoteStorageNodeSet.Labels,
		},
		Spec: api.StorageSpec{
			StorageClusterSpec: b.RemoteStorageNodeSet.Spec.StorageClusterSpec,
			StorageNodeSpec:    b.RemoteStorageNodeSet.Spec.StorageNodeSpec,
		},
	}
}
