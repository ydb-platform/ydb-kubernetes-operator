package v1alpha1

import corev1 "k8s.io/api/core/v1"

// NamespacedRef TODO: replace StorageRef
type NamespacedRef struct {
	// +kubebuilder:validation:Pattern:=[a-z0-9]([-a-z0-9]*[a-z0-9])?
	// +kubebuilder:validation:MaxLength:=63
	// +required
	Name string `json:"name"`

	// +kubebuilder:validation:Pattern:=[a-z0-9]([-a-z0-9]*[a-z0-9])?
	// +kubebuilder:validation:MaxLength:=63
	// +optional
	Namespace string `json:"namespace"`
}

// PodImage represents the image information for a container that is used
// to build the StatefulSet.
type PodImage struct {
	// Container image with supported YDB version.
	// This defaults to the version pinned to the operator and requires a full container and tag/sha name.
	// For example: cr.yandex/crptqonuodf51kdj7a7d/ydb:22.2.22
	// +optional
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

type RemoteSpec struct {
	// (Optional) Remote cloud region to deploy into
	// +optional
	Region string `json:"region,omitempty"`

	// Remote cloud zone to deploy into
	// +required
	Zone string `json:"zone"`
}
