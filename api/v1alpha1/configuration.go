package v1alpha1

import (
	"bytes"
	"crypto/sha256"
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

	dynconfig, err := ParseDynconfig(rawYamlConfiguration)
	if err == nil {
		if dynconfig.Config["hosts"] == nil {
			hosts := generateHosts(cr)
			dynconfig.Config["hosts"] = hosts
		}

		return yaml.Marshal(dynconfig)
	}

	err = yaml.Unmarshal([]byte(rawYamlConfiguration), &config)
	if err != nil {
		return nil, err
	}

	if config["hosts"] == nil {
		hosts := generateHosts(cr)
		config["hosts"] = hosts
	}

	// Will be removed by YDBOPS-9692
	keyConfig := generateKeyConfig(cr, crDB)
	if keyConfig != nil {
		config["key_config"] = keyConfig
	}

	return yaml.Marshal(config)
}

func ParseConfiguration(rawYamlConfiguration string) (schema.Configuration, error) {
	configuration := schema.Configuration{}

	dynconfig, err := ParseDynconfig(rawYamlConfiguration)
	if err == nil {
		config, err := yaml.Marshal(dynconfig.Config)
		if err != nil {
			return configuration, err
		}
		rawYamlConfiguration = string(config)
	}

	dec := yaml.NewDecoder(bytes.NewReader([]byte(rawYamlConfiguration)))
	dec.KnownFields(false)
	err = dec.Decode(&configuration)

	return configuration, err
}

func ParseDynconfig(rawYamlConfiguration string) (schema.Dynconfig, error) {
	dynconfig := schema.Dynconfig{}
	dec := yaml.NewDecoder(bytes.NewReader([]byte(rawYamlConfiguration)))
	dec.KnownFields(true)
	err := dec.Decode(&dynconfig)
	return dynconfig, err
}
