// Package broker tracks upstream MCP servers and manages the relationship from clients to upstream
package broker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"k8s.io/apimachinery/pkg/util/wait"
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

// ClientType defines the type of MCP client being created
type ClientType int

const (
	// ClientTypeValidation is used for validation-only sessions
	ClientTypeValidation ClientType = iota
	// ClientTypeDiscovery is used for tool discovery and notifications
	ClientTypeDiscovery
	// ClientTypeSession is used for regular tool execution sessions
	ClientTypeSession
)

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

	// ValidateAllServers performs comprehensive validation of all registered servers and returns status
	ValidateAllServers(ctx context.Context) StatusResponse

	// HandleStatusRequest handles HTTP status endpoint requests
	HandleStatusRequest(w http.ResponseWriter, r *http.Request)

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
	for _, mcpServer := range conf.Servers {
		if err := m.RegisterServerWithConfig(ctx, mcpServer); err != nil {
			slog.Warn("Could not register upstream MCP", "upstream", mcpServer.URL, "name", mcpServer.Name, "error", err)
		}
	}
}

// RegisterServerWithConfig registers an MCP server with full config
func (m *mcpBrokerImpl) RegisterServerWithConfig(
	ctx context.Context,
	mcpServer *config.MCPServer,
) error {
	existingUpstream, isRegistered := m.mcpServers[upstreamMCPURL(mcpServer.URL)]

	// check if configuration changed for already registered server
	if isRegistered {
		configChanged := existingUpstream.Name != mcpServer.Name ||
			existingUpstream.ToolPrefix != mcpServer.ToolPrefix ||
			existingUpstream.Hostname != mcpServer.Hostname ||
			existingUpstream.CredentialEnvVar != mcpServer.CredentialEnvVar

		if !configChanged {
			m.logger.Info("MCP server is already registered with same config", "mcpURL", mcpServer.URL)
			return nil
		}

		// config changed, unregister and re-register
		m.logger.Info("MCP server config changed, re-registering",
			"mcpURL", mcpServer.URL,
			"oldPrefix", existingUpstream.ToolPrefix,
			"newPrefix", mcpServer.ToolPrefix)

		if err := m.UnregisterServer(ctx, mcpServer.URL); err != nil {
			m.logger.Warn("failed to unregister before re-registration", "error", err)
		}
	}

	slog.Info("Registering server", "mcpURL", mcpServer.URL, "prefix", mcpServer.ToolPrefix)

	upstream := &upstreamMCP{
		MCPServer: *mcpServer,
	}

	newTools, err := m.discoverTools(ctx, upstream)
	if err != nil {
		slog.Info("Failed to discover tools, will retry with backoff", "mcpURL", mcpServer.URL, "error", err)
		// register server even if discovery fails, will retry in background
		m.mcpServers[upstreamMCPURL(mcpServer.URL)] = upstream
		// start background retry with exponential backoff
		go m.retryDiscovery(context.Background(), upstream)
		return nil // don't return error, allow partial registration
	}
	slog.Info("Discovered tools", "mcpURL", mcpServer.URL, "num tools", len(newTools))

	m.mcpServers[upstreamMCPURL(mcpServer.URL)] = upstream
	slog.Info("Server registered", "url", mcpServer.URL, "totalServers", len(m.mcpServers))
	m.listeningMCPServer.AddTools(toolsToServerTools(mcpServer.URL, newTools)...)

	return nil
}

// RegisterServer registers an MCP server
func (m *mcpBrokerImpl) RegisterServer(
	ctx context.Context,
	mcpURL string,
	prefix string,
	name string,
) error {
	return m.RegisterServerWithConfig(ctx, &config.MCPServer{
		Name:       name,
		URL:        mcpURL,
		ToolPrefix: prefix,
		Enabled:    true,
	})
}

