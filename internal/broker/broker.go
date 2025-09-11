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
	mpcClient        *client.Client        // The MCP client we hold open to listen for tool notifications
	initializeResult *mcp.InitializeResult // The init result when we probed at discovery time
	toolsResult      *mcp.ListToolsResult  // The tools when we probed at discovery time (or updated on toolsChanged notification)
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

	// Shutdown closes any resources associated with this Broker
	Shutdown(ctx context.Context) error

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
	m.listeningMCPServer.AddTools(toolsToServerTools(mcpURL, newTools)...)

	return nil
}

func (m *mcpBrokerImpl) UnregisterServer(_ context.Context, mcpURL string) error {
	upstream, ok := m.mcpServers[upstreamMCPURL(mcpURL)]
	if !ok {
		return fmt.Errorf("unknown host")
	}

	err := upstream.mpcClient.Close()
	if err != nil {
		m.logger.Info("Failed to close upstream connection while unregistering",
			"mcpURL", mcpURL,
		)
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

	// Instead of using m.createUpstreamSession() we hand-create so that we can ask for precise notifications.
	// TODO: In the future, perhaps extend m.createUpstreamSession() to do these things?

	// Some MCP servers require a bearer token or other Authorization to init and list tools
	serverAuthHeaderValue := getAuthorizationHeaderForUpstream(upstream)
	if serverAuthHeaderValue != "" {
		options = append(options, transport.WithHTTPHeaders(map[string]string{
			"Authorization": serverAuthHeaderValue,
		}))
	}

	// Continue listening for future tool updates
	options = append(options, transport.WithContinuousListening())

	var err error
	upstream.mpcClient, err = client.NewStreamableHttpClient(
		upstream.URL,
		options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create streamable client: %w", err)
	}

	// Let transport listen for updates
	// TODO Note that currently this pollutes the log, see https://github.com/mark3labs/mcp-go/issues/552
	err = upstream.mpcClient.Start(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to start streamable client: %w", err)
	}

	resInit, err := upstream.mpcClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities: mcp.ClientCapabilities{
				Roots: &struct {
					ListChanged bool "json:\"listChanged,omitempty\""
				}{
					ListChanged: true,
				},
			},
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

	resTools, err := upstream.mpcClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}
	upstream.toolsResult = resTools

	newTools, _ := m.populateToolMapping(upstream, resTools.Tools, nil)

	// TODO probe resources other than tools

	// Keep the tools probe client open and monitor for tool changes
	upstream.mpcClient.OnConnectionLost(func(err error) {
		m.logger.Info("Broker OnConnectionLost",
			"err", err,
			"upstream.URL", upstream.URL,
			"sessionID", upstream.mpcClient.GetSessionId())
	})

	upstream.mpcClient.OnNotification(func(notification mcp.JSONRPCNotification) {
		m.logger.Debug("Broker OnNotification",
			"notification.Method", notification.Method,
			"notification.Params", notification.Params,
		)
		if notification.Method == "notifications/tools/list_changed" {
			resTools, err := upstream.mpcClient.ListTools(ctx, mcp.ListToolsRequest{})
			if err != nil {
				m.logger.Warn("failed to list tools", "err", err)
			} else {
				m.logger.Info("Re-Discovered tools", "mcpURL", upstream.URL, "#tools", len(resTools.Tools))
			}

			addedTools, removedTools := diffTools(upstream.toolsResult.Tools, resTools.Tools)

			newlyAddedTools, newlyRemovedToolNames := m.populateToolMapping(upstream, addedTools, removedTools)

			// Add any tools added since the last notification
			if len(newlyAddedTools) > 0 {
				m.logger.Info("Adding tools", "mcpURL", upstream.URL, "#tools", len(newlyAddedTools))
				m.listeningMCPServer.AddTools(toolsToServerTools(upstream.URL, newlyAddedTools)...)
			}

			// Delete any tools removed since the last notification
			if len(newlyRemovedToolNames) > 0 {
				m.logger.Info("Removing tools", "mcpURL", upstream.URL, "newlyRemovedToolNames", newlyRemovedToolNames)
				m.listeningMCPServer.DeleteTools(newlyRemovedToolNames...)
			}

			// Track the current state of tools
			upstream.toolsResult = resTools
		}
	})

	m.logger.Info("Discovered tools", "mcpURL", upstream.URL, "#tools", len(resTools.Tools))

	return newTools, err
}

