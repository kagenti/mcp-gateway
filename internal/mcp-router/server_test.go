// Package mcprouter ext proc process
package mcprouter

import (
	"log/slog"
	"os"
	"testing"

	"github.com/kagenti/mcp-gateway/internal/broker"
	"github.com/kagenti/mcp-gateway/internal/config"
)

func TestSetupSessionCache(_ *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	server := &ExtProcServer{
		RoutingConfig: &config.MCPServersConfig{
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
		Broker: broker.NewBroker(logger, broker.Opts{}),
	}

	// We can't test the internals of this, because it returns nothing on error...
	server.SetupSessionCache()
}
