package monitoring

import (
	"context"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/metrics"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

const (
	DefaultRequeueDelay = 10 * time.Second
)

type Syncer struct {
	client.Client

	Object   client.Object
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme

	metricsServices []metrics.Service
}

func (r *Syncer) Sync(ctx context.Context, obj client.Object) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	svc, err := r.findService(ctx, obj)

	if svc == nil {
		if err != nil {
			r.Recorder.Eventf(r.Object, corev1.EventTypeWarning, "Error", "Unable to find service for db %s/%s",
				r.Object.GetNamespace(), r.Object.GetName())

			log.FromContext(ctx).Error(err, "Unable to find svc for db", "cr", r.Object, "db", obj)
		} else {
			r.Recorder.Eventf(r.Object, corev1.EventTypeWarning, "Error", "No status service found for db %s/%s. Stop.",
				r.Object.GetNamespace(), r.Object.GetName())
		}
		return ctrl.Result{}, err
	}

	statusServiceLabels := labels.Labels{}
	statusServiceLabels.Merge(svc.Labels)

	monitorLabels := labels.Common(obj.GetName(), obj.GetLabels())
	monitorLabels.Merge(r.Object.GetLabels())

	builder := &resources.ServiceMonitorBuilder{
		Object: obj,

		TargetPort:      api.StatusPort,
		MetricsServices: r.metricsServices,
		Options:         &api.MonitoringOptions{},

		Labels:         monitorLabels,
		SelectorLabels: statusServiceLabels,
	}

	monitor := builder.Placeholder(obj)

	result, err := resources.CreateOrUpdateIgnoreStatus(ctx, r.Client, monitor, func() error {
		if err := builder.Build(monitor); err != nil {
			r.Recorder.Eventf(
				r.Object,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				"Unable to build resource: %s", err,
			)
			return err
		}
		if err = ctrl.SetControllerReference(r.Object, monitor, r.Scheme); err != nil {
			r.Recorder.Eventf(
				r.Object,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				"Unable to set controller reference for resource: %s", err,
			)
			return err
		}

		return nil
	})

	if err != nil {
		logger.Error(err, "unexpected Sync error")
	} else if result != controllerutil.OperationResultNone {
		r.Recorder.Eventf(
			r.Object,
			corev1.EventTypeNormal,
			"Provisioning",
			"Update ServiceMonitor %s/%s for %s/%s status = %s",
			monitor.GetNamespace(), monitor.GetName(),
			obj.GetName(), obj.GetName(),
			result,
		)
	}

	return ctrl.Result{}, err
}

func (r *Syncer) findService(ctx context.Context, obj client.Object) (*corev1.Service, error) {
	var serviceList corev1.ServiceList

	opts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{labels.ServiceComponent: "status"},
	}

	// we are searching for the database/storage status service and will check ownerReference
	if err := r.List(ctx, &serviceList, opts...); err != nil {
		r.Recorder.Eventf(obj, corev1.EventTypeWarning, "Syncing", "Unable to get service list: %s", err.Error())
		return nil, err
	}

	for _, svc := range serviceList.Items {
		for _, owner := range svc.GetOwnerReferences() {
			if owner.UID == obj.GetUID() {
				return &svc, nil
			}
		}
	}
	return nil, nil
}
