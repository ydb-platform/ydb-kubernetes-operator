package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseSpec defines the desired state of Database
type DatabaseSpec struct {
	// Number of nodes (pods) in the cluster
	// +required
	Nodes int32 `json:"nodes"`
	// (Optional) Storage services parameter overrides
	// Default: (not specified)
	// +optional
	Service DatabaseServices `json:"service,omitempty"`
	// YDB Storage cluster reference
	// +required
	StorageClusterRef StorageRef `json:"storageClusterRef"`
	// (Optional) Name of the root storage domain
	// Default: root
	// +kubebuilder:validation:Pattern:=[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?
	// +kubebuilder:validation:MaxLength:=63
	// +kubebuilder:default:="root"
	// +optional
	Domain string `json:"domain"`
	// (Optional) Database container resource limits. Any container limits
	// can be specified.
	// Default: (not specified)
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// (Optional) Public host to advertise on discovery requests
	// Default: ""
	// +optional
	PublicHost string `json:"publicHost,omitempty"`
	// (Optional) YDBVersion sets the explicit version of the YDB image
	// Default: ""
	// +optional
	YDBVersion string `json:"version,omitempty"`
	// (Optional) Yandex Database Image
	// +optional
	Image PodImage `json:"image,omitempty"`
	// NodeSelector is a selector which must be true for the pod to fit on a node.
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
	// (Optional) Additional custom resource labels that are added to all resources
	// +optional
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`
}

// DatabaseStatus defines the observed state of Database
type DatabaseStatus struct {
	State      string             `json:"state"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="The status of this DB"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Database is the Schema for the databases API
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DatabaseSpec `json:"spec,omitempty"`

	// +kubebuilder:default:={state: "Pending"}
	Status DatabaseStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseList contains a list of Database
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}

// PodImage represents the image information for a container that is used
// to build the StatefulSet.
type PodImage struct {
	// Container image with supported YDB version.
	// This defaults to the version pinned to the operator and requires a full container and tag/sha name.
	// For instance: cr.yandex/ydb/ydb:stable-21-4-14
	// +required
	Name string `json:"name,omitempty"`
	// (Optional) PullPolicy for the image, which defaults to IfNotPresent.
	// Default: IfNotPresent
	// +optional
	PullPolicyName *corev1.PullPolicy `json:"pullPolicy,omitempty"`
	// (Optional) Secret name containing the dockerconfig to use for a registry that requires authentication. The secret
	// must be configured first by the user.
	// +optional
	PullSecret *string `json:"pullSecret,omitempty"`
}

// StorageRef todo
type StorageRef struct {
	// +kubebuilder:validation:Pattern:=[a-z0-9]([-a-z0-9]*[a-z0-9])?
	// +kubebuilder:validation:MaxLength:=63
	// +required
	Name string `json:"name"`
	// +kubebuilder:validation:Pattern:=[a-z0-9]([-a-z0-9]*[a-z0-9])?
	// +kubebuilder:validation:MaxLength:=63
	// +optional
	Namespace string `json:"namespace"`
}

type DatabaseServices struct {
	GRPC         Service `json:"grpc,omitempty"`
	Interconnect Service `json:"interconnect,omitempty"`
	Status       Service `json:"status,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{})
}
