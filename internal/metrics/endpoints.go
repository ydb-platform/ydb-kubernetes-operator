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

func GetStorageMetricsServices() []MetricsService {
	services := make([]MetricsService, len(storageMetricsServices))

	for _, serviceName := range storageMetricsServices {
		services = append(services, MetricsService{
			Name:        serviceName,
			Path:        fmt.Sprintf(MetricEndpointFormat, serviceName),
			Relabelings: GetStorageMetricsRelabelings(serviceName),
		})
	}

	return services
}
