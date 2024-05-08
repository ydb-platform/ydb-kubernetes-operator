package schema_test

import (
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
- host:  storage-6
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
  # comment1
config:
  yaml_config_enabled: true
`

func TestSchema(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shema suite")
}

var _ = Describe("Testing schema", func() {
	It("Parse dynconfig", func() {
		dynconfig, err := v1alpha1.TryParseDynconfig(dynconfigExample)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(*dynconfig.Metadata).Should(BeEquivalentTo(schema.Metadata{
			Version: 1,
			Cluster: "unknown",
		}))
		Expect(dynconfig.Config["yaml_config_enabled"]).Should(BeTrue())
	})

	It("Parse static config", func() {
		yamlConfig := schema.Configuration{}
		err := yaml.Unmarshal([]byte(configurationExample), &yamlConfig)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(yamlConfig.Hosts).Should(BeEquivalentTo([]schema.Host{
			{
				Host:         "storage-0",
				NodeID:       1,
				HostConfigID: 1,
				WalleLocation: schema.WalleLocation{
					Body:       0,
					DataCenter: "dcExample",
					Rack:       "0",
				},
			},
			{
				Host:         "storage-1",
				NodeID:       2,
				HostConfigID: 1,
				WalleLocation: schema.WalleLocation{
					Body:       1,
					DataCenter: "dcExample",
					Rack:       "1",
				},
			},
			{
				Host:         "storage-2",
				NodeID:       3,
				HostConfigID: 1,
				WalleLocation: schema.WalleLocation{
					Body:       2,
					DataCenter: "dcExample",
					Rack:       "2",
				},
			},
			{
				Host:         "storage-3",
				NodeID:       4,
				HostConfigID: 1,
				WalleLocation: schema.WalleLocation{
					Body:       3,
					DataCenter: "dcExample",
					Rack:       "3",
				},
			},
			{
				Host:         "storage-4",
				NodeID:       5,
				HostConfigID: 1,
				WalleLocation: schema.WalleLocation{
					Body:       4,
					DataCenter: "dcExample",
					Rack:       "4",
				},
			},
			{
				Host:         "storage-5",
				NodeID:       6,
				HostConfigID: 1,
				WalleLocation: schema.WalleLocation{
					Body:       5,
					DataCenter: "dcExample",
					Rack:       "5",
				},
			},
			{
				Host:         "storage-6",
				NodeID:       7,
				HostConfigID: 1,
				WalleLocation: schema.WalleLocation{
					Body:       6,
					DataCenter: "dcExample",
					Rack:       "6",
				},
			},
			{
				Host:         "storage-7",
				NodeID:       8,
				HostConfigID: 1,
				WalleLocation: schema.WalleLocation{
					Body:       7,
					DataCenter: "dcExample",
					Rack:       "7",
				},
			},
		}))
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
