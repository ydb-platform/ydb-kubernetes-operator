package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// StorageNodeSetSpec describes an group nodes of Storage object
type StorageNodeSetSpec struct {
	// Name of parent Storage object to reference
	// +required
	StorageRef  string `json:"storageRef,omitempty"`
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

func init() {
	SchemeBuilder.Register(&StorageNodeSet{}, &StorageNodeSetList{})
}
