package schema

type Configuration struct {
	Hosts     []Host     `yaml:"hosts"`
	KeyConfig *KeyConfig `yaml:"key_config,omitempty"`
}
