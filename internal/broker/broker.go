// Package broker tracks upstream MCP servers and manages the relationship from clients to upstream
package broker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var _ config.Observer = &mcpBrokerImpl{}

// downstreamSessionID is for session IDs the gateway uses with its own clients
type downstreamSessionID string

// upstreamSessionID is for session IDs the gateway uses with upstream MCP servers
type upstreamSessionID string

// upstreamMCPURL identifies an upstream MCP server
type upstreamMCPURL string

// upstreamMCP identifies what we know about an upstream MCP server
type upstreamMCP struct {
	config.MCPServer
	initializeResult *mcp.InitializeResult // The init result when we probed at discovery time
	toolsResult      *mcp.ListToolsResult  // The tools when we proved at discovery time
	lastContact      time.Time             // The last time this MCP was contacted
}

// upstreamSessionState tracks what we manage about a connection an upstream MCP server
type upstreamSessionState struct {
	initialized bool
	client      *client.Client
	sessionID   upstreamSessionID
	lastContact time.Time
}

// An MCP tool name
type toolName string

// upstreamToolInfo references a single tool on an upstream MCP server
type upstreamToolInfo struct {
	url      upstreamMCPURL // An MCP server URL
	toolName string         // A tool name
}

// MCPBroker manages a set of MCP servers and their sessions
type MCPBroker interface {
	// Implements the https://github.com/kagenti/mcp-gateway/blob/main/docs/design/flows.md#discovery flow
	RegisterServer(ctx context.Context, mcpURL string, prefix string, name string) error

	// Removes a server
	UnregisterServer(ctx context.Context, mcpURL string) error

	IsRegistered(mcpURL string) bool

	// Call a tool by gatewaying to upstream MCP server.  Note that the upstream connection may be created lazily.
	CallTool(
		ctx context.Context,
		downstreamSession downstreamSessionID,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error)

	// Cleanup any upstream connections being held open on behalf of downstreamSessionID
	Close(ctx context.Context, downstreamSession downstreamSessionID) error

	// MCPServer gets an MCP server that federates the upstreams known to this MCPBroker
	MCPServer() *server.MCPServer

	// CreateSession creates a new MCP session for the given authority/host
	CreateSession(ctx context.Context, authority string) (string, error)

	config.Observer
}

// mcpBrokerImpl implements MCPBroker
type mcpBrokerImpl struct {
	// Static map of session IDs we offer to downstream clients
	// value will be false if uninitialized, true if initialized
	// TODO: evict if not used for a while?
	// knownSessionIDs map[downstreamSessionID]clientStatus

	// serverSessions tracks the sessions we maintain with upstream MCP servers
	serverSessions map[upstreamMCPURL]map[downstreamSessionID]*upstreamSessionState

	// mcpServers tracks the known servers
	mcpServers map[upstreamMCPURL]*upstreamMCP

	// toolMapping tracks the unique gateway'ed tool name to its upstream MCP server implementation
	toolMapping map[toolName]*upstreamToolInfo

	// listeningMCPServer returns an actual listening MCP server that federates registered MCP servers
	listeningMCPServer *server.MCPServer

	logger *slog.Logger
}

// this ensures that mcpBrokerImpl implements the MCPBroker interface
var _ MCPBroker = &mcpBrokerImpl{}

// NewBroker creates a new MCPBroker
func NewBroker(logger *slog.Logger) MCPBroker {
	hooks := &server.Hooks{}

	hooks.AddOnUnregisterSession(func(_ context.Context, session server.ClientSession) {
		slog.Info("Client disconnected", "sessionID", session.SessionID())
	})

	// Enhanced session registration to log gateway session assignment
	hooks.AddOnRegisterSession(func(_ context.Context, session server.ClientSession) {
		// Note that AddOnRegisterSession is for GET, not POST, for a session.
		// https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#listening-for-messages-from-the-server
		slog.Info("Gateway client connected with session", "gatewaySessionID", session.SessionID())
	})

	hooks.AddBeforeAny(func(_ context.Context, _ any, method mcp.MCPMethod, _ any) {
		slog.Info("Processing request", "method", method)
	})

	hooks.AddOnError(func(_ context.Context, _ any, method mcp.MCPMethod, _ any, err error) {
		slog.Info("MCP server error", "method", method, "error", err)
	})

	return &mcpBrokerImpl{
		// knownSessionIDs: map[downstreamSessionID]clientStatus{},
		serverSessions: map[upstreamMCPURL]map[downstreamSessionID]*upstreamSessionState{},
		mcpServers:     map[upstreamMCPURL]*upstreamMCP{},
		toolMapping:    map[toolName]*upstreamToolInfo{},
		listeningMCPServer: server.NewMCPServer(
			"Kagenti MCP Broker",
			"0.0.1",
			server.WithHooks(hooks),
			server.WithToolCapabilities(true),
		),
		logger: logger,
	}
}

