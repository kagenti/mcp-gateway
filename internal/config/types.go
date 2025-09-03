// Package config provides configuration types
package config

import "context"

// MCPServersConfig holds server configuration
type MCPServersConfig struct {
	Servers   []*MCPServer
	observers []ConfigObserver
}

func (config *MCPServersConfig) RegisterObserver(obs ConfigObserver) {
	config.observers = append(config.observers, obs)
}

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

type ConfigObserver interface {
	OnConfigChange(ctx context.Context, config *MCPServersConfig)
}
