package annotations

import "strings"

const (
	PrimaryResourceStorageAnnotation  = "ydb.tech/primary-resource-storage"
	PrimaryResourceDatabaseAnnotation = "ydb.tech/primary-resource-database"
	RemoteResourceVersionAnnotation   = "ydb.tech/remote-resource-version"
	ConfigurationChecksum             = "ydb.tech/configuration-checksum"
	StorageFinalizerKey               = "ydb.tech/storage-finalizer"
	RemoteFinalizerKey                = "ydb.tech/remote-finalizer"
	LastAppliedAnnotation             = "ydb.tech/last-applied"
)

func GetYdbTechAnnotations(annotations map[string]string) map[string]string {
	result := make(map[string]string)
	for key, value := range annotations {
		if strings.HasPrefix(key, "ydb.tech/") {
			result[key] = value
		}
	}
	return result
}

func CompareMaps(map1, map2 map[string]string) bool {
	if len(map1) != len(map2) {
		return false
	}
	for key1, value1 := range map1 {
		if value2, ok := map2[key1]; !ok || value2 != value1 {
			return false
		}
	}
	return true
}

func CompareYdbTechAnnotations(map1, map2 map[string]string) bool {
	map1 = GetYdbTechAnnotations(map1)
	map2 = GetYdbTechAnnotations(map2)
	return CompareMaps(map1, map2)
}
