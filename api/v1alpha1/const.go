package v1alpha1

const (
	RegistryPath = "cr.yandex/yc/ydb"
	DefaultTag   = "21.4.29"

	ImagePathFormat = "%s:%s"

	GRPCPort            = 2135
	GRPCServicePortName = "grpc"
	GRPCProto           = "grpc://"
	GRPCSProto          = "grpcs://"

	InterconnectPort            = 19001
	InterconnectServicePortName = "interconnect"

	StatusPort            = 8765
	StatusServicePortName = "status"

	DiskPathPrefix      = "/dev/kikimr_ssd"
	DiskNumberMaxDigits = 2
	DiskFilePath        = "/data"

	ConfigDir      = "/opt/ydb/cfg"
	ConfigFileName = "config.yaml"

	BinariesDir      = "/opt/ydb/bin"
	DaemonBinaryName = "ydbd"

	TenantNameFormat = "/%s/%s"
)

type ErasureType string

const (
	ErasureBlock42   ErasureType = "block-4-2"
	ErasureMirror3DC ErasureType = "mirror-3-dc"
	None             ErasureType = "none"
)
