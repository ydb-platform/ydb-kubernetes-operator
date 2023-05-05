package monitoring

import (
	"context"
	dbcontroller "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/database"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/labels"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/metrics"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/resources"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"

	ydbv1alpha1 "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
)

const (
	DefaultRequeueDelay = 25 * time.Second
)

func refDatabaseNamespacedName(cr *ydbv1alpha1.DatabaseMonitoring) types.NamespacedName {
	dbNamespace := cr.Spec.DatabaseClusterRef.Namespace

	if dbNamespace == "" {
		dbNamespace = cr.GetNamespace()
	}

	return types.NamespacedName{
		Name:      cr.Spec.DatabaseClusterRef.Name,
		Namespace: dbNamespace,
	}
}

func (r *DatabaseMonitoringReconciler) Sync(ctx context.Context, cr *ydbv1alpha1.DatabaseMonitoring) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	db, err := r.waitForDatabase(ctx, cr)

	if db == nil && err == nil {
		return ctrl.Result{RequeueAfter: DefaultRequeueDelay}, err
	}

	svc, err := r.findDatabaseService(ctx, db)

	if svc == nil {
		if err != nil {
			r.Recorder.Eventf(cr, corev1.EventTypeWarning, "Error", "Unable to find service for db %s/%s",
				db.GetNamespace(), db.GetName())

			log.FromContext(ctx).Error(err, "Unable to find svc for db", "cr", cr, "db", db)
		} else {
			r.Recorder.Eventf(cr, corev1.EventTypeWarning, "Error", "No status service found for db %s/%s. Stop.",
				db.GetNamespace(), db.GetName())
		}
		return ctrl.Result{}, err
	}

	statusServiceLabels := labels.Labels{}
	statusServiceLabels.Merge(svc.Labels)

	monitorLabels := labels.Common(db.GetName(), db.Labels)
	monitorLabels.Merge(cr.Labels)

	builder := &resources.ServiceMonitorBuilder{
		Object: cr,

		TargetPort:      ydbv1alpha1.StatusPort,
		MetricsServices: metrics.GetDatabaseMetricsServices(),
		Options:         &ydbv1alpha1.MonitoringOptions{},

		Labels:         monitorLabels,
		SelectorLabels: statusServiceLabels,
	}

	monitor := builder.Placeholder(db)

	result, err := resources.CreateOrUpdateIgnoreStatus(ctx, r.Client, monitor, func() error {
		if err := builder.Build(monitor); err != nil {
			r.Recorder.Eventf(
				db,
				corev1.EventTypeWarning,
				"ProvisioningFailed",
				"Unable to build resource: %s", err,
			)
			return err
		}

		if err = ctrl.SetControllerReference(cr, monitor, r.Scheme); err != nil {
			r.Recorder.Eventf(
				db,
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
			db,
			corev1.EventTypeNormal,
			"Provisioning",
			"Update ServiceMonitor %s/%s for %s/%s status = %s",
			monitor.GetNamespace(), monitor.GetName(),
			db.GetName(), db.GetName(),
			result,
		)
	}

	return ctrl.Result{}, err
}

func (r *DatabaseMonitoringReconciler) waitForDatabase(ctx context.Context, cr *ydbv1alpha1.DatabaseMonitoring) (*ydbv1alpha1.Database, error) {
	found := &ydbv1alpha1.Database{}

	dbNsName := refDatabaseNamespacedName(cr)

	if err := r.Get(ctx, dbNsName, found); err != nil {
		if apierrs.IsNotFound(err) {
			r.Recorder.Eventf(cr, corev1.EventTypeNormal, "Pending",
				"Unknown YDB Database cluster %s",
				dbNsName.String())
			return nil, nil
		}
		r.Recorder.Eventf(cr, corev1.EventTypeWarning, "Error",
			"Unable to find YDB Database %s: %s", dbNsName.String(), err.Error())
		return nil, err
	} else {
		if found.Status.State != string(dbcontroller.Ready) {
			r.Recorder.Eventf(cr, corev1.EventTypeNormal, "Pending",
				"YDB Database %s state %s is not ready",
				dbNsName.String(), found.Status.State)
			return nil, nil

		}
	}

	return found, nil
}

func (r *DatabaseMonitoringReconciler) findDatabaseService(ctx context.Context, database *ydbv1alpha1.Database) (*corev1.Service, error) {
	var serviceList corev1.ServiceList

	opts := []client.ListOption{
		client.InNamespace(database.GetNamespace()),
		client.MatchingLabels{labels.ServiceComponent: "status"},
	}

	// we are searching for the database/storage status service and will check ownerReference
	if err := r.List(ctx, &serviceList, opts...); err != nil {
		r.Recorder.Eventf(database, corev1.EventTypeWarning, "Syncing", "Unable to get service list: %s", err.Error())
		return nil, err
	}

	for _, svc := range serviceList.Items {
		for _, owner := range svc.GetOwnerReferences() {
			if owner.UID == database.GetUID() {
				return &svc, nil
			}
		}
	}
	return nil, nil
}
