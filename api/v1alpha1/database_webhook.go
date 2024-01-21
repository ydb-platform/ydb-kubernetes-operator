package v1alpha1

import (
	"context"
	"errors"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		WithDefaulter(&DatabaseDefaulter{Client: mgr.GetClient()}).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-ydb-tech-v1alpha1-database,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=databases,verbs=create;update,versions=v1alpha1,name=mutate-database.ydb.tech,admissionReviewVersions=v1

func (r *Database) GetDatabasePath() string {
	if r.Spec.Path != "" {
		return r.Spec.Path
	}
	return r.GetLegacyDatabasePath()
}

func (r *Database) GetLegacyDatabasePath() string {
	return fmt.Sprintf(legacyTenantNameFormat, r.Spec.Domain, r.Name) // FIXME: review later in context of multiple namespaces
}

// DatabaseDefaulter mutates Databases
// +k8s:deepcopy-gen=false
type DatabaseDefaulter struct {
	Client client.Client
}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *DatabaseDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	database := obj.(*Database)
	databaselog.Info("default", "name", database.Name)

	if database.Spec.StorageClusterRef.Namespace == "" {
		database.Spec.StorageClusterRef.Namespace = database.Namespace
	}

	if database.Spec.ServerlessResources != nil {
		if database.Spec.ServerlessResources.SharedDatabaseRef.Namespace == "" {
			database.Spec.ServerlessResources.SharedDatabaseRef.Namespace = database.Namespace
		}
	}

	storage := &Storage{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: database.Spec.StorageClusterRef.Namespace,
		Name:      database.Spec.StorageClusterRef.Name,
	}, storage)
	if err != nil {
		return err
	}

	if database.Spec.StorageEndpoint == "" {
		database.Spec.StorageEndpoint = storage.GetGRPCEndpointWithProto()
	}

	configuration, err := buildConfiguration(storage, database)
	if err != nil {
		return err
	}
	database.Spec.Configuration = configuration

	if database.Spec.Image == nil && database.Spec.Image.Name == "" {
		if database.Spec.YDBVersion == "" {
			database.Spec.Image.Name = fmt.Sprintf(ImagePathFormat, RegistryPath, DefaultTag)
		} else {
			database.Spec.Image.Name = fmt.Sprintf(ImagePathFormat, RegistryPath, database.Spec.YDBVersion)
		}
	}

	if database.Spec.Image.PullPolicyName == nil {
		policy := v1.PullIfNotPresent
		database.Spec.Image.PullPolicyName = &policy
	}

	if database.Spec.Service.GRPC.TLSConfiguration == nil {
		database.Spec.Service.GRPC.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}

	if database.Spec.Service.Interconnect.TLSConfiguration == nil {
		database.Spec.Service.Interconnect.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}

	if database.Spec.Service.Datastreams.TLSConfiguration == nil {
		database.Spec.Service.Datastreams.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}

	if database.Spec.Domain == "" {
		database.Spec.Domain = DefaultDatabaseDomain
	}

	if database.Spec.Path == "" {
		database.Spec.Path = database.GetLegacyDatabasePath()
	}

	if database.Spec.StorageEndpoint == "" {
		database.Spec.Path = database.GetLegacyDatabasePath()
	}

	if database.Spec.Encryption == nil {
		database.Spec.Encryption = &EncryptionConfig{Enabled: false}
	}

	if database.Spec.Datastreams == nil {
		database.Spec.Datastreams = &DatastreamsConfig{Enabled: false}
	}

	if database.Spec.Monitoring == nil {
		database.Spec.Monitoring = &MonitoringOptions{
			Enabled: false,
		}
	}

	return nil
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

	if r.Spec.NodeSets != nil {
		var nodesInSetsCount int32
		for _, nodeSetInline := range r.Spec.NodeSets {
			nodesInSetsCount += nodeSetInline.Nodes
		}
		if nodesInSetsCount != r.Spec.Nodes {
			return fmt.Errorf("incorrect value nodes: %d, does not satisfy with nodeSets: %d ", r.Spec.Nodes, nodesInSetsCount)
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

	if r.Spec.NodeSets != nil {
		var nodesInSetsCount int32
		for _, nodeSetInline := range r.Spec.NodeSets {
			nodesInSetsCount += nodeSetInline.Nodes
		}
		if nodesInSetsCount != r.Spec.Nodes {
			return fmt.Errorf("incorrect value nodes: %d, does not satisfy with nodeSets: %d ", r.Spec.Nodes, nodesInSetsCount)
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
