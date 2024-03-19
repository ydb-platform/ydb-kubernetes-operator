package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
)

// StorageNodeSetSpec describes an group nodes of Storage object
type StorageNodeSetSpec struct {
	// YDB Storage reference
	// +required
	StorageRef NamespacedRef `json:"storageRef"`

	StorageClusterSpec `json:",inline"`

	StorageNodeSpec `json:",inline"`
}

// StorageNodeSetStatus defines the observed state
type StorageNodeSetStatus struct {
	State      constants.ClusterState `json:"state"`
	Conditions []metav1.Condition     `json:"conditions,omitempty"`
}

// StorageNodeSetSpecInline describes an group nodes object inside parent object
type StorageNodeSetSpecInline struct {
	// Name of StorageNodeSet object
	// +required
	Name string `json:"name"`

	// Labels for StorageNodeSet object
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations for StorageNodeSet object
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// (Optional) Object should be reference to RemoteStorageNodeSet object
	// +optional
	Remote *RemoteSpec `json:"remote,omitempty"`

	StorageNodeSpec `json:",inline"`
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

func RecastStorageNodeSet(storageNodeSet *StorageNodeSet) *Storage {
	return &Storage{
		ObjectMeta: metav1.ObjectMeta{
			Name:        storageNodeSet.Spec.StorageRef.Name,
			Namespace:   storageNodeSet.Spec.StorageRef.Namespace,
			Labels:      storageNodeSet.Labels,
			Annotations: storageNodeSet.Annotations,
		},
		Spec: StorageSpec{
			StorageClusterSpec: storageNodeSet.Spec.StorageClusterSpec,
			StorageNodeSpec:    storageNodeSet.Spec.StorageNodeSpec,
		},
	}
}
