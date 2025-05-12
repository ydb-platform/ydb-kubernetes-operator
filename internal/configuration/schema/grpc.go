package schema

type GrpcConfig struct {
	Port    int32 `yaml:"port,omitempty"`
	SslPort int32 `yaml:"ssl_port,omitempty"`
}
