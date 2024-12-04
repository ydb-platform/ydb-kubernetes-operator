package schema

type DynConfig struct {
	Metadata       *Metadata              `yaml:"metadata"`
	Config         map[string]interface{} `yaml:"config"`
	AllowedLabels  map[string]interface{} `yaml:"allowed_labels"`
	SelectorConfig []SelectorConfig       `yaml:"selector_config"`
}
type Configuration struct {
	DomainsConfig *DomainsConfig `yaml:"domains_config"`
	Hosts         []Host         `yaml:"hosts,omitempty"`
	KeyConfig     *KeyConfig     `yaml:"key_config,omitempty"`
}

type Metadata struct {
	Kind    string `yaml:"kind"`
	Cluster string `yaml:"cluster"`
	Version uint64 `yaml:"version"`
	ID      uint64 `yaml:"id,omitempty"`
}

type SelectorConfig struct {
	Description string                 `yaml:"description"`
	Selector    map[string]interface{} `yaml:"selector"`
	Config      map[string]interface{} `yaml:"config"`
}
