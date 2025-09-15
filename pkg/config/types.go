// Package config provides configuration types
package config

// BrokerConfig holds broker configuration
type BrokerConfig struct {
	Servers []ServerConfig `json:"servers" yaml:"servers"`
}

// ServerConfig represents server config
type ServerConfig struct {
	Name       string      `json:"name"                 yaml:"name"`
	URL        string      `json:"url"                  yaml:"url"`
	Hostname   string      `json:"hostname,omitempty"   yaml:"hostname,omitempty"`
	ToolPrefix string      `json:"toolPrefix,omitempty" yaml:"toolPrefix,omitempty"`
	Auth       *AuthConfig `json:"auth,omitempty"       yaml:"auth,omitempty"`
	// CredentialEnvVar specifies the environment variable containing the credential
	// This is used when credentials are injected via Kubernetes secrets
	// The env var follows the pattern: KAGENTI_{MCP_NAME}_CRED
	CredentialEnvVar string `json:"credentialEnvVar,omitempty" yaml:"credentialEnvVar,omitempty"`
	Enabled          bool   `json:"enabled"              yaml:"enabled"`
}

// AuthConfig holds auth configuration
type AuthConfig struct {
	Type     string `json:"type"               yaml:"type"`
	Token    string `json:"token,omitempty"    yaml:"token,omitempty"`
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
}
