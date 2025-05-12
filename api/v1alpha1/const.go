package v1alpha1

const (
	RegistryPath = "cr.yandex/crptqonuodf51kdj7a7d/ydb"
	DefaultTag   = "22.2.22"

	ImagePathFormat = "%s:%s"

	DefaultDomainName   = "cluster.local"
	DNSDomainAnnotation = "dns.domain"

	GRPCPort                      = 2135
	GRPCServicePortName           = "grpc"
	GRPCServiceAdditionalPortName = "additional-grpc"
	GRPCProto                     = "grpc://"
	GRPCSProto                    = "grpcs://"
	GRPCServiceFQDNFormat         = "%s-grpc.%s.svc.%s"

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

	AuthTokenSecretName = "ydb-auth-token-file"
	AuthTokenSecretKey  = "ydb-auth-token-file"
	AuthTokenFileArg    = "--auth-token-file"

	DatabaseEncryptionKeySecretDir  = "database_encryption"
	DatabaseEncryptionKeySecretFile = "key"
	DatabaseEncryptionKeyConfigFile = "key.txt"

	ConfigDir      = "/opt/ydb/cfg"
	ConfigFileName = "config.yaml"

	BinariesDir      = "/opt/ydb/bin"
	DaemonBinaryName = "ydbd"

	AdditionalSecretsDir = "/opt/ydb/secrets"
	AdditionalVolumesDir = "/opt/ydb/volumes"

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
	AnnotationGRPCPublicPort         = "ydb.tech/grpc-public-port"
	AnnotationNodeHost               = "ydb.tech/node-host"
	AnnotationNodeDomain             = "ydb.tech/node-domain"
	AnnotationAuthTokenSecretName    = "ydb.tech/auth-token-secret-name"
	AnnotationAuthTokenSecretKey     = "ydb.tech/auth-token-secret-key"

	AnnotationValueTrue = "true"

	legacyTenantNameFormat = "/%s/%s"
)

type ErasureType string

const (
	ErasureBlock42   ErasureType = "block-4-2"
	ErasureMirror3DC ErasureType = "mirror-3-dc"
	None             ErasureType = "none"
)
