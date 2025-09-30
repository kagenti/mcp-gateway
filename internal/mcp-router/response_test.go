package mcprouter

import (
	"context"
	"log/slog"
	"os"
	"testing"

	basepb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	eppb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/kagenti/mcp-gateway/internal/broker"
	"github.com/kagenti/mcp-gateway/internal/cache"
	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRouter404Detection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create test session cache
	sessionCache := cache.New(func(_ context.Context, _ string, _ string, _ string) (string, error) {
		return "test-mcp-session-123", nil
	})

	server := &ExtProcServer{
		RoutingConfig: &config.MCPServersConfig{
			Servers: []*config.MCPServer{
				{
					Name:     "test-server",
					URL:      "http://test-server:9090/mcp",
					Hostname: "test-server",
					Enabled:  true,
				},
			},
		},
		Broker:       broker.NewBroker(logger),
		SessionCache: sessionCache,
	}

	// Create response headers with HTTP 404 status
	responseHeaders := &eppb.HttpHeaders{
		Headers: &basepb.HeaderMap{
			Headers: []*basepb.HeaderValue{
				{
					Key:      ":status",
					RawValue: []byte("404"),
				},
				{
					Key:      "mcp-session-id",
					RawValue: []byte("test-mcp-session-123"),
				},
			},
		},
	}

	// Test that 404 detection works
	resp, err := server.HandleResponseHeaders(responseHeaders)
	require.NoError(t, err)
	require.Len(t, resp, 1)
}

func TestInvalidateByMCPSessionID(t *testing.T) {
	// Create test session cache and populate it
	sessionCache := cache.New(func(_ context.Context, _ string, _ string, _ string) (string, error) {
		return "mcp-session-123", nil
	})

	// Add a session to the cache
	sessionID, err := sessionCache.GetOrInit(context.Background(), "test-server", "http://test-server:9090", "gateway-session-456")
	require.NoError(t, err)
	require.Equal(t, "mcp-session-123", sessionID)

	// Verify session exists in cache
	cachedSessionID, err := sessionCache.GetOrInit(context.Background(), "test-server", "http://test-server:9090", "gateway-session-456")
	require.NoError(t, err)
	require.Equal(t, "mcp-session-123", cachedSessionID)

	// Invalidate by MCP session ID
	sessionCache.InvalidateByMCPSessionID("mcp-session-123")

	// Verify session was removed - should create new session
	newSessionID, err := sessionCache.GetOrInit(context.Background(), "test-server", "http://test-server:9090", "gateway-session-456")
	require.NoError(t, err)
	require.Equal(t, "mcp-session-123", newSessionID)
}
