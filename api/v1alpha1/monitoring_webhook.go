package v1alpha1

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	v1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
)

// generateValidatePath is a copy from controller-runtime
func generateValidatePath(gvk schema.GroupVersionKind) string {
	return "/validate-" + strings.ReplaceAll(gvk.Group, ".", "-") + "-" +
		gvk.Version + "-" + strings.ToLower(gvk.Kind)
}

//+kubebuilder:webhook:path=/validate-ydb-tech-v1alpha1-databasemonitoring,mutating=false,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=databasemonitorings,verbs=create,versions=v1alpha1,name=vdatabasemonitoring.kb.io,admissionReviewVersions=v1
//+kubebuilder:webhook:path=/validate-ydb-tech-v1alpha1-storagemonitoring,mutating=false,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=storagemonitorings,verbs=create,versions=v1alpha1,name=vstoragemonitoring.kb.io,admissionReviewVersions=v1

func RegisterMonitoringValidatingWebhook(mgr ctrl.Manager, enbSvcMon bool) error {
	// We are using low-level api here because we need pass client to handler

	srv := mgr.GetWebhookServer()

	var registerWebHook = func(typ runtime.Object, logName string) error {
		gvk, err := apiutil.GVKForObject(typ, mgr.GetScheme())

		if err != nil {
			return err
		}

		path := generateValidatePath(gvk)
		logf.Log.WithName("monitoring-webhooks").Info("Registering a validating webhook", "GVK", gvk, "path", path)
		srv.Register(path, &webhook.Admission{
			Handler: &monitoringValidationHandler{
				logger:    logf.Log.WithName(logName),
				enbSvcMon: enbSvcMon,
				mgr:       mgr,
			},
		})
		return nil
	}

	if err := registerWebHook(&DatabaseMonitoring{}, "databasemonitoring-resource"); err != nil {
		return err
	}

	if err := registerWebHook(&StorageMonitoring{}, "storagemonitoring-resource"); err != nil {
		return err
	}

	return nil
}

func ensureNoServiceMonitor(ctx context.Context, client client.Client, namespace string, name string) error {
	found := &v1.ServiceMonitor{}
	err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, found)
	fmt.Printf("err = %v, namespace=%s, name=%s\n", err, namespace, name)
	if err == nil {
		return fmt.Errorf("service monitor with name %s already exists", name)
	}

	if errors.IsNotFound(err) {
		return nil
	}

	return err
}

type monitoringValidationHandler struct {
	enbSvcMon bool
	logger    logr.Logger
	client    client.Client
	mgr       ctrl.Manager
	decoder   *admission.Decoder
}

var _ inject.Client = &monitoringValidationHandler{}
var _ admission.Handler = &monitoringValidationHandler{}

func (v *monitoringValidationHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if !v.enbSvcMon {
		return webhook.Denied("the ydb-operator is running without a service monitoring feature.")
	}

	err := checkMonitoringCRD(v.mgr, v.logger, false)
	if err != nil {
		return webhook.Denied(err.Error())
	}

	// Name can't be an empty string here because it's required in the schema
	err = ensureNoServiceMonitor(ctx, v.client, req.Namespace, req.Name)
	if err != nil {
		return webhook.Denied(err.Error())
	}

	return admission.Allowed("")
}

func (v *monitoringValidationHandler) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

func (v *monitoringValidationHandler) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
