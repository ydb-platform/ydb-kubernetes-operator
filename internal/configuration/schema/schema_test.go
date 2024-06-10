package schema_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
)

var configurationExample = `
---
hosts:
- host: storage-0
  location: {body: 0, data_center: 'dcExample', rack: '0'}
  node_id: 1
  host_config_id: 1
- host: storage-1
  location: {body: 1, data_center: 'dcExample', rack: '1'}
  node_id: 2
  host_config_id: 1
- host: storage-2
  location: {body: 2, data_center: 'dcExample', rack: '2'}
  node_id: 3
  host_config_id: 1
- host: storage-3
  location: {body: 3, data_center: 'dcExample', rack: '3'}
  node_id: 4
  host_config_id: 1
- host: storage-4
  location: {body: 4, data_center: 'dcExample', rack: '4'}
  node_id: 5
  host_config_id: 1
- host: storage-5
  location: {body: 5, data_center: 'dcExample', rack: '5'}
  node_id: 6
  host_config_id: 1
- host: storage-6
  location: {body: 6, data_center: 'dcExample', rack: '6'}
  node_id: 7
  host_config_id: 1
- host: storage-7
  location: {body: 7, data_center: 'dcExample', rack: '7'}
  node_id: 8
  host_config_id: 1
key_config:
  keys:
  - container_path: "/opt/ydb/secrets/database_encryption/key"
    id: "1"
    version: 1
`

var dynconfigExample = `
---
metadata:
  version: 1
  cluster: "unknown"
  kind: MainConfig
  # comment1
config:
  yaml_config_enabled: true
selector_config: []
allowed_labels: {}
`

func TestSchema(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shema suite")
}

var _ = Describe("Testing schema", func() {
	It("Parse dynconfig", func() {
		dynconfig, err := v1alpha1.ParseDynconfig(dynconfigExample)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(*dynconfig.Metadata).Should(BeEquivalentTo(schema.Metadata{
			Version: 1,
			Cluster: "unknown",
			Kind:    "MainConfig",
		}))
		Expect(dynconfig.AllowedLabels).ShouldNot(BeNil())
		Expect(dynconfig.SelectorConfig).ShouldNot(BeNil())
		Expect(dynconfig.Config["yaml_config_enabled"]).Should(BeTrue())
	})
	It("Try parse static config as dynconfig", func() {
		_, err := v1alpha1.ParseDynconfig(configurationExample)
		Expect(err).Should(HaveOccurred())
	})

	It("Parse static config", func() {
		yamlConfig := schema.Configuration{}
		err := yaml.Unmarshal([]byte(configurationExample), &yamlConfig)
		Expect(err).ShouldNot(HaveOccurred())
		hosts := []schema.Host{}
		for i := 0; i < 8; i++ {
			hosts = append(hosts, schema.Host{
				Host:         fmt.Sprintf("storage-%d", i),
				NodeID:       i + 1,
				HostConfigID: 1,
				Location: schema.Location{
					Body:       i,
					DataCenter: "dcExample",
					Rack:       fmt.Sprint(i),
				},
			})
		}
		Expect(yamlConfig.Hosts).Should(BeEquivalentTo(hosts))
		Expect(*yamlConfig.KeyConfig).Should(BeEquivalentTo(schema.KeyConfig{
			Keys: []schema.Key{
				{
					ContainerPath: "/opt/ydb/secrets/database_encryption/key",
					ID:            "1",
					Version:       1,
				},
			},
		}))
	})
})
