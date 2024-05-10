package v1alpha1

import (
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/ydb-platform/ydb-kubernetes-operator/internal/configuration/schema"
)

func generateHosts(storage *Storage) []schema.Host {
	var hosts []schema.Host

	for i := 0; i < int(storage.Spec.Nodes); i++ {
		datacenter := "az-1"
		if storage.Spec.Erasure == ErasureMirror3DC {
			datacenter = fmt.Sprintf("az-%d", i%3)
		}

		hosts = append(hosts, schema.Host{
			Host:         fmt.Sprintf("%v-%d", storage.GetName(), i),
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

	if storage.Spec.NodeSets != nil {
		hostIndex := 0
		for _, nodeSetSpec := range storage.Spec.NodeSets {
			for podIndex := 0; podIndex < int(nodeSetSpec.Nodes); podIndex++ {
				podName := storage.GetName() + "-" + nodeSetSpec.Name + "-" + strconv.Itoa(podIndex)
				hosts[hostIndex].Host = podName
				hostIndex++
			}
		}
	}

	return hosts
}

func buildConfiguration(storage *Storage) (string, error) {
	config := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(storage.Spec.Configuration), &config)
	if err != nil {
		return "", err
	}

	if config["hosts"] == nil {
		config["hosts"] = generateHosts(storage)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
