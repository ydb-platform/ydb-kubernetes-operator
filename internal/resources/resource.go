package resources

import (
	"fmt"
	"context"

	appsv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"github.com/banzaicloud/k8s-objectmatcher/patch"
)

const (
	grpcServiceNameFormat         = "%s-grpc"
	interconnectServiceNameFormat = "%s-interconnect"
	statusServiceNameFormat       = "%s-status"

	lastAppliedAnnotation = "ydb.tech/last-applied"
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
