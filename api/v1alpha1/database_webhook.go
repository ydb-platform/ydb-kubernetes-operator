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
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
)

// log is for logging in this package.
var databaselog = logf.Log.WithName("database-resource")

var manager ctrl.Manager

func (r *Database) SetupWebhookWithManager(mgr ctrl.Manager) error {
	manager = mgr
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(&DatabaseDefaulter{Client: mgr.GetClient()}).
		WithValidator(r).
		Complete()
}

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

// +kubebuilder:webhook:path=/mutate-ydb-tech-v1alpha1-database,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=databases,verbs=create;update,versions=v1alpha1,name=mutate-database.ydb.tech,admissionReviewVersions=v1

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *DatabaseDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	database := obj.(*Database)
	databaselog.Info("default", "name", database.Name)

	if !database.Spec.OperatorSync {
		return nil
	}

	if database.Spec.StorageClusterRef.Namespace == "" {
		database.Spec.StorageClusterRef.Namespace = database.Namespace
	}

	if database.Spec.ServerlessResources != nil {
		if database.Spec.ServerlessResources.SharedDatabaseRef.Namespace == "" {
			database.Spec.ServerlessResources.SharedDatabaseRef.Namespace = database.Namespace
		}
	}

	if database.Spec.Image == nil {
		database.Spec.Image = &PodImage{}
	}

	if database.Spec.Image.Name == "" {
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

	if database.Spec.Service == nil {
		database.Spec.Service = &DatabaseServices{
			GRPC:         GRPCService{},
			Interconnect: InterconnectService{},
			Status:       StatusService{},
			Datastreams:  DatastreamsService{},
		}
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

	if database.Spec.Service.Status.TLSConfiguration == nil {
		database.Spec.Service.Status.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}

	if database.Spec.Domain == "" {
		database.Spec.Domain = DefaultDatabaseDomain
	}

	if database.Spec.Path == "" {
		database.Spec.Path = database.GetLegacyDatabasePath()
	}

	if database.Spec.Encryption == nil {
		database.Spec.Encryption = &EncryptionConfig{Enabled: false}
	}

	if database.Spec.Encryption.Enabled && database.Spec.Encryption.Key == nil {
		if database.Spec.Encryption.Pin == nil || len(*database.Spec.Encryption.Pin) == 0 {
			encryptionPin := DefaultDatabaseEncryptionPin
			database.Spec.Encryption.Pin = &encryptionPin
		}
	}

	if database.Spec.Datastreams == nil {
		database.Spec.Datastreams = &DatastreamsConfig{Enabled: false}
	}

	if database.Spec.Monitoring == nil {
		database.Spec.Monitoring = &MonitoringOptions{
			Enabled: false,
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
		database.Spec.StorageEndpoint = storage.GetStorageEndpointWithProto()
	}

	if database.Spec.Configuration != "" {
		configuration, err := BuildConfiguration(storage, database)
		if err != nil {
			return err
		}
		database.Spec.Configuration = string(configuration)
	}

	return nil
}

//+kubebuilder:webhook:path=/validate-ydb-tech-v1alpha1-database,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=databases,verbs=create;update,versions=v1alpha1,name=validate-database.ydb.tech,admissionReviewVersions=v1

var _ admission.CustomValidator = &Database{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Database) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	database := obj.(*Database)
	databaselog.Info("validate create", "name", database.Name)

	if database.Spec.Domain != "" && database.Spec.Path != "" {
		if !strings.HasPrefix(database.Spec.Path, fmt.Sprintf("/%s", database.Spec.Domain)) {
			return nil, fmt.Errorf("incorrect database path, must start with domain: \"/%s\"", database.Spec.Domain)
		}
	}

	if database.Spec.Resources == nil && database.Spec.SharedResources == nil && database.Spec.ServerlessResources == nil {
		return nil, errors.New("incorrect database resources configuration, must be one of: Resources, SharedResources, ServerlessResources")
	}

	if database.Spec.Volumes != nil {
		for _, volume := range database.Spec.Volumes {
			if volume.HostPath == nil {
				return nil, fmt.Errorf("unsupported volume source, %v. Only hostPath is supported ", volume.VolumeSource)
			}
		}
	}

	if database.Spec.NodeSets != nil {
		var nodesInSetsCount int32
		for _, nodeSetInline := range database.Spec.NodeSets {
			nodesInSetsCount += nodeSetInline.Nodes
		}
		if nodesInSetsCount != database.Spec.Nodes {
			return nil, fmt.Errorf("incorrect value nodes: %d, does not satisfy with nodeSets: %d ", database.Spec.Nodes, nodesInSetsCount)
		}
	}

	crdCheckError := checkMonitoringCRD(manager, databaselog, database.Spec.Monitoring != nil)
	if crdCheckError != nil {
		return nil, crdCheckError
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Database) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	database := newObj.(*Database)
	databaselog.Info("validate update", "name", database.Name)

	oldDatabase, _ := oldObj.(*Database)
	if database.Spec.Domain != oldDatabase.Spec.Domain {
		return nil, errors.New("database domain cannot be changed")
	}

	if oldDatabase.GetDatabasePath() != database.GetDatabasePath() {
		return nil, errors.New("database path cannot be changed")
	}

	if database.Spec.NodeSets != nil {
		var nodesInSetsCount int32
		for _, nodeSetInline := range database.Spec.NodeSets {
			nodesInSetsCount += nodeSetInline.Nodes
		}
		if nodesInSetsCount != database.Spec.Nodes {
			return nil, fmt.Errorf("incorrect value nodes: %d, does not satisfy with nodeSets: %d ", database.Spec.Nodes, nodesInSetsCount)
		}
	}

	crdCheckError := checkMonitoringCRD(manager, databaselog, database.Spec.Monitoring != nil)
	if crdCheckError != nil {
		return nil, crdCheckError
	}

	return nil, nil
}

func (r *Database) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	database := obj.(*Database)
	if database.Status.State != DatabasePaused {
		return nil, fmt.Errorf("database deletion is only possible from `Paused` state, current state %v", database.Status.State)
	}
	return nil, nil
}
