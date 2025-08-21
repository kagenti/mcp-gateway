package broker

import (
	"context"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestRegisterServer(t *testing.T) {
	// TODO introduce mocking, or stand up test servers in a test setup
	if os.Getenv("RUN_TESTS") != "TRUE" {
		t.Skip("Skipping testing")
	}

	broker := NewBroker()

	// err := broker.RegisterServer(context.Background(), "http://mcp-time:9090/mcp", "mcp_add_service_cluster")
	err := broker.RegisterServer(
		context.Background(),
		"http://localhost:9090/mcp",
		"testprefix-reg",
		"mcp_add_service_cluster",
	)
	require.NoError(t, err)

	// err = broker.RegisterServer(context.Background(), "http://mcp-time:8000/mcp", "mcp_time_service_cluster")
	// require.NoError(t, err)
}

func TestUnregisterServer(t *testing.T) {
	// TODO introduce mocking, or stand up test servers in a test setup
	if os.Getenv("RUN_TESTS") != "TRUE" {
		t.Skip("Skipping testing")
	}

	broker := NewBroker()
	err := broker.RegisterServer(
		context.Background(),
		"http://localhost:9090/mcp",
		"testprefix-unreg",
		"mcp_add_service_cluster",
	)
	// err := broker.RegisterServer(context.Background(), "http://mcp-add:8000/mcp", "mcp_add_service_cluster")
	require.NoError(t, err)

	// It is an error to attempt to unregister an unknown server
	err = broker.UnregisterServer(context.Background(), "http://mcp-time:8000/mcp")
	require.Error(t, err)

	// We should be able to unregister a known server
	err = broker.UnregisterServer(context.Background(), "http://localhost:9090/mcp")
	// err = broker.UnregisterServer(context.Background(), "http://mcp-add:8000/mcp")
	require.NoError(t, err)

	// After the first unregister, the server should be unknown, and removing it again should fail
	err = broker.UnregisterServer(context.Background(), "http://localhost:9090/mcp")
	// err = broker.UnregisterServer(context.Background(), "http://mcp-add:8000/mcp")
	require.Error(t, err)
}

func TestToolCall(t *testing.T) {
	// TODO introduce mocking, or stand up test servers in a test setup
	if os.Getenv("RUN_TESTS") != "TRUE" {
		t.Skip("Skipping testing")
	}

	broker := NewBroker()
	err := broker.RegisterServer(
		context.Background(),
		"http://localhost:9090/mcp",
		"testprefix-call",
		"mcp_add_service_cluster",
	)
	require.NoError(t, err)

	res, err := broker.CallTool(context.Background(), "test-session-id", mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "testprefix-call-greet", // Note that this is the gateway tool name, not the upstream tool name
			Arguments: map[string]any{
				"Name": "Fred",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, res.Content, 1)
	require.IsType(t, mcp.TextContent{}, res.Content[0])
	require.Equal(t, "Hi Fred", res.Content[0].(mcp.TextContent).Text)

	err = broker.Close(context.Background(), "test-session-id")
	require.NoError(t, err)
}
