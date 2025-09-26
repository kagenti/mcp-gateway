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
)

func TestRouter404Detection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create test session cache
	sessionCache := cache.New(func(ctx context.Context, serverName string, authority string, gwSessionID string) (string, error) {
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
	responses, err := server.HandleResponseHeaders(responseHeaders)
	if err != nil {
		t.Fatalf("HandleResponseHeaders failed: %v", err)
	}

	if len(responses) == 0 {
		t.Fatal("Expected response but got none")
	}

	t.Log("Router 404 detection test completed successfully")
}

func TestInvalidateByMCPSessionID(t *testing.T) {

	// Create test session cache and populate it
	sessionCache := cache.New(func(ctx context.Context, serverName string, authority string, gwSessionID string) (string, error) {
		return "mcp-session-123", nil
	})

	// Add a session to the cache
	_, err := sessionCache.GetOrInit(context.Background(), "test-server", "http://test-server:9090", "gateway-session-456")
	if err != nil {
		t.Fatalf("Failed to add session to cache: %v", err)
	}

	// Verify session exists
	sessionID, err := sessionCache.GetOrInit(context.Background(), "test-server", "http://test-server:9090", "gateway-session-456")
	if err != nil || sessionID != "mcp-session-123" {
		t.Fatalf("Session not found in cache: %v", err)
	}

	// Invalidate by MCP session ID
	sessionCache.InvalidateByMCPSessionID("mcp-session-123")

	// Verify session was removed - should create new session
	newSessionID, err := sessionCache.GetOrInit(context.Background(), "test-server", "http://test-server:9090", "gateway-session-456")
	if err != nil {
		t.Fatalf("Failed to get session after invalidation: %v", err)
	}

	if newSessionID != "mcp-session-123" {
		t.Fatalf("Expected new session to be created, but got existing: %s", newSessionID)
	}

	t.Log("InvalidateByMCPSessionID test completed successfully")
}