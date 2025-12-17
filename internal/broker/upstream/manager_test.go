package upstream

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/kagenti/mcp-gateway/internal/tests/server2"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
)

const (
	// MCPPort is the port the test server should listen on
	MCPPort = "8089"
	// MCPAddr is the URL the client will use to contact the test server
	MCPAddr = "http://localhost:8089/mcp"
)

// TestMain starts an MCP server that we will run actual tests against
func TestMain(m *testing.M) {
	startFunc, shutdownFunc, err := server2.RunServer("http", MCPPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Server setup error: %v\n", err)
		os.Exit(1)
	}

	go func() {
		_ = startFunc()
	}()

	// wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	code := m.Run()

	err = shutdownFunc()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Server shutdown error: %v\n", err)
	}

	os.Exit(code)
}

func TestDiffTools(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	upstream := &MCPServer{
		MCPServer: &config.MCPServer{
			Name:       "test-server",
			ToolPrefix: "test_",
		},
	}
	manager := NewUpstreamMCPManager(upstream, nil, nil, logger, 0)

	tests := []struct {
		name            string
		oldTools        []mcp.Tool
		newTools        []mcp.Tool
		expectedAdded   int
		expectedRemoved int
		addedNames      []string
		removedNames    []string
	}{
		{
			name:            "no changes",
			oldTools:        []mcp.Tool{{Name: "tool1"}, {Name: "tool2"}},
			newTools:        []mcp.Tool{{Name: "tool1"}, {Name: "tool2"}},
			expectedAdded:   0,
			expectedRemoved: 0,
		},
		{
			name:            "add new tool",
			oldTools:        []mcp.Tool{{Name: "tool1"}},
			newTools:        []mcp.Tool{{Name: "tool1"}, {Name: "tool2"}},
			expectedAdded:   1,
			expectedRemoved: 0,
			addedNames:      []string{"test_tool2"},
		},
		{
			name:            "remove tool",
			oldTools:        []mcp.Tool{{Name: "tool1"}, {Name: "tool2"}},
			newTools:        []mcp.Tool{{Name: "tool1"}},
			expectedAdded:   0,
			expectedRemoved: 1,
			removedNames:    []string{"tool2"},
		},
		{
			name:            "add and remove tools",
			oldTools:        []mcp.Tool{{Name: "tool1"}, {Name: "tool2"}},
			newTools:        []mcp.Tool{{Name: "tool1"}, {Name: "tool3"}},
			expectedAdded:   1,
			expectedRemoved: 1,
			addedNames:      []string{"test_tool3"},
			removedNames:    []string{"tool2"},
		},
		{
			name:            "empty old tools",
			oldTools:        []mcp.Tool{},
			newTools:        []mcp.Tool{{Name: "tool1"}, {Name: "tool2"}},
			expectedAdded:   2,
			expectedRemoved: 0,
		},
		{
			name:            "empty new tools",
			oldTools:        []mcp.Tool{{Name: "tool1"}, {Name: "tool2"}},
			newTools:        []mcp.Tool{},
			expectedAdded:   0,
			expectedRemoved: 2,
		},
		{
			name:            "both empty",
			oldTools:        []mcp.Tool{},
			newTools:        []mcp.Tool{},
			expectedAdded:   0,
			expectedRemoved: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, removed := manager.diffTools(tt.oldTools, tt.newTools)
			assert.Len(t, added, tt.expectedAdded, "unexpected number of added tools")
			assert.Len(t, removed, tt.expectedRemoved, "unexpected number of removed tools")

			if len(tt.addedNames) > 0 {
				addedToolNames := make([]string, len(added))
				for i, tool := range added {
					addedToolNames[i] = tool.Tool.Name
				}
				for _, expectedName := range tt.addedNames {
					assert.Contains(t, addedToolNames, expectedName)
				}
			}

			if len(tt.removedNames) > 0 {
				for _, expectedName := range tt.removedNames {
					assert.Contains(t, removed, expectedName)
				}
			}
		})
	}
}

