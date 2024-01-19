package resources

import (
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

type StorageClusterBuilder struct {
	*api.Storage
}

func NewCluster(ydbCr *api.Storage) StorageClusterBuilder {
	cr := ydbCr.DeepCopy()

	return StorageClusterBuilder{cr}
}

func (b *StorageClusterBuilder) SetStatusOnFirstReconcile() (bool, ctrl.Result, error) {
	if b.Status.Conditions == nil {
		b.Status.Conditions = []metav1.Condition{}

		if b.Spec.Pause {
			meta.SetStatusCondition(&b.Status.Conditions, metav1.Condition{
				Type:    string(StoragePaused),
				Status:  "True",
				Reason:  ReasonCompleted,
				Message: "State Storage set to Paused",
			})

			return Stop, ctrl.Result{RequeueAfter: StatusUpdateRequeueDelay}, nil
		}
	}

	return Continue, ctrl.Result{}, nil
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
		for _, nodeSetSpecInline := range b.Spec.NodeSets {
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
	nodeSetSpec := api.StorageNodeSetSpec{}

	nodeSetSpec.StorageRef = api.NamespacedRef{
		Name:      b.Name,
		Namespace: b.Namespace,
	}

	nodeSetSpec.StorageClusterSpec = b.Spec.StorageClusterSpec
	nodeSetSpec.StorageNodeSpec = b.Spec.StorageNodeSpec

	nodeSetSpec.Configuration = configuration

	nodeSetSpec.Nodes = nodeSetSpecInline.Nodes

	if nodeSetSpecInline.DataStore != nil {
		nodeSetSpec.DataStore = nodeSetSpecInline.DataStore
	}

	if nodeSetSpecInline.Resources != nil {
		nodeSetSpec.Resources = nodeSetSpecInline.Resources
	}

	if nodeSetSpecInline.HostNetwork != nodeSetSpec.HostNetwork {
		nodeSetSpec.HostNetwork = nodeSetSpecInline.HostNetwork
	}

	if nodeSetSpecInline.NodeSelector != nil {
		if nodeSetSpec.NodeSelector == nil {
			nodeSetSpec.NodeSelector = make(map[string]string)
		}
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

	if nodeSetSpecInline.PriorityClassName != nodeSetSpec.PriorityClassName {
		nodeSetSpec.PriorityClassName = nodeSetSpecInline.PriorityClassName
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

	return nodeSetSpec
}
