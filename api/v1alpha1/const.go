package v1alpha1

const (
	RegistryPath = "cr.yandex/crptqonuodf51kdj7a7d/ydb"
	DefaultTag   = "22.2.22"

	ImagePathFormat = "%s:%s"

	DefaultDomainName   = "cluster.local"
	DNSDomainAnnotation = "dns.domain"

	GRPCPort              = 2135
	GRPCServicePortName   = "grpc"
	GRPCProto             = "grpc://"
	GRPCSProto            = "grpcs://"
	GRPCServiceFQDNFormat = "%s-grpc.%s.svc.%s"

	InterconnectPort              = 19001
	InterconnectServicePortName   = "interconnect"
	InterconnectServiceFQDNFormat = "%s-interconnect.%s.svc.%s"

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

	DatabaseEncryptionKeySecretDir  = "database_encryption"
	DatabaseEncryptionKeySecretFile = "key"
	DatabaseEncryptionKeyConfigFile = "key.txt"

	BinariesDir      = "/opt/ydb/bin"
	DaemonBinaryName = "ydbd"

	DefaultRootUsername          = "root"
	DefaultRootPassword          = ""
	DefaultDatabaseDomain        = "Root"
	DefaultDatabaseEncryptionPin = "EmptyPin"
	DefaultSignAlgorithm         = "RS256"

	LabelDeploymentKey             = "deployment"
	LabelDeploymentValueKubernetes = "kubernetes"
	LabelSharedDatabaseKey         = "shared"
	LabelSharedDatabaseValueTrue   = "true"
	LabelSharedDatabaseValueFalse  = "false"

	AnnotationUpdateStrategyOnDelete = "ydb.tech/update-strategy-on-delete"
	AnnotationUpdateDNSPolicy        = "ydb.tech/update-dns-policy"
	AnnotationSkipInitialization     = "ydb.tech/skip-initialization"
	AnnotationDisableLivenessProbe   = "ydb.tech/disable-liveness-probe"
	AnnotationDataCenter             = "ydb.tech/data-center"
	AnnotationGRPCPublicHost         = "ydb.tech/grpc-public-host"
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
