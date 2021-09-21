package metrics

import "strings"

const (
	StorageMetricsEndpointFormat = "/counters/counters=%s/prometheus"
)

type MetricEndpoint struct {
	Name        string
	MonitorName string
}

var storageMetricEndpoints = []string{
	"ydb",
	"auth",
	"coordinator",
	"dsproxy_queue",
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

func GetStorageMetricEndpoints() []MetricEndpoint {
	result := make([]MetricEndpoint, len(storageMetricEndpoints))

	for i, e := range storageMetricEndpoints {
		result[i] = MetricEndpoint{
			Name:        e,
			MonitorName: strings.Replace(e, "_", "-", -1),
		}
	}

	return result
}
