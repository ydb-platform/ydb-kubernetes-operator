package v1alpha1

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

const (
	DefaultDatabaseDomain = "root"
)

// SetDatabaseSpecDefaults sets various values to the default vars.
func SetDatabaseSpecDefaults(ydbCr *Database, ydbSpec *DatabaseSpec) {
	if ydbSpec.StorageClusterRef.Namespace == "" {
		ydbSpec.StorageClusterRef.Namespace = ydbCr.Namespace
	}

	if ydbSpec.ServerlessResources != nil {
		if ydbSpec.ServerlessResources.SharedDatabaseRef.Namespace == "" {
			ydbSpec.ServerlessResources.SharedDatabaseRef.Namespace = ydbCr.Namespace
		}
	}

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

	if ydbSpec.Domain == "" {
		ydbSpec.Domain = DefaultDatabaseDomain
	}

	if ydbSpec.Service.GRPC.TLSConfiguration == nil {
		ydbSpec.Service.GRPC.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}
	if ydbSpec.Service.Interconnect.TLSConfiguration == nil {
		ydbSpec.Service.Interconnect.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}
	if ydbSpec.Service.Datastreams.TLSConfiguration == nil {
		ydbSpec.Service.Datastreams.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}
}
