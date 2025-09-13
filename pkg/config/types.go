// Package config provides configuration types
package config

// BrokerConfig holds broker configuration
type BrokerConfig struct {
	Servers        []ServerConfig        `json:"servers" yaml:"servers"`
	VirtualServers []VirtualServerConfig `json:"virtualServers,omitempty" yaml:"virtualServers,omitempty"`
}

// ServerConfig represents server config
type ServerConfig struct {
	Name       string      `json:"name"                 yaml:"name"`
	URL        string      `json:"url"                  yaml:"url"`
	Hostname   string      `json:"hostname,omitempty"   yaml:"hostname,omitempty"`
	ToolPrefix string      `json:"toolPrefix,omitempty" yaml:"toolPrefix,omitempty"`
	Auth       *AuthConfig `json:"auth,omitempty"       yaml:"auth,omitempty"`
	Enabled    bool        `json:"enabled"              yaml:"enabled"`
}

// AuthConfig holds auth configuration
type AuthConfig struct {
	Type     string `json:"type"               yaml:"type"`
	Token    string `json:"token,omitempty"    yaml:"token,omitempty"`
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
}

// VirtualServerConfig represents virtual server config
type VirtualServerConfig struct {
	Name  string   `json:"name"  yaml:"name"`
	Tools []string `json:"tools" yaml:"tools"`
}
