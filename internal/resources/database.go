package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/metrics"
)

type DatabaseBuilder struct {
	*api.Database
	Storage *api.Storage
}

func NewDatabase(ydbCr *api.Database) DatabaseBuilder {
	cr := ydbCr.DeepCopy()

	return DatabaseBuilder{Database: cr, Storage: nil}
}

func (b *DatabaseBuilder) SetStatusOnFirstReconcile() {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}
	}
}

func (b *DatabaseBuilder) Unwrap() *api.Database {
	return b.DeepCopy()
}

func (b *DatabaseBuilder) GetStorageEndpointWithProto() string {
	proto := api.GRPCProto
	if b.IsStorageEndpointSecure() {
		proto = api.GRPCSProto
	}

	return fmt.Sprintf("%s%s", proto, b.GetStorageEndpoint())
}

func (b *DatabaseBuilder) GetStorageEndpoint() string {
	host := fmt.Sprintf(api.GRPCServiceFQDNFormat, b.Spec.StorageClusterRef.Name, b.Spec.StorageClusterRef.Namespace)
	if b.Storage.Spec.Service.GRPC.ExternalHost != "" {
		host = b.Storage.Spec.Service.GRPC.ExternalHost
	}

	return fmt.Sprintf("%s:%d", host, api.GRPCPort)
}

func (b *DatabaseBuilder) IsStorageEndpointSecure() bool {
	return b.Storage.Spec.Service.GRPC.TLSConfiguration.Enabled
}

