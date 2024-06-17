package schema

type DomainsConfig struct {
	SecurityConfig *SecurityConfig `yaml:"security_config"`
}

type SecurityConfig struct {
	EnforceUserTokenRequirement bool `yaml:"enforce_user_token_requirement"`
}