func (m *mcpBrokerImpl) IsRegistered(mcpURL string) bool {
	_, ok := m.mcpServers[upstreamMCPURL(mcpURL)]
	return ok
}

func (m *mcpBrokerImpl) OnConfigChange(ctx context.Context, conf *config.MCPServersConfig) {
	m.logger.Debug("Broker OnConfigChange called")
	// unregister decommissioned servers
	for upstreamHost := range m.mcpServers {
		if !slices.ContainsFunc(conf.Servers, func(s *config.MCPServer) bool {
			return upstreamHost == upstreamMCPURL(s.URL)
		}) {
			if err := m.UnregisterServer(ctx, string(upstreamHost)); err != nil {
				m.logger.Warn("unregister failed ", "server", upstreamHost)
			}
		}

	}
	// ensure new servers registered
	for _, server := range conf.Servers {
		if err := m.RegisterServer(ctx, server.URL, server.ToolPrefix, server.Name); err != nil {
			slog.Warn("Could not register upstream MCP", "upstream", server.URL, "name", server.Name, "error", err)
		}
	}
}

// RegisterServer registers an MCP server
func (m *mcpBrokerImpl) RegisterServer(
	ctx context.Context,
	mcpURL string,
	prefix string,
	name string,
) error {
	if m.IsRegistered(mcpURL) {
		m.logger.Info("mcp server is already registered", "mcpURL", mcpURL)
		return nil
	}
	slog.Info("Registering server", "mcpURL", mcpURL, "prefix", prefix)

	upstream := &upstreamMCP{
		MCPServer: config.MCPServer{
			Name:       name,
			URL:        mcpURL,
			ToolPrefix: prefix,
			Enabled:    true,
		},
	}

	newTools, err := m.discoverTools(ctx, upstream)
	if err != nil {
		slog.Info("Failed to discover tools", "mcpURL", mcpURL, "error", err)
		return err
	}
	slog.Info("Discovered tools", "mcpURL", mcpURL, "num tools", len(newTools))

	m.mcpServers[upstreamMCPURL(mcpURL)] = upstream

	tools := make([]server.ServerTool, 0)
	for _, newTool := range newTools {
		slog.Info("Federating tool", "mcpURL", mcpURL, "federated name", newTool.Name)
		tools = append(tools, server.ServerTool{
			Tool: newTool,
			Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return mcp.NewToolResultError("Kagenti MCP Broker doesn't forward tool calls"), nil
			},
			/* UNCOMMENT THIS TO TURN THE BROKER INTO A STAND-ALONE GATEWAY
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				result, err := m.CallTool(ctx,
					downstreamSessionID(request.GetString("Mcp-Session-Id", "")),
					request,
				)
				return result, err
			}
			*/
		})
	}
	m.listeningMCPServer.AddTools(tools...)

	return nil
}

func (m *mcpBrokerImpl) UnregisterServer(_ context.Context, mcpURL string) error {
	_, ok := m.mcpServers[upstreamMCPURL(mcpURL)]
	if !ok {
		return fmt.Errorf("unknown host")
	}

	delete(m.mcpServers, upstreamMCPURL(mcpURL))

	// Find tools registered to this server
	toolsToDelete := make([]string, 0)
	for toolName, upstreamToolInfo := range m.toolMapping {
		if upstreamToolInfo.url == upstreamMCPURL(mcpURL) {
			toolsToDelete = append(toolsToDelete, string(toolName))
		}
	}
	m.listeningMCPServer.DeleteTools(toolsToDelete...)

	// Close any connections to the upstream server
	mapping, ok := m.serverSessions[upstreamMCPURL(mcpURL)]
	if ok {
		for downstreamSessionID, upstreamSessionState := range mapping {
			err := upstreamSessionState.client.Close()
			if err != nil {
				slog.Warn(
					"Could not close upstream session",
					"mcpURL",
					mcpURL,
					"sessionID",
					downstreamSessionID,
				)
			}
		}
	}

	return nil
}

func (m *mcpBrokerImpl) CallTool(
	ctx context.Context,
	downstreamSession downstreamSessionID,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	// First, identify the upstream MCP server
	upstreamToolInfo, ok := m.toolMapping[toolName(request.Params.Name)]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", request.Params.Name)
	}

	upstreamSessionMap, ok := m.serverSessions[upstreamToolInfo.url]
	if !ok {
		upstreamSessionMap = make(map[downstreamSessionID]*upstreamSessionState)
		m.serverSessions[upstreamToolInfo.url] = upstreamSessionMap
	}

	upstreamSession, ok := upstreamSessionMap[downstreamSession]
	if !ok {
		var err error
		upstreamSession, err = m.createUpstreamSession(ctx, upstreamToolInfo.url)
		if err != nil {
			return nil, fmt.Errorf("could not open upstream: %w", err)
		}
		upstreamSessionMap[downstreamSession] = upstreamSession
	}

	request.Params.Name = upstreamToolInfo.toolName
	res, err := upstreamSession.client.CallTool(ctx, request)
	if err != nil {
		upstreamSession.lastContact = time.Now()
	}

	return res, err
}