func (m *mcpBrokerImpl) UnregisterServer(_ context.Context, mcpURL string) error {
	upstream, ok := m.mcpServers[upstreamMCPURL(mcpURL)]
	if !ok {
		return fmt.Errorf("unknown host")
	}

	// only close client if it exists (might be nil if discovery failed)
	if upstream.mpcClient != nil {
		err := upstream.mpcClient.Close()
		if err != nil {
			m.logger.Info("Failed to close upstream connection while unregistering",
				"mcpURL", mcpURL,
				"error", err,
			)
		}
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
		// check for auth
		upstream, ok := m.mcpServers[upstreamToolInfo.url]
		if !ok {
			return nil, fmt.Errorf("upstream server not found: %s", upstreamToolInfo.url)
		}

		// auth options
		var options []transport.StreamableHTTPCOption
		serverAuthHeaderValue := getAuthorizationHeaderForUpstream(upstream)
		if serverAuthHeaderValue != "" {
			slog.Debug("Creating upstream session with authentication", "url", upstreamToolInfo.url)
			options = append(options, transport.WithHTTPHeaders(map[string]string{
				"Authorization": serverAuthHeaderValue,
			}))
		}

		var err error
		upstreamSession, err = m.createUpstreamSession(ctx, upstreamToolInfo.url, options...)
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

func (m *mcpBrokerImpl) discoverTools(ctx context.Context, upstream *upstreamMCP, options ...transport.StreamableHTTPCOption) ([]mcp.Tool, error) {

	// Some MCP servers require a bearer token or other Authorization to init and list tools
	serverAuthHeaderValue := getAuthorizationHeaderForUpstream(upstream)
	if serverAuthHeaderValue != "" {
		slog.Info("Adding auth header for discovery",
			"url", upstream.URL,
			"credentialEnvVar", upstream.CredentialEnvVar)
		options = append(options, transport.WithHTTPHeaders(map[string]string{
			"Authorization": serverAuthHeaderValue,
		}))
	}

	// Continue listening for future tool updates
	options = append(options, transport.WithContinuousListening())

	var err error
	var resInit *mcp.InitializeResult
	upstream.mpcClient, resInit, err = m.createMCPClient(ctx, upstream.URL, upstream.Name, ClientTypeDiscovery, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create streamable client: %w", err)
	}

	// Let transport listen for updates
	// TODO Note that currently this pollutes the log, see https://github.com/mark3labs/mcp-go/issues/552
	err = upstream.mpcClient.Start(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to start streamable client: %w", err)
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
		m.logger.Info("Broker OnConnectionLost, will retry with backoff",
			"err", err,
			"upstream.URL", upstream.URL,
			"sessionID", upstream.mpcClient.GetSessionId())
		// start background retry when connection is lost
		go m.retryDiscovery(context.Background(), upstream)
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

// retryDiscovery attempts to discover tools with exponential backoff
func (m *mcpBrokerImpl) retryDiscovery(ctx context.Context, upstream *upstreamMCP) {
	// configurable via env vars with sensible defaults
	baseDelay := 5 * time.Second
	if v := os.Getenv("MCP_GATEWAY_DISCOVERY_RETRY_BASE_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			baseDelay = d
		}
	}

	maxDelay := 5 * time.Minute
	if v := os.Getenv("MCP_GATEWAY_DISCOVERY_RETRY_MAX_DELAY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			maxDelay = d
		}
	}

	maxRetries := 10
	if v := os.Getenv("MCP_GATEWAY_DISCOVERY_RETRY_MAX_ATTEMPTS"); v != "" {
		if r, err := strconv.Atoi(v); err == nil && r > 0 {
			maxRetries = r
		}
	}

	backoff := wait.Backoff{
		Duration: baseDelay,
		Factor:   2.0,
		Steps:    maxRetries,
		Cap:      maxDelay,
	}

	attempt := 0
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		attempt++
		m.logger.Info("attempting discovery",
			"url", upstream.URL,
			"attempt", attempt)

		newTools, err := m.discoverTools(ctx, upstream)
		if err != nil {
			m.logger.Warn("retry discovery failed",
				"url", upstream.URL,
				"attempt", attempt,
				"error", err)
			return false, nil
		}

		m.logger.Info("retry discovery succeeded",
			"url", upstream.URL,
			"attempt", attempt,
			"tools", len(newTools))

		m.listeningMCPServer.AddTools(toolsToServerTools(upstream.URL, newTools)...)
		return true, nil
	})

	if err != nil {
		if wait.Interrupted(err) {
			m.logger.Error("max retries exceeded for discovery",
				"url", upstream.URL,
				"maxRetries", maxRetries,
				"error", err)
		} else {
			m.logger.Info("retry discovery cancelled",
				"url", upstream.URL,
				"error", err)
		}
	}
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

func (m *mcpBrokerImpl) createUpstreamSession(ctx context.Context, host upstreamMCPURL, options ...transport.StreamableHTTPCOption) (*upstreamSessionState, error) {
	retval := &upstreamSessionState{}

	// Find the server name for auth - look up in registered servers
	var serverName string
	for url, upstream := range m.mcpServers {
		if url == host {
			serverName = upstream.Name
			break
		}
	}

	var err error
	retval.client, _, err = m.createMCPClient(ctx, string(host), serverName, ClientTypeSession, options...)
	if err != nil {
		return nil, err
	}

	retval.initialized = true
	retval.sessionID = upstreamSessionID(retval.client.GetSessionId())
	retval.lastContact = time.Now()

	return retval, nil
}

// CreateSession creates a new MCP session for the given authority - wrapper for createUpstreamSession
func (m *mcpBrokerImpl) CreateSession(ctx context.Context, authority string) (string, error) {
	host := upstreamMCPURL(authority)

	slog.Info("CreateSession called", "authority", authority)

	// get the upstream server config to check for auth
	upstream, ok := m.mcpServers[host]
	if !ok {
		// server not registered yet
		sessionState, err := m.createUpstreamSession(ctx, host)
		if err != nil {
			return "", fmt.Errorf("failed to create session: %w", err)
		}
		return string(sessionState.sessionID), nil
	}

	// auth options
	var options []transport.StreamableHTTPCOption
	serverAuthHeaderValue := getAuthorizationHeaderForUpstream(upstream)
	if serverAuthHeaderValue != "" {
		slog.Info("Creating session with authentication",
			"authority", authority,
			"credentialEnvVar", upstream.CredentialEnvVar)
		options = append(options, transport.WithHTTPHeaders(map[string]string{
			"Authorization": serverAuthHeaderValue,
		}))
	}

	sessionState, err := m.createUpstreamSession(ctx, host, options...)
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

// HandleStatusRequest handles HTTP status endpoint requests
func (m *mcpBrokerImpl) HandleStatusRequest(w http.ResponseWriter, r *http.Request) {
	handler := NewStatusHandler(m, *m.logger)
	handler.ServeHTTP(w, r)
}

// createMCPClient creates and initializes an MCP client with the appropriate configuration
func (m *mcpBrokerImpl) createMCPClient(ctx context.Context, mcpURL, name string, clientType ClientType, options ...transport.StreamableHTTPCOption) (*client.Client, *mcp.InitializeResult, error) {
	// Add auth headers if needed
	authHeader := getAuthorizationHeaderForUpstream(&upstreamMCP{
		MCPServer: config.MCPServer{Name: name, URL: mcpURL},
	})
	if authHeader != "" {
		options = append(options, transport.WithHTTPHeaders(map[string]string{
			"Authorization": authHeader,
		}))
	}

	httpClient, err := client.NewStreamableHttpClient(mcpURL, options...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Configure capabilities and client info based on type
	var capabilities mcp.ClientCapabilities
	var clientName string

	switch clientType {
	case ClientTypeValidation:
		capabilities = mcp.ClientCapabilities{}
		clientName = "kagenti-mcp-broker-validation"

	case ClientTypeDiscovery:
		capabilities = mcp.ClientCapabilities{
			Roots: &struct {
				ListChanged bool `json:"listChanged,omitempty"`
			}{
				ListChanged: true,
			},
		}
		clientName = "kagenti-mcp-broker"

	case ClientTypeSession:
		capabilities = mcp.ClientCapabilities{}
		clientName = "kagenti-mcp-broker"
	}

	initResp, err := httpClient.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    capabilities,
			ClientInfo: mcp.Implementation{
				Name:    clientName,
				Version: "0.0.1",
			},
		},
	})
	if err != nil {
		_ = httpClient.Close()
		return nil, nil, fmt.Errorf("initialization failed: %w", err)
	}

	return httpClient, initResp, nil
}

