package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
)

// StorageSpec defines the desired state of Storage
type StorageSpec struct {
	StorageClusterSpec `json:",inline"`

	StorageNodeSpec `json:",inline"`

	// (Optional) NodeSet inline configuration to split into multiple StatefulSets
	// Default: (not specified)
	// +optional
	NodeSets []StorageNodeSetSpecInline `json:"nodeSets,omitempty"`
}

type StorageClusterSpec struct {
	// (Optional) Name of the root storage domain
	// Default: root
	// +kubebuilder:validation:Pattern:=[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?
	// +kubebuilder:validation:MaxLength:=63
	// +kubebuilder:default:="Root"
	// +optional
	Domain string `json:"domain"`

	// (Optional) Operator connection settings
	// Default: (not specified)
	// +optional
	OperatorConnection *ConnectionOptions `json:"operatorConnection,omitempty"`

	// Data storage topology mode
	// For details, see https://ydb.tech/docs/en/cluster/topology
	// FIXME mirror-3-dc is only supported with external configuration
	// +kubebuilder:validation:Enum=mirror-3-dc;block-4-2;none
	// +kubebuilder:default:=block-4-2
	Erasure ErasureType `json:"erasure"`

	// (Optional) Container image information
	// +optional
	Image *PodImage `json:"image,omitempty"`

	// (Optional) YDBVersion sets the explicit version of the YDB image
	// Default: ""
	// +optional
	YDBVersion string `json:"version,omitempty"`

	// (Optional) List of initialization containers belonging to the pod.
	// Init containers are executed in order prior to containers being started.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/init-containers/
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// YDB configuration in YAML format. Will be applied on top of generated one in internal/configuration
	// +optional
	Configuration string `json:"configuration"`

	// (Optional) Storage services parameter overrides
	// Default: (not specified)
	// +optional
	Service *StorageServices `json:"service,omitempty"`

	// The state of the Storage processes.
	// `true` means all the Storage Pods are being killed, but the Storage resource is persisted.
	// `false` means the default state of the system, all Pods running.
	// +kubebuilder:default:=false
	// +optional
	Pause bool `json:"pause"`

	// Enables or disables operator's reconcile loop.
	// `false` means all the Pods are running, but the reconcile is effectively turned off.
	// `true` means the default state of the system, all Pods running, operator reacts
	// to specification change of this Storage resource.
	// +kubebuilder:default:=true
	// +optional
	OperatorSync bool `json:"operatorSync"`

	// (Optional) Monitoring sets configuration options for YDB observability
	// Default: ""
	// +optional
	Monitoring *MonitoringOptions `json:"monitoring,omitempty"`

	// User-defined root certificate authority that is added to system trust
	// store of Storage pods on startup.
	// +optional
	CABundle string `json:"caBundle,omitempty"`

	// Additional volumes that will be mounted into the well-known directory of
	// every storage pod. Directory: `/opt/ydb/volumes/<volume_name>`.
	// Only `hostPath` volume type is supported for now.
	// +optional
	Volumes []*corev1.Volume `json:"volumes,omitempty"`

	// Secret names that will be mounted into the well-known directory of
	// every storage pod. Directory: `/opt/ydb/secrets/<secret_name>/<secret_key>`
	// +optional
	Secrets []*corev1.LocalObjectReference `json:"secrets,omitempty"`
}

type StorageNodeSpec struct {
	// Number of nodes (pods)
	// +required
	Nodes int32 `json:"nodes"`

	// (Optional) Where cluster data should be kept
	// +optional
	DataStore []corev1.PersistentVolumeClaimSpec `json:"dataStore,omitempty"`

	// (Optional) Container resource limits. Any container limits
	// can be specified.
	// Default: (not specified)
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// (Optional) Whether host network should be enabled.
	// Default: false
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

	// (Optional) NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// (Optional) If specified, the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// (Optional) If specified, the pod's tolerations.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// (Optional) If specified, the pod's topologySpreadConstraints.
	// All topologySpreadConstraints are ANDed.
	// +optional
	// +patchMergeKey=topologyKey
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=topologyKey
	// +listMapKey=whenUnsatisfiable
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty" patchStrategy:"merge" patchMergeKey:"topologyKey"`

	// (Optional) If specified, the pod's priorityClassName.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// (Optional) Additional custom resource labels that are added to all resources
	// +optional
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`

	// (Optional) Additional custom resource annotations that are added to all resources
	// +optional
	AdditionalAnnotations map[string]string `json:"additionalAnnotations,omitempty"`
}

// StorageStatus defines the observed state of Storage
type StorageStatus struct {
	State      constants.ClusterState `json:"state"`
	Conditions []metav1.Condition     `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="The status of this DB"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Storage is the Schema for the Storages API
type Storage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec StorageSpec `json:"spec,omitempty"`

	// +kubebuilder:default:={state: "Pending"}
	Status StorageStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StorageList contains a list of Storage
type StorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Storage `json:"items"`
}

// StorageServices defines parameter overrides for Storage Services
type StorageServices struct {
	GRPC         GRPCService         `json:"grpc,omitempty"`
	Interconnect InterconnectService `json:"interconnect,omitempty"`
	Status       StatusService       `json:"status,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Storage{}, &StorageList{})
}
