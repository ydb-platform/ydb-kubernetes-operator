package schema

type DomainsConfig struct {
	Domain         map[string]interface{} `yaml:"domain"`
	StateStorage   map[string]interface{} `yaml:"state_storage"`
	SecurityConfig *SecurityConfig        `yaml:"security_config"`
}

type SecurityConfig struct {
	EnforceUserTokenRequirement bool `yaml:"enforce_user_token_requirement"`
}
