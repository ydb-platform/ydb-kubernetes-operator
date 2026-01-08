package v1alpha1

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
	. "github.com/ydb-platform/ydb-kubernetes-operator/internal/controllers/constants" //nolint:revive,stylecheck
)

// log is for logging in this package.
var storagelog = logf.Log.WithName("storage-resource")

func (r *Storage) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(&StorageDefaulter{Client: mgr.GetClient()}).
		WithValidator(r).
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
	domain := DefaultDomainName
	if dnsAnnotation, ok := r.GetAnnotations()[DNSDomainAnnotation]; ok {
		domain = dnsAnnotation
	}
	host := fmt.Sprintf(GRPCServiceFQDNFormat, r.Name, r.Namespace, domain)
	if r.Spec.Service.GRPC.ExternalHost != "" {
		host = r.Spec.Service.GRPC.ExternalHost
	}

	return fmt.Sprintf("%s:%d", host, GRPCPort)
}

func (r *Storage) GetHostFromConfigEndpoint() string {
	rawYamlConfiguration := r.getRawYamlConfiguration()

	configuration, _ := ParseConfiguration(rawYamlConfiguration)
	randNum := rand.Intn(len(configuration.Hosts)) // #nosec G404
	return fmt.Sprintf("%s:%d", configuration.Hosts[randNum].Host, GRPCPort)
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

	if storage.Spec.OperatorConnection != nil {
		if storage.Spec.OperatorConnection.Oauth2TokenExchange != nil {
			if storage.Spec.OperatorConnection.Oauth2TokenExchange.SignAlg == "" {
				storage.Spec.OperatorConnection.Oauth2TokenExchange.SignAlg = DefaultSignAlgorithm
			}
		}
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

var _ admission.CustomValidator = &Storage{}

func isSignAlgorithmSupported(alg string) bool {
	supportedAlgs := jwt.GetAlgorithms()

	for _, supportedAlg := range supportedAlgs {
		if alg == supportedAlg {
			return true
		}
	}
	return false
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Storage) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	storage := obj.(*Storage)
	storagelog.Info("validate create", "name", storage.Name)

	var rawYamlConfiguration string
	success, dynConfig, err := ParseDynConfig(storage.Spec.Configuration)
	if success {
		if err != nil {
			return nil, fmt.Errorf("failed to parse dynconfig, error: %w", err)
		}
		config, err := yaml.Marshal(dynConfig.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize YAML config, error: %w", err)
		}
		rawYamlConfiguration = string(config)
	} else {
		rawYamlConfiguration = storage.Spec.Configuration
	}

	var configuration schema.Configuration
	configuration, err = ParseConfiguration(rawYamlConfiguration)
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration, error: %w", err)
	}

	var nodesNumber int32
	if len(configuration.Hosts) == 0 {
		nodesNumber = storage.Spec.Nodes
	} else {
		nodesNumber = int32(len(configuration.Hosts))
	}

	minNodesPerErasure := map[ErasureType]int32{
		ErasureMirror3DC: 3,
		ErasureBlock42:   8,
		None:             1,
	}

	if nodesNumber < minNodesPerErasure[storage.Spec.Erasure] {
		return nil, fmt.Errorf("erasure type %v requires at least %v storage nodes", storage.Spec.Erasure, minNodesPerErasure[storage.Spec.Erasure])
	}

	var authEnabled bool
	if configuration.DomainsConfig.SecurityConfig != nil {
		if configuration.DomainsConfig.SecurityConfig.EnforceUserTokenRequirement != nil {
			authEnabled = *configuration.DomainsConfig.SecurityConfig.EnforceUserTokenRequirement
		}
	}

	if authEnabled && storage.Spec.OperatorConnection == nil {
		return nil, fmt.Errorf("field 'spec.operatorConnection' does not satisfy with config option `enforce_user_token_requirement: %t`", authEnabled)
	}

	if storage.Spec.OperatorConnection != nil && storage.Spec.OperatorConnection.Oauth2TokenExchange != nil {
		auth := storage.Spec.OperatorConnection.Oauth2TokenExchange
		if auth.KeyID == nil {
			return nil, fmt.Errorf("field keyID is required for OAuth2TokenExchange type")
		}
		if !isSignAlgorithmSupported(auth.SignAlg) {
			return nil, fmt.Errorf("signAlg %s does not supported for OAuth2TokenExchange type", auth.SignAlg)
		}
	}

	if storage.Spec.NodeSets != nil {
		var nodesInSetsCount int32
		for _, nodeSetInline := range storage.Spec.NodeSets {
			nodesInSetsCount += nodeSetInline.Nodes
		}
		if nodesInSetsCount != storage.Spec.Nodes {
			return nil, fmt.Errorf("incorrect value nodes: %d, does not satisfy with nodeSets: %d ", storage.Spec.Nodes, nodesInSetsCount)
		}
	}

	reservedSecretNames := []string{
		"database_encryption",
		"datastreams",
	}

	for _, secret := range storage.Spec.Secrets {
		if slices.Contains(reservedSecretNames, secret.Name) {
			return nil, fmt.Errorf("the secret name %s is reserved, use another one", secret.Name)
		}
	}
	if storage.Spec.Volumes != nil {
		for _, volume := range storage.Spec.Volumes {
			if volume.HostPath == nil {
				return nil, fmt.Errorf("unsupported volume source, %v. Only hostPath is supported ", volume.VolumeSource)
			}
		}
	}

	crdCheckError := checkMonitoringCRD(manager, storagelog, storage.Spec.Monitoring != nil)
	if crdCheckError != nil {
		return nil, crdCheckError
	}

	return nil, nil
}

