package resources

import (
	"fmt"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DatabaseBuilder struct {
	*api.Database
}

func NewDatabase(ydbCr *api.Database) DatabaseBuilder {
	cr := ydbCr.DeepCopy()

	api.SetDatabaseSpecDefaults(cr, &cr.Spec)

	return DatabaseBuilder{cr}
}

func (b *DatabaseBuilder) SetStatusOnFirstReconcile() bool {
	changed := false
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}
		changed = true
	}
	return changed
}

func (b *DatabaseBuilder) Unwrap() *api.Database {
	return b.DeepCopy()
}

func (b *DatabaseBuilder) GetStorageEndpointWithProto() string {
	proto := api.GRPCProto
	if b.StorageRef.Spec.Service.GRPC.TLSConfiguration != nil && b.StorageRef.Spec.Service.GRPC.TLSConfiguration.Enabled {
		proto = api.GRPCSProto
	}

	return fmt.Sprintf("%s%s", proto, b.GetStorageEndpoint())
}

func (b *DatabaseBuilder) GetStorageEndpoint() string {
	host := fmt.Sprintf("%s-grpc.%s.svc.cluster.local", b.Spec.StorageClusterRef.Name, b.Spec.StorageClusterRef.Namespace)
	if b.StorageRef.Spec.Service.GRPC.ExternalHost != "" {
		host = b.StorageRef.Spec.Service.GRPC.ExternalHost
	}

	return fmt.Sprintf("%s:%d", host, api.GRPCPort)
}

func (b *DatabaseBuilder) GetPath() string {
	return fmt.Sprintf(api.TenantNameFormat, b.Spec.Domain, b.Name)
}

func (b *DatabaseBuilder) GetResourceBuilders() ([]ResourceBuilder, error) {
	ll := labels.DatabaseLabels(b.Unwrap())

	var optionalBuilders []ResourceBuilder

	cfg, err := configuration.Build(b.StorageRef)
	if err != nil {
		return []ResourceBuilder{}, err
	}

	optionalBuilders = append(
		optionalBuilders,
		&ConfigMapBuilder{
			Object: b,
			Data:   cfg,
			Labels: ll,
		},
	)

	if b.Spec.ServerlessResources != nil {
		return []ResourceBuilder{}, nil
	}
	return append(
		optionalBuilders,
		&ServiceBuilder{
			Object:         b,
			NameFormat:     grpcServiceNameFormat,
			Labels:         ll.MergeInPlace(b.Spec.Service.GRPC.AdditionalLabels),
			SelectorLabels: ll,
			Annotations:    b.Spec.Service.GRPC.AdditionalAnnotations,
			Ports: []corev1.ServicePort{{
				Name: api.GRPCServicePortName,
				Port: api.GRPCPort,
			}},
			IPFamilies:     b.Spec.Service.GRPC.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.GRPC.IPFamilyPolicy,
		},
		&ServiceBuilder{
			Object:         b,
			NameFormat:     interconnectServiceNameFormat,
			Labels:         ll.MergeInPlace(b.Spec.Service.Interconnect.AdditionalLabels),
			SelectorLabels: ll,
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
			Object:         b,
			NameFormat:     statusServiceNameFormat,
			Labels:         ll.MergeInPlace(b.Spec.Service.Status.AdditionalLabels),
			SelectorLabels: ll,
			Annotations:    b.Spec.Service.Status.AdditionalAnnotations,
			Ports: []corev1.ServicePort{{
				Name: api.StatusServicePortName,
				Port: api.StatusPort,
			}},
			IPFamilies:     b.Spec.Service.Status.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.Status.IPFamilyPolicy,
		},
		&DatabaseStatefulSetBuilder{Database: b.Unwrap(), Labels: ll},
	), nil
}
