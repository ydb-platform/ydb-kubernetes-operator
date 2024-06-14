package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
)

// DatabaseSpec defines the desired state of Database
type DatabaseSpec struct {
	DatabaseClusterSpec `json:",inline"`

	DatabaseNodeSpec `json:",inline"`

	// (Optional) NodeSet inline configuration to split into multiple StatefulSets
	// Default: (not specified)
	// +optional
	NodeSets []DatabaseNodeSetSpecInline `json:"nodeSets,omitempty"`
}

type DatabaseClusterSpec struct {
	// YDB Storage cluster reference
	// +required
	StorageClusterRef NamespacedRef `json:"storageClusterRef"`

	// YDB Storage Node broker address
	// +optional
	StorageEndpoint string `json:"storageEndpoint"`

	// (Optional) Name of the root storage domain
	// Default: Root
	// +kubebuilder:validation:Pattern:=[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?
	// +kubebuilder:validation:MaxLength:=63
	// +kubebuilder:default:="Root"
	// +optional
	Domain string `json:"domain"`

	// (Optional) Custom database path in schemeshard
	// Default: /<spec.domain>/<metadata.name>
	// +kubebuilder:validation:Pattern:=/[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?/[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?(/[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?)*
	// +kubebuilder:validation:MaxLength:=255
	// +optional
	Path string `json:"path,omitempty"`

	// (Optional) If specified, created database will be "serverless".
	// +optional
	ServerlessResources *ServerlessDatabaseResources `json:"serverlessResources,omitempty"`

	// Encryption configuration
	// +optional
	Encryption *EncryptionConfig `json:"encryption,omitempty"`

	// (Optional) YDB Image
	// +optional
	Image *PodImage `json:"image,omitempty"`

	// (Optional) YDBVersion sets the explicit version of the YDB image
	// Default: ""
	// +optional
	YDBVersion string `json:"version,omitempty"`

	// (Optional) List of initialization containers belonging to the pod.
	// Init containers are executed in order prior to containers being started.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/init-containers/
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// YDB configuration in YAML format. Will be applied on top of generated one in internal/configuration
	// +optional
	Configuration string `json:"configuration"`

	// (Optional) Storage services parameter overrides
	// Default: (not specified)
	// +optional
	Service *DatabaseServices `json:"service,omitempty"`

	// Datastreams config
	// +optional
	Datastreams *DatastreamsConfig `json:"datastreams,omitempty"`

	// The state of the Database processes.
	// `true` means all the Database Pods are being killed, but the Database resource is persisted.
	// `false` means the default state of the system, all Pods running.
	// +kubebuilder:default:=false
	// +optional
	Pause bool `json:"pause"`

	// Enables or disables operator's reconcile loop.
	// `false` means all the Pods are running, but the reconcile is effectively turned off.
	// `true` means the default state of the system, all Pods running, operator reacts
	// to specification change of this Database resource.
	// +kubebuilder:default:=true
	// +optional
	OperatorSync bool `json:"operatorSync"`

	// (Optional) Monitoring sets configuration options for YDB observability
	// Default: ""
	// +optional
	Monitoring *MonitoringOptions `json:"monitoring,omitempty"`

	// User-defined root certificate authority that is added to system trust
	// store of Storage pods on startup.
	// +optional
	CABundle string `json:"caBundle,omitempty"`

	// Secret names that will be mounted into the well-known directory of
	// every storage pod. Directory: `/opt/ydb/secrets/<secret_name>/<secret_key>`
	// +optional
	Secrets []*corev1.LocalObjectReference `json:"secrets,omitempty"`

	// Additional volumes that will be mounted into the well-known directory of
	// every storage pod. Directory: `/opt/ydb/volumes/<volume_name>`.
	// Only `hostPath` volume type is supported for now.
	// +optional
	Volumes []*corev1.Volume `json:"volumes,omitempty"`
}

type DatabaseNodeSpec struct {
	// Number of nodes (pods) in the cluster
	// +required
	Nodes int32 `json:"nodes"`

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

	// (Optional) If specified, the pod's terminationGracePeriodSeconds.
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// (Optional) Additional custom resource labels that are added to all resources
	// +optional
	AdditionalLabels map[string]string `json:"additionalLabels,omitempty"`

	// (Optional) Additional custom resource annotations that are added to all resources
	// +optional
	AdditionalAnnotations map[string]string `json:"additionalAnnotations,omitempty"`
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
	SharedDatabaseRef NamespacedRef `json:"sharedDatabaseRef"`
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
	State      constants.ClusterState `json:"state"`
	Conditions []metav1.Condition     `json:"conditions,omitempty"`
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

// EncryptionConfig todo
type EncryptionConfig struct {
	// +required
	Enabled bool `json:"enabled"`

	// +optional
	Key *corev1.SecretKeySelector `json:"key,omitempty"`

	// +optional
	Pin *string `json:"pin,omitempty"`
}

// Datastreams config todo
type DatastreamsConfig struct {
	// +required
	Enabled bool `json:"enabled"`

	// +required
	IAMServiceAccountKey *corev1.SecretKeySelector `json:"iam_service_account_key,omitempty"`
}

type DatabaseServices struct {
	GRPC         GRPCService         `json:"grpc,omitempty"`
	Interconnect InterconnectService `json:"interconnect,omitempty"`
	Status       StatusService       `json:"status,omitempty"`
	Datastreams  DatastreamsService  `json:"datastreams,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{})
}

func (r *Database) AnyCertificatesAdded() bool {
	return len(r.Spec.CABundle) > 0 ||
		r.Spec.Service.GRPC.TLSConfiguration.Enabled ||
		r.Spec.Service.Interconnect.TLSConfiguration.Enabled ||
		(r.Spec.Service.Status.TLSConfiguration != nil && r.Spec.Service.Status.TLSConfiguration.Enabled)
}
