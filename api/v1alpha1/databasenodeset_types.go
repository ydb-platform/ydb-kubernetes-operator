package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

func init() {
	SchemeBuilder.Register(&DatabaseNodeSet{}, &DatabaseNodeSetList{})
}
