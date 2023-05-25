package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageMonitoringSpec defines the desired state of StorageMonitoring
type StorageMonitoringSpec struct {
	StorageRef NamespacedRef `json:"storageRef"`

	// (Optional) Additional labels that will be added to the ServiceMonitor
	// +optional
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`
}

// StorageMonitoringStatus defines the observed state of StorageMonitoring
type StorageMonitoringStatus struct {
	State      string             `json:"state"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="Monitoring status"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// StorageMonitoring is the Schema for the storagemonitorings API
type StorageMonitoring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StorageMonitoringSpec   `json:"spec,omitempty"`
	Status StorageMonitoringStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StorageMonitoringList contains a list of StorageMonitoring
type StorageMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageMonitoring `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageMonitoring{}, &StorageMonitoringList{})
}
