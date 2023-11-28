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

// +kubebuilder:webhook:path=/validate-ydb-tech-v1alpha1-storagenodeset,mutating=false,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=storagenodesets,verbs=create,versions=v1alpha1,name=validate-storagenodeset.ydb.tech,admissionReviewVersions=v1
func RegisterStorageNodeSetValidatingWebhook(mgr ctrl.Manager) error {
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
			Handler: &storageNodeSetValidationHandler{
				logger: logf.Log.WithName(logName),
				mgr:    mgr,
			},
		})
		return nil
	}

	return registerWebHook(&StorageNodeSet{}, "storagenodeset-resource")
}

type storageNodeSetValidationHandler struct {
	logger  logr.Logger
	client  client.Client
	mgr     ctrl.Manager
	decoder *admission.Decoder
}

var (
	_ inject.Client     = &storageNodeSetValidationHandler{}
	_ admission.Handler = &storageNodeSetValidationHandler{}
)

func (v *storageNodeSetValidationHandler) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

func (v *storageNodeSetValidationHandler) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

func (v *storageNodeSetValidationHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	storageNodeSet, ok := req.Object.Object.(*StorageNodeSet)
	if !ok {
		return webhook.Denied("failed to cast to StorageNodeSet object")
	}
	storage, err := getStorageRef(ctx, v.client, req.Namespace, storageNodeSet.Spec.StorageRef)
	if err != nil {
		return webhook.Denied(err.Error())
	}

	var foundInlineSpec *NodeSetSpecInline
	for _, specInline := range storage.Spec.NodeSet {
		if storageNodeSet.GetName() == specInline.Name {
			foundInlineSpec = &specInline
			break
		}
	}
	if foundInlineSpec == nil {
		// does not found
		return webhook.Denied(fmt.Sprintf("does not found nodeSet inline spec in storageRef object: %s", storage.Name))
	}
	if !reflect.DeepEqual(storageNodeSet.Spec.NodeSetSpec, foundInlineSpec.NodeSetSpec) {
		// does not match
		return webhook.Denied(fmt.Sprintf("does not match nodeSet inline spec in storageRef with StorageNodeSet object: %s", storage.Name))
	}

	return admission.Allowed("")
}
