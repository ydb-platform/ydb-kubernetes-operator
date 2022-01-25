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
var databaselog = logf.Log.WithName("database-resource")

func (r *Database) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-ydb-tech-v1alpha1-database,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=databases,verbs=create;update,versions=v1alpha1,name=mutate-database.ydb.tech,admissionReviewVersions=v1

var _ webhook.Defaulter = &Database{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Database) Default() {
	databaselog.Info("default", "name", r.Name)

	if r.Spec.StorageClusterRef.Namespace == "" {
		r.Spec.StorageClusterRef.Namespace = r.Namespace
	}

	if r.Spec.ServerlessResources != nil {
		if r.Spec.ServerlessResources.SharedDatabaseRef.Namespace == "" {
			r.Spec.ServerlessResources.SharedDatabaseRef.Namespace = r.Namespace
		}
	}

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

//+kubebuilder:webhook:path=/validate-ydb-tech-v1alpha1-database,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=databases,verbs=create;update,versions=v1alpha1,name=validate-database.ydb.tech,admissionReviewVersions=v1

var _ webhook.Validator = &Database{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Database) ValidateCreate() error {
	databaselog.Info("validate create", "name", r.Name)

	if r.Spec.Resources == nil && r.Spec.SharedResources == nil && r.Spec.ServerlessResources == nil {
		return errors.New("incorrect database resources configuration, must be one of: Resources, SharedResources, ServerlessResources")
	}

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Database) ValidateUpdate(old runtime.Object) error {
	databaselog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

func (r *Database) ValidateDelete() error {
	return nil
}
