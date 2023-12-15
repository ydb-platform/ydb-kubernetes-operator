package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/metrics"
)

type StorageClusterBuilder struct {
	*api.Storage
}

func NewCluster(ydbCr *api.Storage) StorageClusterBuilder {
	cr := ydbCr.DeepCopy()

	return StorageClusterBuilder{cr}
}

func (b *StorageClusterBuilder) SetStatusOnFirstReconcile() {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}
	}
}

func (b *StorageClusterBuilder) Unwrap() *api.Storage {
	return b.DeepCopy()
}

func (b *StorageClusterBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	storageLabels := labels.StorageLabels(b.Unwrap())

	var optionalBuilders []ResourceBuilder

	cfg, _ := configuration.Build(b.Unwrap(), nil)

	optionalBuilders = append(
		optionalBuilders,
		&ConfigMapBuilder{
			Object: b,
			Name:   b.Storage.GetName(),
			Data:   cfg,
			Labels: storageLabels,
		},
	)

	grpcServiceLabels := storageLabels.Copy()
	grpcServiceLabels.Merge(b.Spec.Service.GRPC.AdditionalLabels)
	grpcServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.GRPCComponent})

	interconnectServiceLabels := storageLabels.Copy()
	interconnectServiceLabels.Merge(b.Spec.Service.Interconnect.AdditionalLabels)
	interconnectServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.InterconnectComponent})

	statusServiceLabels := storageLabels.Copy()
	statusServiceLabels.Merge(b.Spec.Service.Status.AdditionalLabels)
	statusServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.StatusComponent})

	if b.Spec.Monitoring.Enabled {
		optionalBuilders = append(optionalBuilders,
			&ServiceMonitorBuilder{
				Object: b,

				TargetPort:      api.StatusPort,
				MetricsServices: metrics.GetStorageMetricsServices(),
				Options:         b.Spec.Monitoring,

				Labels:         storageLabels,
				SelectorLabels: statusServiceLabels,
			},
		)
	}

	if b.Spec.NodeSet == nil {
		optionalBuilders = append(
			optionalBuilders,
			&StorageStatefulSetBuilder{
				Storage:    b.Unwrap(),
				RestConfig: restConfig,

				Name:   b.Name,
				Labels: storageLabels,
			},
		)
	} else {
		for _, nodeSetSpecInline := range b.Spec.NodeSet {
			nodeSetLabels := storageLabels.Copy()
			nodeSetLabels = nodeSetLabels.Merge(nodeSetSpecInline.AdditionalLabels)
			nodeSetLabels = nodeSetLabels.Merge(map[string]string{labels.StorageNodeSetComponent: nodeSetSpecInline.Name})

			optionalBuilders = append(
				optionalBuilders,
				&StorageNodeSetBuilder{
					Object: b,

					Name:   b.Name + "-" + nodeSetSpecInline.Name,
					Labels: nodeSetLabels,

					StorageNodeSetSpec: b.recastStorageNodeSetSpecInline(
						nodeSetSpecInline.DeepCopy(),
						cfg[api.ConfigFileName],
					),
				},
			)
		}
	}

	return append(
		optionalBuilders,
		&ServiceBuilder{
			Object:         b,
			NameFormat:     grpcServiceNameFormat,
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
			Object:         b,
			NameFormat:     interconnectServiceNameFormat,
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
			Object:         b,
			NameFormat:     statusServiceNameFormat,
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
}

func (b *StorageClusterBuilder) recastStorageNodeSetSpecInline(nodeSetSpecInline *api.StorageNodeSetSpecInline, configuration string) api.StorageNodeSetSpec {
	snsSpec := api.StorageNodeSetSpec{}

	snsSpec.StorageRef = api.NamespacedRef{
		Name:      b.Name,
		Namespace: b.Namespace,
	}

	snsSpec.Nodes = nodeSetSpecInline.Nodes
	snsSpec.Configuration = configuration
	snsSpec.Erasure = b.Spec.Erasure

	snsSpec.DataStore = b.Spec.DataStore
	if nodeSetSpecInline.DataStore != nil {
		snsSpec.DataStore = nodeSetSpecInline.DataStore
	}

	snsSpec.Service = b.Spec.Service
	if nodeSetSpecInline.Service != nil {
		snsSpec.Service = *nodeSetSpecInline.Service
	}

	snsSpec.Resources = b.Spec.Resources
	if nodeSetSpecInline.Resources != nil {
		snsSpec.Resources = nodeSetSpecInline.Resources
	}

	snsSpec.Image = b.Spec.Image
	// if nodeSetSpecInline.Image != nil {
	// 	snsSpec.Image = *nodeSetSpecInline.Image.DeepCopy()
	// }

	snsSpec.InitContainers = b.Spec.InitContainers
	snsSpec.CABundle = b.Spec.CABundle
	snsSpec.Secrets = b.Spec.Secrets
	snsSpec.Volumes = b.Spec.Volumes
	snsSpec.HostNetwork = b.Spec.HostNetwork

	snsSpec.NodeSelector = b.Spec.NodeSelector
	if nodeSetSpecInline.NodeSelector != nil {
		snsSpec.NodeSelector = nodeSetSpecInline.NodeSelector
	}

	snsSpec.Affinity = b.Spec.Affinity
	if nodeSetSpecInline.Affinity != nil {
		snsSpec.Affinity = nodeSetSpecInline.Affinity
	}

	snsSpec.Tolerations = b.Spec.Tolerations
	if nodeSetSpecInline.Tolerations != nil {
		snsSpec.Tolerations = nodeSetSpecInline.Tolerations
	}

	snsSpec.TopologySpreadConstraints = b.Spec.TopologySpreadConstraints
	if nodeSetSpecInline.TopologySpreadConstraints != nil {
		snsSpec.TopologySpreadConstraints = nodeSetSpecInline.TopologySpreadConstraints
	}

	snsSpec.AdditionalLabels = make(map[string]string)
	if b.Spec.AdditionalLabels != nil {
		for k, v := range b.Spec.AdditionalLabels {
			snsSpec.AdditionalLabels[k] = v
		}
	}
	if nodeSetSpecInline.AdditionalLabels != nil {
		for k, v := range nodeSetSpecInline.AdditionalLabels {
			snsSpec.AdditionalLabels[k] = v
		}
	}
	snsSpec.AdditionalLabels[labels.StorageNodeSetComponent] = nodeSetSpecInline.Name

	snsSpec.AdditionalAnnotations = make(map[string]string)
	if b.Spec.AdditionalAnnotations != nil {
		for k, v := range b.Spec.AdditionalAnnotations {
			snsSpec.AdditionalAnnotations[k] = v
		}
	}
	if nodeSetSpecInline.AdditionalAnnotations != nil {
		for k, v := range nodeSetSpecInline.AdditionalAnnotations {
			snsSpec.AdditionalAnnotations[k] = v
		}
	}

	snsSpec.PriorityClassName = b.Spec.PriorityClassName
	if nodeSetSpecInline.PriorityClassName != snsSpec.PriorityClassName {
		snsSpec.PriorityClassName = nodeSetSpecInline.PriorityClassName
	}

	return snsSpec
}
