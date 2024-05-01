package resources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	ydbannotations "github.com/ydb-platform/ydb-kubernetes-operator/internal/annotations"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
)

const (
	GRPCServiceNameFormat         = "%s-grpc"
	InterconnectServiceNameFormat = "%s-interconnect"
	StatusServiceNameFormat       = "%s-status"
	DatastreamsServiceNameFormat  = "%s-datastreams"

	grpcTLSVolumeName         = "grpc-tls-volume"
	interconnectTLSVolumeName = "interconnect-tls-volume"
	datastreamsTLSVolumeName  = "datastreams-tls-volume"

	grpcTLSVolumeMountPath         = "/tls/grpc"
	interconnectTLSVolumeMountPath = "/tls/interconnect"
	datastreamsTLSVolumeMountPath  = "/tls/datastreams"

	InitJobNameFormat             = "%s-blobstorage-init"
	OperatorTokenSecretNameFormat = "%s-operator-token"

	systemCertsVolumeName   = "init-main-shared-certs-volume"
	localCertsVolumeName    = "init-main-shared-source-dir-volume"
	operatorTokenVolumeName = "operator-token-volume"

	wellKnownDirForAdditionalSecrets = "/opt/ydb/secrets"
	wellKnownDirForAdditionalVolumes = "/opt/ydb/volumes"
	wellKnownNameForOperatorToken    = "token-file"

	caBundleEnvName         = "CA_BUNDLE"
	caBundleFileName        = "userCABundle.crt"
	updateCACertificatesBin = "update-ca-certificates"

	localCertsDir  = "/usr/local/share/ca-certificates"
	systemCertsDir = "/etc/ssl/certs"

	encryptionVolumeName                      = "encryption"
	datastreamsIAMServiceAccountKeyVolumeName = "datastreams-iam-sa-key"
	defaultEncryptionSecretKey                = "key"
	defaultPin                                = "EmptyPin"
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

func DoNotIgnoreChanges() IgnoreChangesFunction {
	return func(oldObj, newObj runtime.Object) bool {
		return false
	}
}

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

	if !apierrors.IsNotFound(err) {
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
	createdObj.SetManagedFields([]metav1.ManagedFieldsEntry{})

	if svc, ok := createdObj.(*corev1.Service); ok {
		svc.Spec.ClusterIP = ""
		svc.Spec.ClusterIPs = nil
	}

	// Set remoteResourceVersion annotation
	SetRemoteResourceVersionAnnotation(createdObj, obj.GetResourceVersion())

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
	updatedObj.SetManagedFields(oldObj.GetManagedFields())

	// Specific fields to save for Service object
	if svc, ok := updatedObj.(*corev1.Service); ok {
		svc.Spec.ClusterIP = oldObj.(*corev1.Service).Spec.ClusterIP
		svc.Spec.ClusterIPs = append([]string{}, oldObj.(*corev1.Service).Spec.ClusterIPs...)
	}

	// Copy primaryResource annotations
	CopyPrimaryResourceObjectAnnotation(updatedObj, oldObj.GetAnnotations())

	// Set remoteResourceVersion annotation
	SetRemoteResourceVersionAnnotation(updatedObj, newObj.GetResourceVersion())

	return updatedObj
}

func CopyPrimaryResourceObjectAnnotation(obj client.Object, oldAnnotations map[string]string) {
	annotations := CopyDict(obj.GetAnnotations())
	for key, value := range oldAnnotations {
		if key == ydbannotations.PrimaryResourceDatabaseAnnotation ||
			key == ydbannotations.PrimaryResourceStorageAnnotation {
			annotations[key] = value
		}
	}
	obj.SetAnnotations(annotations)
}

func SetRemoteResourceVersionAnnotation(obj client.Object, resourceVersion string) {
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

func GetPatchResult(
	localObj client.Object,
	remoteObj client.Object,
) (*patch.PatchResult, error) {
	// Get diff resources and compare bytes by k8s-objectmatcher PatchMaker
	updatedObj := UpdateResource(localObj, remoteObj)
	patchResult, err := patchMaker.Calculate(localObj, updatedObj,
		[]patch.CalculateOption{
			patch.IgnoreStatusFields(),
		}...,
	)
	if err != nil {
		return nil, err
	}

	return patchResult, nil
}

func EqualRemoteResourceWithObject(
	remoteResource *api.RemoteResource,
	remoteObj client.Object,
) bool {
	if remoteObj.GetName() == remoteResource.Name &&
		remoteObj.GetObjectKind().GroupVersionKind().Kind == remoteResource.Kind &&
		remoteObj.GetObjectKind().GroupVersionKind().Group == remoteResource.Group &&
		remoteObj.GetObjectKind().GroupVersionKind().Version == remoteResource.Version {
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

func IsServicePredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			_, isService := e.ObjectOld.(*corev1.Service)
			return isService
		},
	}
}

func IsSecretPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			_, isSecret := e.ObjectOld.(*corev1.Secret)
			return isSecret
		},
	}
}

