package annotations

import "strings"

const (
	PrimaryResourceNameAnnotation      = "ydb.tech/primary-resource-name"
	PrimaryResourceNamespaceAnnotation = "ydb.tech/primary-resource-namespace"
	PrimaryResourceTypeAnnotation      = "ydb.tech/primary-resource-type"
	PrimaryResourceUIDAnnotation       = "ydb.tech/primary-resource-uid"

	RemoteResourceVersionAnnotation = "ydb.tech/remote-resource-version"
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
