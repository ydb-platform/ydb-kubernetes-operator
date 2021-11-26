package resources

import (
	"fmt"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
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

func (b *DatabaseBuilder) SetStatusOnFirstReconcile() {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}
	}
}

func (b *DatabaseBuilder) Unwrap() *api.Database {
	return b.DeepCopy()
}

func (b *DatabaseBuilder) GetStorageEndpoint() string {
	host := fmt.Sprintf("%s-grpc.%s.svc.cluster.local", b.Spec.StorageClusterRef.Name, b.Spec.StorageClusterRef.Namespace)

	return fmt.Sprintf("%s:%d", host, api.GRPCPort)
}

func (b *DatabaseBuilder) GetTenantName() string {
	return fmt.Sprintf(api.TenantNameFormat, b.Spec.Domain, b.Name)
}

func (b *DatabaseBuilder) GetResourceBuilders() []ResourceBuilder {
	ll := labels.DatabaseLabels(b.Unwrap())

	return []ResourceBuilder{
		&ServiceBuilder{
			Object:         b,
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
			Labels:         ll.MergeInPlace(b.Spec.Service.Status.AdditionalLabels),
			SelectorLabels: ll,
			Annotations:    b.Spec.Service.Status.AdditionalAnnotations,
			NameFormat:     statusServiceNameFormat,
			Ports: []corev1.ServicePort{{
				Name: api.StatusServicePortName,
				Port: api.StatusPort,
			}},
			IPFamilies:     b.Spec.Service.Status.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.Status.IPFamilyPolicy,
		},
		&DatabaseStatefulSetBuilder{Database: b.Unwrap(), Labels: ll},
	}
}
