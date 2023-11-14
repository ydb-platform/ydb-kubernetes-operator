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
	YdbAuthToken        = "ydb-auth-token-file"

	ConfigDir      = "/opt/ydb/cfg"
	ConfigFileName = "config.yaml"

	BinariesDir      = "/opt/ydb/bin"
	DaemonBinaryName = "ydbd"

	DefaultRootUsername = "root"
	DefaultRootPassword = ""

	AnnotationUpdateStrategyOnDelete = "ydb.tech/update-strategy-on-delete"
	AnnotationUpdateDNSPolicy        = "ydb.tech/update-dns-policy"
	AnnotationSkipInitialization     = "ydb.tech/skip-initialization"
	AnnotationDisableLivenessProbe   = "ydb.tech/disable-liveness-probe"
	AnnotationDataCenter             = "ydb.tech/data-center"
	AnnotationNodeHost               = "ydb.tech/node-host"
	AnnotationNodeDomain             = "ydb.tech/node-domain"
	AnnotationCABundleSecret         = "ydb.tech/ca-bundle-secret"

	AnnotationValueTrue = "true"

	legacyTenantNameFormat = "/%s/%s"
)

type ErasureType string

const (
	ErasureBlock42   ErasureType = "block-4-2"
	ErasureMirror3DC ErasureType = "mirror-3-dc"
	None             ErasureType = "none"
)
