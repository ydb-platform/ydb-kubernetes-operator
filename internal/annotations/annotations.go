package annotations

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

const (
	PrimaryResourceStorage  = "ydb.tech/primary-resource-storage"
	PrimaryResourceDatabase = "ydb.tech/primary-resource-database"
	RemoteResourceVersion   = "ydb.tech/remote-resource-version"
	ConfigurationChecksum   = "ydb.tech/configuration-checksum"
	LastApplied             = "ydb.tech/last-applied"
)

var (
	AcceptedDNSPolicy = []string{
		string(corev1.DNSClusterFirstWithHostNet),
		string(corev1.DNSClusterFirst),
		string(corev1.DNSDefault),
		string(corev1.DNSNone),
	}
	UserAnnotations = map[string]struct{}{
		v1alpha1.AnnotationSkipInitialization:     {},
		v1alpha1.AnnotationUpdateStrategyOnDelete: {},
		v1alpha1.AnnotationUpdateDNSPolicy:        {},
		v1alpha1.AnnotationDisableLivenessProbe:   {},
		v1alpha1.AnnotationDataCenter:             {},
		v1alpha1.AnnotationGRPCPublicHost:         {},
		v1alpha1.AnnotationNodeHost:               {},
		v1alpha1.AnnotationNodeDomain:             {},
	}
)

type Annotations map[string]string

func Common(objAnnotations Annotations) Annotations {
	an := Annotations{}

	an.Merge(getUserAnnotations(objAnnotations))

	return an
}

func (an Annotations) Merge(other map[string]string) map[string]string {
	if other == nil {
		return an
	}

	for k, v := range other {
		an[k] = v
	}

	return an
}

func (an Annotations) AsMap() map[string]string {
	return an
}

func (an Annotations) Copy() Annotations {
	res := Annotations{}

	for k, v := range an {
		res[k] = v
	}

	return res
}

func getUserAnnotations(annotations map[string]string) map[string]string {
	common := make(map[string]string)

	for key, value := range annotations {
		if _, exists := UserAnnotations[key]; exists {
			common[key] = value
		}
	}

	return common
}

func CompareLastAppliedAnnotation(map1, map2 map[string]string) bool {
	value1 := getLastAppliedAnnotation(map1)
	value2 := getLastAppliedAnnotation(map2)
	return value1 == value2
}

func getLastAppliedAnnotation(annotations map[string]string) string {
	for key, value := range annotations {
		if key == LastApplied {
			return value
		}
	}
	return ""
}