// Get the authorization header needed for a particular MCP upstream
func getAuthorizationHeaderForUpstream(upstream *upstreamMCP) string {
	// We don't store the authorization in the config.yaml, which comes from a ConfigMap.
	// Instead it is passed to the Broker pod through env vars (typically from Secrets)
	if upstream.CredentialEnvVar != "" {
		return os.Getenv(upstream.CredentialEnvVar)
	}
	// The format is KAGENTAI_{MCP_NAME}_CRED=xxxxxxxx
	// e.g. KAGENTAI_test_CRED="Bearer 1234"
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

// validateMCPServer validates a single MCP server using existing session data
func (m *mcpBrokerImpl) validateMCPServer(_ context.Context, mcpURL, name, toolPrefix string) ServerValidationStatus {
	// Use already-discovered data for registered servers
	upstream, exists := m.mcpServers[upstreamMCPURL(mcpURL)]
	if !exists {
		m.logger.Warn("Server validation failed: server not registered", "url", mcpURL, "name", name)
		return ServerValidationStatus{
			URL:        mcpURL,
			Name:       name,
			ToolPrefix: toolPrefix,
			ConnectionStatus: ConnectionStatus{
				IsReachable: false,
				Error:       "Server not registered",
			},
		}
	}

	connectionStatus := m.checkSessionHealth(upstream)

	var protocolValidation ProtocolValidation
	var tools []mcp.Tool

	if upstream.initializeResult != nil {
		protocolValidation = validateProtocol(upstream.initializeResult)
		if !protocolValidation.IsValid {
			m.logger.Warn("Protocol validation failed", "url", mcpURL,
				"expected", protocolValidation.ExpectedVersion,
				"actual", protocolValidation.SupportedVersion)
		}
	} else {
		m.logger.Warn("No initialization result available for validation", "url", mcpURL)
		protocolValidation = ProtocolValidation{
			IsValid:         false,
			ExpectedVersion: mcp.LATEST_PROTOCOL_VERSION,
		}
	}

	if upstream.toolsResult != nil {
		tools = upstream.toolsResult.Tools
		if len(tools) == 0 {
			m.logger.Warn("Server has no tools available", "url", mcpURL)
		}
	} else {
		m.logger.Warn("No tools result available for validation", "url", mcpURL)
	}

	status := buildValidationStatus(connectionStatus, protocolValidation, tools, mcpURL, name, toolPrefix)
	status.ToolConflicts = m.checkToolConflicts(tools, toolPrefix, mcpURL)

	if len(status.ToolConflicts) > 0 {
		m.logger.Warn("Tool conflicts detected", "url", mcpURL, "conflicts", len(status.ToolConflicts))
	}

	return status
}

// checkSessionHealth checks the health of an existing session and reinitializes if needed
func (m *mcpBrokerImpl) checkSessionHealth(upstream *upstreamMCP) ConnectionStatus {
	if upstream.mpcClient == nil {
		// Create a temporary validation-only session
		return m.validateServerConnectivity(upstream)
	}

	// Test existing session with a quick ping
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := upstream.mpcClient.Ping(ctx)
	if err != nil {
		m.logger.Debug("Session ping failed during validation", "url", upstream.URL, "error", err)
		return ConnectionStatus{
			IsReachable: false,
			Error:       fmt.Sprintf("Session ping failed: %v", err),
		}
	}

	return ConnectionStatus{IsReachable: true}
}

// validateServerConnectivity creates a temporary session just for validation
func (m *mcpBrokerImpl) validateServerConnectivity(upstream *upstreamMCP) ConnectionStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Create a temporary client just for validation
	client, _, err := m.createMCPClient(ctx, upstream.URL, upstream.Name, ClientTypeValidation)
	if err != nil {
		return ConnectionStatus{
			IsReachable: false,
			Error:       fmt.Sprintf("Failed to connect: %v", err),
		}
	}

	// Clean up immediately - this is just for validation
	defer func() {
		if client != nil {
			_ = client.Close()
		}
	}()

	return ConnectionStatus{IsReachable: true}
}

