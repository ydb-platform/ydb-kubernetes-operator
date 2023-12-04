package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageNodeSetSpec describes an group nodes of Storage object
type StorageNodeSetSpec struct {
	// Number of nodes (pods) in the set
	// +required
	Nodes int32 `json:"nodes"`

	// (Optional) Container image information
	// +optional
	Image *PodImage `json:"image,omitempty"`

	// YDB configuration in YAML format. Will be applied on top of generated one in internal/configuration
	// +optional
	Configuration string `json:"configuration"`

	// Secret names that will be mounted into the well-known directory of
	// every storage pod. Directory: `/opt/ydb/secrets/<secret_name>/<secret_key>`
	// +optional
	Secrets []corev1.LocalObjectReference `json:"secrets,omitempty"`

	// Additional volumes that will be mounted into the well-known directory of
	// every storage pod. Directiry: `/opt/ydb/volumes/<volume_name>`.
	// Only `hostPath` volume type is supported for now.
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Whether host network should be enabled. Automatically sets
	// `dnsPolicy` to `clusterFirstWithHostNet`.
	// Default: false
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

	// (Optional) Storage container resource limits. Any container limits
	// can be specified.
	// Default: (not specified)
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Data storage mode.
	// For details, see https://cloud.yandex.ru/docs/ydb/oss/public/administration/deploy/production_checklist#topologiya
	// TODO English docs link
	// FIXME mirror-3-dc is only supported with external configuration
	// +kubebuilder:validation:Enum=mirror-3-dc;block-4-2;none
	// +kubebuilder:default:=block-4-2
	Erasure ErasureType `json:"erasure"`

	// (Optional) Where cluster data should be kept
	// +optional
	DataStore []corev1.PersistentVolumeClaimSpec `json:"dataStore,omitempty"`

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

	// (Optional) Storage services parameter overrides
	// Default: (not specified)
	// +optional
	Service StorageServices `json:"service,omitempty"`

	// List of initialization containers belonging to the pod.
	// Init containers are executed in order prior to containers being started. If any
	// init container fails, the pod is considered to have failed and is handled according
	// to its restartPolicy. The name for an init container or normal container must be
	// unique among all containers.
	// Init containers may not have Lifecycle actions, Readiness probes, Liveness probes, or Startup probes.
	// The resourceRequirements of an init container are taken into account during scheduling
	// by finding the highest request/limit for each resource type, and then using the max of
	// that value or the sum of the normal containers. Limits are applied to init containers
	// in a similar fashion.
	// Init containers cannot currently be added or removed.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/init-containers/
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// User-defined root certificate authority that is added to system trust
	// store of Storage pods on startup.
	// +optional
	CABundle string `json:"caBundle,omitempty"`
}

// StorageNodeSetStatus defines the observed state
type StorageNodeSetStatus struct {
	State      string             `json:"state"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// StorageNodeSetSpecInline describes an group nodes object inside parent object
type StorageNodeSetSpecInline struct {

	// Name of child *NodeSet object
	// +required
	Name string `json:"name,omitempty"`

	// (Optional) Object should be reference to remote object
	// +optional
	Remote bool `json:"remote,omitempty"`

	// (Optional) Additional labels that will be added to the *NodeSet
	// +optional
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`

	// (Optional) Additional annotations that will be added to the *NodeSet
	// +optional
	AdditionalAnnotations map[string]string `json:"additionalAnnotations,omitempty"`

	// Number of nodes (pods) in the set
	// +required
	Nodes int32 `json:"nodes"`

	// (Optional) Storage services parameter overrides
	// Default: (not specified)
	// +optional
	Service *StorageServices `json:"service,omitempty"`

	// (Optional) Container image information
	// +optional
	Image *PodImage `json:"image,omitempty"`

	// (Optional) Storage container resource limits. Any container limits
	// can be specified.
	// Default: (not specified)
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// (Optional) Where cluster data should be kept
	// +optional
	DataStore []corev1.PersistentVolumeClaimSpec `json:"dataStore,omitempty"`

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
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="The status of this StorageNodeSet"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// StorageNodeSet declares StatefulSet parameters
type StorageNodeSet struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +optional
	Spec StorageNodeSetSpec `json:"spec,omitempty"`
	// +optional
	// +kubebuilder:default:={state: "Pending"}
	Status StorageNodeSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StorageNodeSetList contains a list of StorageNodeSet
type StorageNodeSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageNodeSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageNodeSet{}, &StorageNodeSetList{})
}
