package v1alpha1

import (
	"errors"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
)

const (
	DefaultDatabaseDomain = "Root"
)

// log is for logging in this package.
var databaselog = logf.Log.WithName("database-resource")

var manager ctrl.Manager

func (r *Database) SetupWebhookWithManager(mgr ctrl.Manager) error {
	manager = mgr
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-ydb-tech-v1alpha1-database,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=databases,verbs=create;update,versions=v1alpha1,name=mutate-database.ydb.tech,admissionReviewVersions=v1

var _ webhook.Defaulter = &Database{}

func (r *Database) GetDatabasePath() string {
	if r.Spec.Path != "" {
		return r.Spec.Path
	}
	return r.GetLegacyDatabasePath()
}

func (r *Database) GetLegacyDatabasePath() string {
	return fmt.Sprintf(legacyTenantNameFormat, r.Spec.Domain, r.Name) // FIXME: review later in context of multiple namespaces
}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Database) Default() {
	databaselog.Info("default", "name", r.Name)

	if r.Spec.StorageClusterRef.Namespace == "" {
		r.Spec.StorageClusterRef.Namespace = r.Namespace
	}

	if r.Spec.StorageDomains == nil {
		r.Spec.StorageDomains = []string{
			fmt.Sprintf(InterconnectServiceFQDNFormat, r.Spec.StorageClusterRef.Name, r.Spec.StorageClusterRef.Namespace),
		}
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

	if r.Spec.Service.Datastreams.TLSConfiguration == nil {
		r.Spec.Service.Datastreams.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}

	if r.Spec.Domain == "" {
		r.Spec.Domain = DefaultDatabaseDomain
	}

	if r.Spec.Path == "" {
		r.Spec.Path = r.GetLegacyDatabasePath()
	}

	if r.Spec.Encryption == nil {
		r.Spec.Encryption = &EncryptionConfig{Enabled: false}
	}

	if r.Spec.Datastreams == nil {
		r.Spec.Datastreams = &DatastreamsConfig{Enabled: false}
	}

	if r.Spec.Monitoring == nil {
		r.Spec.Monitoring = &MonitoringOptions{
			Enabled: false,
		}
	}
}

//+kubebuilder:webhook:path=/validate-ydb-tech-v1alpha1-database,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=databases,verbs=create;update,versions=v1alpha1,name=validate-database.ydb.tech,admissionReviewVersions=v1

var _ webhook.Validator = &Database{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Database) ValidateCreate() error {
	databaselog.Info("validate create", "name", r.Name)

	if r.Spec.Domain != "" && r.Spec.Path != "" {
		if !strings.HasPrefix(r.Spec.Path, fmt.Sprintf("/%s", r.Spec.Domain)) {
			return fmt.Errorf("incorrect database path, must start with domain: \"/%s\"", r.Spec.Domain)
		}
	}

	if r.Spec.Resources == nil && r.Spec.SharedResources == nil && r.Spec.ServerlessResources == nil {
		return errors.New("incorrect database resources configuration, must be one of: Resources, SharedResources, ServerlessResources")
	}

	if r.Spec.Volumes != nil {
		for _, volume := range r.Spec.Volumes {
			if volume.HostPath == nil {
				return fmt.Errorf("unsupported volume source, %v. Only hostPath is supported ", volume.VolumeSource)
			}
		}
	}

	if r.Spec.NodeSet != nil {
		var nodesInSetsCount int32
		for _, nodeSetInline := range r.Spec.NodeSet {
			nodesInSetsCount += nodeSetInline.Nodes
		}
		if nodesInSetsCount != r.Spec.Nodes {
			return fmt.Errorf("incorrect value nodes: %d, does not satisfy with nodeSet: %d ", r.Spec.Nodes, nodesInSetsCount)
		}
	}

	crdCheckError := checkMonitoringCRD(manager, databaselog, r.Spec.Monitoring != nil)
	if crdCheckError != nil {
		return crdCheckError
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Database) ValidateUpdate(old runtime.Object) error {
	databaselog.Info("validate update", "name", r.Name)

	oldDatabase, _ := old.(*Database)
	if r.Spec.Domain != oldDatabase.Spec.Domain {
		return errors.New("database domain cannot be changed")
	}

	if oldDatabase.GetDatabasePath() != r.GetDatabasePath() {
		return errors.New("database path cannot be changed")
	}

	if r.Spec.NodeSet != nil {
		var nodesInSetsCount int32
		for _, nodeSetInline := range r.Spec.NodeSet {
			nodesInSetsCount += nodeSetInline.Nodes
		}
		if nodesInSetsCount != r.Spec.Nodes {
			return fmt.Errorf("incorrect value nodes: %d, does not satisfy with nodeSet: %d ", r.Spec.Nodes, nodesInSetsCount)
		}
	}

	crdCheckError := checkMonitoringCRD(manager, databaselog, r.Spec.Monitoring != nil)
	if crdCheckError != nil {
		return crdCheckError
	}

	return nil
}

func (r *Database) ValidateDelete() error {
	if r.Status.State != DatabasePaused {
		return fmt.Errorf("database deletion is only possible from `Paused` state, current state %v", r.Status.State)
	}
	return nil
}
