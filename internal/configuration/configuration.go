package configuration

import (
	"crypto/sha256"
	"fmt"
	"path"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
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

func generateSomeDefaults(cr *v1alpha1.Storage, crDB *v1alpha1.Database) schema.Configuration {
	var hosts []schema.Host

	for i := 0; i < int(cr.Spec.Nodes); i++ {
		datacenter := "az-1"
		if cr.Spec.Erasure == v1alpha1.ErasureMirror3DC {
			datacenter = fmt.Sprintf("az-%d", i%3)
		}

		hosts = append(hosts, schema.Host{
			Host:         fmt.Sprintf("%v-%d", cr.GetName(), i),
			HostConfigID: 1, // TODO
			NodeID:       i + 1,
			Port:         v1alpha1.InterconnectPort,
			WalleLocation: schema.WalleLocation{
				Body:       12340 + i,
				DataCenter: datacenter,
				Rack:       strconv.Itoa(i),
			},
		})
	}

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

	return schema.Configuration{
		Hosts:     hosts,
		KeyConfig: keyConfig,
	}
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

func Build(cr *v1alpha1.Storage, crDB *v1alpha1.Database) (map[string]string, error) {
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

	err := yaml.Unmarshal([]byte(rawYamlConfiguration), &config)
	if err != nil {
		return nil, err
	}

	generatedConfig := generateSomeDefaults(cr, crDB)
	tryFillMissingSections(config, generatedConfig)

	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}

	result := string(data)

	return map[string]string{
		v1alpha1.ConfigFileName: result,
	}, nil
}
