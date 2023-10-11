package annotations

import "strings"

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
	for key, value := range map1 {
		if val, ok := map2[key]; !ok || val != value {
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
