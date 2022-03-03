package schema

type Key struct {
	ContainerPath string  `yaml:"container_path"`
	Id            string  `yaml:"id"`
	Pin           *string `yaml:"pin,omitempty"`
	Version       int     `yaml:"version"`
}

type KeyConfig struct {
	Keys []Key `yaml:"keys"`
}
