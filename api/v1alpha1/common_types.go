package v1alpha1

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
