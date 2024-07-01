package annotations

const (
	PrimaryResourceStorageAnnotation  = "ydb.tech/primary-resource-storage"
	PrimaryResourceDatabaseAnnotation = "ydb.tech/primary-resource-database"
	RemoteResourceVersionAnnotation   = "ydb.tech/remote-resource-version"
	ConfigurationChecksum             = "ydb.tech/configuration-checksum"
	RemoteFinalizerKey                = "ydb.tech/remote-finalizer"
	LastAppliedAnnotation             = "ydb.tech/last-applied"
)

func CompareLastAppliedAnnotation(map1, map2 map[string]string) bool {
	value1 := getLastAppliedAnnotation(map1)
	value2 := getLastAppliedAnnotation(map2)
	return value1 == value2
}

func getLastAppliedAnnotation(annotations map[string]string) string {
	for key, value := range annotations {
		if key == LastAppliedAnnotation {
			return value
		}
	}
	return ""
}
