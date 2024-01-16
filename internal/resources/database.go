package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration"
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
				Type:    string(DatabasePaused),
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
	nodeSetSpec := api.DatabaseNodeSetSpec{}

	nodeSetSpec.DatabaseRef = api.NamespacedRef{
		Name:      b.Name,
		Namespace: b.Namespace,
	}
	nodeSetSpec.StorageEndpoint = b.GetStorageEndpointWithProto()

	nodeSetSpec.Nodes = nodeSetSpecInline.Nodes
	nodeSetSpec.Configuration = configuration

	nodeSetSpec.Service = b.Spec.Service
	if nodeSetSpecInline.Service != nil {
		nodeSetSpec.Service = *nodeSetSpecInline.Service.DeepCopy()
	}
	nodeSetSpec.StorageDomains = b.Spec.StorageDomains
	nodeSetSpec.Encryption = b.Spec.Encryption
	nodeSetSpec.Volumes = b.Spec.Volumes
	nodeSetSpec.Datastreams = b.Spec.Datastreams
	nodeSetSpec.Pause = b.Spec.Pause
	nodeSetSpec.OperatorSync = b.Spec.OperatorSync
	nodeSetSpec.Domain = b.Spec.Domain
	nodeSetSpec.Path = b.Spec.Path

	nodeSetSpec.Resources = b.Spec.Resources
	if nodeSetSpecInline.Resources != nil {
		nodeSetSpec.Resources = nodeSetSpecInline.Resources
	}

	nodeSetSpec.SharedResources = b.Spec.SharedResources
	if nodeSetSpecInline.SharedResources != nil {
		nodeSetSpec.SharedResources = nodeSetSpecInline.SharedResources
	}

	nodeSetSpec.ServerlessResources = b.Spec.ServerlessResources
	nodeSetSpec.Image = b.Spec.Image
	// if nodeSetSpecInline.Image != nil {
	// 	nodeSetSpec.Image = *nodeSetSpecInline.Image.DeepCopy()
	// }

	nodeSetSpec.InitContainers = b.Spec.InitContainers
	nodeSetSpec.CABundle = b.Spec.CABundle
	nodeSetSpec.Secrets = b.Spec.Secrets

	nodeSetSpec.NodeSelector = b.Spec.NodeSelector
	if nodeSetSpecInline.NodeSelector != nil {
		nodeSetSpec.NodeSelector = nodeSetSpecInline.NodeSelector
	}

	nodeSetSpec.Affinity = b.Spec.Affinity
	if nodeSetSpecInline.Affinity != nil {
		nodeSetSpec.Affinity = nodeSetSpecInline.Affinity
	}

	nodeSetSpec.Tolerations = b.Spec.Tolerations
	if nodeSetSpecInline.Tolerations != nil {
		nodeSetSpec.Tolerations = nodeSetSpecInline.Tolerations
	}

	nodeSetSpec.TopologySpreadConstraints = b.Spec.TopologySpreadConstraints
	if nodeSetSpecInline.TopologySpreadConstraints != nil {
		nodeSetSpec.TopologySpreadConstraints = nodeSetSpecInline.TopologySpreadConstraints
	}

	nodeSetSpec.AdditionalLabels = make(map[string]string)
	if b.Spec.AdditionalLabels != nil {
		for k, v := range b.Spec.AdditionalLabels {
			nodeSetSpec.AdditionalLabels[k] = v
		}
	}
	if nodeSetSpecInline.AdditionalLabels != nil {
		for k, v := range nodeSetSpecInline.AdditionalLabels {
			nodeSetSpec.AdditionalLabels[k] = v
		}
	}
	nodeSetSpec.AdditionalLabels[labels.StorageNodeSetComponent] = nodeSetSpecInline.Name

	nodeSetSpec.AdditionalAnnotations = make(map[string]string)
	if b.Spec.AdditionalAnnotations != nil {
		for k, v := range b.Spec.AdditionalAnnotations {
			nodeSetSpec.AdditionalAnnotations[k] = v
		}
	}
	if nodeSetSpecInline.AdditionalAnnotations != nil {
		for k, v := range nodeSetSpecInline.AdditionalAnnotations {
			nodeSetSpec.AdditionalAnnotations[k] = v
		}
	}

	nodeSetSpec.PriorityClassName = b.Spec.PriorityClassName
	if nodeSetSpecInline.PriorityClassName != nodeSetSpec.PriorityClassName {
		nodeSetSpec.PriorityClassName = nodeSetSpecInline.PriorityClassName
	}

	return nodeSetSpec
}
