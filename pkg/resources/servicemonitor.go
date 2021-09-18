package resources

import (
	"errors"
	"fmt"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/ydb-platform/ydb-kubernetes-operator/pkg/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/pkg/metrics"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceMonitorBuilder struct {
	client.Object

	Name string

	Labels labels.Labels
}

func (b *ServiceMonitorBuilder) Build(obj client.Object) error {
	sm, ok := obj.(*monitoringv1.ServiceMonitor)
	if !ok {
		return errors.New("failed to cast to ServiceMonitor object")
	}

	if sm.ObjectMeta.Name == "" {
		sm.ObjectMeta.Name = b.Name
	}

	sm.ObjectMeta.Namespace = b.GetNamespace()
	sm.ObjectMeta.Labels = b.Labels

	sm.Spec.Endpoints = []monitoringv1.Endpoint{
		{
			TargetPort: nil,
			Path:       fmt.Sprintf(metrics.StorageMetricsEndpointFormat, b.Name),
			MetricRelabelConfigs: metrics.GetStorageMetricRelabelings(
				b.Name,
			),
		},
	}
	sm.Spec.NamespaceSelector = monitoringv1.NamespaceSelector{
		MatchNames: []string{
			b.GetNamespace(),
		},
	}

	sm.Spec.Selector = metav1.LabelSelector{
		MatchLabels: b.GetLabels(),
	}

	return nil
}

func (b *ServiceMonitorBuilder) Placeholder(cr client.Object) client.Object {
	name := strings.Replace(b.Name, "_", "-", -1)

	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.GetNamespace(),
		},
	}
}
