package schema

type Host struct {
	Address      string   `yaml:"address,omitempty"`
	Host         string   `yaml:"host"`
	HostConfigID int      `yaml:"host_config_id"`
	NodeID       int      `yaml:"node_id"`
	Port         int      `yaml:"port,omitempty"`
	Location     Location `yaml:"location,omitempty"`
}

type Location struct {
	Body       int    `yaml:"body"`
	DataCenter string `yaml:"data_center"`
	Rack       string `yaml:"rack"`
}
