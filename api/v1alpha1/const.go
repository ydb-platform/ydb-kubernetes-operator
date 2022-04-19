package v1alpha1

const (
	RegistryPath = "cr.yandex/crptqonuodf51kdj7a7d/ydb"
	DefaultTag   = "22.2.22"

	ImagePathFormat = "%s:%s"

	GRPCPort            = 2135
	GRPCServicePortName = "grpc"
	GRPCProto           = "grpc://"
	GRPCSProto          = "grpcs://"

	InterconnectPort            = 19001
	InterconnectServicePortName = "interconnect"

	StatusPort            = 8765
	StatusServicePortName = "status"

	DatastreamsPort            = 8443
	DatastreamsServicePortName = "datastreams"

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
