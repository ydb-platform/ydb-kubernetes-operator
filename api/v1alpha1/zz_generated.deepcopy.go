//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AccessTokenAuth) DeepCopyInto(out *AccessTokenAuth) {
	*out = *in
	if in.CredentialSource != nil {
		in, out := &in.CredentialSource, &out.CredentialSource
		*out = new(CredentialSource)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AccessTokenAuth.
func (in *AccessTokenAuth) DeepCopy() *AccessTokenAuth {
	if in == nil {
		return nil
	}
	out := new(AccessTokenAuth)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ConnectionOptions) DeepCopyInto(out *ConnectionOptions) {
	*out = *in
	if in.AccessToken != nil {
		in, out := &in.AccessToken, &out.AccessToken
		*out = new(AccessTokenAuth)
		(*in).DeepCopyInto(*out)
	}
	if in.StaticCredentials != nil {
		in, out := &in.StaticCredentials, &out.StaticCredentials
		*out = new(StaticCredentialsAuth)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ConnectionOptions.
func (in *ConnectionOptions) DeepCopy() *ConnectionOptions {
	if in == nil {
		return nil
	}
	out := new(ConnectionOptions)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CredentialSource) DeepCopyInto(out *CredentialSource) {
	*out = *in
	if in.SecretKeyRef != nil {
		in, out := &in.SecretKeyRef, &out.SecretKeyRef
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CredentialSource.
func (in *CredentialSource) DeepCopy() *CredentialSource {
	if in == nil {
		return nil
	}
	out := new(CredentialSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Database) DeepCopyInto(out *Database) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Database.
func (in *Database) DeepCopy() *Database {
	if in == nil {
		return nil
	}
	out := new(Database)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Database) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseList) DeepCopyInto(out *DatabaseList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Database, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseList.
func (in *DatabaseList) DeepCopy() *DatabaseList {
	if in == nil {
		return nil
	}
	out := new(DatabaseList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseMonitoring) DeepCopyInto(out *DatabaseMonitoring) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseMonitoring.
func (in *DatabaseMonitoring) DeepCopy() *DatabaseMonitoring {
	if in == nil {
		return nil
	}
	out := new(DatabaseMonitoring)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseMonitoring) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseMonitoringList) DeepCopyInto(out *DatabaseMonitoringList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DatabaseMonitoring, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseMonitoringList.
func (in *DatabaseMonitoringList) DeepCopy() *DatabaseMonitoringList {
	if in == nil {
		return nil
	}
	out := new(DatabaseMonitoringList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseMonitoringList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseMonitoringSpec) DeepCopyInto(out *DatabaseMonitoringSpec) {
	*out = *in
	out.DatabaseClusterRef = in.DatabaseClusterRef
	if in.AdditionalLabels != nil {
		in, out := &in.AdditionalLabels, &out.AdditionalLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseMonitoringSpec.
func (in *DatabaseMonitoringSpec) DeepCopy() *DatabaseMonitoringSpec {
	if in == nil {
		return nil
	}
	out := new(DatabaseMonitoringSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseMonitoringStatus) DeepCopyInto(out *DatabaseMonitoringStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseMonitoringStatus.
func (in *DatabaseMonitoringStatus) DeepCopy() *DatabaseMonitoringStatus {
	if in == nil {
		return nil
	}
	out := new(DatabaseMonitoringStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseNodeSet) DeepCopyInto(out *DatabaseNodeSet) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseNodeSet.
func (in *DatabaseNodeSet) DeepCopy() *DatabaseNodeSet {
	if in == nil {
		return nil
	}
	out := new(DatabaseNodeSet)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseNodeSet) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseNodeSetList) DeepCopyInto(out *DatabaseNodeSetList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DatabaseNodeSet, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseNodeSetList.
func (in *DatabaseNodeSetList) DeepCopy() *DatabaseNodeSetList {
	if in == nil {
		return nil
	}
	out := new(DatabaseNodeSetList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseNodeSetList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseNodeSetSpec) DeepCopyInto(out *DatabaseNodeSetSpec) {
	*out = *in
	in.NodeSetSpec.DeepCopyInto(&out.NodeSetSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseNodeSetSpec.
func (in *DatabaseNodeSetSpec) DeepCopy() *DatabaseNodeSetSpec {
	if in == nil {
		return nil
	}
	out := new(DatabaseNodeSetSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseNodeSetStatus) DeepCopyInto(out *DatabaseNodeSetStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseNodeSetStatus.
func (in *DatabaseNodeSetStatus) DeepCopy() *DatabaseNodeSetStatus {
	if in == nil {
		return nil
	}
	out := new(DatabaseNodeSetStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseResources) DeepCopyInto(out *DatabaseResources) {
	*out = *in
	in.ContainerResources.DeepCopyInto(&out.ContainerResources)
	if in.StorageUnits != nil {
		in, out := &in.StorageUnits, &out.StorageUnits
		*out = make([]StorageUnit, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseResources.
func (in *DatabaseResources) DeepCopy() *DatabaseResources {
	if in == nil {
		return nil
	}
	out := new(DatabaseResources)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseServices) DeepCopyInto(out *DatabaseServices) {
	*out = *in
	in.GRPC.DeepCopyInto(&out.GRPC)
	in.Interconnect.DeepCopyInto(&out.Interconnect)
	in.Status.DeepCopyInto(&out.Status)
	in.Datastreams.DeepCopyInto(&out.Datastreams)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseServices.
func (in *DatabaseServices) DeepCopy() *DatabaseServices {
	if in == nil {
		return nil
	}
	out := new(DatabaseServices)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseSpec) DeepCopyInto(out *DatabaseSpec) {
	*out = *in
	in.Service.DeepCopyInto(&out.Service)
	out.StorageClusterRef = in.StorageClusterRef
	if in.NodeSet != nil {
		in, out := &in.NodeSet, &out.NodeSet
		*out = make([]NodeSetSpecInline, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Encryption != nil {
		in, out := &in.Encryption, &out.Encryption
		*out = new(EncryptionConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Volumes != nil {
		in, out := &in.Volumes, &out.Volumes
		*out = make([]*v1.Volume, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(v1.Volume)
				(*in).DeepCopyInto(*out)
			}
		}
	}
	if in.Datastreams != nil {
		in, out := &in.Datastreams, &out.Datastreams
		*out = new(DatastreamsConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(DatabaseResources)
		(*in).DeepCopyInto(*out)
	}
	if in.SharedResources != nil {
		in, out := &in.SharedResources, &out.SharedResources
		*out = new(DatabaseResources)
		(*in).DeepCopyInto(*out)
	}
	if in.ServerlessResources != nil {
		in, out := &in.ServerlessResources, &out.ServerlessResources
		*out = new(ServerlessDatabaseResources)
		**out = **in
	}
	in.Image.DeepCopyInto(&out.Image)
	if in.InitContainers != nil {
		in, out := &in.InitContainers, &out.InitContainers
		*out = make([]v1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Monitoring != nil {
		in, out := &in.Monitoring, &out.Monitoring
		*out = new(MonitoringOptions)
		(*in).DeepCopyInto(*out)
	}
	if in.Secrets != nil {
		in, out := &in.Secrets, &out.Secrets
		*out = make([]*v1.LocalObjectReference, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(v1.LocalObjectReference)
				**out = **in
			}
		}
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(v1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]v1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.TopologySpreadConstraints != nil {
		in, out := &in.TopologySpreadConstraints, &out.TopologySpreadConstraints
		*out = make([]v1.TopologySpreadConstraint, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.AdditionalLabels != nil {
		in, out := &in.AdditionalLabels, &out.AdditionalLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.AdditionalAnnotations != nil {
		in, out := &in.AdditionalAnnotations, &out.AdditionalAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseSpec.
func (in *DatabaseSpec) DeepCopy() *DatabaseSpec {
	if in == nil {
		return nil
	}
	out := new(DatabaseSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseStatus) DeepCopyInto(out *DatabaseStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseStatus.
func (in *DatabaseStatus) DeepCopy() *DatabaseStatus {
	if in == nil {
		return nil
	}
	out := new(DatabaseStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatastreamsConfig) DeepCopyInto(out *DatastreamsConfig) {
	*out = *in
	if in.IAMServiceAccountKey != nil {
		in, out := &in.IAMServiceAccountKey, &out.IAMServiceAccountKey
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatastreamsConfig.
func (in *DatastreamsConfig) DeepCopy() *DatastreamsConfig {
	if in == nil {
		return nil
	}
	out := new(DatastreamsConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatastreamsService) DeepCopyInto(out *DatastreamsService) {
	*out = *in
	in.Service.DeepCopyInto(&out.Service)
	if in.TLSConfiguration != nil {
		in, out := &in.TLSConfiguration, &out.TLSConfiguration
		*out = new(TLSConfiguration)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatastreamsService.
func (in *DatastreamsService) DeepCopy() *DatastreamsService {
	if in == nil {
		return nil
	}
	out := new(DatastreamsService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EncryptionConfig) DeepCopyInto(out *EncryptionConfig) {
	*out = *in
	if in.Key != nil {
		in, out := &in.Key, &out.Key
		*out = new(v1.SecretKeySelector)
		(*in).DeepCopyInto(*out)
	}
	if in.Pin != nil {
		in, out := &in.Pin, &out.Pin
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EncryptionConfig.
func (in *EncryptionConfig) DeepCopy() *EncryptionConfig {
	if in == nil {
		return nil
	}
	out := new(EncryptionConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GRPCService) DeepCopyInto(out *GRPCService) {
	*out = *in
	in.Service.DeepCopyInto(&out.Service)
	if in.TLSConfiguration != nil {
		in, out := &in.TLSConfiguration, &out.TLSConfiguration
		*out = new(TLSConfiguration)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GRPCService.
func (in *GRPCService) DeepCopy() *GRPCService {
	if in == nil {
		return nil
	}
	out := new(GRPCService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InterconnectService) DeepCopyInto(out *InterconnectService) {
	*out = *in
	in.Service.DeepCopyInto(&out.Service)
	if in.TLSConfiguration != nil {
		in, out := &in.TLSConfiguration, &out.TLSConfiguration
		*out = new(TLSConfiguration)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InterconnectService.
func (in *InterconnectService) DeepCopy() *InterconnectService {
	if in == nil {
		return nil
	}
	out := new(InterconnectService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MonitoringOptions) DeepCopyInto(out *MonitoringOptions) {
	*out = *in
	if in.MetricRelabelings != nil {
		in, out := &in.MetricRelabelings, &out.MetricRelabelings
		*out = make([]*monitoringv1.RelabelConfig, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(monitoringv1.RelabelConfig)
				(*in).DeepCopyInto(*out)
			}
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MonitoringOptions.
func (in *MonitoringOptions) DeepCopy() *MonitoringOptions {
	if in == nil {
		return nil
	}
	out := new(MonitoringOptions)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespacedRef) DeepCopyInto(out *NamespacedRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespacedRef.
func (in *NamespacedRef) DeepCopy() *NamespacedRef {
	if in == nil {
		return nil
	}
	out := new(NamespacedRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeSetSpec) DeepCopyInto(out *NodeSetSpec) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(v1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.DataStore != nil {
		in, out := &in.DataStore, &out.DataStore
		*out = make([]v1.PersistentVolumeClaimSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(v1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]v1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.TopologySpreadConstraints != nil {
		in, out := &in.TopologySpreadConstraints, &out.TopologySpreadConstraints
		*out = make([]v1.TopologySpreadConstraint, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeSetSpec.
func (in *NodeSetSpec) DeepCopy() *NodeSetSpec {
	if in == nil {
		return nil
	}
	out := new(NodeSetSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NodeSetSpecInline) DeepCopyInto(out *NodeSetSpecInline) {
	*out = *in
	if in.AdditionalLabels != nil {
		in, out := &in.AdditionalLabels, &out.AdditionalLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.NodeSetSpec.DeepCopyInto(&out.NodeSetSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NodeSetSpecInline.
func (in *NodeSetSpecInline) DeepCopy() *NodeSetSpecInline {
	if in == nil {
		return nil
	}
	out := new(NodeSetSpecInline)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodImage) DeepCopyInto(out *PodImage) {
	*out = *in
	if in.PullPolicyName != nil {
		in, out := &in.PullPolicyName, &out.PullPolicyName
		*out = new(v1.PullPolicy)
		**out = **in
	}
	if in.PullSecret != nil {
		in, out := &in.PullSecret, &out.PullSecret
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodImage.
func (in *PodImage) DeepCopy() *PodImage {
	if in == nil {
		return nil
	}
	out := new(PodImage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ServerlessDatabaseResources) DeepCopyInto(out *ServerlessDatabaseResources) {
	*out = *in
	out.SharedDatabaseRef = in.SharedDatabaseRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ServerlessDatabaseResources.
func (in *ServerlessDatabaseResources) DeepCopy() *ServerlessDatabaseResources {
	if in == nil {
		return nil
	}
	out := new(ServerlessDatabaseResources)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Service) DeepCopyInto(out *Service) {
	*out = *in
	if in.AdditionalLabels != nil {
		in, out := &in.AdditionalLabels, &out.AdditionalLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.AdditionalAnnotations != nil {
		in, out := &in.AdditionalAnnotations, &out.AdditionalAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.IPFamilies != nil {
		in, out := &in.IPFamilies, &out.IPFamilies
		*out = make([]v1.IPFamily, len(*in))
		copy(*out, *in)
	}
	if in.IPFamilyPolicy != nil {
		in, out := &in.IPFamilyPolicy, &out.IPFamilyPolicy
		*out = new(v1.IPFamilyPolicy)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Service.
func (in *Service) DeepCopy() *Service {
	if in == nil {
		return nil
	}
	out := new(Service)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SharedDatabaseRef) DeepCopyInto(out *SharedDatabaseRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SharedDatabaseRef.
func (in *SharedDatabaseRef) DeepCopy() *SharedDatabaseRef {
	if in == nil {
		return nil
	}
	out := new(SharedDatabaseRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StaticCredentialsAuth) DeepCopyInto(out *StaticCredentialsAuth) {
	*out = *in
	if in.Password != nil {
		in, out := &in.Password, &out.Password
		*out = new(CredentialSource)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StaticCredentialsAuth.
func (in *StaticCredentialsAuth) DeepCopy() *StaticCredentialsAuth {
	if in == nil {
		return nil
	}
	out := new(StaticCredentialsAuth)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StatusService) DeepCopyInto(out *StatusService) {
	*out = *in
	in.Service.DeepCopyInto(&out.Service)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StatusService.
func (in *StatusService) DeepCopy() *StatusService {
	if in == nil {
		return nil
	}
	out := new(StatusService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Storage) DeepCopyInto(out *Storage) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Storage.
func (in *Storage) DeepCopy() *Storage {
	if in == nil {
		return nil
	}
	out := new(Storage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Storage) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageList) DeepCopyInto(out *StorageList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Storage, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageList.
func (in *StorageList) DeepCopy() *StorageList {
	if in == nil {
		return nil
	}
	out := new(StorageList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StorageList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageMonitoring) DeepCopyInto(out *StorageMonitoring) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageMonitoring.
func (in *StorageMonitoring) DeepCopy() *StorageMonitoring {
	if in == nil {
		return nil
	}
	out := new(StorageMonitoring)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StorageMonitoring) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageMonitoringList) DeepCopyInto(out *StorageMonitoringList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]StorageMonitoring, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageMonitoringList.
func (in *StorageMonitoringList) DeepCopy() *StorageMonitoringList {
	if in == nil {
		return nil
	}
	out := new(StorageMonitoringList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StorageMonitoringList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageMonitoringSpec) DeepCopyInto(out *StorageMonitoringSpec) {
	*out = *in
	out.StorageRef = in.StorageRef
	if in.AdditionalLabels != nil {
		in, out := &in.AdditionalLabels, &out.AdditionalLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageMonitoringSpec.
func (in *StorageMonitoringSpec) DeepCopy() *StorageMonitoringSpec {
	if in == nil {
		return nil
	}
	out := new(StorageMonitoringSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageMonitoringStatus) DeepCopyInto(out *StorageMonitoringStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageMonitoringStatus.
func (in *StorageMonitoringStatus) DeepCopy() *StorageMonitoringStatus {
	if in == nil {
		return nil
	}
	out := new(StorageMonitoringStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageNodeSet) DeepCopyInto(out *StorageNodeSet) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageNodeSet.
func (in *StorageNodeSet) DeepCopy() *StorageNodeSet {
	if in == nil {
		return nil
	}
	out := new(StorageNodeSet)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StorageNodeSet) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageNodeSetList) DeepCopyInto(out *StorageNodeSetList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]StorageNodeSet, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageNodeSetList.
func (in *StorageNodeSetList) DeepCopy() *StorageNodeSetList {
	if in == nil {
		return nil
	}
	out := new(StorageNodeSetList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StorageNodeSetList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageNodeSetSpec) DeepCopyInto(out *StorageNodeSetSpec) {
	*out = *in
	in.NodeSetSpec.DeepCopyInto(&out.NodeSetSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageNodeSetSpec.
func (in *StorageNodeSetSpec) DeepCopy() *StorageNodeSetSpec {
	if in == nil {
		return nil
	}
	out := new(StorageNodeSetSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageNodeSetStatus) DeepCopyInto(out *StorageNodeSetStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageNodeSetStatus.
func (in *StorageNodeSetStatus) DeepCopy() *StorageNodeSetStatus {
	if in == nil {
		return nil
	}
	out := new(StorageNodeSetStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageRef) DeepCopyInto(out *StorageRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageRef.
func (in *StorageRef) DeepCopy() *StorageRef {
	if in == nil {
		return nil
	}
	out := new(StorageRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageServices) DeepCopyInto(out *StorageServices) {
	*out = *in
	in.GRPC.DeepCopyInto(&out.GRPC)
	in.Interconnect.DeepCopyInto(&out.Interconnect)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageServices.
func (in *StorageServices) DeepCopy() *StorageServices {
	if in == nil {
		return nil
	}
	out := new(StorageServices)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageSpec) DeepCopyInto(out *StorageSpec) {
	*out = *in
	if in.DataStore != nil {
		in, out := &in.DataStore, &out.DataStore
		*out = make([]v1.PersistentVolumeClaimSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.NodeSet != nil {
		in, out := &in.NodeSet, &out.NodeSet
		*out = make([]NodeSetSpecInline, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.OperatorConnection != nil {
		in, out := &in.OperatorConnection, &out.OperatorConnection
		*out = new(ConnectionOptions)
		(*in).DeepCopyInto(*out)
	}
	in.Service.DeepCopyInto(&out.Service)
	in.Resources.DeepCopyInto(&out.Resources)
	in.Image.DeepCopyInto(&out.Image)
	if in.InitContainers != nil {
		in, out := &in.InitContainers, &out.InitContainers
		*out = make([]v1.Container, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Monitoring != nil {
		in, out := &in.Monitoring, &out.Monitoring
		*out = new(MonitoringOptions)
		(*in).DeepCopyInto(*out)
	}
	if in.Secrets != nil {
		in, out := &in.Secrets, &out.Secrets
		*out = make([]*v1.LocalObjectReference, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(v1.LocalObjectReference)
				**out = **in
			}
		}
	}
	if in.Volumes != nil {
		in, out := &in.Volumes, &out.Volumes
		*out = make([]*v1.Volume, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(v1.Volume)
				(*in).DeepCopyInto(*out)
			}
		}
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(v1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]v1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.TopologySpreadConstraints != nil {
		in, out := &in.TopologySpreadConstraints, &out.TopologySpreadConstraints
		*out = make([]v1.TopologySpreadConstraint, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.AdditionalLabels != nil {
		in, out := &in.AdditionalLabels, &out.AdditionalLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.AdditionalAnnotations != nil {
		in, out := &in.AdditionalAnnotations, &out.AdditionalAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageSpec.
func (in *StorageSpec) DeepCopy() *StorageSpec {
	if in == nil {
		return nil
	}
	out := new(StorageSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageStatus) DeepCopyInto(out *StorageStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageStatus.
func (in *StorageStatus) DeepCopy() *StorageStatus {
	if in == nil {
		return nil
	}
	out := new(StorageStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StorageUnit) DeepCopyInto(out *StorageUnit) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StorageUnit.
func (in *StorageUnit) DeepCopy() *StorageUnit {
	if in == nil {
		return nil
	}
	out := new(StorageUnit)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSConfiguration) DeepCopyInto(out *TLSConfiguration) {
	*out = *in
	in.CertificateAuthority.DeepCopyInto(&out.CertificateAuthority)
	in.Certificate.DeepCopyInto(&out.Certificate)
	in.Key.DeepCopyInto(&out.Key)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSConfiguration.
func (in *TLSConfiguration) DeepCopy() *TLSConfiguration {
	if in == nil {
		return nil
	}
	out := new(TLSConfiguration)
	in.DeepCopyInto(out)
	return out
}
