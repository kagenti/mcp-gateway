// Package mcp provides utilities for creating and managing MCP (Model Context Protocol) client connections.
package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// InitializeMCPClient performs the standard MCP initialization
func InitializeMCPClient(ctx context.Context, mcpClient *client.Client) (*mcp.InitializeResult, error) {
	return mcpClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "kagenti-mcp-broker",
				Version: "0.0.1",
			},
		},
	})
}

// CreateClient creates and initializes a new MCP client
func CreateClient(ctx context.Context, host string, options ...transport.StreamableHTTPCOption) (*client.Client, *mcp.InitializeResult, error) {
	mcpClient, err := client.NewStreamableHttpClient(host, options...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create streamable client: %w", err)
	}

	initResult, err := InitializeMCPClient(ctx, mcpClient)
	if err != nil {
		if closeErr := mcpClient.Close(); closeErr != nil {
			return nil, nil, fmt.Errorf("failed to initialize MCP client: %w (close error: %v)", err, closeErr)
		}
		return nil, nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return mcpClient, initResult, nil
}

// ListTools retrieves the list of available tools from an MCP server
func ListTools(ctx context.Context, mcpClient *client.Client) (*mcp.ListToolsResult, error) {
	return mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
}
