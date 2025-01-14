package resources

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/ptr"
)

func contains(s []corev1.Capability, v corev1.Capability) bool {
	for _, vs := range s {
		if vs == v {
			return true
		}
	}
	return false
}

func mergeSecurityContextWithDefaults(context *corev1.SecurityContext) *corev1.SecurityContext {
	var result *corev1.SecurityContext
	defaultCapabilities := []corev1.Capability{"SYS_RAWIO"}

	if context != nil {
		result = context.DeepCopy()
	} else {
		result = &corev1.SecurityContext{}
	}

	// set defaults

	if result.Privileged == nil {
		result.Privileged = ptr.Bool(false)
	}

	if result.Capabilities == nil {
		result.Capabilities = &corev1.Capabilities{
			Add: []corev1.Capability{},
		}
	}

	for _, defaultCapability := range defaultCapabilities {
		if !contains(result.Capabilities.Add, defaultCapability) {
			result.Capabilities.Add = append(result.Capabilities.Add, defaultCapability)
		}
	}

	return result
}