// populateToolMapping maps tools to names that this gateway recognizes
// and returns a list of the new uniquely prefixed tools,
// and a list of the removed prefixed tools
func (m *mcpBrokerImpl) populateToolMapping(upstream *upstreamMCP, addTools []mcp.Tool, removeTools []mcp.Tool) ([]mcp.Tool, []string) {

	// Remove any tools no longer present in the upstream
	retvalRemovals := make([]string, 0)
	for _, tool := range removeTools {
		gatewayToolName := upstream.prefixedName(tool.Name)

		retvalRemovals = append(retvalRemovals, string(gatewayToolName))

		delete(m.toolMapping, gatewayToolName)
	}

	// Add new tools to the upstream
	retvalAdditions := make([]mcp.Tool, 0)
	for _, tool := range addTools {
		gatewayToolName := upstream.prefixedName(tool.Name)

		gatewayTool := tool // Note: shallow
		gatewayTool.Name = string(gatewayToolName)
		retvalAdditions = append(retvalAdditions, gatewayTool)

		m.toolMapping[gatewayToolName] = &upstreamToolInfo{
			url:      upstreamMCPURL(upstream.URL),
			toolName: tool.Name,
		}
	}
	return retvalAdditions, retvalRemovals
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
				sessionState.client = nil
			}
		}
	}

	return lastErr
}

func (m *mcpBrokerImpl) Shutdown(_ context.Context) error {
	// Close any user sessions
	for _, sessionMap := range m.serverSessions {
		for _, sessionState := range sessionMap {
			if sessionState.client != nil {
				_ = sessionState.client.Close()
			}
		}
	}

	// Close the long-running notification channel
	for _, mcpServer := range m.mcpServers {
		if mcpServer.mpcClient != nil {
			_ = mcpServer.mpcClient.Close()
		}
	}

	return nil
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
	// KAGENTAI_test_CRED="Bearer 1234"
	return os.Getenv(fmt.Sprintf("KAGENTAI_%s_CRED", upstream.Name))
}

// prefixedName returns the name the gateway will advertise the tool as
func (upstream *upstreamMCP) prefixedName(tool string) toolName {
	return toolName(fmt.Sprintf("%s%s", upstream.ToolPrefix, tool))
}

func toolToServerTool(newTool mcp.Tool) server.ServerTool {
	return server.ServerTool{
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
	}
}

func toolsToServerTools(mcpURL string, newTools []mcp.Tool) []server.ServerTool {

	tools := make([]server.ServerTool, 0)
	for _, newTool := range newTools {
		slog.Info("Federating tool", "mcpURL", mcpURL, "federated name", newTool.Name)
		tools = append(tools, toolToServerTool(newTool))
	}

	return tools
}

// diffTools compares two lists of tools, and returns a list of additions and a list of subtractions
func diffTools(oldTools, newTools []mcp.Tool) ([]mcp.Tool, []mcp.Tool) {
	oldToolMap := make(map[string]mcp.Tool)
	for _, oldTool := range oldTools {
		oldToolMap[oldTool.Name] = oldTool
	}

	newToolMap := make(map[string]mcp.Tool)
	for _, newTool := range newTools {
		newToolMap[newTool.Name] = newTool
	}

	// Any tools with name in the new map but not the old are additions
	addedTools := make([]mcp.Tool, 0)
	for _, newTool := range newToolMap {
		_, ok := oldToolMap[newTool.Name]
		if !ok {
			addedTools = append(addedTools, newTool)
		}
	}

	// Any tools with name in the old map but not the new are removals
	removedTools := make([]mcp.Tool, 0)
	for _, oldTool := range oldToolMap {
		_, ok := newToolMap[oldTool.Name]
		if !ok {
			removedTools = append(removedTools, oldTool)
		}
	}

	return addedTools, removedTools
}
