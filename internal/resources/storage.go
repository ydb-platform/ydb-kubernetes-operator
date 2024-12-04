package resources

import (
	"fmt"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
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

func (b *StorageClusterBuilder) NewLabels() labels.Labels {
	l := labels.Common(b.Name, b.Labels)
	l.Merge(map[string]string{labels.ComponentKey: labels.StorageComponent})

	return l
}

func (b *StorageClusterBuilder) NewAnnotations() ydbannotations.Annotations {
	annotations := ydbannotations.Common(b.Annotations)
	annotations.Merge(map[string]string{ydbannotations.ConfigurationChecksum: GetSHA256Checksum(b.Spec.Configuration)})

	return annotations
}

func (b *StorageClusterBuilder) NewInitJobLabels() labels.Labels {
	l := labels.Common(b.Name, b.Labels)

	if b.Spec.InitJob != nil {
		l.Merge(b.Spec.InitJob.AdditionalLabels)
	}
	l.Merge(map[string]string{labels.ComponentKey: labels.BlobstorageInitComponent})

	return l
}

func (b *StorageClusterBuilder) NewInitJobAnnotations() ydbannotations.Annotations {
	annotations := ydbannotations.Common(b.Annotations)

	if b.Spec.InitJob != nil {
		annotations.Merge(b.Spec.InitJob.AdditionalLabels)
	}
	annotations.Merge(map[string]string{ydbannotations.ConfigurationChecksum: GetSHA256Checksum(b.Spec.Configuration)})

	return annotations
}

func (b *StorageClusterBuilder) GetInitJobBuilder() ResourceBuilder {
	jobName := fmt.Sprintf(InitJobNameFormat, b.Name)
	jobLabels := b.NewInitJobLabels()
	jobAnnotations := b.NewInitJobAnnotations()

	return &StorageInitJobBuilder{
		Storage: b.Unwrap(),

		Name:        jobName,
		Labels:      jobLabels,
		Annotations: jobAnnotations,
	}
}

func (b *StorageClusterBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	storageLabels := b.NewLabels()
	storageAnnotations := b.NewAnnotations()

	statefulSetLabels := storageLabels.Copy()
	statefulSetLabels.Merge(b.Spec.AdditionalLabels)
	statefulSetLabels.Merge(map[string]string{labels.StatefulsetComponent: b.Name})

	statefulSetAnnotations := storageAnnotations.Copy()
	statefulSetAnnotations.Merge(b.Spec.AdditionalAnnotations)

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
				Labels:      statefulSetLabels,
				Annotations: statefulSetAnnotations,
			},
		)
	} else {
		optionalBuilders = append(
			optionalBuilders,
			b.getNodeSetBuilders(storageLabels, storageAnnotations)...,
		)
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

func (b *StorageClusterBuilder) getNodeSetBuilders(
	storageLabels labels.Labels,
	storageAnnotations ydbannotations.Annotations,
) []ResourceBuilder {
	var nodeSetBuilders []ResourceBuilder

	for _, nodeSetSpecInline := range b.Spec.NodeSets {
		nodeSetName := fmt.Sprintf("%s-%s", b.Name, nodeSetSpecInline.Name)

		nodeSetLabels := storageLabels.Copy()
		nodeSetLabels.Merge(nodeSetSpecInline.Labels)
		nodeSetLabels.Merge(map[string]string{labels.StorageNodeSetComponent: nodeSetSpecInline.Name})
		if nodeSetSpecInline.Remote != nil {
			nodeSetLabels.Merge(map[string]string{labels.RemoteClusterKey: nodeSetSpecInline.Remote.Cluster})
		}

		nodeSetAnnotations := storageAnnotations.Copy()
		nodeSetAnnotations.Merge(nodeSetSpecInline.Annotations)

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
