// Package mcprouter ext proc process
package mcprouter

import (

	// "github.com/kagenti/mcp-gateway/internal/config"

	"testing"

	"github.com/kagenti/mcp-gateway/internal/broker"
	"github.com/kagenti/mcp-gateway/internal/config"
)

func TestSetupSessionCache(_ *testing.T) {
	server := &ExtProcServer{
		MCPConfig: &config.MCPServersConfig{
			Servers: []*config.MCPServer{
				{
					Name:       "dummy",
					URL:        "http://localhost:8080/mcp",
					ToolPrefix: "s_",
					Enabled:    true,
					Hostname:   "localhost",
				},
			},
		},
		Broker: broker.NewBroker(),
	}

	// We can't test the internals of this, because it returns nothing on error...
	server.SetupSessionCache()
}
