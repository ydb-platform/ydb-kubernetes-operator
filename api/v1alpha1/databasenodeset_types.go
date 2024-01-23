package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
)

// DatabaseNodeSetSpec describes an group nodes of Database object
type DatabaseNodeSetSpec struct {
	// YDB Database namespaced reference
	// +required
	DatabaseRef NamespacedRef `json:"databaseRef"`

	DatabaseClusterSpec `json:",inline"`

	DatabaseNodeSpec `json:",inline"`
}

// DatabaseNodeSetStatus defines the observed state
type DatabaseNodeSetStatus struct {
	State                      constants.ClusterState `json:"state"`
	Conditions                 []metav1.Condition     `json:"conditions,omitempty"`
	ObservedDatabaseGeneration int64                  `json:"observedDatabaseGeneration,omitempty"`
}

// DatabaseNodeSetSpecInline describes an group nodes object inside parent object
type DatabaseNodeSetSpecInline struct {
	// Name of DatabaseNodeSet object
	// +required
	Name string `json:"name,omitempty"`

	// (Optional) Object should be reference to remote object
	// +optional
	Remote bool `json:"remote,omitempty"`

	DatabaseNodeSpec `json:",inline"`
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
