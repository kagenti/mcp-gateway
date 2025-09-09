// Package config provides configuration types
package config

import "context"

// MCPServersConfig holds server configuration
type MCPServersConfig struct {
	Servers   []*MCPServer
	observers []Observer
}

// RegisterObserver registers an observer to be notified of changes to the config
func (config *MCPServersConfig) RegisterObserver(obs Observer) {
	config.observers = append(config.observers, obs)
}

// Notify notifies registered observers of config changes
func (config *MCPServersConfig) Notify(ctx context.Context) {
	for _, observer := range config.observers {
		observer.OnConfigChange(ctx, config)
	}
}

// MCPServer represents a server
type MCPServer struct {
	Name       string
	URL        string
	ToolPrefix string
	Enabled    bool
	Hostname   string
}

// Observer provides an interface to implement in order to register as an Observer of config changes
type Observer interface {
	OnConfigChange(ctx context.Context, config *MCPServersConfig)
}
