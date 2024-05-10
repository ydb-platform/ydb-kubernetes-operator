package resources

import (
	"errors"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/metrics"
)

type ServiceMonitorBuilder struct {
	client.Object

	Name            string
	MetricsServices []metrics.Service
	TargetPort      int
	Options         *api.MonitoringOptions

	Labels         labels.Labels
	SelectorLabels labels.Labels
}

func (b *ServiceMonitorBuilder) Build(obj client.Object) error {
	sm, ok := obj.(*monitoringv1.ServiceMonitor)
	if !ok {
		return errors.New("failed to cast to ServiceMonitor object")
	}

	if sm.ObjectMeta.Name == "" {
		sm.ObjectMeta.Name = b.Object.GetName()
	}

	sm.ObjectMeta.Namespace = b.GetNamespace()
	sm.ObjectMeta.Labels = b.Labels

	sm.Spec.Endpoints = b.buildEndpoints()
	sm.Spec.NamespaceSelector = monitoringv1.NamespaceSelector{
		MatchNames: []string{
			b.GetNamespace(),
		},
	}

	sm.Spec.Selector = metav1.LabelSelector{
		MatchLabels: b.SelectorLabels,
	}

	return nil
}

func (b *ServiceMonitorBuilder) buildEndpoints() []monitoringv1.Endpoint {
	endpoints := make([]monitoringv1.Endpoint, 0, len(b.MetricsServices))

	for _, service := range b.MetricsServices {
		metricRelabelings := service.Relabelings
		if len(b.Options.MetricRelabelings) > 0 {
			metricRelabelings = append(metricRelabelings, b.Options.MetricRelabelings...)
		}

		endpoints = append(endpoints, monitoringv1.Endpoint{
			Path:                 service.Path,
			TargetPort:           &intstr.IntOrString{IntVal: int32(b.TargetPort)},
			MetricRelabelConfigs: metricRelabelings,
		})
	}

	return endpoints
}

func (b *ServiceMonitorBuilder) Placeholder(cr client.Object) client.Object {
	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.GetName(),
			Namespace: cr.GetNamespace(),
		},
	}
}
