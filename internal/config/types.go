// Package config provides configuration types
package config

// MCPServersConfig holds server configuration
type MCPServersConfig struct {
	Servers []*MCPServer
}

// MCPServer represents a server
type MCPServer struct {
	Name       string
	URL        string
	ToolPrefix string
	Enabled    bool
	Hostname   string
}