// TestValidate is an integration test that uses a small test server defined in test main to test the validation logic
func TestValidate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx := context.Background()

	t.Run("validates protocol version correctly with valid version", func(t *testing.T) {
		upstream := NewUpstreamMCP(&config.MCPServer{
			Name:       "test-server",
			ToolPrefix: "test_",
			URL:        MCPAddr,
		})
		manager := NewUpstreamMCPManager(upstream, nil, nil, logger, 0)

		status := manager.Validate(ctx)

		assert.Equal(t, "test-server:test_:"+MCPAddr, status.ID)
		assert.Equal(t, "test-server", status.Name)
		assert.Equal(t, "test_", status.ToolPrefix)
		assert.True(t, status.ConnectionStatus.IsReachable, "server should be reachable")
		assert.True(t, status.ProtocolValidation.IsValid, "protocol should be valid for latest version")
		assert.Contains(t, status.ProtocolValidation.ExpectedVersion, mcp.LATEST_PROTOCOL_VERSION)

		// cleanup
		_ = upstream.Disconnect()
	})

	t.Run("validates capabilities with tools capability", func(t *testing.T) {
		upstream := NewUpstreamMCP(&config.MCPServer{
			Name:       "test-server",
			ToolPrefix: "test_",
			URL:        MCPAddr,
		})
		manager := NewUpstreamMCPManager(upstream, nil, nil, logger, 0)

		status := manager.Validate(ctx)

		assert.True(t, status.ConnectionStatus.IsReachable, "server should be reachable")
		assert.True(t, status.CapabilitiesValidation.HasToolCapabilities)
		assert.True(t, status.CapabilitiesValidation.IsValid)

		// cleanup
		_ = upstream.Disconnect()
	})

	t.Run("sets expected version from valid protocol versions", func(t *testing.T) {
		upstream := NewUpstreamMCP(&config.MCPServer{
			Name:       "test-server",
			ToolPrefix: "test_",
			URL:        MCPAddr,
		})
		manager := NewUpstreamMCPManager(upstream, nil, nil, logger, 0)

		status := manager.Validate(ctx)

		expectedVersions := strings.Join(mcp.ValidProtocolVersions, ",")
		assert.Equal(t, expectedVersions, status.ProtocolValidation.ExpectedVersion)

		// cleanup
		_ = upstream.Disconnect()
	})

	t.Run("reports tool count from server tools", func(t *testing.T) {
		upstream := NewUpstreamMCP(&config.MCPServer{
			Name:       "test-server",
			ToolPrefix: "test_",
			URL:        MCPAddr,
		})
		manager := NewUpstreamMCPManager(upstream, nil, nil, logger, 0)
		// simulate having discovered tools
		manager.serverTools = []server.ServerTool{
			{Tool: mcp.Tool{Name: "tool1"}},
			{Tool: mcp.Tool{Name: "tool2"}},
			{Tool: mcp.Tool{Name: "tool3"}},
		}

		status := manager.Validate(ctx)

		assert.True(t, status.ConnectionStatus.IsReachable, "server should be reachable")
		assert.Equal(t, 3, status.CapabilitiesValidation.ToolCount)

		// cleanup
		_ = upstream.Disconnect()
	})

	t.Run("connection error when server unreachable", func(t *testing.T) {
		upstream := NewUpstreamMCP(&config.MCPServer{
			Name:       "unreachable-server",
			ToolPrefix: "test_",
			URL:        "http://localhost:1/mcp", // invalid port
		})
		manager := NewUpstreamMCPManager(upstream, nil, nil, logger, 0)

		status := manager.Validate(ctx)

		assert.False(t, status.ConnectionStatus.IsReachable, "unreachable server should not be reachable")
		assert.NotEmpty(t, status.ConnectionStatus.Error)
	})
}
