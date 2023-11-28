package v1alpha1

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-ydb-tech-v1alpha1-databasenodeset,mutating=false,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=databasenodesets,verbs=create,versions=v1alpha1,name=validate-databasenodeset.ydb.tech,admissionReviewVersions=v1
func RegisterDatabaseNodeSetValidatingWebhook(mgr ctrl.Manager) error {
	// We are using low-level api here because we need pass client to handler
	srv := mgr.GetWebhookServer()

	registerWebHook := func(typ runtime.Object, logName string) error {
		gvk, err := apiutil.GVKForObject(typ, mgr.GetScheme())
		if err != nil {
			return err
		}

		path := generateValidatePath(gvk)
		logf.Log.WithName("nodeset-webhooks").Info("Registering a validating webhook", "GVK", gvk, "path", path)
		srv.Register(path, &webhook.Admission{
			Handler: &databaseNodeSetValidationHandler{
				logger: logf.Log.WithName(logName),
				mgr:    mgr,
			},
		})
		return nil
	}

	return registerWebHook(&DatabaseNodeSet{}, "databasenodeset-resource")
}

type databaseNodeSetValidationHandler struct {
	logger  logr.Logger
	client  client.Client
	mgr     ctrl.Manager
	decoder *admission.Decoder
}

var (
	_ inject.Client     = &databaseNodeSetValidationHandler{}
	_ admission.Handler = &databaseNodeSetValidationHandler{}
)

func (v *databaseNodeSetValidationHandler) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

func (v *databaseNodeSetValidationHandler) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

func (v *databaseNodeSetValidationHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	databaseNodeSet, ok := req.Object.Object.(*DatabaseNodeSet)
	if !ok {
		return webhook.Denied("failed to cast to databaseNodeSet object")
	}
	database, err := getDatabaseRef(ctx, v.client, req.Namespace, databaseNodeSet.Spec.DatabaseRef)
	if err != nil {
		return webhook.Denied(err.Error())
	}

	var foundInlineSpec *NodeSetSpecInline
	for _, specInline := range database.Spec.NodeSet {
		if databaseNodeSet.GetName() == specInline.Name {
			foundInlineSpec = &specInline
			break
		}
	}
	if foundInlineSpec == nil {
		// does not found
		return webhook.Denied(fmt.Sprintf("does not found nodeSet inline spec in databaseRef object: %s", database.Name))
	}
	if !reflect.DeepEqual(databaseNodeSet.Spec.NodeSetSpec, foundInlineSpec.NodeSetSpec) {
		// does not match
		return webhook.Denied(fmt.Sprintf("does not match nodeSet inline spec in databaseRef with databaseNodeSet object: %s", database.Name))
	}

	return admission.Allowed("")
}
