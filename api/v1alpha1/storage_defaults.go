package v1alpha1

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

// SetStorageClusterSpecDefaults sets various values to the
// default vars.
func SetStorageClusterSpecDefaults(ydbSpec *StorageSpec) {
	if ydbSpec.Image.Name == "" {
		if ydbSpec.YDBVersion == "" {
			ydbSpec.Image.Name = fmt.Sprintf(ImagePathFormat, RegistryPath, DefaultTag)
		} else {
			ydbSpec.Image.Name = fmt.Sprintf(ImagePathFormat, RegistryPath, ydbSpec.YDBVersion)
		}
	}

	if ydbSpec.Image.PullPolicyName == nil {
		policy := v1.PullIfNotPresent
		ydbSpec.Image.PullPolicyName = &policy
	}

	if ydbSpec.Service.GRPC.TLSConfiguration == nil {
		ydbSpec.Service.GRPC.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}
	if ydbSpec.Service.Interconnect.TLSConfiguration == nil {
		ydbSpec.Service.Interconnect.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}
}
