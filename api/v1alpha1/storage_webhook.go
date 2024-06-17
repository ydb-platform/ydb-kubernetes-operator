package v1alpha1

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
)

// log is for logging in this package.
var storagelog = logf.Log.WithName("storage-resource")

func (r *Storage) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(&StorageDefaulter{Client: mgr.GetClient()}).
		Complete()
}

func (r *Storage) GetStorageEndpointWithProto() string {
	return fmt.Sprintf("%s%s", r.GetStorageProto(), r.GetStorageEndpoint())
}

func (r *Storage) GetStorageProto() string {
	proto := GRPCProto
	if r.IsStorageEndpointSecure() {
		proto = GRPCSProto
	}

	return proto
}

func (r *Storage) GetStorageEndpoint() string {
	endpoint := r.GetGRPCServiceEndpoint()
	if r.IsRemoteNodeSetsOnly() {
		endpoint = r.GetHostFromConfigEndpoint()
	}

	return endpoint
}

func (r *Storage) GetGRPCServiceEndpoint() string {
	host := fmt.Sprintf(GRPCServiceFQDNFormat, r.Name, r.Namespace)
	if r.Spec.Service.GRPC.ExternalHost != "" {
		host = r.Spec.Service.GRPC.ExternalHost
	}

	return fmt.Sprintf("%s:%d", host, GRPCPort)
}

func (r *Storage) GetHostFromConfigEndpoint() string {
	configuration := make(map[string]interface{})

	// skip handle error because we already checked in webhook
	_ = yaml.Unmarshal([]byte(r.Spec.Configuration), &configuration)
	hostsConfig := configuration["hosts"].([]schema.Host)

	randNum := rand.Int31n(r.Spec.Nodes) // #nosec G404
	host := hostsConfig[randNum].Host
	return fmt.Sprintf("%s:%d", host, GRPCPort)
}

func (r *Storage) IsStorageEndpointSecure() bool {
	if r.Spec.Service.GRPC.TLSConfiguration != nil {
		return r.Spec.Service.GRPC.TLSConfiguration.Enabled
	}
	return false
}

func (r *Storage) IsRemoteNodeSetsOnly() bool {
	if len(r.Spec.NodeSets) == 0 {
		return false
	}

	for _, nodeSet := range r.Spec.NodeSets {
		if nodeSet.Remote == nil {
			return false
		}
	}

	return true
}

// StorageDefaulter mutates Storages
// +k8s:deepcopy-gen=false
type StorageDefaulter struct {
	Client client.Client
}

//+kubebuilder:webhook:path=/mutate-ydb-tech-v1alpha1-storage,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=storages,verbs=create;update,versions=v1alpha1,name=mutate-storage.ydb.tech,admissionReviewVersions=v1

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *StorageDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	storage := obj.(*Storage)
	storagelog.Info("default", "name", storage.Name)

	if !storage.Spec.OperatorSync {
		return nil
	}

	if storage.Spec.Image == nil {
		storage.Spec.Image = &PodImage{}
	}

	if storage.Spec.Image.Name == "" {
		if storage.Spec.YDBVersion == "" {
			storage.Spec.Image.Name = fmt.Sprintf(ImagePathFormat, RegistryPath, DefaultTag)
		} else {
			storage.Spec.Image.Name = fmt.Sprintf(ImagePathFormat, RegistryPath, storage.Spec.YDBVersion)
		}
	}

	if storage.Spec.Image.PullPolicyName == nil {
		policy := corev1.PullIfNotPresent
		storage.Spec.Image.PullPolicyName = &policy
	}

	if storage.Spec.Resources == nil {
		storage.Spec.Resources = &corev1.ResourceRequirements{}
	}

	if storage.Spec.Service == nil {
		storage.Spec.Service = &StorageServices{
			GRPC:         GRPCService{},
			Interconnect: InterconnectService{},
			Status:       StatusService{},
		}
	}

	if storage.Spec.Service.GRPC.TLSConfiguration == nil {
		storage.Spec.Service.GRPC.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}

	if storage.Spec.Service.Interconnect.TLSConfiguration == nil {
		storage.Spec.Service.Interconnect.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}

	if storage.Spec.Service.Status.TLSConfiguration == nil {
		storage.Spec.Service.Status.TLSConfiguration = &TLSConfiguration{Enabled: false}
	}

	if storage.Spec.Monitoring == nil {
		storage.Spec.Monitoring = &MonitoringOptions{
			Enabled: false,
		}
	}

	if storage.Spec.Domain == "" {
		storage.Spec.Domain = DefaultDatabaseDomain
	}

	configuration, err := BuildConfiguration(storage, nil)
	if err != nil {
		return err
	}
	storage.Spec.Configuration = string(configuration)

	return nil
}

//+kubebuilder:webhook:path=/validate-ydb-tech-v1alpha1-storage,mutating=true,failurePolicy=fail,sideEffects=None,groups=ydb.tech,resources=storages,verbs=create;update,versions=v1alpha1,name=validate-storage.ydb.tech,admissionReviewVersions=v1

