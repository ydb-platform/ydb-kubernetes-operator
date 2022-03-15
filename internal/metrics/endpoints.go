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
		metricsServices = append(metricsServices, MetricsService{
			Name:        serviceName,
			Path:        fmt.Sprintf(MetricEndpointFormat, serviceName),
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
