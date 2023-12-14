package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseNodeSetSpec describes an group nodes of Database object
type DatabaseNodeSetSpec struct {
	// YDB Database namespaced reference
	// +required
	DatabaseRef NamespacedRef `json:"databaseRef"`

	// Number of nodes (pods) in the set
	// +required
	Nodes int32 `json:"nodes"`

	// YDB configuration in YAML format. Will be applied on top of generated one in internal/configuration
	// +optional
	Configuration string `json:"configuration"`

	// (Optional) Storage services parameter overrides
	// Default: (not specified)
	// +optional
	Service DatabaseServices `json:"service,omitempty"`

	// YDB Storage cluster reference
	// +required
	StorageClusterRef NamespacedRef `json:"storageClusterRef"`

	// (Optional) Node broker address host:port
	// +optional
	NodeBroker string `json:"nodeBroker,omitempty"`

	// Encryption
	// +optional
	Encryption *EncryptionConfig `json:"encryption,omitempty"`

	// Additional volumes that will be mounted into the well-known directory of
	// every storage pod. Directiry: `/opt/ydb/volumes/<volume_name>`.
	// Only `hostPath` volume type is supported for now.
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Datastreams config
	// +optional
	Datastreams *DatastreamsConfig `json:"datastreams,omitempty"`

	// (Optional) Name of the root storage domain
	// Default: root
	// +kubebuilder:validation:Pattern:=[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?
	// +kubebuilder:validation:MaxLength:=63
	// +kubebuilder:default:="root"
	// +optional
	Domain string `json:"domain"`

	// (Optional) Custom database path in schemeshard
	// Default: /<spec.domain>/<metadata.name>
	// +kubebuilder:validation:Pattern:=/[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?/[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?(/[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?)*
	// +kubebuilder:validation:MaxLength:=255
	// +optional
	Path string `json:"path,omitempty"`

	// (Optional) Database storage and compute resources
	// +optional
	Resources *DatabaseResources `json:"resources,omitempty"` // TODO: Add validation webhook: some resources must be specified

	// (Optional) Shared resources can be used by serverless databases.
	// +optional
	SharedResources *DatabaseResources `json:"sharedResources,omitempty"`

	// (Optional) If specified, created database will be "serverless".
	// +optional
	ServerlessResources *ServerlessDatabaseResources `json:"serverlessResources,omitempty"`

	// (Optional) Container image information
	// +optional
	Image PodImage `json:"image,omitempty"`

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

	// User-defined root certificate authority that is added to system trust
	// store of Storage pods on startup.
	// +optional
	CABundle string `json:"caBundle,omitempty"`

	// Secret names that will be mounted into the well-known directory of
	// every storage pod. Directory: `/opt/ydb/secrets/<secret_name>/<secret_key>`
	// +optional
	Secrets []corev1.LocalObjectReference `json:"secrets,omitempty"`

	// (Optional) NodeSelector is a selector which must be true for the pod to fit on a node.
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

	// (Optional) If specified, the pod's topologySpreadConstraints.
	// All topologySpreadConstraints are ANDed.
	// +optional
	// +patchMergeKey=topologyKey
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=topologyKey
	// +listMapKey=whenUnsatisfiable
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty" patchStrategy:"merge" patchMergeKey:"topologyKey"`

	// (Optional) Additional custom resource labels that are added to all resources
	// +optional
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`

	// (Optional) Additional custom resource annotations that are added to all resources
	// +optional
	AdditionalAnnotations map[string]string `json:"additionalAnnotations,omitempty"`

	// (Optional) If specified, the pod's priorityClassName.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
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

// DatabaseNodeSetStatus defines the observed state
type DatabaseNodeSetStatus struct {
	State                      string             `json:"state"`
	Conditions                 []metav1.Condition `json:"conditions,omitempty"`
	ObservedDatabaseGeneration int64              `json:"observedDatabaseGeneration,omitempty"`
}

// DatabaseNodeSetSpecInline describes an group nodes object inside parent object
type DatabaseNodeSetSpecInline struct {

	// Name of DatabaseNodeSet object
	// +required
	Name string `json:"name,omitempty"`

	// (Optional) Object should be reference to remote object
	// +optional
	Remote bool `json:"remote,omitempty"`

	// (Optional) Additional labels that will be added to the DatabaseNodeSet
	// +optional
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`

	// (Optional) Additional annotations that will be added to the DatabaseNodeSet
	// +optional
	AdditionalAnnotations map[string]string `json:"additionalAnnotations,omitempty"`

	// Number of nodes (pods) in the set
	// +required
	Nodes int32 `json:"nodes"`

	// (Optional) Storage services parameter overrides
	// Default: (not specified)
	// +optional
	Service *DatabaseServices `json:"service,omitempty"`

	// (Optional) Database storage and compute resources
	// +optional
	Resources *DatabaseResources `json:"resources,omitempty"` // TODO: Add validation webhook: some resources must be specified

	// (Optional) Shared resources can be used by serverless databases.
	// +optional
	SharedResources *DatabaseResources `json:"sharedResources,omitempty"`

	// (Optional) NodeSelector is a selector which must be true for the pod to fit on a node.
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

	// (Optional) If specified, the pod's topologySpreadConstraints.
	// All topologySpreadConstraints are ANDed.
	// +optional
	// +patchMergeKey=topologyKey
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=topologyKey
	// +listMapKey=whenUnsatisfiable
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty" patchStrategy:"merge" patchMergeKey:"topologyKey"`

	// (Optional) If specified, the pod's priorityClassName.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
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