func hasUpdatesBesidesFrozen(oldStorage, newStorage *Storage) (bool, string) {
	oldStorageCopy := oldStorage.DeepCopy()
	newStorageCopy := newStorage.DeepCopy()

	// If we set Frozen field to the same value,
	// the remaining diff must be empty.
	oldStorageCopy.Spec.OperatorSync = false
	newStorageCopy.Spec.OperatorSync = false

	// We will allow configuration diffs if they are limited to
	// formatting, order of keys in the map etc. If two maps are
	// meaningfully different (not deep-equal), we still disallow
	// the diff of course.
	configurationCompareOpt := cmp.FilterPath(
		func(path cmp.Path) bool {
			if sf, ok := path.Last().(cmp.StructField); ok {
				return sf.Name() == "Configuration"
			}
			return false
		},
		cmp.Comparer(func(a, b string) bool {
			var o1, o2 any

			if err := yaml.Unmarshal([]byte(a), &o1); err != nil {
				return false
			}
			if err := yaml.Unmarshal([]byte(b), &o2); err != nil {
				return false
			}

			diff := cmp.Diff(o1, o2)
			if diff != "" {
				storagelog.Info(fmt.Sprintf("Configurations are different:\n%v\n%v", o1, o2))
			}

			return diff == ""
		}),
	)

	ignoreNonSpecFields := cmpopts.IgnoreFields(Storage{}, "Status", "ObjectMeta", "TypeMeta")
	diff := cmp.Diff(
		oldStorageCopy,
		newStorageCopy,
		ignoreNonSpecFields,
		configurationCompareOpt,
	)
	return diff != "", diff
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Storage) ValidateUpdate(ctx context.Context, oldObj runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	storage := newObj.(*Storage)
	storagelog.Info("validate update", "name", storage.Name)

	var rawYamlConfiguration string
	success, dynConfig, err := ParseDynConfig(storage.Spec.Configuration)
	if success {
		if err != nil {
			return nil, fmt.Errorf("failed to parse dynconfig, error: %w", err)
		}
		config, err := yaml.Marshal(dynConfig.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize YAML config, error: %w", err)
		}
		rawYamlConfiguration = string(config)
	} else {
		rawYamlConfiguration = storage.Spec.Configuration
	}

	var configuration schema.Configuration
	configuration, err = ParseConfiguration(rawYamlConfiguration)
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration, error: %w", err)
	}

	var nodesNumber int32
	if len(configuration.Hosts) == 0 {
		nodesNumber = storage.Spec.Nodes
	} else {
		nodesNumber = int32(len(configuration.Hosts))
	}

	minNodesPerErasure := map[ErasureType]int32{
		ErasureMirror3DC: 3,
		ErasureBlock42:   8,
		None:             1,
	}

	if nodesNumber < minNodesPerErasure[storage.Spec.Erasure] {
		return nil, fmt.Errorf("erasure type %v requires at least %v storage nodes", storage.Spec.Erasure, minNodesPerErasure[storage.Spec.Erasure])
	}

	var authEnabled bool
	if configuration.DomainsConfig.SecurityConfig != nil {
		if configuration.DomainsConfig.SecurityConfig.EnforceUserTokenRequirement != nil {
			authEnabled = *configuration.DomainsConfig.SecurityConfig.EnforceUserTokenRequirement
		}
	}

	if authEnabled && storage.Spec.OperatorConnection == nil {
		return nil, fmt.Errorf("field 'spec.operatorConnection' does not align with config option `enforce_user_token_requirement: %t`", authEnabled)
	}

	if storage.Spec.OperatorConnection != nil && storage.Spec.OperatorConnection.Oauth2TokenExchange != nil {
		auth := storage.Spec.OperatorConnection.Oauth2TokenExchange
		if auth.KeyID == nil {
			return nil, fmt.Errorf("field keyID is required for OAuth2TokenExchange type")
		}
		if !isSignAlgorithmSupported(auth.SignAlg) {
			return nil, fmt.Errorf("signAlg %s does not supported for OAuth2TokenExchange type", auth.SignAlg)
		}
	}

	if !storage.Spec.OperatorSync {
		oldStorage := oldObj.(*Storage)

		hasIllegalUpdates, diff := hasUpdatesBesidesFrozen(oldStorage, storage)

		if hasIllegalUpdates {
			if oldStorage.Spec.OperatorSync {
				return nil, fmt.Errorf(
					"it is illegal to update spec.OperatorSync and any other "+
						"spec fields at the same time. Here is what you else tried to update: %s", diff)
			}

			return nil, fmt.Errorf(
				"it is illegal to update any spec fields when spec.OperatorSync is false. "+
					"Here is what you else tried to update: %s", diff)
		}
	}

	if storage.Spec.NodeSets != nil {
		var nodesInSetsCount int32
		for _, nodeSetInline := range storage.Spec.NodeSets {
			nodesInSetsCount += nodeSetInline.Nodes
		}
		if nodesInSetsCount != storage.Spec.Nodes {
			return nil, fmt.Errorf("incorrect value nodes: %d, does not satisfy with nodeSets: %d ", storage.Spec.Nodes, nodesInSetsCount)
		}
	}

	crdCheckError := checkMonitoringCRD(manager, storagelog, storage.Spec.Monitoring != nil)
	if crdCheckError != nil {
		return nil, crdCheckError
	}

	if err := storage.validateGrpcPorts(); err != nil {
		return nil, err
	}

	return nil, nil
}

