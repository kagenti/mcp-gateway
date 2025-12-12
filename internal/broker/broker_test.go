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
	"time"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/kagenti/mcp-gateway/internal/tests/server2"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

// See https://stackoverflow.com/questions/28817992/how-to-set-bool-pointer-to-true-in-struct-literal
func pointer[T any](d T) *T {
	return &d
}

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

	// wait for server to be ready
	time.Sleep(100 * time.Millisecond)

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
	if b.IsRegistered(server1.ID()) {
		t.Fatalf("server1 should not be registered but is")
	}

	conf.Servers = append(conf.Servers, server1)
	b.OnConfigChange(context.TODO(), conf)
	if !b.IsRegistered(server1.ID()) {
		t.Fatalf("server1 should be registered but is not")
	}

	conf.Servers = []*config.MCPServer{}
	b.OnConfigChange(context.TODO(), conf)
	if b.IsRegistered(server1.ID()) {
		t.Fatalf("server1 should not be registered but is")
	}

	_ = b.Shutdown(context.Background())
}

func TestRegisterServer(t *testing.T) {
	fmt.Fprintf(os.Stderr, "TestRegisterServer\n")

	broker := NewBroker(logger)
	brokerImpl := broker.(*mcpBrokerImpl)

	tools, err := brokerImpl.RegisterServerWithConfig(
		context.Background(),
		&config.MCPServer{
			Name:       "test-server",
			URL:        MCPAddr,
			ToolPrefix: "testprefix-reg",
			Hostname:   "mcp_add_service_cluster",
			Enabled:    true,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, brokerImpl.listeningMCPServer)
	// RegisterServerWithConfig returns tools but doesn't add them to listeningMCPServer
	// (that's done by OnConfigChange), so we add them here to test the tool content
	brokerImpl.listeningMCPServer.AddTools(toolsToServerTools(tools)...)

	expectedTools := map[string]*server.ServerTool{
		"testprefix-regheaders": {
			Tool: mcp.Tool{
				Description: "get HTTP headers",
				Annotations: mcp.ToolAnnotation{
					Title:           "header inspector",
					ReadOnlyHint:    pointer(true),
					DestructiveHint: pointer(false),
					IdempotentHint:  pointer(true),
					OpenWorldHint:   pointer(false),
				},
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}(nil),
				},
			},
		},
		"testprefix-regtime": {
			Tool: mcp.Tool{
				Description: "Get the current time",
				Annotations: mcp.ToolAnnotation{
					Title:           "Clock",
					ReadOnlyHint:    pointer(true),
					DestructiveHint: pointer(false),
					IdempotentHint:  pointer(true),
					OpenWorldHint:   pointer(false),
				},
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}(nil),
				},
			},
		},
		"testprefix-reghello_world": {
			Tool: mcp.Tool{
				Description: "Say hello to someone",
				Annotations: mcp.ToolAnnotation{
					Title:           "greeter tool",
					ReadOnlyHint:    pointer(true),
					DestructiveHint: pointer(false),
					IdempotentHint:  pointer(true),
					OpenWorldHint:   pointer(false),
				},
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Name of the person to greet",
						},
					},
					Required: []string{"name"},
				},
			},
		},
		"testprefix-regpour_chocolate_into_mold": {
			Tool: mcp.Tool{
				Description: "Pour chocolate into mold",
				Annotations: mcp.ToolAnnotation{
					Title:           "chocolate fill tool",
					ReadOnlyHint:    pointer(false),
					DestructiveHint: pointer(true),
					IdempotentHint:  pointer(false),
					OpenWorldHint:   pointer(true),
				},
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"quantity": map[string]interface{}{
							"type":        "string",
							"description": "milliliters",
						},
					},
					Required: []string{"quantity"},
				},
			},
		},
		"testprefix-regset_time": {
			Tool: mcp.Tool{
				Description: "Set the clock",
				Annotations: mcp.ToolAnnotation{
					Title:           "set time tool",
					ReadOnlyHint:    pointer(false),
					DestructiveHint: pointer(true),
					IdempotentHint:  pointer(true),
					OpenWorldHint:   pointer(false),
				},
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"time": map[string]interface{}{
							"type":        "string",
							"description": "new time",
						},
					},
					Required: []string{"time"},
				},
			},
		},
		"testprefix-regslow": {
			Tool: mcp.Tool{
				Description: "Delay for N seconds",
				Annotations: mcp.ToolAnnotation{
					Title:           "delay tool",
					ReadOnlyHint:    pointer(true),
					DestructiveHint: pointer(false),
					IdempotentHint:  pointer(true),
					OpenWorldHint:   pointer(false),
				},
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"seconds": map[string]interface{}{
							"type":        "string",
							"description": "number of seconds to wait",
						},
					},
					Required: []string{"seconds"},
				},
			},
		},
		"testprefix-regauth1234": {
			Tool: mcp.Tool{
				Description: "check authorization header",
				Annotations: mcp.ToolAnnotation{
					Title:           "auth header verifier",
					ReadOnlyHint:    pointer(true),
					DestructiveHint: pointer(false),
					IdempotentHint:  pointer(true),
					OpenWorldHint:   pointer(false),
				},
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}(nil),
				},
			},
		},
	}

	require.Len(t, brokerImpl.listeningMCPServer.ListTools(), len(expectedTools))
	for name, tool := range brokerImpl.listeningMCPServer.ListTools() {
		expectedTool, ok := expectedTools[name]
		require.True(t, ok, "Found unexpected tool named %q", name)
		require.Equal(t, expectedTool.Tool.Description, tool.Tool.Description, "Description for tool %q", name)
		require.Equal(t, expectedTool.Tool.Annotations, tool.Tool.Annotations, "Annotations for tool %q", name)
		require.Equal(t, expectedTool.Tool.InputSchema.Properties, tool.Tool.InputSchema.Properties, "InputSchema properties for tool %q", name)
		require.Equal(t, expectedTool.Tool.InputSchema, tool.Tool.InputSchema, "InputSchema for tool %q", name)
		require.Equal(t, expectedTool.Tool.Meta, tool.Tool.Meta, "Meta for tool %q", name)
		require.Equal(t, expectedTool.Tool.OutputSchema, tool.Tool.OutputSchema, "OutputSchema for tool %q", name)
	}

	_ = broker.Shutdown(context.Background())
}

func TestUnregisterServer(t *testing.T) {
	fmt.Fprintf(os.Stderr, "TestUnregisterServer\n")

	broker := NewBroker(logger)
	brokerImpl := broker.(*mcpBrokerImpl)
	serverConfig := &config.MCPServer{
		Name:       "test-server-unreg",
		URL:        MCPAddr,
		ToolPrefix: "testprefix-unreg",
		Hostname:   "mcp_add_service_cluster",
		Enabled:    true,
	}
	_, err := brokerImpl.RegisterServerWithConfig(
		context.Background(),
		serverConfig,
	)
	require.NoError(t, err)

	// It is an error to attempt to unregister an unknown server
	err = broker.UnregisterServer(context.Background(), "unknown:unknown:http://mcp-time:8000/mcp")
	require.Error(t, err)

	// We should be able to unregister a known server
	err = broker.UnregisterServer(context.Background(), serverConfig.ID())
	require.NoError(t, err)

	// After the first unregister, the server should be unknown, and removing it again should fail
	err = broker.UnregisterServer(context.Background(), serverConfig.ID())
	require.Error(t, err)

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
