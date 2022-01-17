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
func (in *Database) DeepCopyInto(out *Database) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	if in.StorageRef != nil {
		in, out := &in.StorageRef, &out.StorageRef
		*out = new(Storage)
		(*in).DeepCopyInto(*out)
	}
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
	if in.AdditionalLabels != nil {
		in, out := &in.AdditionalLabels, &out.AdditionalLabels
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
		*out = new(v1.IPFamilyPolicyType)
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
	in.Service.DeepCopyInto(&out.Service)
	in.Resources.DeepCopyInto(&out.Resources)
	in.Image.DeepCopyInto(&out.Image)
	in.Monitoring.DeepCopyInto(&out.Monitoring)
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
	if in.AdditionalLabels != nil {
		in, out := &in.AdditionalLabels, &out.AdditionalLabels
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