func (b *DatabaseBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	if b.Spec.ServerlessResources != nil {
		return []ResourceBuilder{}
	}

	databaseLabels := labels.DatabaseLabels(b.Unwrap())

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

	var optionalBuilders []ResourceBuilder

	cfg, _ := configuration.Build(b.Storage, b.Unwrap())

	optionalBuilders = append(
		optionalBuilders,
		&ConfigMapBuilder{
			Object: b,
			Name:   b.GetName(),
			Data:   cfg,
			Labels: databaseLabels,
		},
	)

	if b.Spec.Monitoring != nil && b.Spec.Monitoring.Enabled {
		optionalBuilders = append(optionalBuilders,
			&ServiceMonitorBuilder{
				Object: b,

				TargetPort:      api.StatusPort,
				MetricsServices: metrics.GetDatabaseMetricsServices(),
				Options:         b.Spec.Monitoring,

				Labels:         databaseLabels,
				SelectorLabels: statusServiceLabels,
			},
		)
	}

	if b.Spec.Encryption != nil && b.Spec.Encryption.Enabled && b.Spec.Encryption.Key == nil {
		var pin string
		if b.Spec.Encryption.Pin == nil || len(*b.Spec.Encryption.Pin) == 0 {
			pin = defaultPin
		} else {
			pin = *b.Spec.Encryption.Pin
		}
		optionalBuilders = append(
			optionalBuilders,
			&EncryptionSecretBuilder{
				Object: b,
				Labels: databaseLabels,
				Pin:    pin,
			},
		)
	}

	optionalBuilders = append(
		optionalBuilders,
		&ServiceBuilder{
			Object:         b,
			NameFormat:     grpcServiceNameFormat,
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
			Object:         b,
			NameFormat:     interconnectServiceNameFormat,
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
			Object:         b,
			NameFormat:     statusServiceNameFormat,
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
		optionalBuilders = append(
			optionalBuilders,
			&ServiceBuilder{
				Object:         b,
				NameFormat:     datastreamsServiceNameFormat,
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

	if b.Spec.NodeSet == nil {
		optionalBuilders = append(
			optionalBuilders,
			&DatabaseStatefulSetBuilder{
				Database:   b.Unwrap(),
				RestConfig: restConfig,

				Name:            b.Name,
				Labels:          databaseLabels,
				StorageEndpoint: b.GetStorageEndpointWithProto(),
			},
		)
	} else {
		for _, nodeSetSpecInline := range b.Spec.NodeSet {
			nodeSetLabels := databaseLabels.Copy()
			nodeSetLabels = nodeSetLabels.Merge(nodeSetSpecInline.AdditionalLabels)
			nodeSetLabels = nodeSetLabels.Merge(map[string]string{labels.DatabaseNodeSetComponent: nodeSetSpecInline.Name})

			optionalBuilders = append(
				optionalBuilders,
				&DatabaseNodeSetBuilder{
					Object: b,

					Name:   b.Name + "-" + nodeSetSpecInline.Name,
					Labels: nodeSetLabels,

					DatabaseNodeSetSpec: b.recastDatabaseNodeSetSpecInline(
						nodeSetSpecInline.DeepCopy(),
						cfg[api.ConfigFileName],
					),
				},
			)
		}
	}

	return optionalBuilders
}

func (b *DatabaseBuilder) recastDatabaseNodeSetSpecInline(nodeSetSpecInline *api.DatabaseNodeSetSpecInline, configuration string) api.DatabaseNodeSetSpec {
	dnsSpec := api.DatabaseNodeSetSpec{}

	dnsSpec.DatabaseRef = api.NamespacedRef{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
	dnsSpec.StorageEndpoint = b.GetStorageEndpointWithProto()

	dnsSpec.Nodes = nodeSetSpecInline.Nodes
	dnsSpec.Configuration = configuration

	dnsSpec.Service = b.Spec.Service
	if nodeSetSpecInline.Service != nil {
		dnsSpec.Service = *nodeSetSpecInline.Service.DeepCopy()
	}
	dnsSpec.StorageDomains = b.Spec.StorageDomains
	dnsSpec.Encryption = b.Spec.Encryption
	dnsSpec.Volumes = b.Spec.Volumes
	dnsSpec.Datastreams = b.Spec.Datastreams
	dnsSpec.Domain = b.Spec.Domain
	dnsSpec.Path = b.Spec.Path

	dnsSpec.Resources = b.Spec.Resources
	if nodeSetSpecInline.Resources != nil {
		dnsSpec.Resources = nodeSetSpecInline.Resources
	}

	dnsSpec.SharedResources = b.Spec.SharedResources
	if nodeSetSpecInline.SharedResources != nil {
		dnsSpec.SharedResources = nodeSetSpecInline.SharedResources
	}

	dnsSpec.ServerlessResources = b.Spec.ServerlessResources
	dnsSpec.Image = b.Spec.Image
	// if nodeSetSpecInline.Image != nil {
	// 	dnsSpec.Image = *nodeSetSpecInline.Image.DeepCopy()
	// }

	dnsSpec.InitContainers = b.Spec.InitContainers
	dnsSpec.CABundle = b.Spec.CABundle
	dnsSpec.Secrets = b.Spec.Secrets

	dnsSpec.NodeSelector = b.Spec.NodeSelector
	if nodeSetSpecInline.NodeSelector != nil {
		dnsSpec.NodeSelector = nodeSetSpecInline.NodeSelector
	}

	dnsSpec.Affinity = b.Spec.Affinity
	if nodeSetSpecInline.Affinity != nil {
		dnsSpec.Affinity = nodeSetSpecInline.Affinity
	}

	dnsSpec.Tolerations = b.Spec.Tolerations
	if nodeSetSpecInline.Tolerations != nil {
		dnsSpec.Tolerations = nodeSetSpecInline.Tolerations
	}

	dnsSpec.TopologySpreadConstraints = b.Spec.TopologySpreadConstraints
	if nodeSetSpecInline.TopologySpreadConstraints != nil {
		dnsSpec.TopologySpreadConstraints = nodeSetSpecInline.TopologySpreadConstraints
	}

	dnsSpec.AdditionalLabels = make(map[string]string)
	if b.Spec.AdditionalLabels != nil {
		for k, v := range b.Spec.AdditionalLabels {
			dnsSpec.AdditionalLabels[k] = v
		}
	}
	if nodeSetSpecInline.AdditionalLabels != nil {
		for k, v := range nodeSetSpecInline.AdditionalLabels {
			dnsSpec.AdditionalLabels[k] = v
		}
	}
	dnsSpec.AdditionalLabels[labels.StorageNodeSetComponent] = nodeSetSpecInline.Name

	dnsSpec.AdditionalAnnotations = make(map[string]string)
	if b.Spec.AdditionalAnnotations != nil {
		for k, v := range b.Spec.AdditionalAnnotations {
			dnsSpec.AdditionalAnnotations[k] = v
		}
	}
	if nodeSetSpecInline.AdditionalAnnotations != nil {
		for k, v := range nodeSetSpecInline.AdditionalAnnotations {
			dnsSpec.AdditionalAnnotations[k] = v
		}
	}

	dnsSpec.PriorityClassName = b.Spec.PriorityClassName
	if nodeSetSpecInline.PriorityClassName != dnsSpec.PriorityClassName {
		dnsSpec.PriorityClassName = nodeSetSpecInline.PriorityClassName
	}

	return dnsSpec
}
