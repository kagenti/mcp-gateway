package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/kagenti/mcp-gateway/internal/tests/server2"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

const (
	// MCPPort is the port the test server should listen on (TODO make dynamic?)
	MCPPort = "8088"

	// MCPAddr is the URL the client will use to contact the test server
	MCPAddr = "http://localhost:8088/mcp"

	// MCPAddrForgetAddr is the URL the client will use to force the server to forget a session
	MCPAddrForgetAddr = "http://localhost:8088/admin/forget"
)

var logger = slog.New(slog.NewTextHandler(os.Stdout, nil))

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
		// Don't fail if the server doesn't shut down; it might have open clients
		// os.Exit(1)
	}

	os.Exit(code)
}

func TestOnConfigChange(t *testing.T) {
	b := NewBroker(logger)
	conf := &config.MCPServersConfig{}
	server1 := &config.MCPServer{
		Name:       "test1",
		URL:        MCPAddr,
		ToolPrefix: "_test1",
	}
	b.OnConfigChange(context.TODO(), conf)
	if b.IsRegistered(server1.URL) {
		t.Fatalf("server1 should not be registered but is")
	}

	conf.Servers = append(conf.Servers, server1)
	b.OnConfigChange(context.TODO(), conf)
	if !b.IsRegistered(server1.URL) {
		t.Fatalf("server1 should be registered but is not")
	}

	conf.Servers = []*config.MCPServer{}
	b.OnConfigChange(context.TODO(), conf)
	if b.IsRegistered(server1.URL) {
		t.Fatalf("server1 should not be registered but is")
	}

	_ = b.Shutdown(context.Background())
}

func TestRegisterServer(t *testing.T) {
	fmt.Fprintf(os.Stderr, "TestRegisterServer\n")

	broker := NewBroker(logger)

	err := broker.RegisterServer(
		context.Background(),
		MCPAddr,
		"testprefix-reg",
		"mcp_add_service_cluster",
	)
	require.NoError(t, err)

	_ = broker.Shutdown(context.Background())
}

func TestUnregisterServer(t *testing.T) {
	fmt.Fprintf(os.Stderr, "TestUnregisterServer\n")

	broker := NewBroker(logger)
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

	_ = broker.Shutdown(context.Background())
}

func TestToolCall(t *testing.T) {
	fmt.Fprintf(os.Stderr, "TestToolCall\n")

	broker := NewBroker(logger)
	err := broker.RegisterServer(
		context.Background(),
		MCPAddr,
		"testprefix-call",
		"mcp_add_service_cluster",
	)
	require.NoError(t, err)

	res, err := broker.CallTool(context.Background(), "test-session-id", mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "testprefix-callhello_world", // Note that this is the gateway tool name, not the upstream tool name
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

	_ = broker.Shutdown(context.Background())
}

// TestToolCallAfterMCPDisconnect tests the case where the server disconnects the session.
// **Currently this test does not test broker function, as the broker doesn't do long-running connections.*
// This does test the ability of the test server to handle the /admin/forget API.
func TestToolCallAfterMCPDisconnect(t *testing.T) {
	fmt.Fprintf(os.Stderr, "TestToolCall\n")

	broker := NewBroker(logger)
	err := broker.RegisterServer(
		context.Background(),
		MCPAddr,
		"testprefix-call",
		"mcp_add_service_cluster",
	)
	require.NoError(t, err)

	res, err := broker.CallTool(context.Background(), "test-session-id", mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "testprefix-callhello_world", // Note that this is the gateway tool name, not the upstream tool name
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

	// Get the real upstream session ID from the downstream "test-session-id" session ID
	require.IsType(t, &mcpBrokerImpl{}, broker)
	brokerImpl := broker.(*mcpBrokerImpl)
	upstreamSessionMap, ok := brokerImpl.serverSessions[MCPAddr]
	require.True(t, ok)
	upstreamSessionState, ok := upstreamSessionMap["test-session-id"]
	require.True(t, ok)

	// Tell the server to forget our broker's session ID
	client := &http.Client{}
	req, err := http.NewRequest("POST", MCPAddrForgetAddr,
		strings.NewReader(string(upstreamSessionState.sessionID)))
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Make the same call
	res, err = broker.CallTool(context.Background(), "test-session-id", mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "testprefix-callhello_world", // Note that this is the gateway tool name, not the upstream tool name
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

	_ = broker.Shutdown(context.Background())
}

var _ http.ResponseWriter = &simpleResponseWriter{}

type simpleResponseWriter struct {
	Status  int
	Body    []byte
	Headers []http.Header
}

func (srw *simpleResponseWriter) Header() http.Header {
	h := http.Header{}
	srw.Headers = append(srw.Headers, h)
	return h
}

func (srw *simpleResponseWriter) WriteHeader(status int) {
	srw.Status = status
}
func (srw *simpleResponseWriter) Write(b []byte) (int, error) {
	srw.Body = b
	return len(b), nil
}

func TestOauthResourceHandler(t *testing.T) {
	var (
		resourceName = "mcp gateway"
		resource     = "https://test.com/mcp"
		idp          = "https://idp.com"
		bearerMethod = "header"
		scopes       = "groups,audience,roles"
	)
	t.Setenv(envOAuthResourceName, resourceName)
	t.Setenv(envOAuthResource, resource)
	t.Setenv(envOAuthAuthorizationServers, idp)
	t.Setenv(envOAuthBearerMethodsSupported, bearerMethod)
	t.Setenv(envOAuthScopesSupported, scopes)

	r := &http.Request{
		Method: http.MethodGet,
	}
	pr := &ProtectedResourceHandler{Logger: logger}
	recorder := &simpleResponseWriter{}
	pr.Handle(recorder, r)
	if recorder.Status != 200 {
		t.Fatalf("expected 200 status code got %v", recorder.Status)
	}
	config := &OAuthProtectedResource{}
	if err := json.Unmarshal(recorder.Body, config); err != nil {
		t.Fatalf("unexpected error %s", err)
	}
	if !slices.Contains(config.AuthorizationServers, idp) {
		t.Fatalf("expected %s to be in %v", idp, config.AuthorizationServers)
	}
	if config.Resource != resource {
		t.Fatalf("expected resource to be %s but was %s", resource, config.Resource)
	}
	if config.ResourceName != resourceName {
		t.Fatalf("expected resource to be %s but was %s", resourceName, config.ResourceName)
	}
	if !slices.ContainsFunc(config.ScopesSupported, func(val string) bool {
		return slices.Contains(strings.Split(scopes, ","), val)
	}) {
		t.Fatalf("expected %s to be in %v", scopes, config.ScopesSupported)
	}
	if !slices.Contains(config.BearerMethodsSupported, bearerMethod) {
		t.Fatalf("expected %s to be in %v", bearerMethod, config.BearerMethodsSupported)
	}

}
