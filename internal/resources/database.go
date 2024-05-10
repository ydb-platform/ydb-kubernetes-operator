package resources

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
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

func (b *DatabaseBuilder) SetStatusOnFirstReconcile() (bool, ctrl.Result, error) {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}

		if b.Spec.Pause {
			meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
				Type:    DatabasePausedCondition,
				Status:  "True",
				Reason:  ReasonCompleted,
				Message: "State Database set to Paused",
			})

			return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
		}
	}

	return Continue, ctrl.Result{}, nil
}

func (b *DatabaseBuilder) Unwrap() *api.Database {
	return b.DeepCopy()
}

func (b *DatabaseBuilder) GetResourceBuilders(restConfig *rest.Config) []ResourceBuilder {
	if b.Spec.ServerlessResources != nil {
		return []ResourceBuilder{}
	}

	databaseLabels := labels.DatabaseLabels(b.Unwrap())
	databaseAnnotations := annotations.GetYdbTechAnnotations(b.Annotations)

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

	statefulSetLabels := databaseLabels.Copy()
	statefulSetLabels.Merge(b.Spec.AdditionalLabels)
	statefulSetLabels.Merge(map[string]string{labels.DatabaseGeneration: strconv.FormatInt(b.ObjectMeta.Generation, 10)})

	statefulSetAnnotations := CopyDict(b.Spec.AdditionalAnnotations)
	if b.Spec.Configuration != "" {
		statefulSetAnnotations[annotations.ConfigurationChecksum] = GetSHA256Checksum(b.Spec.Configuration)
	} else {
		statefulSetAnnotations[annotations.ConfigurationChecksum] = GetSHA256Checksum(b.Storage.Spec.Configuration)
	}

	var optionalBuilders []ResourceBuilder

	if b.Spec.Configuration != "" {
		optionalBuilders = append(
			optionalBuilders,
			&ConfigMapBuilder{
				Object: b,

				Name: b.GetName(),
				Data: map[string]string{
					api.ConfigFileName: b.Spec.Configuration,
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
		if b.Spec.Encryption.Key == nil {
			optionalBuilders = append(
				optionalBuilders,
				&EncryptionSecretBuilder{
					Object: b,

					Pin:    *b.Spec.Encryption.Pin,
					Labels: databaseLabels,
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
					ID:      GetSHA256Checksum(b.Name),
					Pin:     b.Spec.Encryption.Pin,
					Version: 1,
				},
			},
		}

		optionalBuilders = append(
			optionalBuilders,
			&EncryptionConfigBuilder{
				Object: b,

				Name:      fmt.Sprintf(EncryptionKeyConfigNameFormat, b.GetName()),
				KeyConfig: keyConfig,
				Labels:    databaseLabels,
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
		optionalBuilders = append(
			optionalBuilders,
			b.getNodeSetBuilders(databaseLabels, databaseAnnotations)...,
		)
	}

	return optionalBuilders
}

func (b *DatabaseBuilder) getNodeSetBuilders(
	databaseLabels map[string]string,
	databaseAnnotations map[string]string,
) []ResourceBuilder {
	var nodeSetBuilders []ResourceBuilder

	for _, nodeSetSpecInline := range b.Spec.NodeSets {
		databaseNodeSetSpec := b.recastDatabaseNodeSetSpecInline(nodeSetSpecInline.DeepCopy())

		databaseNodeSetLabels := CopyDict(databaseLabels)
		for k, v := range nodeSetSpecInline.Labels {
			databaseNodeSetLabels[k] = v
		}
		databaseNodeSetLabels[labels.DatabaseNodeSetComponent] = nodeSetSpecInline.Name
		if nodeSetSpecInline.Remote != nil {
			databaseNodeSetLabels[labels.RemoteClusterKey] = nodeSetSpecInline.Remote.Cluster
		}

		databaseNodeSetAnnotations := CopyDict(databaseAnnotations)
		for k, v := range nodeSetSpecInline.Annotations {
			databaseNodeSetAnnotations[k] = v
		}
		delete(databaseNodeSetAnnotations, annotations.LastAppliedAnnotation)

		if nodeSetSpecInline.Remote != nil {
			nodeSetBuilders = append(
				nodeSetBuilders,
				&RemoteDatabaseNodeSetBuilder{
					Object: b,

					Name:        fmt.Sprintf("%s-%s", b.Name, nodeSetSpecInline.Name),
					Labels:      databaseNodeSetLabels,
					Annotations: databaseNodeSetAnnotations,

					DatabaseNodeSetSpec: databaseNodeSetSpec,
				},
			)
		} else {
			nodeSetBuilders = append(
				nodeSetBuilders,
				&DatabaseNodeSetBuilder{
					Object: b,

					Name:        fmt.Sprintf("%s-%s", b.Name, nodeSetSpecInline.Name),
					Labels:      databaseNodeSetLabels,
					Annotations: databaseNodeSetAnnotations,

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

	nodeSetAdditionalLabels := CopyDict(b.Spec.AdditionalLabels)
	nodeSetAdditionalLabels[labels.DatabaseGeneration] = strconv.FormatInt(b.ObjectMeta.Generation, 10)
	if nodeSetSpecInline.AdditionalLabels != nil {
		for k, v := range nodeSetSpecInline.AdditionalLabels {
			nodeSetAdditionalLabels[k] = v
		}
	}
	nodeSetSpec.AdditionalLabels = CopyDict(nodeSetAdditionalLabels)

	nodeSetAdditionalAnnotations := CopyDict(b.Spec.AdditionalAnnotations)
	if b.Spec.Configuration != "" {
		nodeSetAdditionalAnnotations[annotations.ConfigurationChecksum] = GetSHA256Checksum(b.Spec.Configuration)
	} else {
		nodeSetAdditionalAnnotations[annotations.ConfigurationChecksum] = GetSHA256Checksum(b.Storage.Spec.Configuration)
	}
	if nodeSetSpecInline.AdditionalAnnotations != nil {
		for k, v := range nodeSetSpecInline.AdditionalAnnotations {
			nodeSetAdditionalAnnotations[k] = v
		}
	}
	nodeSetSpec.AdditionalAnnotations = CopyDict(nodeSetAdditionalAnnotations)

	return nodeSetSpec
}