func (m *mcpBrokerImpl) discoverTools(
	ctx context.Context,
	upstream *upstreamMCP,
	options ...transport.StreamableHTTPCOption,
) ([]mcp.Tool, error) {

	// Some MCP servers require a bearer token or other Authorization to init and list tools
	serverAuthHeaderValue := getAuthorizationHeaderForUpstream(upstream)
	if serverAuthHeaderValue != "" {
		options = append(options, transport.WithHTTPHeaders(map[string]string{
			"Authorization": serverAuthHeaderValue,
		}))
	}

	httpTransportClient, err := client.NewStreamableHttpClient(
		upstream.URL,
		options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create streamable client: %w", err)
	}

	resInit, err := httpTransportClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "kagenti-mcp-broker",
				Version: "0.0.1",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	upstream.initializeResult = resInit

	resTools, err := httpTransportClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}
	upstream.toolsResult = resTools

	upstream.lastContact = time.Now()
	err = httpTransportClient.Close()

	newTools := m.populateToolMapping(upstream)

	// TODO probe resources other than tools

	// Don't keep open the probe
	// TODO: Keep it open and monitor for tool changes?
	return newTools, err
}

// populateToolMapping creates maps tools to names that this gateway recognizes
// and returns a list of the new uniquely prefixed tools
func (m *mcpBrokerImpl) populateToolMapping(upstream *upstreamMCP) []mcp.Tool {
	retval := make([]mcp.Tool, 0)
	for _, tool := range upstream.toolsResult.Tools {
		gatewayToolName := toolName(fmt.Sprintf("%s%s", upstream.ToolPrefix, tool.Name))

		gatewayTool := tool // Note: shallow
		gatewayTool.Name = string(gatewayToolName)
		retval = append(retval, gatewayTool)

		m.toolMapping[gatewayToolName] = &upstreamToolInfo{
			url:      upstreamMCPURL(upstream.URL),
			toolName: tool.Name,
		}
	}
	return retval
}

func (m *mcpBrokerImpl) createUpstreamSession(
	ctx context.Context,
	host upstreamMCPURL,
	options ...transport.StreamableHTTPCOption,
) (*upstreamSessionState, error) {
	retval := &upstreamSessionState{}

	var err error
	retval.client, err = client.NewStreamableHttpClient(string(host), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create streamable client: %w", err)
	}

	_, err = retval.client.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "kagenti-mcp-broker",
				Version: "0.0.1",
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	retval.initialized = true
	retval.sessionID = upstreamSessionID(retval.client.GetSessionId())
	retval.lastContact = time.Now()

	return retval, nil
}

// CreateSession creates a new MCP session for the given authority - wrapper for createUpstreamSession
func (m *mcpBrokerImpl) CreateSession(ctx context.Context, authority string) (string, error) {
	host := upstreamMCPURL(authority)

	sessionState, err := m.createUpstreamSession(ctx, host)
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	return string(sessionState.sessionID), nil
}

func (m *mcpBrokerImpl) Close(_ context.Context, downstreamSession downstreamSessionID) error {
	var lastErr error

	for _, sessionMap := range m.serverSessions {
		for session, sessionState := range sessionMap {
			if session == downstreamSession {
				err := sessionState.client.Close()
				if err != nil {
					// Save all of the failures into a combined error?  Currently we only show last failure
					lastErr = err
				}
			}
		}
	}

	return lastErr
}

// MCPServer is a listening MCP server that federates the endpoints
func (m *mcpBrokerImpl) MCPServer() *server.MCPServer {
	return m.listeningMCPServer
}

// Get the authorization header needed for a particular MCP upstream
func getAuthorizationHeaderForUpstream(upstream *upstreamMCP) string {
	// We don't store the authorization in the config.yaml, which comes from a ConfigMap.
	// Instead it is passed to the Broker pod through env vars (typically from Secrets)
	// The format is
	// KAGENTAI_{MCP_NAME}_CRED=xxxxxxxx
	// e.g.
	// KAGENTAI_test_CRED=Bearer 1234
	return os.Getenv(fmt.Sprintf("KAGENTAI_%s_CRED", upstream.Name))
}
