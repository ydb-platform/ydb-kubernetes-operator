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

func generate(cr *v1alpha1.Storage, crDB *v1alpha1.Database) schema.Configuration {
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

func Build(cr *v1alpha1.Storage, crDB *v1alpha1.Database, rawYamlConfiguration string) (map[string]string, error) {
	crdConfig := make(map[string]interface{})
	generatedConfig := generate(cr, crDB)

	err := yaml.Unmarshal([]byte(rawYamlConfiguration), &crdConfig)
	if err != nil {
		return nil, err
	}

	if crdConfig["hosts"] == nil {
		crdConfig["hosts"] = generatedConfig.Hosts
	}
	if generatedConfig.KeyConfig != nil {
		crdConfig["key_config"] = generatedConfig.KeyConfig
	}

	data, err := yaml.Marshal(crdConfig)
	if err != nil {
		return nil, err
	}

	result := string(data)

	return map[string]string{
		v1alpha1.ConfigFileName: result,
	}, nil
}
