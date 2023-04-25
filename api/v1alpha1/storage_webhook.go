package v1alpha1

import (
	"fmt"

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/strings/slices"
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

	if r.Spec.Monitoring == nil {
		r.Spec.Monitoring = &MonitoringOptions{
			Enabled: false,
		}
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

	configuration := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(r.Spec.Configuration), &configuration)
	if err != nil {
		return fmt.Errorf("failed to parse Storage.spec.configuration, error: %w", err)
	}
	var nodesNumber int32
	if configuration["hosts"] == nil {
		nodesNumber = r.Spec.Nodes
	} else {
		hosts, ok := configuration["hosts"].([]interface{})
		if !ok {
			return fmt.Errorf("failed to parse Storage.spec.configuration, error: invalid hosts section")
		}
		nodesNumber = int32(len(hosts))
	}

	minNodesPerErasure := map[ErasureType]int32{
		ErasureMirror3DC: 9,
		ErasureBlock42:   8,
		None:             1,
	}
	if nodesNumber < minNodesPerErasure[r.Spec.Erasure] {
		return fmt.Errorf("erasure type %v requires at least %v storage nodes", r.Spec.Erasure, minNodesPerErasure[r.Spec.Erasure])
	}

	reservedSecretNames := []string{
		"database_encryption",
		"datastreams",
	}

	for _, secret := range r.Spec.Secrets {
		if slices.Contains(reservedSecretNames, secret.Name) {
			return fmt.Errorf("the secret name %s is reserved, use another one", secret.Name)
		}
	}

	if r.Spec.Volumes != nil {
		for _, volume := range r.Spec.Volumes {
			if volume.HostPath == nil {
				return fmt.Errorf("unsupported volume source, %v. Only hostPath is supported ", volume.VolumeSource)
			}
		}
	}

	crdCheckError := checkMonitoringCRD(manager, storagelog, r.Spec.Monitoring != nil)
	if crdCheckError != nil {
		return crdCheckError
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Storage) ValidateUpdate(old runtime.Object) error {
	storagelog.Info("validate update", "name", r.Name)

	crdCheckError := checkMonitoringCRD(manager, storagelog, r.Spec.Monitoring != nil)
	if crdCheckError != nil {
		return crdCheckError
	}

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

func (r *Storage) ValidateDelete() error {
	return nil
}
