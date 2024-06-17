package resources

import (
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
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

func (b *StorageClusterBuilder) Unwrap() *api.Storage {
	return b.DeepCopy()
}

func (b *StorageClusterBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	storageLabels := labels.StorageLabels(b.Unwrap())

	grpcServiceLabels := storageLabels.Copy()
	grpcServiceLabels.Merge(b.Spec.Service.GRPC.AdditionalLabels)
	grpcServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.GRPCComponent})

	interconnectServiceLabels := storageLabels.Copy()
	interconnectServiceLabels.Merge(b.Spec.Service.Interconnect.AdditionalLabels)
	interconnectServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.InterconnectComponent})

	statusServiceLabels := storageLabels.Copy()
	statusServiceLabels.Merge(b.Spec.Service.Status.AdditionalLabels)
	statusServiceLabels.Merge(map[string]string{labels.ServiceComponent: labels.StatusComponent})

	var optionalBuilders []ResourceBuilder

	dynconfig, err := api.ParseDynconfig(b.Spec.Configuration)
	if err != nil {
		// YDBOPS-9722 backward compatibility
		cfg, _ := api.BuildConfiguration(b.Unwrap(), nil)
		optionalBuilders = append(
			optionalBuilders,
			&ConfigMapBuilder{
				Object: b,
				Name:   b.Storage.GetName(),
				Data: map[string]string{
					api.ConfigFileName: string(cfg),
				},
				Labels: storageLabels,
			},
		)
	} else {
		cfg, _ := yaml.Marshal(dynconfig.Config)
		optionalBuilders = append(
			optionalBuilders,
			&ConfigMapBuilder{
				Object: b,
				Name:   b.Storage.GetName(),
				Data: map[string]string{
					api.ConfigFileName: string(cfg),
				},
				Labels: storageLabels,
			},
		)
	}

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

	if b.Spec.NodeSets == nil {
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
		optionalBuilders = append(optionalBuilders, b.getNodeSetBuilders(storageLabels)...)
	}

	return append(
		optionalBuilders,
		&ServiceBuilder{
			Object:         b,
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
			Object:         b,
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
			Object:         b,
			NameFormat:     StatusServiceNameFormat,
			Labels:         statusServiceLabels,
			SelectorLabels: storageLabels,
			Annotations:    b.Spec.Service.Status.AdditionalAnnotations,
			Ports: []corev1.ServicePort{{
				Name: api.StatusServicePortName,
				Port: api.StatusPort,
			}},
			IPFamilies:     b.Spec.Service.Status.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.Status.IPFamilyPolicy,
		},
	)
}

func (b *StorageClusterBuilder) getNodeSetBuilders(storageLabels labels.Labels) []ResourceBuilder {
	var nodeSetBuilders []ResourceBuilder

	for _, nodeSetSpecInline := range b.Spec.NodeSets {
		nodeSetLabels := storageLabels.Copy()
		nodeSetLabels.Merge(nodeSetSpecInline.Labels)
		nodeSetLabels.Merge(map[string]string{labels.StorageNodeSetComponent: nodeSetSpecInline.Name})

		nodeSetAnnotations := CopyDict(b.Annotations)
		if nodeSetSpecInline.Annotations != nil {
			for k, v := range nodeSetSpecInline.Annotations {
				nodeSetAnnotations[k] = v
			}
		}

		storageNodeSetSpec := b.recastStorageNodeSetSpecInline(nodeSetSpecInline.DeepCopy())
		if nodeSetSpecInline.Remote != nil {
			nodeSetLabels = nodeSetLabels.Merge(map[string]string{
				labels.RemoteClusterKey: nodeSetSpecInline.Remote.Cluster,
			})
			nodeSetBuilders = append(
				nodeSetBuilders,
				&RemoteStorageNodeSetBuilder{
					Object: b,

					Name:        b.Name + "-" + nodeSetSpecInline.Name,
					Labels:      nodeSetLabels,
					Annotations: nodeSetAnnotations,

					StorageNodeSetSpec: storageNodeSetSpec,
				},
			)
		} else {
			nodeSetBuilders = append(
				nodeSetBuilders,
				&StorageNodeSetBuilder{
					Object: b,

					Name:        b.Name + "-" + nodeSetSpecInline.Name,
					Labels:      nodeSetLabels,
					Annotations: nodeSetAnnotations,

					StorageNodeSetSpec: storageNodeSetSpec,
				},
			)
		}
	}

	return nodeSetBuilders
}

func (b *StorageClusterBuilder) recastStorageNodeSetSpecInline(nodeSetSpecInline *api.StorageNodeSetSpecInline) api.StorageNodeSetSpec {
	nodeSetSpec := api.StorageNodeSetSpec{}

	nodeSetSpec.StorageRef = api.NamespacedRef{
		Name:      b.Name,
		Namespace: b.Namespace,
	}

	nodeSetSpec.StorageClusterSpec = b.Spec.StorageClusterSpec
	nodeSetSpec.StorageNodeSpec = b.Spec.StorageNodeSpec

	nodeSetSpec.Nodes = nodeSetSpecInline.Nodes

	if nodeSetSpecInline.DataStore != nil {
		nodeSetSpec.DataStore = nodeSetSpecInline.DataStore
	}

	if nodeSetSpecInline.Resources != nil {
		nodeSetSpec.Resources = nodeSetSpecInline.Resources
	}

	nodeSetSpec.NodeSelector = CopyDict(b.Spec.NodeSelector)
	if nodeSetSpecInline.NodeSelector != nil {
		for k, v := range nodeSetSpecInline.NodeSelector {
			nodeSetSpec.NodeSelector[k] = v
		}
	}

	if nodeSetSpecInline.Affinity != nil {
		nodeSetSpec.Affinity = nodeSetSpecInline.Affinity
	}

	if nodeSetSpecInline.Tolerations != nil {
		nodeSetSpec.Tolerations = append(nodeSetSpec.Tolerations, nodeSetSpecInline.Tolerations...)
	}

	if nodeSetSpecInline.TopologySpreadConstraints != nil {
		nodeSetSpec.TopologySpreadConstraints = append(nodeSetSpec.TopologySpreadConstraints, nodeSetSpecInline.TopologySpreadConstraints...)
	}

	nodeSetSpec.AdditionalLabels = CopyDict(b.Spec.AdditionalLabels)
	if nodeSetSpecInline.AdditionalLabels != nil {
		for k, v := range nodeSetSpecInline.AdditionalLabels {
			nodeSetSpec.AdditionalLabels[k] = v
		}
	}

	nodeSetSpec.AdditionalAnnotations = CopyDict(b.Spec.AdditionalAnnotations)
	if nodeSetSpecInline.AdditionalAnnotations != nil {
		for k, v := range nodeSetSpecInline.AdditionalAnnotations {
			nodeSetSpec.AdditionalAnnotations[k] = v
		}
	}

	return nodeSetSpec
}
