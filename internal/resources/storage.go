package resources

import (
	"fmt"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/metrics"
)

type StorageClusterBuilder struct {
	*api.Storage
}

func NewCluster(ydbCr *api.Storage) StorageClusterBuilder {
	cr := ydbCr.DeepCopy()

	if cr.Spec.Service.Status.TLSConfiguration == nil {
		cr.Spec.Service.Status.TLSConfiguration = &api.TLSConfiguration{Enabled: false}
	}

	return StorageClusterBuilder{cr}
}

func (b *StorageClusterBuilder) Unwrap() *api.Storage {
	return b.DeepCopy()
}

func (b *StorageClusterBuilder) buildSelectorLabels(component string) labels.Labels {
	l := labels.Common(b.Name, b.Labels)
	l.Merge(map[string]string{labels.ComponentKey: component})

	return l
}

func (b *StorageClusterBuilder) buildLabels() labels.Labels {
	l := b.buildSelectorLabels(labels.StorageComponent)
	l.Merge(b.Spec.AdditionalLabels)

	return l
}

func (b *StorageClusterBuilder) buildInitJobLabels() labels.Labels {
	l := b.buildSelectorLabels(labels.BlobstorageInitComponent)
	l.Merge(b.Spec.AdditionalLabels)

	if b.Spec.InitJob != nil {
		l.Merge(b.Spec.InitJob.AdditionalLabels)
	}

	return l
}

func (b *StorageClusterBuilder) buildNodeSetLabels(nodeSetSpecInline api.StorageNodeSetSpecInline) labels.Labels {
	l := b.buildLabels()
	l.Merge(nodeSetSpecInline.Labels)
	l.Merge(map[string]string{labels.StorageNodeSetComponent: nodeSetSpecInline.Name})
	if nodeSetSpecInline.Remote != nil {
		l.Merge(map[string]string{labels.RemoteClusterKey: nodeSetSpecInline.Remote.Cluster})
	}

	return l
}

func (b *StorageClusterBuilder) GetInitJobBuilder() ResourceBuilder {
	jobName := fmt.Sprintf(InitJobNameFormat, b.Name)
	jobLabels := b.buildInitJobLabels()

	jobAnnotations := CopyDict(b.Annotations)
	if b.Spec.InitJob != nil {
		for k, v := range b.Spec.InitJob.AdditionalAnnotations {
			jobAnnotations[k] = v
		}
	}

	return &StorageInitJobBuilder{
		Storage: b.Unwrap(),

		Name:        jobName,
		Labels:      jobLabels,
		Annotations: jobAnnotations,
	}
}

func (b *StorageClusterBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	storageLabels := b.buildLabels()
	storageSelectorLabels := b.buildSelectorLabels(labels.StorageComponent)

	statefulSetAnnotations := CopyDict(b.Spec.AdditionalAnnotations)
	statefulSetAnnotations[annotations.ConfigurationChecksum] = SHAChecksum(b.Spec.Configuration)

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

	success, dynconfig, _ := api.ParseDynConfig(b.Spec.Configuration)
	if !success {
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

				Name:        b.Name,
				Labels:      storageLabels,
				Annotations: statefulSetAnnotations,
			},
		)
	} else {
		optionalBuilders = append(optionalBuilders, b.getNodeSetBuilders()...)
	}

	return append(
		optionalBuilders,
		&ServiceBuilder{
			Object:         b,
			NameFormat:     GRPCServiceNameFormat,
			Labels:         grpcServiceLabels,
			SelectorLabels: storageSelectorLabels,
			Annotations:    b.Spec.Service.GRPC.AdditionalAnnotations,
			Ports:          b.buildGrpcServicePorts(),
			IPFamilies:     b.Spec.Service.GRPC.IPFamilies,
			IPFamilyPolicy: b.Spec.Service.GRPC.IPFamilyPolicy,
		},
		&ServiceBuilder{
			Object:         b,
			NameFormat:     InterconnectServiceNameFormat,
			Labels:         interconnectServiceLabels,
			SelectorLabels: storageSelectorLabels,
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
			SelectorLabels: storageSelectorLabels,
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

func (b *StorageClusterBuilder) buildGrpcServicePorts() []corev1.ServicePort {
	ports := []corev1.ServicePort{{
		Name: api.GRPCServicePortName,
		Port: api.GRPCPort,
	}}

	if b.Spec.Service.GRPC.AdditionalPort != 0 {
		ports = append(ports, corev1.ServicePort{
			Name: api.GRPCServiceAdditionalPortName,
			Port: b.Spec.Service.GRPC.AdditionalPort,
		})
	}

	return ports
}

func (b *StorageClusterBuilder) getNodeSetBuilders() []ResourceBuilder {
	var nodeSetBuilders []ResourceBuilder

	for _, nodeSetSpecInline := range b.Spec.NodeSets {
		nodeSetName := fmt.Sprintf("%s-%s", b.Name, nodeSetSpecInline.Name)
		nodeSetLabels := b.buildNodeSetLabels(nodeSetSpecInline)

		nodeSetAnnotations := CopyDict(b.Annotations)
		if nodeSetSpecInline.Annotations != nil {
			for k, v := range nodeSetSpecInline.Annotations {
				nodeSetAnnotations[k] = v
			}
		}

		storageNodeSetSpec := b.recastStorageNodeSetSpecInline(nodeSetSpecInline.DeepCopy())
		if nodeSetSpecInline.Remote != nil {
			nodeSetBuilders = append(
				nodeSetBuilders,
				&RemoteStorageNodeSetBuilder{
					Object: b,

					Name:        nodeSetName,
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

					Name:        nodeSetName,
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
