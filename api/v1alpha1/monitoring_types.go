package v1alpha1

import v1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

type MonitoringOptions struct {
	Enabled bool `json:"enabled"`

	// Interval at which metrics should be scraped
	Interval string `json:"interval,omitempty"`
	// RelabelConfig allows dynamic rewriting of the label set, being applied to sample before ingestion.
	MetricRelabelings []*v1.RelabelConfig `json:"metricRelabelings,omitempty"`
}
