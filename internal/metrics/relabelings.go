package metrics

import (
	"fmt"

	v1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

func GetMetricsRelabelings(metricsService string) []*v1.RelabelConfig {
	return []*v1.RelabelConfig{{
		SourceLabels: []string{"__name__"},
		TargetLabel:  "__name__",
		Regex:        "(.*)",
		Replacement:  fmt.Sprintf("%s_$1", metricsService),
	}}
}
