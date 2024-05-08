package schema

type Dynconfig struct {
	Metadata *Metadata              `yaml:"metadata"`
	Config   map[string]interface{} `yaml:"config"`
}

type Configuration struct {
	Hosts     []Host     `yaml:"hosts"`
	KeyConfig *KeyConfig `yaml:"key_config,omitempty"`
}

type Metadata struct {
	Kind    string `yaml:"kind,omitempty"`
	Cluster string `yaml:"cluster"`
	Version uint64 `yaml:"version"`
	ID      uint64 `yaml:"id,omitempty"`
}
