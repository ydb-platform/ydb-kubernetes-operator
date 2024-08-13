package v1alpha1

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
)

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

	success, dynConfig, err := ParseDynConfig(rawYamlConfiguration)
	if success {
		if err != nil {
			return nil, fmt.Errorf("failed to parse dynconfig, error: %w", err)
		}
		if dynConfig.Config["hosts"] == nil {
			hosts := generateHosts(cr)
			dynConfig.Config["hosts"] = hosts
		}

		return yaml.Marshal(dynConfig)
	}

	err = yaml.Unmarshal([]byte(rawYamlConfiguration), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize YAML config, error: %w", err)
	}

	if config["hosts"] == nil {
		hosts := generateHosts(cr)
		config["hosts"] = hosts
	}

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

func ParseDynConfig(rawYamlConfiguration string) (bool, schema.DynConfig, error) {
	dec := yaml.NewDecoder(bytes.NewReader([]byte(rawYamlConfiguration)))
	dec.KnownFields(true)

	var dynConfig schema.DynConfig
	err := dec.Decode(&dynConfig)
	if err != nil {
		return false, schema.DynConfig{}, fmt.Errorf("error unmarshal yaml to dynconfig: %w", err)
	}

	err = validateDynConfig(dynConfig)
	if err != nil {
		return true, dynConfig, fmt.Errorf("error validate dynconfig: %w", err)
	}

	return true, dynConfig, err
}

func validateDynConfig(dynConfig schema.DynConfig) error {
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

func GetConfigForCMS(dynConfig schema.DynConfig) ([]byte, error) {
	delete(dynConfig.Config, "static_erasure")
	delete(dynConfig.Config, "host_configs")
	delete(dynConfig.Config, "nameservice_config")
	delete(dynConfig.Config, "blob_storage_config")
	delete(dynConfig.Config, "hosts")

	return yaml.Marshal(dynConfig)
}
