package schema_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ydb-platform/ydb-kubernetes-operator/api/v1alpha1"
	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
)

var configurationExample = `
---
yaml_config_enabled: true
domains_config:
  domain:
  - name: Root
    storage_pool_types:
    - kind: ssd
      pool_config:
        box_id: 1
        erasure_species: block-4-2
        kind: ssd
        pdisk_filter:
        - property:
          - type: SSD
        vdisk_kind: Default
  state_storage:
  - ring:
      node: [1, 2, 3, 4, 5, 6, 7, 8]
      nto_select: 5
    ssid: 1
hosts:
- host: storage-0
  walle_location: {body: 0, data_center: 'dcExample', rack: '0'}
  node_id: 1
  host_config_id: 1
- host: storage-1
  walle_location: {body: 1, data_center: 'dcExample', rack: '1'}
  node_id: 2
  host_config_id: 1
- host: storage-2
  walle_location: {body: 2, data_center: 'dcExample', rack: '2'}
  node_id: 3
  host_config_id: 1
- host: storage-3
  walle_location: {body: 3, data_center: 'dcExample', rack: '3'}
  node_id: 4
  host_config_id: 1
- host: storage-4
  walle_location: {body: 4, data_center: 'dcExample', rack: '4'}
  node_id: 5
  host_config_id: 1
- host: storage-5
  walle_location: {body: 5, data_center: 'dcExample', rack: '5'}
  node_id: 6
  host_config_id: 1
- host: storage-6
  walle_location: {body: 6, data_center: 'dcExample', rack: '6'}
  node_id: 7
  host_config_id: 1
- host: storage-7
  walle_location: {body: 7, data_center: 'dcExample', rack: '7'}
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
  kind: MainConfig
  version: 0
  cluster: "unknown"
  # comment1
selector_config:
- description: actor system config for dynnodes
  selector:
    node_type: slot
  config:
    actor_system_config:
      cpu_count: 10
      node_type: COMPUTE
      use_auto_config: true
allowed_labels:
  node_id:
    type: string
  host:
    type: string
  tenant:
    type: string
config:
  yaml_config_enabled: true
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
			Kind:    "MainConfig",
			Version: 0,
			Cluster: "unknown",
		}))
		Expect(dynconfig.Config["yaml_config_enabled"]).Should(BeTrue())
	})

	It("Try parse static config as dynconfig", func() {
		_, err := v1alpha1.ParseDynconfig(configurationExample)
		Expect(err).Should(HaveOccurred())
	})

	It("Parse static config", func() {
		yamlConfig, err := v1alpha1.ParseConfig(configurationExample)
		Expect(err).ShouldNot(HaveOccurred())
		hosts := []schema.Host{}
		for i := 0; i < 8; i++ {
			hosts = append(hosts, schema.Host{
				Host:         fmt.Sprintf("storage-%d", i),
				NodeID:       i + 1,
				HostConfigID: 1,
				WalleLocation: schema.WalleLocation{
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
