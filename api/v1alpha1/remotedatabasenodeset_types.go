package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="The status of this RemoteDatabaseNodeSet"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RemoteDatabaseNodeSet declares NodeSet spec and status for objects in remote cluster
type RemoteDatabaseNodeSet struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +optional
	Spec DatabaseNodeSetSpec `json:"spec,omitempty"`
	// +optional
	// +kubebuilder:default:={state: "Pending"}
	Status DatabaseNodeSetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RemoteDatabaseNodeSetList contains a list of RemoteDatabaseNodeSet
type RemoteDatabaseNodeSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteDatabaseNodeSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteDatabaseNodeSet{}, &RemoteDatabaseNodeSetList{})
}
