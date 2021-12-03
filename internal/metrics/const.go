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
	"slaves",
	"storage_pool_stat",
	"streaming",
	"tablets",
	"utils",
	"yq",
	"yql",
	"dsproxynode",
	"interconnect",
	"vdisks",
}