var _ webhook.Validator = &Storage{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Storage) ValidateCreate() error {
	storagelog.Info("validate create", "name", r.Name)

	var configuration schema.Configuration

	rawYamlConfiguration := r.Spec.Configuration
	dynconfig, err := ParseDynconfig(r.Spec.Configuration)
	if err == nil {
		validErr := ValidateDynconfig(dynconfig)
		if validErr != nil {
			return fmt.Errorf("failed to validate dynconfig, error: %w", err)
		}
		config, err := yaml.Marshal(dynconfig.Config)
		if err != nil {
			return fmt.Errorf("failed to serialize to YAML configuration, error: %w", err)
		}
		rawYamlConfiguration = string(config)
	}

	configuration, err = ParseConfig(rawYamlConfiguration)
	if err != nil {
		return fmt.Errorf("failed to parse static configuration, error: %w", err)
	}

	var nodesNumber int32
	if len(configuration.Hosts) == 0 {
		nodesNumber = r.Spec.Nodes
	} else {
		nodesNumber = int32(len(configuration.Hosts))
	}

	minNodesPerErasure := map[ErasureType]int32{
		ErasureMirror3DC: 9,
		ErasureBlock42:   8,
		None:             1,
	}
	if nodesNumber < minNodesPerErasure[r.Spec.Erasure] {
		return fmt.Errorf("erasure type %v requires at least %v storage nodes", r.Spec.Erasure, minNodesPerErasure[r.Spec.Erasure])
	}

	var authEnabled bool
	if configuration.DomainsConfig.SecurityConfig != nil {
		if configuration.DomainsConfig.SecurityConfig.EnforceUserTokenRequirement != nil {
			authEnabled = *configuration.DomainsConfig.SecurityConfig.EnforceUserTokenRequirement
		}
	}

	if (authEnabled && r.Spec.OperatorConnection == nil) || (!authEnabled && r.Spec.OperatorConnection != nil) {
		return fmt.Errorf("field 'spec.operatorConnection' does not satisfy with config option `enforce_user_token_requirement: %t`", authEnabled)
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

	reservedSecretNames := []string{"datastreams"}

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

func hasUpdatesBesidesFrozen(oldStorage, newStorage *Storage) (bool, string) {
	oldStorageCopy := oldStorage.DeepCopy()
	newStorageCopy := newStorage.DeepCopy()

	// If we set Frozen field to the same value,
	// the remaining diff must be empty.
	oldStorageCopy.Spec.OperatorSync = false
	newStorageCopy.Spec.OperatorSync = false

	ignoreNonSpecFields := cmpopts.IgnoreFields(Storage{}, "Status", "ObjectMeta", "TypeMeta")

	diff := cmp.Diff(oldStorageCopy, newStorageCopy, ignoreNonSpecFields)
	return diff != "", diff
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Storage) ValidateUpdate(old runtime.Object) error {
	storagelog.Info("validate update", "name", r.Name)

	var configuration schema.Configuration

	rawYamlConfiguration := r.Spec.Configuration
	dynconfig, err := ParseDynconfig(r.Spec.Configuration)
	if err == nil {
		config, err := yaml.Marshal(dynconfig.Config)
		if err != nil {
			return fmt.Errorf("failed to parse .config from dynconfig, error: %w", err)
		}
		rawYamlConfiguration = string(config)
	}

	configuration, err = ParseConfig(rawYamlConfiguration)
	if err != nil {
		return fmt.Errorf("failed to parse .spec.configuration, error: %w", err)
	}

	var nodesNumber int32
	if len(configuration.Hosts) == 0 {
		nodesNumber = r.Spec.Nodes
	} else {
		nodesNumber = int32(len(configuration.Hosts))
	}

	minNodesPerErasure := map[ErasureType]int32{
		ErasureMirror3DC: 9,
		ErasureBlock42:   8,
		None:             1,
	}
	if nodesNumber < minNodesPerErasure[r.Spec.Erasure] {
		return fmt.Errorf("erasure type %v requires at least %v storage nodes", r.Spec.Erasure, minNodesPerErasure[r.Spec.Erasure])
	}

	var authEnabled bool
	if configuration.DomainsConfig.SecurityConfig != nil {
		if configuration.DomainsConfig.SecurityConfig.EnforceUserTokenRequirement != nil {
			authEnabled = *configuration.DomainsConfig.SecurityConfig.EnforceUserTokenRequirement
		}
	}

	if (authEnabled && r.Spec.OperatorConnection == nil) || (!authEnabled && r.Spec.OperatorConnection != nil) {
		return fmt.Errorf("field 'spec.operatorConnection' does not align with config option `enforce_user_token_requirement: %t`", authEnabled)
	}

	if !r.Spec.OperatorSync {
		oldStorage := old.(*Storage)

		hasIllegalUpdates, diff := hasUpdatesBesidesFrozen(old.(*Storage), r)

		if hasIllegalUpdates {
			if oldStorage.Spec.OperatorSync {
				return fmt.Errorf(
					"it is illegal to update spec.OperatorSync and any other "+
						"spec fields at the same time. Here is what you else tried to update: %s", diff)
			}

			return fmt.Errorf(
				"it is illegal to update any spec fields when spec.OperatorSync is false. "+
					"Here is what you else tried to update: %s", diff)
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

	crdCheckError := checkMonitoringCRD(manager, storagelog, r.Spec.Monitoring != nil)
	if crdCheckError != nil {
		return crdCheckError
	}

	return nil
}

func (r *Storage) ValidateDelete() error {
	if r.Status.State != StoragePaused {
		return fmt.Errorf("storage deletion is only possible from `Paused` state, current state %v", r.Status.State)
	}
	return nil
}
