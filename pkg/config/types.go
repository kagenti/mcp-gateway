package config

type BrokerConfig struct {
	Servers  []ServerConfig `json:"servers" yaml:"servers"`
	Port     int            `json:"port,omitempty" yaml:"port,omitempty"`
	BindAddr string         `json:"bindAddr,omitempty" yaml:"bindAddr,omitempty"`
	LogLevel string         `json:"logLevel,omitempty" yaml:"logLevel,omitempty"`
}

type ServerConfig struct {
	Name       string      `json:"name" yaml:"name"`
	URL        string      `json:"url" yaml:"url"`
	Hostname   string      `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	ToolPrefix string      `json:"toolPrefix,omitempty" yaml:"toolPrefix,omitempty"`
	Auth       *AuthConfig `json:"auth,omitempty" yaml:"auth,omitempty"`
	Enabled    bool        `json:"enabled" yaml:"enabled"`
}

type AuthConfig struct {
	Type     string `json:"type" yaml:"type"`
	Token    string `json:"token,omitempty" yaml:"token,omitempty"`
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
}
