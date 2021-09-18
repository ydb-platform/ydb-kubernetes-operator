package v1alpha1

const (
	//RegistryPath = "cr.yandex/ydb/ydb"
	RegistryPath = "cr.yandex/crpbo4q9lbgkn85vr1rm/ydb" // fixme
	DefaultTag   = "21.4.14"

	ImagePathFormat = "%s:%s"

	GRPCPort         = 2135
	InterconnectPort = 19001
	StatusPort       = 8765

	DiskPath = "/dev/kikimr_ssd_01"
)
