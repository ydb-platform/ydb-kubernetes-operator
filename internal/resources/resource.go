package resources

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
)

const (
	GRPCServiceNameFormat         = "%s-grpc"
	InterconnectServiceNameFormat = "%s-interconnect"
	StatusServiceNameFormat       = "%s-status"
	DatastreamsServiceNameFormat  = "%s-datastreams"

	grpcTLSVolumeName         = "grpc-tls-volume"
	interconnectTLSVolumeName = "interconnect-tls-volume"
	datastreamsTLSVolumeName  = "datastreams-tls-volume"

	systemCertsVolumeName = "init-main-shared-certs-volume"
	localCertsVolumeName  = "init-main-shared-source-dir-volume"

	wellKnownDirForAdditionalSecrets = "/opt/ydb/secrets"
	wellKnownDirForAdditionalVolumes = "/opt/ydb/volumes"

	caBundleEnvName  = "CA_BUNDLE"
	caBundleFileName = "userCABundle.crt"

	localCertsDir  = "/usr/local/share/ca-certificates"
	systemCertsDir = "/etc/ssl/certs"

	encryptionVolumeName                      = "encryption"
	datastreamsIAMServiceAccountKeyVolumeName = "datastreams-iam-sa-key"
	defaultEncryptionSecretKey                = "key"
	defaultPin                                = "EmptyPin"

	updateCACertificatesBin = "update-ca-certificates"
)

type ResourceBuilder interface {
	Placeholder(cr client.Object) client.Object
	Build(client.Object) error
}

var (
	annotator  = patch.NewAnnotator(ydbannotations.LastAppliedAnnotation)
	patchMaker = patch.NewPatchMaker(annotator)
)

var CreateOrUpdate = ctrl.CreateOrUpdate

func mutate(f ctrlutil.MutateFn, key client.ObjectKey, obj client.Object) error {
	if err := f(); err != nil {
		return err
	}
	newKey := client.ObjectKeyFromObject(obj)
	if key.String() != newKey.String() {
		return fmt.Errorf("MutateFn cannot mutate object name and/or object namespace")
	}
	return nil
}

type IgnoreChangesFunction func(oldObj, newObj runtime.Object) bool

func tryProcessNonExistingObject(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	f ctrlutil.MutateFn,
	shouldIgnoreChange IgnoreChangesFunction,
) (bool, ctrlutil.OperationResult, error) {
	key := client.ObjectKeyFromObject(obj)

	err := c.Get(ctx, key, obj)

	if err == nil {
		return true, ctrlutil.OperationResultNone, nil
	}

	if !errors.IsNotFound(err) {
		return false, ctrlutil.OperationResultNone, err
	}

	// Object did not exist, but the creation of new
	// object is not needed (e.g. paused Storage)
	if shouldIgnoreChange(nil, obj) {
		return false, ctrlutil.OperationResultNone, nil
	}

	if err := mutate(f, key, obj); err != nil {
		return false, ctrlutil.OperationResultNone, err
	}

	if err := annotator.SetLastAppliedAnnotation(obj); err != nil {
		return false, ctrlutil.OperationResultNone, err
	}

	if err := c.Create(ctx, obj); err != nil {
		return false, ctrlutil.OperationResultNone, err
	}

	return false, ctrlutil.OperationResultCreated, nil
}

func CreateOrUpdateOrMaybeIgnore(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	f ctrlutil.MutateFn,
	shouldIgnoreChange IgnoreChangesFunction,
) (ctrlutil.OperationResult, error) {
	objectExisted, opResult, err := tryProcessNonExistingObject(ctx, c, obj, f, shouldIgnoreChange)

	if !objectExisted {
		return opResult, err
	}

	key := client.ObjectKeyFromObject(obj)
	existing := obj.DeepCopyObject()
	if err := mutate(f, key, obj); err != nil {
		return ctrlutil.OperationResultNone, err
	}

	if shouldIgnoreChange(existing, obj) {
		return ctrlutil.OperationResultNone, nil
	}

	changed, err := CheckObjectUpdatedIgnoreStatus(existing, obj)
	if err != nil || !changed {
		return ctrlutil.OperationResultNone, err
	}
	if err := annotator.SetLastAppliedAnnotation(obj); err != nil {
		return ctrlutil.OperationResultNone, err
	}
	if err := c.Update(ctx, obj); err != nil {
		return ctrlutil.OperationResultNone, err
	}
	return ctrlutil.OperationResultUpdated, nil
}

