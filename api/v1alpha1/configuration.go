package v1alpha1

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"path"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
)

const (
	DatabaseEncryptionKeyPath           = "/opt/ydb/secrets/database_encryption"
	DatabaseEncryptionKeyFile           = "key"
	DatastreamsIAMServiceAccountKeyPath = "/opt/ydb/secrets/datastreams"
	DatastreamsIAMServiceAccountKeyFile = "sa_key.json"
)

func hash(text string) string {
	h := sha256.New()
	h.Write([]byte(text))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func generateHosts(cr *Storage) []schema.Host {
	var hosts []schema.Host

	for i := 0; i < int(cr.Spec.Nodes); i++ {
		datacenter := "az-1"
		if cr.Spec.Erasure == ErasureMirror3DC {
			datacenter = fmt.Sprintf("az-%d", i%3)
		}

		hosts = append(hosts, schema.Host{
			Host:         fmt.Sprintf("%v-%d", cr.GetName(), i),
			HostConfigID: 1, // TODO
			NodeID:       i + 1,
			Port:         InterconnectPort,
			WalleLocation: schema.WalleLocation{
				Body:       12340 + i,
				DataCenter: datacenter,
				Rack:       strconv.Itoa(i),
			},
		})
	}

	if cr.Spec.NodeSets != nil {
		hostIndex := 0
		for _, nodeSetSpec := range cr.Spec.NodeSets {
			for podIndex := 0; podIndex < int(nodeSetSpec.Nodes); podIndex++ {
				podName := cr.GetName() + "-" + nodeSetSpec.Name + "-" + strconv.Itoa(podIndex)
				hosts[hostIndex].Host = podName
				hostIndex++
			}
		}
	}

	return hosts
}

func generateKeyConfig(cr *Storage, crDB *Database) *schema.KeyConfig {
	var keyConfig *schema.KeyConfig
	if crDB != nil && crDB.Spec.Encryption != nil && crDB.Spec.Encryption.Enabled {
		keyConfig = &schema.KeyConfig{
			Keys: []schema.Key{
				{
					ContainerPath: path.Join(DatabaseEncryptionKeyPath, DatabaseEncryptionKeyFile),
					ID:            hash(cr.Name),
					Pin:           crDB.Spec.Encryption.Pin,
					Version:       1,
				},
			},
		}
	}

	return keyConfig
}

func tryFillMissingSections(
	resultConfig map[string]interface{},
	generatedConfig schema.Configuration,
) {
	if resultConfig["hosts"] == nil {
		resultConfig["hosts"] = generatedConfig.Hosts
	}
	if generatedConfig.KeyConfig != nil {
		resultConfig["key_config"] = generatedConfig.KeyConfig
	}
}

func BuildConfiguration(cr *Storage, crDB *Database) ([]byte, error) {
	config := make(map[string]interface{})

	// If any kind of configuration exists on Database object, then
	// it will be used to fully override storage object.
	// This is a temporary solution that should go away when it would
	// be possible to override Database configuration partially.
	var rawYamlConfiguration string
	if crDB != nil && crDB.Spec.Configuration != "" {
		rawYamlConfiguration = crDB.Spec.Configuration
	} else {
		rawYamlConfiguration = cr.Spec.Configuration
	}

	hosts := generateHosts(cr)
	keyConfig := generateKeyConfig(cr, crDB)
	generatedConfig := schema.Configuration{
		Hosts:     hosts,
		KeyConfig: keyConfig,
	}

	success, dynconfig, err := TryParseDynconfig(rawYamlConfiguration)
	if success {
		if err != nil {
			return nil, fmt.Errorf("failed to parse dynconfig, error: %w", err)
		}
		tryFillMissingSections(dynconfig.Config, generatedConfig)
		return yaml.Marshal(dynconfig)
	}

	err = yaml.Unmarshal([]byte(rawYamlConfiguration), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize YAML config, error: %w", err)
	}

	tryFillMissingSections(config, generatedConfig)
	return yaml.Marshal(config)
}

func ParseConfiguration(rawYamlConfiguration string) (schema.Configuration, error) {
	dec := yaml.NewDecoder(bytes.NewReader([]byte(rawYamlConfiguration)))
	dec.KnownFields(false)

	var configuration schema.Configuration
	err := dec.Decode(&configuration)
	if err != nil {
		return schema.Configuration{}, nil
	}

	return configuration, nil
}

func TryParseDynconfig(rawYamlConfiguration string) (bool, schema.Dynconfig, error) {
	dec := yaml.NewDecoder(bytes.NewReader([]byte(rawYamlConfiguration)))
	dec.KnownFields(true)

	var dynconfig schema.Dynconfig
	err := dec.Decode(&dynconfig)
	if err != nil {
		return false, schema.Dynconfig{}, fmt.Errorf("error unmarshal yaml to dynconfig: %w", err)
	}

	err = validateDynconfig(dynconfig)
	if err != nil {
		return true, dynconfig, fmt.Errorf("error validate dynconfig: %w", err)
	}

	return true, dynconfig, nil
}

func validateDynconfig(dynConfig schema.Dynconfig) error {
	if _, exist := dynConfig.Config["yaml_config_enabled"]; !exist {
		return errors.New("failed to find mandatory `yaml_config_enabled` field inside config")
	}

	if _, exist := dynConfig.Config["static_erasure"]; !exist {
		return errors.New("failed to find mandatory `static_erasure` field inside config")
	}

	if _, exist := dynConfig.Config["host_configs"]; !exist {
		return errors.New("failed to find mandatory `host_configs` field inside config")
	}

	if _, exist := dynConfig.Config["blob_storage_config"]; !exist {
		return errors.New("failed to find mandatory `blob_storage_config` field inside config")
	}

	return nil
}

func GetConfigForCMS(dynconfig schema.Dynconfig) ([]byte, error) {
	delete(dynconfig.Config, "static_erasure")
	delete(dynconfig.Config, "host_configs")
	delete(dynconfig.Config, "nameservice_config")
	delete(dynconfig.Config, "blob_storage_config")
	delete(dynconfig.Config, "hosts")

	return yaml.Marshal(dynconfig)
}
