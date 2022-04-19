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

	// ConfigMap name with custom YDB configuration, where key is config file name and value is config file content.
	// +optional
	// +deprecated
	ClusterConfig string `json:"config,omitempty"`

	// YDB configuration in YAML format. Will be applied on top of generated one in internal/configuration
	// +optional
	Configuration string `json:"configuration"`

	// (Optional) Storage services parameter overrides
	// Default: (not specified)
	// +optional
	Service DatabaseServices `json:"service,omitempty"`

	// YDB Storage cluster reference
	// +required
	StorageClusterRef StorageRef `json:"storageClusterRef"`

	// Encryption
	// +optional
	Encryption *EncryptionConfig `json:"encryption,omitempty"`

	// (Optional) Name of the root storage domain
	// Default: root
	// +kubebuilder:validation:Pattern:=[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?
	// +kubebuilder:validation:MaxLength:=63
	// +kubebuilder:default:="root"
	// +optional
	Domain string `json:"domain"`

	// (Optional) Database storage and compute resources
	// +optional
	Resources *DatabaseResources `json:"resources,omitempty"` // TODO: Add validation webhook: some resources must be specified

	// (Optional) Shared resources can be used by serverless databases.
	// +optional
	SharedResources *DatabaseResources `json:"sharedResources,omitempty"`

	// (Optional) If specified, created database will be "serverless".
	// +optional
	ServerlessResources *ServerlessDatabaseResources `json:"serverlessResources,omitempty"`

	// (Optional) Public host to advertise on discovery requests
	// Default: ""
	// +optional
	PublicHost string `json:"publicHost,omitempty"`

	// List of initialization containers belonging to the pod.
	// Init containers are executed in order prior to containers being started. If any
	// init container fails, the pod is considered to have failed and is handled according
	// to its restartPolicy. The name for an init container or normal container must be
	// unique among all containers.
	// Init containers may not have Lifecycle actions, Readiness probes, Liveness probes, or Startup probes.
	// The resourceRequirements of an init container are taken into account during scheduling
	// by finding the highest request/limit for each resource type, and then using the max of
	// that value or the sum of the normal containers. Limits are applied to init containers
	// in a similar fashion.
	// Init containers cannot currently be added or removed.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/init-containers/
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// (Optional) Monitoring sets configuration options for YDB observability
	// Default: ""
	// +optional
	Monitoring *MonitoringOptions `json:"monitoring,omitempty"`

	// (Optional) YDBVersion sets the explicit version of the YDB image
	// Default: ""
	// +optional
	YDBVersion string `json:"version,omitempty"`

	// (Optional) YDB Image
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

type DatabaseResources struct {
	// (Optional) Database container resource limits. Any container limits
	// can be specified.
	// Default: (not specified)
	// +optional
	ContainerResources corev1.ResourceRequirements `json:"containerResources,omitempty"`

	// Kind of the storage unit. Determine guarantees
	// for all main unit parameters: used hard disk type, capacity
	// throughput, IOPS etc.
	// +required
	StorageUnits []StorageUnit `json:"storageUnits,omitempty"`
}

type ServerlessDatabaseResources struct {
	// Reference to YDB Database with configured shared resources
	// +required
	SharedDatabaseRef SharedDatabaseRef `json:"sharedDatabaseRef,omitempty"`
}

type StorageUnit struct {
	// Kind of the storage unit. Determine guarantees
	// for all main unit parameters: used hard disk type, capacity
	// throughput, IOPS etc.
	// +required
	UnitKind string `json:"unitKind"`

	// Number of units in this set.
	// +required
	Count uint64 `json:"count"`
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

	StorageRef *Storage `json:"storageRef,omitempty"`
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
	// For instance: cr.yandex/crptqonuodf51kdj7a7d/ydb:22.2.22
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

// EncryptionConfig todo
type EncryptionConfig struct {
	// +required
	Enabled bool `json:"enabled"`

	// +optional
	Key *corev1.SecretKeySelector `json:"key,omitempty"`

	// +optional
	Pin *string `json:"pin,omitempty"`
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

type SharedDatabaseRef struct {
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
	GRPC         GRPCService         `json:"grpc,omitempty"`
	Interconnect InterconnectService `json:"interconnect,omitempty"`
	Status       StatusService       `json:"status,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{})
}