// validateProtocol validates the MCP protocol version
func validateProtocol(initResp *mcp.InitializeResult) ProtocolValidation {
	return ProtocolValidation{
		IsValid:          initResp.ProtocolVersion == mcp.LATEST_PROTOCOL_VERSION,
		SupportedVersion: initResp.ProtocolVersion,
		ExpectedVersion:  mcp.LATEST_PROTOCOL_VERSION,
	}
}

// buildValidationStatus constructs a ServerValidationStatus from validation results
func buildValidationStatus(connectionStatus ConnectionStatus, protocolValidation ProtocolValidation, tools []mcp.Tool, mcpURL, name, toolPrefix string) ServerValidationStatus {
	validation := CapabilitiesValidation{
		IsValid:             true,
		HasToolCapabilities: len(tools) > 0,
		ToolCount:           len(tools),
		MissingCapabilities: []string{},
	}

	if !validation.HasToolCapabilities {
		validation.IsValid = false
		validation.MissingCapabilities = append(validation.MissingCapabilities, "tools")
	}

	return ServerValidationStatus{
		URL:                    mcpURL,
		Name:                   name,
		ToolPrefix:             toolPrefix,
		ConnectionStatus:       connectionStatus,
		ProtocolValidation:     protocolValidation,
		CapabilitiesValidation: validation,
		ToolConflicts:          []ToolConflict{},
		LastValidated:          time.Now(),
	}
}

