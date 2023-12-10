package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants"
)

// StorageSpec defines the desired state of Storage
type StorageSpec struct {
	// Number of nodes (pods) in the cluster
	// +required
	Nodes int32 `json:"nodes"`

	// YDB configuration in YAML format. Will be applied on top of generated one in internal/configuration
	// +optional
	Configuration string `json:"configuration"`

	// Data storage mode.
	// For details, see https://cloud.yandex.ru/docs/ydb/oss/public/administration/deploy/production_checklist#topologiya
	// TODO English docs link
	// FIXME mirror-3-dc is only supported with external configuration
	// +kubebuilder:validation:Enum=mirror-3-dc;block-4-2;none
	// +kubebuilder:default:=block-4-2
	Erasure ErasureType `json:"erasure"`

	// Where cluster data should be kept
	// +required
	DataStore []corev1.PersistentVolumeClaimSpec `json:"dataStore"`

	// (Optional) Operator connection settings
	// Default: (not specified)
	// +optional
	OperatorConnection *ConnectionOptions `json:"operatorConnection,omitempty"`

	// (Optional) Storage services parameter overrides
	// Default: (not specified)
	// +optional
	Service StorageServices `json:"service,omitempty"`

	// The state of the Storage processes. Can be one of `Paused`, `Running` or `Frozen`.
	// `Paused` means all the Storage Pods are being killed, but the Storage resource is persisted.
	// `Frozen` means all the Pods are running, but the reconcile is effectively turned off.
	// `Running` means the default state of the system, all Pods running.
	// +kubebuilder:default:=Running
	// +optional
	Pause string `json:"pause,omitempty"`

	// (Optional) Name of the root storage domain
	// Default: root
	// +kubebuilder:validation:Pattern:=[a-zA-Z0-9]([-_a-zA-Z0-9]*[a-zA-Z0-9])?
	// +kubebuilder:validation:MaxLength:=63
	// +kubebuilder:default:="root"
	// +optional
	Domain string `json:"domain"`

	// (Optional) Storage container resource limits. Any container limits
	// can be specified.
	// Default: (not specified)
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// (Optional) YDBVersion sets the explicit version of the YDB image
	// Default: ""
	// +optional
	YDBVersion string `json:"version,omitempty"`

	// Container image information
	// +required
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

	// Whether host network should be enabled. Automatically sets
	// `dnsPolicy` to `clusterFirstWithHostNet`.
	// Default: false
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

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

// ConnectedDatabase is a reference to Database object which is
// currently running on top of this Storage object.
type ConnectedDatabase struct {
	Name  string                 `json:"name"`
	State constants.ClusterState `json:"state"`
}

// StorageStatus defines the observed state of Storage
type StorageStatus struct {
	State              constants.ClusterState `json:"state"`
	ConnectedDatabases []ConnectedDatabase    `json:"connectedDatabases,omitempty"`
	Conditions         []metav1.Condition     `json:"conditions,omitempty"`
	StateBeforePausing constants.ClusterState `json:"stateBeforePausing,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="The status of this DB"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Storage is the Schema for the Storages API
type Storage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec StorageSpec `json:"spec,omitempty"`

	// +kubebuilder:default:={state: "Pending"}
	Status StorageStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StorageList contains a list of Storage
type StorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Storage `json:"items"`
}

// StorageServices defines parameter overrides for Storage Services
type StorageServices struct {
	GRPC         GRPCService         `json:"grpc,omitempty"`
	Interconnect InterconnectService `json:"interconnect,omitempty"`
	Status       StatusService       `json:"status,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Storage{}, &StorageList{})
}
