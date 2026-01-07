package upstream

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// MockMCP implements the MCP interface for testing
type MockMCP struct {
	name            string
	prefix          string
	id              config.UpstreamMCPID
	cfg             *config.MCPServer
	connectErr      error
	pingErr         error
	tools           []mcp.Tool
	listToolsErr    error
	protocolVersion string
	hasToolsCap     bool
	connected       bool
}

func (m *MockMCP) GetName() string {
	return m.name
}

func (m *MockMCP) GetConfig() config.MCPServer {
	return *m.cfg
}

func (m *MockMCP) ID() config.UpstreamMCPID {
	return m.id
}

func (m *MockMCP) GetPrefix() string {
	return m.prefix
}

func (m *MockMCP) Connect(_ context.Context, onConnected func()) error {
	if m.connectErr != nil {
		return m.connectErr
	}
	m.connected = true
	if onConnected != nil {
		onConnected()
	}
	return nil
}

func (m *MockMCP) SupportsToolsListChanged() bool {
	return m.hasToolsCap
}

func (m *MockMCP) Disconnect() error {
	m.connected = false
	return nil
}

func (m *MockMCP) ListTools(_ context.Context, _ mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	if m.listToolsErr != nil {
		return nil, m.listToolsErr
	}
	return &mcp.ListToolsResult{Tools: m.tools}, nil
}

func (m *MockMCP) OnNotification(_ func(notification mcp.JSONRPCNotification)) {}

func (m *MockMCP) OnConnectionLost(_ func(err error)) {}

func (m *MockMCP) Ping(_ context.Context) error {
	return m.pingErr
}

func (m *MockMCP) ProtocolInfo() *mcp.InitializeResult {
	result := &mcp.InitializeResult{
		ProtocolVersion: m.protocolVersion,
		Capabilities:    mcp.ServerCapabilities{},
	}
	if m.hasToolsCap {
		result.Capabilities.Tools = &struct {
			ListChanged bool `json:"listChanged,omitempty"`
		}{}
	}
	return result
}

// newMockMCP creates a MockMCP with sensible defaults for testing
func newMockMCP(name, prefix string) *MockMCP {
	id := config.UpstreamMCPID(fmt.Sprintf("%s:%s:http://mock/mcp", name, prefix))
	return &MockMCP{
		name:            name,
		prefix:          prefix,
		id:              id,
		cfg:             &config.MCPServer{Name: name, ToolPrefix: prefix, URL: "http://mock/mcp"},
		protocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		hasToolsCap:     true,
		tools:           []mcp.Tool{{Name: "mock_tool"}},
	}
}

func TestDiffTools(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mock := newMockMCP("test-server", "test_")
	manager := NewUpstreamMCPManager(mock, nil, logger, 0)

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
			removedNames:    []string{"test_tool2"},
		},
		{
			name:            "add and remove tools",
			oldTools:        []mcp.Tool{{Name: "tool1"}, {Name: "tool2"}},
			newTools:        []mcp.Tool{{Name: "tool1"}, {Name: "tool3"}},
			expectedAdded:   1,
			expectedRemoved: 1,
			addedNames:      []string{"test_tool3"},
			removedNames:    []string{"test_tool2"},
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
