package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DatabaseMonitoringSpec defines the desired state of DatabaseMonitoring
type DatabaseMonitoringSpec struct {
	DatabaseClusterRef NamespacedRef `json:"databaseRef"`

	// (Optional) Additional labels that will be added to the ServiceMonitor
	// +optional
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`
}

// DatabaseMonitoringStatus defines the observed state of DatabaseMonitoring
type DatabaseMonitoringStatus struct {
	State      string             `json:"state"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="Monitoring status"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DatabaseMonitoring is the Schema for the databasemonitorings API
type DatabaseMonitoring struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseMonitoringSpec   `json:"spec,omitempty"`
	Status DatabaseMonitoringStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseMonitoringList contains a list of DatabaseMonitoring
type DatabaseMonitoringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseMonitoring `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseMonitoring{}, &DatabaseMonitoringList{})
}
