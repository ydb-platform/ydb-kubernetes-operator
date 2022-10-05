package resources

import (
	"context"
	"fmt"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	grpcServiceNameFormat         = "%s-grpc"
	interconnectServiceNameFormat = "%s-interconnect"
	statusServiceNameFormat       = "%s-status"
	datastreamsServiceNameFormat  = "%s-datastreams"

	grpcTLSVolumeName         = "grpc-tls-volume"
	interconnectTLSVolumeName = "interconnect-tls-volume"
	datastreamsTLSVolumeName  = "datastreams-tls-volume"

	initMainSharedCertsVolumeName = "init-main-shared-certs-volume"
	caBundleVolumeName = "ca-bundle-volume"

	caBundleConfigMap = "init-container-cert-auths"

	defaultPathForLocalCerts = "/usr/local/share/ca-certificates"
	systemSslStorePath = "/etc/ssl/certs"

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

var annotator = patch.NewAnnotator(lastAppliedAnnotation)
var patchMaker = patch.NewPatchMaker(annotator)

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

func CreateOrUpdateIgnoreStatus(ctx context.Context, c client.Client, obj client.Object, f ctrlutil.MutateFn) (ctrlutil.OperationResult, error) {
	key := client.ObjectKeyFromObject(obj)
	if err := c.Get(ctx, key, obj); err != nil {
		if !errors.IsNotFound(err) {
			return ctrlutil.OperationResultNone, err
		}
		if err := mutate(f, key, obj); err != nil {
			return ctrlutil.OperationResultNone, err
		}
		if err := annotator.SetLastAppliedAnnotation(obj); err != nil {
			return ctrlutil.OperationResultNone, err
		}
		if err := c.Create(ctx, obj); err != nil {
			return ctrlutil.OperationResultNone, err
		}
		return ctrlutil.OperationResultCreated, nil
	}

	existing := obj.DeepCopyObject()
	if err := mutate(f, key, obj); err != nil {
		return ctrlutil.OperationResultNone, err
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
	switch updated.(type) {
	case *appsv1.StatefulSet:
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
