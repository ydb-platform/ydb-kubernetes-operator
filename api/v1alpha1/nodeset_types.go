package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeSetSpec describes an group nodes object
type NodeSetSpec struct {
	// Number of nodes (pods) in the set
	// +required
	Nodes int32 `json:"nodes"`

	// (Optional) Container image information
	// +optional
	//Image *PodImage `json:"image,omitempty"`

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
}

// NodeSetSpecInline describes an group nodes object inside parent object
type NodeSetSpecInline struct {

	// Name of child *NodeSet object
	// +required
	Name string `json:"name,omitempty"`

	// (Optional) Object should be reference to remote object
	// +optional
	Remote bool `json:"remote,omitempty"`

	// (Optional) Additional labels that will be added to the *NodeSet
	// +optional
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`

	// (Optional) Common NodeSetSpec configuration
	// +optional
	NodeSetSpec `json:",inline"`
}

// StorageNodeSetSpec describes an group nodes of Storage object
type StorageNodeSetSpec struct {
	// Name of parent Storage object to reference
	// +required
	StorageRef string `json:"storageRef,omitempty"`

	NodeSetSpec `json:",inline"`
}

// StorageNodeSetStatus defines the observed state
type StorageNodeSetStatus struct {
	State      string             `json:"state"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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

// DatabaseNodeSetSpec describes an group nodes of Database object
type DatabaseNodeSetSpec struct {
	// Name of parent Database object to reference
	// +required
	DatabaseRef string `json:"databaseRef,omitempty"`

	NodeSetSpec `json:",inline"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="The status of this DatabaseNodeSet"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DatabaseNodeSet declares StatefulSet parameters for storageRef
type DatabaseNodeSet struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +optional
	Spec DatabaseNodeSetSpec `json:"spec,omitempty"`
	// +optional
	// +kubebuilder:default:={state: "Pending"}
	Status DatabaseNodeSetStatus `json:"status,omitempty"`
}

// DatabaseNodeSetStatus defines the observed state
type DatabaseNodeSetStatus struct {
	State      string             `json:"state"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseNodeSetList contains a list of DatabaseNodeSet
type DatabaseNodeSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseNodeSet `json:"items"`
}