// checkToolConflicts identifies tool name conflicts between servers
func (m *mcpBrokerImpl) checkToolConflicts(tools []mcp.Tool, toolPrefix, currentURL string) []ToolConflict {
	var conflicts []ToolConflict

	for _, tool := range tools {
		prefixedToolName := fmt.Sprintf("%s%s", toolPrefix, tool.Name)
		var conflictingServers []string

		for existingToolName, existingToolInfo := range m.toolMapping {
			if string(existingToolName) == prefixedToolName && string(existingToolInfo.url) != currentURL {
				conflictingServers = append(conflictingServers, string(existingToolInfo.url))
			}
		}

		if len(conflictingServers) > 0 {
			conflicts = append(conflicts, ToolConflict{
				ToolName:      tool.Name,
				PrefixedName:  prefixedToolName,
				ConflictsWith: conflictingServers,
			})
		}
	}

	return conflicts
}

// ValidateAllServers performs comprehensive validation of all registered servers and returns status
func (m *mcpBrokerImpl) ValidateAllServers(ctx context.Context) StatusResponse {
	response := StatusResponse{
		Servers:          make([]ServerValidationStatus, 0),
		OverallValid:     true,
		TotalServers:     len(m.mcpServers),
		HealthyServers:   0,
		UnHealthyServers: 0,
		ToolConflicts:    0,
		Timestamp:        time.Now(),
	}

	for url, upstream := range m.mcpServers {
		status := m.validateMCPServer(ctx, string(url), upstream.Name, upstream.ToolPrefix)
		response.Servers = append(response.Servers, status)

		response.ToolConflicts += len(status.ToolConflicts)

		hasErrors := !status.ConnectionStatus.IsReachable ||
			!status.ProtocolValidation.IsValid ||
			!status.CapabilitiesValidation.IsValid

		hasWarnings := len(status.ToolConflicts) > 0

		if hasErrors || hasWarnings {
			response.UnHealthyServers++
			response.OverallValid = false
		} else {
			response.HealthyServers++
		}
	}

	m.logger.Info("Server validation completed",
		"totalServers", response.TotalServers,
		"healthyServers", response.HealthyServers,
		"unhealthyServers", response.UnHealthyServers,
		"toolConflicts", response.ToolConflicts,
		"overallValid", response.OverallValid)

	return response
}

func diffTools(oldTools, newTools []mcp.Tool) ([]mcp.Tool, []mcp.Tool) {
	oldToolMap := make(map[string]mcp.Tool)
	for _, oldTool := range oldTools {
		oldToolMap[oldTool.Name] = oldTool
	}

	newToolMap := make(map[string]mcp.Tool)
	for _, newTool := range newTools {
		newToolMap[newTool.Name] = newTool
	}

	addedTools := make([]mcp.Tool, 0)
	for _, newTool := range newToolMap {
		_, ok := oldToolMap[newTool.Name]
		if !ok {
			addedTools = append(addedTools, newTool)
		}
	}

	removedTools := make([]mcp.Tool, 0)
	for _, oldTool := range oldToolMap {
		_, ok := newToolMap[oldTool.Name]
		if !ok {
			removedTools = append(removedTools, oldTool)
		}
	}

	return addedTools, removedTools
}
