package upstream

import (
	"context"
	"fmt"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPServer represents a connection to an upstream MCP server. It wraps the
// configuration and client, managing the connection lifecycle and storing
// initialization state from the MCP handshake.
type MCPServer struct {
	*config.MCPServer
	*client.Client
	headers map[string]string
	init    *mcp.InitializeResult
}

// NewUpstreamMCP creates a new MCPServer instance from the provided configuration.
// It sets up default headers including user-agent and gateway-server-id, and adds
// an Authorization header if credentials are configured.
func NewUpstreamMCP(config *config.MCPServer) *MCPServer {
	up := &MCPServer{
		MCPServer: config,
	}
	up.headers = map[string]string{
		"user-agent":        "mcp-broker",
		"gateway-server-id": string(up.ID()),
	}
	if up.Credential != "" {
		up.headers["Authorization"] = up.Credential
	}
	return up
}

// Connect establishes a connection to the upstream MCP server. It creates a
// streamable HTTP client, starts it for continuous listening, and performs
// the MCP initialization handshake. If already connected, this is a no-op.
// The initialization result is stored for later validation of protocol version
// and capabilities.
func (up *MCPServer) Connect(ctx context.Context) error {
	if up.Client != nil {
		//if we already have a valid connection nothing to do
		return nil
	}
	options := []transport.StreamableHTTPCOption{
		transport.WithContinuousListening(),
		transport.WithHTTPHeaders(up.headers),
	}

	httpClient, err := client.NewStreamableHttpClient(up.URL, options...)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	// Start the client before initialize to listen for notifications
	err = httpClient.Start(ctx)
	if err != nil {
		return fmt.Errorf("failed to start streamable client: %w", err)
	}
	initResp, err := httpClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities: mcp.ClientCapabilities{
				Roots: &struct {
					ListChanged bool `json:"listChanged,omitempty"`
				}{
					ListChanged: true,
				},
			},
			ClientInfo: mcp.Implementation{
				Name:    "mcp-broker",
				Version: "0.0.1",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to initialize client for upstream %s : %w", up.ID(), err)
	}
	// whenever we do an init store the response and session id for validation a future use
	up.init = initResp
	up.Client = httpClient
	return nil
}

// Disconnect closes the connection to the upstream MCP server. If no client
// connection exists, this is a no-op and returns nil.
func (up *MCPServer) Disconnect() error {
	if up.Client != nil {
		return up.Close()
	}
	return nil
}
