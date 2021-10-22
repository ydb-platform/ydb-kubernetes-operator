package v1alpha1

const (
	RegistryPath = "cr.yandex/yc/ydb"
	DefaultTag   = "21.4.29"

	ImagePathFormat = "%s:%s"

	GRPCPort         = 2135
	InterconnectPort = 19001
	StatusPort       = 8765

	DiskPathPrefix      = "/dev/kikimr_ssd"
	DiskNumberMaxDigits = 2

	ConfigDir = "/opt/kikimr/cfg"
)
