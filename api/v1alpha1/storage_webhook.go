package v1alpha1

import (
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var storagelog = logf.Log.WithName("storage-resource")

func (r *Storage) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-ydb-tech-v1alpha1-storage,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=storages,verbs=create;update,versions=v1alpha1,name=mutate-storage.ydb.tech,admissionReviewVersions=v1

var _ webhook.Defaulter = &Storage{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Storage) Default() {
	storagelog.Info("default", "name", r.Name)

	if r.Spec.Image.Name == "" {
		if r.Spec.YDBVersion == "" {
			r.Spec.Image.Name = fmt.Sprintf(ImagePathFormat, RegistryPath, DefaultTag)
		} else {
			r.Spec.Image.Name = fmt.Sprintf(ImagePathFormat, RegistryPath, r.Spec.YDBVersion)
		}
	}

	if r.Spec.Image.PullPolicyName == nil {
		policy := v1.PullIfNotPresent
		r.Spec.Image.PullPolicyName = &policy
	}

	if r.Spec.Service.GRPC.TLSConfiguration == nil {
		r.Spec.Service.GRPC.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}
	if r.Spec.Service.Interconnect.TLSConfiguration == nil {
		r.Spec.Service.Interconnect.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}

	if r.Spec.Domain == "" {
		r.Spec.Domain = "root" // FIXME
	}
}

//+kubebuilder:webhook:path=/validate-ydb-tech-v1alpha1-storage,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=storages,verbs=create;update,versions=v1alpha1,name=validate-storage.ydb.tech,admissionReviewVersions=v1

var _ webhook.Validator = &Storage{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Storage) ValidateCreate() error {
	storagelog.Info("validate create", "name", r.Name)

	minNodesPerErasure := map[ErasureType]int32{
		ErasureMirror3DC: 9,
		ErasureBlock42:   8,
		None:             1,
	}

	if r.Spec.Nodes < minNodesPerErasure[r.Spec.Erasure] {
		return errors.New(fmt.Sprintf("erasure type %v requires at least %v storage nodes", r.Spec.Erasure, minNodesPerErasure[r.Spec.Erasure]))
	}

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Storage) ValidateUpdate(old runtime.Object) error {
	storagelog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

func (r *Storage) ValidateDelete() error {
	return nil
}