func LabelExistsPredicate(selector labels.Selector) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(o client.Object) bool {
		return selector.Matches(labels.Set(o.GetLabels()))
	})
}

func getYDBStaticCredentials(
	ctx context.Context,
	storage *api.Storage,
	restConfig *rest.Config,
) (ydbCredentials.Credentials, error) {
	auth := storage.Spec.OperatorConnection
	username := auth.StaticCredentials.Username
	password := api.DefaultRootPassword
	if auth.StaticCredentials.Password != nil {
		var err error
		password, err = GetSecretKey(
			ctx,
			storage.Namespace,
			restConfig,
			auth.StaticCredentials.Password.SecretKeyRef,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get password for StaticCredentials from secret: %s, key: %s, error: %w",
				auth.StaticCredentials.Password.SecretKeyRef.Name,
				auth.StaticCredentials.Password.SecretKeyRef.Key,
				err)
		}
	}
	endpoint := storage.GetStorageEndpoint()

	var caBundle []byte
	if storage.IsStorageEndpointSecure() {
		var err error
		caBundle, err = getStorageGrpcServiceCABundle(ctx, storage, restConfig)
		if err != nil {
			return nil, err
		}
	}
	dialOptions, err := connection.LoadTLSCredentials(storage.IsStorageEndpointSecure(), caBundle)
	if err != nil {
		return nil, err
	}
	return ydbCredentials.NewStaticCredentials(username, password, endpoint, dialOptions), nil
}

func GetYDBCredentials(
	ctx context.Context,
	storage *api.Storage,
	restConfig *rest.Config,
) (ydbCredentials.Credentials, error) {
	auth := storage.Spec.OperatorConnection
	if auth == nil {
		return ydbCredentials.NewAnonymousCredentials(), nil
	}

	if auth.AccessToken != nil {
		token, err := GetSecretKey(
			ctx,
			storage.Namespace,
			restConfig,
			auth.AccessToken.SecretKeyRef,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get token for AccessToken from secret: %s, key: %s, error: %w",
				auth.AccessToken.SecretKeyRef.Name,
				auth.AccessToken.SecretKeyRef.Key,
				err)
		}

		return ydbCredentials.NewAccessTokenCredentials(token), nil
	}

	if auth.StaticCredentials != nil {
		return getYDBStaticCredentials(ctx, storage, restConfig)
	}

	return nil, errors.New("unsupported auth type for GetYDBCredentials")
}

func getStorageGrpcServiceCABundle(
	ctx context.Context,
	storage *api.Storage,
	restConfig *rest.Config,
) ([]byte, error) {
	if !storage.IsStorageEndpointSecure() {
		return nil, errors.New("can't get storage grpc CA for insecure endpoint")
	}

	tlsConfig := storage.Spec.Service.GRPC.TLSConfiguration
	caBody, err := GetSecretKey(
		ctx,
		storage.Namespace,
		restConfig,
		&tlsConfig.CertificateAuthority,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get CA for storage grpc service from secret: %s, key: %s, error: %w",
			tlsConfig.CertificateAuthority.Name,
			tlsConfig.CertificateAuthority.Key,
			err)
	}
	return []byte(caBody), nil
}

func GetYDBTLSOption(
	ctx context.Context,
	storage *api.Storage,
	restConfig *rest.Config,
) (ydb.Option, error) {
	if !storage.IsStorageEndpointSecure() {
		return ydb.WithInsecure(), nil
	}
	caBundle, err := getStorageGrpcServiceCABundle(ctx, storage, restConfig)
	if err != nil {
		return nil, err
	}
	return ydb.WithCertificatesFromPem(caBundle), nil
}

func buildCAStorePatchingCommandArgs(
	caBundle string,
	grpcService api.GRPCService,
	interconnectService api.InterconnectService,
) ([]string, []string) {
	command := []string{"/bin/bash", "-c"}

	arg := ""

	if len(caBundle) > 0 {
		arg += fmt.Sprintf("printf $%s | base64 --decode > %s/%s && ", caBundleEnvName, localCertsDir, caBundleFileName)
	}

	if grpcService.TLSConfiguration.Enabled {
		arg += fmt.Sprintf("cp %s/ca.crt %s/grpcRoot.crt && ", grpcTLSVolumeMountPath, localCertsDir)
	}

	if interconnectService.TLSConfiguration.Enabled {
		arg += fmt.Sprintf("cp %s/ca.crt %s/interconnectRoot.crt && ", interconnectTLSVolumeMountPath, localCertsDir)
	}

	if arg != "" {
		arg += updateCACertificatesBin
	}

	args := []string{arg}

	return command, args
}

func GetConfigurationChecksum(configuration string) string {
	hasher := sha256.New()
	hasher.Write([]byte(configuration))
	return hex.EncodeToString(hasher.Sum(nil))
}