func CheckObjectUpdatedIgnoreStatus(current, updated runtime.Object) (bool, error) {
	opts := []patch.CalculateOption{
		patch.IgnoreStatusFields(),
	}
	if _, ok := updated.(*appsv1.StatefulSet); ok {
		opts = append(opts, patch.IgnoreVolumeClaimTemplateTypeMetaAndStatus())
	}
	patchResult, err := patchMaker.Calculate(current, updated, opts...)
	if err != nil {
		return false, err
	}
	return !patchResult.IsEmpty(), nil
}

func CopyDict(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func CreateResource(obj client.Object) client.Object {
	createdObj := obj.DeepCopyObject().(client.Object)

	// Remove or reset fields
	createdObj.SetResourceVersion("")
	createdObj.SetCreationTimestamp(metav1.Time{})
	createdObj.SetUID("")
	createdObj.SetOwnerReferences([]metav1.OwnerReference{})
	createdObj.SetFinalizers([]string{})

	if svc, ok := createdObj.(*corev1.Service); ok {
		svc.Spec.ClusterIP = ""
		svc.Spec.ClusterIPs = nil
	}

	setRemoteResourceVersionAnnotation(createdObj, obj.GetResourceVersion())

	return createdObj
}

func UpdateResource(oldObj, newObj client.Object) client.Object {
	updatedObj := newObj.DeepCopyObject().(client.Object)

	// Save current fields
	updatedObj.SetResourceVersion(oldObj.GetResourceVersion())
	updatedObj.SetCreationTimestamp(oldObj.GetCreationTimestamp())
	updatedObj.SetUID(oldObj.GetUID())
	updatedObj.SetOwnerReferences(oldObj.GetOwnerReferences())
	updatedObj.SetFinalizers(oldObj.GetFinalizers())

	if svc, ok := updatedObj.(*corev1.Service); ok {
		svc.Spec.ClusterIP = oldObj.(*corev1.Service).Spec.ClusterIP
		svc.Spec.ClusterIPs = append([]string{}, oldObj.(*corev1.Service).Spec.ClusterIPs...)
	}

	setRemoteResourceVersionAnnotation(updatedObj, newObj.GetResourceVersion())

	return updatedObj
}

func setRemoteResourceVersionAnnotation(obj client.Object, resourceVersion string) {
	annotations := make(map[string]string)
	for key, value := range obj.GetAnnotations() {
		annotations[key] = value
	}
	annotations[ydbannotations.RemoteResourceVersionAnnotation] = resourceVersion
	obj.SetAnnotations(annotations)
}

func ConvertRemoteResourceToObject(remoteResource api.RemoteResource, namespace string) (client.Object, error) {
	// Create an unstructured object
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": remoteResource.Version,
			"group":      remoteResource.Group,
			"kind":       remoteResource.Kind,
			"metadata": map[string]interface{}{
				"name":      remoteResource.Name,
				"namespace": namespace,
			},
		},
	}

	// Convert unstructured object to runtime.Object
	var runtimeObj runtime.Object
	runtimeObj, err := scheme.Scheme.New(
		schema.GroupVersionKind{
			Group:   remoteResource.Group,
			Version: remoteResource.Version,
			Kind:    remoteResource.Kind,
		},
	)
	if err != nil {
		return nil, err
	}

	// Copy data from unstructured to runtime object
	err = scheme.Scheme.Convert(obj, runtimeObj, nil)
	if err != nil {
		return nil, err
	}

	// Assert runtime.Object to client.Object
	return runtimeObj.(client.Object), nil
}

func CompareRemoteResourceWithObject(
	remoteResource *api.RemoteResource,
	namespace string,
	remoteObj client.Object,
	remoteObjGVK schema.GroupVersionKind,
) bool {
	if remoteObj.GetName() == remoteResource.Name &&
		remoteObj.GetNamespace() == namespace &&
		remoteObjGVK.Kind == remoteResource.Kind &&
		remoteObjGVK.Group == remoteResource.Group &&
		remoteObjGVK.Version == remoteResource.Version {
		return true
	}
	return false
}

func LastAppliedAnnotationPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !ydbannotations.CompareYdbTechAnnotations(
				e.ObjectOld.GetAnnotations(),
				e.ObjectNew.GetAnnotations(),
			)
		},
	}
}

func IgnoreDeletetionPredicate() predicate.Predicate {
	return predicate.Funcs{
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Evaluates to false if the object has been confirmed deleted.
			return !e.DeleteStateUnknown
		},
	}
}

func ResourcesPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			_, isService := e.ObjectOld.(*corev1.Service)
			_, isSecret := e.ObjectOld.(*corev1.Secret)
			return isSecret || isService
		},
	}
}