func (r *Storage) getRawYamlConfiguration() string {
	var rawYamlConfiguration string
	// skip handle error because we already checked in webhook
	success, dynConfig, _ := ParseDynConfig(r.Spec.Configuration)
	if success {
		config, _ := yaml.Marshal(dynConfig.Config)
		rawYamlConfiguration = string(config)
	} else {
		rawYamlConfiguration = r.Spec.Configuration
	}

	return rawYamlConfiguration
}

func (r *Storage) validateGrpcPorts() error {
	// There are three possible ways to configure grpc ports:

	// service:
	//    grpc:                  == this means one insecure port, tls is disabled
	//      port: 2135

	// service:
	//    grpc:
	//      port: 2136           == this means one secure port, tls is enabled
	//      tls:
	//        enabled: true

	//  service:
	//    grpc:
	//      insecurePort: 2135   == this means two ports, one secure \ one insecure
	//      port: 2136
	//      tls:
	//        enabled: true

	rawYamlConfiguration := r.getRawYamlConfiguration()
	configuration, err := ParseConfiguration(rawYamlConfiguration)
	if err != nil {
		return fmt.Errorf("failed to parse configuration immediately after building it, should not happen, %w", err)
	}
	configurationPort := int32(GRPCPort)
	if configuration.GrpcConfig.Port != 0 {
		configurationPort = configuration.GrpcConfig.Port
	}
	configurationSslPort := int32(0)
	if configuration.GrpcConfig.SslPort != 0 {
		configurationSslPort = configuration.GrpcConfig.SslPort
	}

	if !r.Spec.Service.GRPC.TLSConfiguration.Enabled {
		// there should be only 1 port, both in service and in config, insecure
		servicePort := int32(GRPCPort)
		if r.Spec.Service.GRPC.Port != 0 {
			servicePort = r.Spec.Service.GRPC.Port
		}
		if configurationPort != servicePort {
			return fmt.Errorf(
				"inconsistent grpc ports: spec.service.grpc.port (%v) != configuration.grpc_config.port (%v)",
				servicePort,
				configurationPort,
			)
		}

		if r.Spec.Service.GRPC.InsecurePort != 0 {
			return fmt.Errorf(
				"spec.service.grpc.tls.enabled is false, use `port` instead of `insecurePort` field to assign non-tls grpc port",
			)
		}
		return nil
	}

	// otherwise, there might be 1 (secure only) port...
	servicePort := int32(GRPCPort)
	if r.Spec.Service.GRPC.Port != 0 {
		servicePort = r.Spec.Service.GRPC.Port
	}
	if configurationSslPort == 0 {
		return fmt.Errorf(
			"configuration.grpc_config.ssl_port is absent in cluster configuration, but spec.service.grpc has tls enabled and port %v",
			servicePort,
		)
	}
	if configurationSslPort != servicePort {
		return fmt.Errorf(
			"inconsistent grpc ports: spec.service.grpc.port (%v) != configuration.grpc_config.ssl_port (%v)",
			servicePort,
			configurationSslPort,
		)
	}

	// or, optionally, one more: insecure port
	if r.Spec.Service.GRPC.InsecurePort != 0 {
		serviceInsecurePort := r.Spec.Service.GRPC.InsecurePort

		if configurationPort != serviceInsecurePort {
			return fmt.Errorf(
				"inconsistent grpc insecure ports: spec.service.grpc.insecure_port (%v) != configuration.grpc_config.port (%v)",
				serviceInsecurePort,
				configurationPort,
			)
		}
	}

	return nil
}

func (r *Storage) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	storage := obj.(*Storage)
	if storage.Status.State != StoragePaused {
		return nil, fmt.Errorf("storage deletion is only possible from `Paused` state, current state %v", storage.Status.State)
	}
	return nil, nil
}
