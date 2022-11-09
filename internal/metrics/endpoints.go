package metrics

import (
	"fmt"

	v1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

type Service struct {
	Name        string
	Path        string
	Relabelings []*v1.RelabelConfig
}

func getMetricsServices(services []string) []Service {
	_ = services

	metricsServices := make([]Service, 0, len(storageMetricsServices))
	for _, serviceName := range storageMetricsServices {
		var servicePath string
		if serviceName == "ydb" || serviceName == "ydb_serverless" {
			servicePath = fmt.Sprintf(MetricEndpointFormat, serviceName+"/name_label=name")
		} else {
			servicePath = fmt.Sprintf(MetricEndpointFormat, serviceName)
		}
		metricsServices = append(metricsServices, Service{
			Name:        serviceName,
			Path:        servicePath,
			Relabelings: GetMetricsRelabelings(serviceName),
		})
	}

	return metricsServices
}

func GetStorageMetricsServices() []Service {
	return getMetricsServices(storageMetricsServices)
}

func GetDatabaseMetricsServices() []Service {
	return getMetricsServices(databaseMetricsServices)
}
