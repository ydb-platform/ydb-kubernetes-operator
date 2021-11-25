package resources

import (
	"fmt"

	"github.com/go-logr/logr"
	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/metrics"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StorageClusterBuilder struct {
	*api.Storage
	Log logr.Logger
}

func NewCluster(ydbCr *api.Storage, Log logr.Logger) StorageClusterBuilder {
	cr := ydbCr.DeepCopy()

	api.SetStorageClusterSpecDefaults(&cr.Spec)

	return StorageClusterBuilder{cr, Log}
}

func (b *StorageClusterBuilder) SetStatusOnFirstReconcile() {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}
	}
}

func (b *StorageClusterBuilder) Unwrap() *api.Storage {
	return b.DeepCopy()
}

func (b *StorageClusterBuilder) GetGRPCEndpoint() string {
	host := fmt.Sprintf("%s-grpc.%s.svc.cluster.local", b.Name, b.Namespace)
	return fmt.Sprintf("%s:%d", host, api.GRPCPort)
}

func (b *StorageClusterBuilder) GetResourceBuilders() []ResourceBuilder {
	ll := labels.ClusterLabels(b.Unwrap())

	serviceMonitors := make([]ResourceBuilder, len(metrics.GetStorageMetricEndpoints()))

	for i, e := range metrics.GetStorageMetricEndpoints() {
		smLabels := ll.Copy()
		serviceMonitors[i] = &ServiceMonitorBuilder{
			Object: b,
			Name:   fmt.Sprintf("%s-%s", b.Name, e.MonitorName),
			Labels: smLabels,
		}
	}

	if b.Spec.ClusterConfig == "" {
		cfg, err := configuration.Build(b.Unwrap())

		if err != nil {
			b.Log.Error(err, "failed to generate configuration")
		}
		serviceMonitors = append(
			serviceMonitors,
			&ConfigMapBuilder{
				Object: b,
				Data:   cfg,
				Labels: ll,
			},
		)

	}

	return append(
		serviceMonitors,
		&ServiceBuilder{
			Object:         b,
			Labels:         ll.MergeInPlace(b.Spec.Service.GRPC.AdditionalLabels),
			SelectorLabels: ll,
			Annotations:    b.Spec.Service.GRPC.AdditionalAnnotations,
			NameFormat:     grpcServiceNameFormat,
			Ports: []corev1.ServicePort{{
				Name: api.GRPCServicePortName,
				Port: api.GRPCPort,
			}},
			IPFamilies:     b.Spec.Service.GRPC.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.GRPC.IPFamilyPolicy,
		},
		&ServiceBuilder{
			Object:         b,
			Labels:         ll.MergeInPlace(b.Spec.Service.Interconnect.AdditionalLabels),
			SelectorLabels: ll,
			Annotations:    b.Spec.Service.Interconnect.AdditionalAnnotations,
			Headless:       true,
			NameFormat:     interconnectServiceNameFormat,
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
			NameFormat:     statusServiceNameFormat,
			Annotations:    b.Spec.Service.GRPC.AdditionalAnnotations,
			Ports: []corev1.ServicePort{{
				Name: api.StatusServicePortName,
				Port: api.StatusPort,
			}},
			IPFamilies:     b.Spec.Service.Status.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.Status.IPFamilyPolicy,
		},
		&StorageStatefulSetBuilder{Storage: b.Unwrap(), Labels: ll},
	)
}
