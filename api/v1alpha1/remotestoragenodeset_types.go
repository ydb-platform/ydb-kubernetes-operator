package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="The status of this RemoteStorageNodeSet"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// RemoteStorageNodeSet declares NodeSet spec and status for objects in remote cluster
type RemoteStorageNodeSet struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +optional
	Spec StorageNodeSetSpec `json:"spec,omitempty"`
	// +optional
	// +kubebuilder:default:={state: "Pending"}
	Status RemoteStorageNodeSetStatus `json:"status,omitempty"`
}

// StorageNodeSetStatus defines the observed state
type RemoteStorageNodeSetStatus struct {
	State           constants.ClusterState `json:"state"`
	Conditions      []metav1.Condition     `json:"conditions,omitempty"`
	RemoteResources []RemoteResource       `json:"remoteResources,omitempty"`
}

//+kubebuilder:object:root=true

// RemoteStorageNodeSetList contains a list of RemoteStorageNodeSet
type RemoteStorageNodeSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteStorageNodeSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteStorageNodeSet{}, &RemoteStorageNodeSetList{})
}
