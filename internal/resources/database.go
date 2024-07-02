package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/metrics"
)

type DatabaseBuilder struct {
	*api.Database
	Storage *api.Storage
}

func NewDatabase(ydbCr *api.Database) DatabaseBuilder {
	cr := ydbCr.DeepCopy()

	if cr.Spec.Service.Status.TLSConfiguration == nil {
		cr.Spec.Service.Status.TLSConfiguration = &api.TLSConfiguration{Enabled: false}
	}

	return DatabaseBuilder{Database: cr, Storage: nil}
}

func (b *DatabaseBuilder) Unwrap() *api.Database {
	return b.DeepCopy()
}

func (b *DatabaseBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	if b.Spec.ServerlessResources != nil {
		return []ResourceBuilder{}
	}

	databaseLabels := labels.DatabaseLabels(b.Unwrap())

	statefulSetLabels := databaseLabels.Copy()
	statefulSetLabels.Merge(map[string]string{labels.StatefulsetComponent: b.Name})

	statefulSetAnnotations := CopyDict(b.Spec.AdditionalAnnotations)
	statefulSetAnnotations[annotations.ConfigurationChecksum] = GetConfigurationChecksum(b.Spec.Configuration)

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

	if b.Spec.Configuration != "" {
		// YDBOPS-9722 backward compatibility
		cfg, _ := api.BuildConfiguration(b.Storage, b.Unwrap())

		optionalBuilders = append(
			optionalBuilders,
			&ConfigMapBuilder{
				Object: b,

				Name: b.GetName(),
				Data: map[string]string{
					api.ConfigFileName: string(cfg),
				},
				Labels: databaseLabels,
			},
		)
	}

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

	if b.Spec.Encryption != nil && b.Spec.Encryption.Enabled {
		// backward compatibility
		if b.Spec.Encryption.Pin == nil || len(*b.Spec.Encryption.Pin) == 0 {
			encryptionPin := api.DefaultDatabaseEncryptionPin
			b.Spec.Encryption.Pin = &encryptionPin
		}

		if b.Spec.Encryption.Key == nil {
			optionalBuilders = append(
				optionalBuilders,
				&EncryptionSecretBuilder{
					Object: b,

					Labels: databaseLabels,
					Pin:    *b.Spec.Encryption.Pin,
				},
			)
		}

		keyConfig := schema.KeyConfig{
			Keys: []schema.Key{
				{
					ContainerPath: fmt.Sprintf("%s/%s/%s",
						wellKnownDirForAdditionalSecrets,
						api.DatabaseEncryptionKeySecretDir,
						api.DatabaseEncryptionKeySecretFile,
					),
					ID:      b.Name,
					Pin:     b.Spec.Encryption.Pin,
					Version: 1,
				},
			},
		}

		optionalBuilders = append(
			optionalBuilders,
			&EncryptionConfigBuilder{
				Object: b,

				Name:   fmt.Sprintf(EncryptionKeyConfigNameFormat, b.GetName()),
				Labels: databaseLabels,

				KeyConfig: keyConfig,
			},
		)
	}

	optionalBuilders = append(
		optionalBuilders,
		&ServiceBuilder{
			Object:         b,
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
			Object:         b,
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
			Object:         b,
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
		optionalBuilders = append(
			optionalBuilders,
			&ServiceBuilder{
				Object:         b,
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

	if b.Spec.NodeSets == nil {
		optionalBuilders = append(
			optionalBuilders,
			&DatabaseStatefulSetBuilder{
				Database:   b.Unwrap(),
				RestConfig: restConfig,

				Name:        b.Name,
				Labels:      statefulSetLabels,
				Annotations: statefulSetAnnotations,
			},
		)
	} else {
		optionalBuilders = append(optionalBuilders, b.getNodeSetBuilders(databaseLabels)...)
	}

	return optionalBuilders
}

func (b *DatabaseBuilder) getNodeSetBuilders(databaseLabels labels.Labels) []ResourceBuilder {
	var nodeSetBuilders []ResourceBuilder

	for _, nodeSetSpecInline := range b.Spec.NodeSets {
		nodeSetLabels := databaseLabels.Copy()
		nodeSetLabels.Merge(nodeSetSpecInline.Labels)
		nodeSetLabels.Merge(map[string]string{labels.DatabaseNodeSetComponent: nodeSetSpecInline.Name})
		if nodeSetSpecInline.Remote != nil {
			nodeSetLabels.Merge(map[string]string{labels.RemoteClusterKey: nodeSetSpecInline.Remote.Cluster})
		}

		nodeSetAnnotations := CopyDict(b.Annotations)
		if nodeSetSpecInline.Annotations != nil {
			for k, v := range nodeSetSpecInline.Annotations {
				nodeSetAnnotations[k] = v
			}
		}

		databaseNodeSetSpec := b.recastDatabaseNodeSetSpecInline(nodeSetSpecInline.DeepCopy())
		if nodeSetSpecInline.Remote != nil {
			nodeSetBuilders = append(
				nodeSetBuilders,
				&RemoteDatabaseNodeSetBuilder{
					Object: b,

					Name:        b.Name + "-" + nodeSetSpecInline.Name,
					Labels:      nodeSetLabels,
					Annotations: nodeSetAnnotations,

					DatabaseNodeSetSpec: databaseNodeSetSpec,
				},
			)
		} else {
			nodeSetBuilders = append(
				nodeSetBuilders,
				&DatabaseNodeSetBuilder{
					Object: b,

					Name:        b.Name + "-" + nodeSetSpecInline.Name,
					Labels:      nodeSetLabels,
					Annotations: nodeSetAnnotations,

					DatabaseNodeSetSpec: databaseNodeSetSpec,
				},
			)
		}
	}

	return nodeSetBuilders
}

func (b *DatabaseBuilder) recastDatabaseNodeSetSpecInline(nodeSetSpecInline *api.DatabaseNodeSetSpecInline) api.DatabaseNodeSetSpec {
	nodeSetSpec := api.DatabaseNodeSetSpec{}

	nodeSetSpec.DatabaseRef = api.NamespacedRef{
		Name:      b.Name,
		Namespace: b.Namespace,
	}

	nodeSetSpec.DatabaseClusterSpec = b.Spec.DatabaseClusterSpec
	nodeSetSpec.DatabaseNodeSpec = b.Spec.DatabaseNodeSpec

	nodeSetSpec.Nodes = nodeSetSpecInline.Nodes

	if nodeSetSpecInline.Resources != nil {
		nodeSetSpec.Resources = nodeSetSpecInline.Resources
	}

	if nodeSetSpecInline.SharedResources != nil {
		nodeSetSpec.Resources = nodeSetSpecInline.SharedResources
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

	if nodeSetSpecInline.TopologySpreadConstraints != nil {
		nodeSetSpec.TopologySpreadConstraints = nodeSetSpecInline.TopologySpreadConstraints
	}

	if nodeSetSpecInline.Tolerations != nil {
		nodeSetSpec.Tolerations = append(nodeSetSpec.Tolerations, nodeSetSpecInline.Tolerations...)
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
