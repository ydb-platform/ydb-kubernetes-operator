package resources

import (
	"context"
	"errors"
	"fmt"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	ydbCredentials "github.com/ydb-platform/ydb-go-sdk/v3/credentials"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	api "github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/connection"
)

const (
	grpcServiceNameFormat         = "%s-grpc"
	interconnectServiceNameFormat = "%s-interconnect"
	statusServiceNameFormat       = "%s-status"
	datastreamsServiceNameFormat  = "%s-datastreams"

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

	lastAppliedAnnotation                     = "ydb.tech/last-applied"
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
	annotator  = patch.NewAnnotator(lastAppliedAnnotation)
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
		secure := connection.LoadTLSCredentials(storage.IsStorageEndpointSecure())
		return ydbCredentials.NewStaticCredentials(username, password, endpoint, secure), nil
	}

	return nil, errors.New("unsupported auth type for GetYDBCredentials")
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
