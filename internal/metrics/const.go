package metrics

const (
	MetricEndpointFormat = "/counters/counters=%s/prometheus"
)

var storageMetricsServices = []string{ // NB dashes are not allowed in the metric service name
	"ydb",
	"auth",
	"coordinator",
	"dsproxy_queue",
	"dsproxy",
	"grpc",
	"kqp",
	"pdisks",
	"processing",
	"proxy",
	"followers",
	"storage_pool_stat",
	"streaming",
	"tablets",
	"utils",
	"yql",
	"dsproxynode",
	"interconnect",
	"vdisks",
}

var databaseMetricsServices = []string{ // NB dashes are not allowed in the metric service name
	"ydb",
	"ydb_serverless",
	"auth",
	"coordinator",
	"dsproxy_queue",
	"dsproxy",
	"grpc",
	"kqp",
	"processing",
	"proxy",
	"followers",
	"storage_pool_stat",
	"streaming",
	"tablets",
	"utils",
	"yql",
	"dsproxynode",
	"interconnect",
	"vdisks",
}
