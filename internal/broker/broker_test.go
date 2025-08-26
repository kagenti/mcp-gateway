package broker

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/kagenti/mcp-gateway/internal/tests/server2"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

const (
	MCPPort = "8088"
	MCPAddr = "http://localhost:8088/mcp"
)

// TestMain starts an MCP server that we will run actual tests against
func TestMain(m *testing.M) {
	// Start an MCP server to test our broker client logic
	startFunc, shutdownFunc, err := server2.RunServer("http", MCPPort)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Server setup error: %v\n", err)
		os.Exit(1)
	}

	go func() {
		// Start the server in a Goroutine
		_ = startFunc()
	}()

	code := m.Run()

	err = shutdownFunc()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Server shutdown error: %v\n", err)
		os.Exit(1)
	}

	os.Exit(code)
}

func TestRegisterServer(t *testing.T) {
	fmt.Fprintf(os.Stderr, "TestRegisterServer\n")

	broker := NewBroker()

	err := broker.RegisterServer(
		context.Background(),
		MCPAddr,
		"testprefix-reg",
		"mcp_add_service_cluster",
	)
	require.NoError(t, err)
}

func TestUnregisterServer(t *testing.T) {
	fmt.Fprintf(os.Stderr, "TestUnregisterServer\n")

	broker := NewBroker()
	err := broker.RegisterServer(
		context.Background(),
		MCPAddr,
		"testprefix-unreg",
		"mcp_add_service_cluster",
	)
	// err := broker.RegisterServer(context.Background(), "http://mcp-add:8000/mcp", "mcp_add_service_cluster")
	require.NoError(t, err)

	// It is an error to attempt to unregister an unknown server
	err = broker.UnregisterServer(context.Background(), "http://mcp-time:8000/mcp")
	require.Error(t, err)

	// We should be able to unregister a known server
	err = broker.UnregisterServer(context.Background(), MCPAddr)
	require.NoError(t, err)

	// After the first unregister, the server should be unknown, and removing it again should fail
	err = broker.UnregisterServer(context.Background(), MCPAddr)
	require.Error(t, err)
}

func TestToolCall(t *testing.T) {
	fmt.Fprintf(os.Stderr, "TestToolCall\n")

	broker := NewBroker()
	err := broker.RegisterServer(
		context.Background(),
		MCPAddr,
		"testprefix-call",
		"mcp_add_service_cluster",
	)
	require.NoError(t, err)

	res, err := broker.CallTool(context.Background(), "test-session-id", mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "testprefix-call-hello_world", // Note that this is the gateway tool name, not the upstream tool name
			Arguments: map[string]any{
				"name": "Fred",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, res.Content, 1)
	require.IsType(t, mcp.TextContent{}, res.Content[0])
	require.Equal(t, "Hello, Fred!", res.Content[0].(mcp.TextContent).Text)

	err = broker.Close(context.Background(), "test-session-id")
	require.NoError(t, err)
}
