package schema

type DomainsConfig struct {
	SecurityConfig *SecurityConfig `yaml:"security_config,omitempty"`
}

type SecurityConfig struct {
	EnforceUserTokenRequirement *bool `yaml:"enforce_user_token_requirement,omitempty"`
}
