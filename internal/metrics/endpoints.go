package metrics

import (
	"fmt"

	v1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

type MetricsService struct {
	Name        string
	Path        string
	Relabelings []*v1.RelabelConfig
}

func getMetricsServices(services []string) []MetricsService {
	var metricsServices []MetricsService

	for _, serviceName := range storageMetricsServices {
		var servicePath string
		if serviceName == "ydb" || serviceName == "ydb_serverless" {
			servicePath = fmt.Sprintf(MetricEndpointFormat, serviceName+"/name_label=name")
		} else {
			servicePath = fmt.Sprintf(MetricEndpointFormat, serviceName)
		}
		metricsServices = append(metricsServices, MetricsService{
			Name:        serviceName,
			Path:        servicePath,
			Relabelings: GetMetricsRelabelings(serviceName),
		})
	}

	return metricsServices
}

func GetStorageMetricsServices() []MetricsService {
	return getMetricsServices(storageMetricsServices)
}

func GetDatabaseMetricsServices() []MetricsService {
	return getMetricsServices(databaseMetricsServices)
}
