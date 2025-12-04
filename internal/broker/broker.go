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

// upstreamMCPID identifies an upstream MCP server
type upstreamMCPID string

// upstreamMCP identifies what we know about an upstream MCP server
type upstreamMCP struct {
	config.MCPServer
	mcpClient        *client.Client        // The MCP client we hold open to listen for tool notifications
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
	client *client.Client
}

// An MCP tool name
type toolName string

// upstreamToolInfo references a single tool on an upstream MCP server
type upstreamToolInfo struct {
	id          upstreamMCPID // A deterministic ID
	toolName    string        // A tool name
	annotations mcp.ToolAnnotation
}

// MCPBroker manages a set of MCP servers and their sessions
type MCPBroker interface {

	// Removes a server
	UnregisterServer(ctx context.Context, mcpURL string) error

	IsRegistered(mcpURL string) bool

	// Returns tool annotations for a given tool name
	ToolAnnotations(tool string) (mcp.ToolAnnotation, bool)

	// Cleanup any upstream connections being held open on behalf of downstreamSessionID
	Close(ctx context.Context, downstreamSession downstreamSessionID) error

	// MCPServer gets an MCP server that federates the upstreams known to this MCPBroker
	MCPServer() *server.MCPServer

	// ValidateAllServers performs comprehensive validation of all registered servers and returns status
	ValidateAllServers(ctx context.Context) StatusResponse

	// HandleStatusRequest handles HTTP status endpoint requests
	HandleStatusRequest(w http.ResponseWriter, r *http.Request)

	// Shutdown closes any resources associated with this Broker
	Shutdown(ctx context.Context) error

	config.Observer
}

// TODO this probably should move to a sync.Map
type mcpServers map[upstreamMCPID]*upstreamMCP

func (mcps mcpServers) findByHost(host string) *upstreamMCP {
	for _, mcp := range mcps {
		if mcp.Hostname == host {
			return mcp
		}
	}
	return nil
}

// mcpBrokerImpl implements MCPBroker
type mcpBrokerImpl struct {
	// Static map of session IDs we offer to downstream clients
	// value will be false if uninitialized, true if initialized
	// TODO: evict if not used for a while?
	// knownSessionIDs map[downstreamSessionID]clientStatus

	// serverSessions tracks the sessions we maintain with upstream MCP servers
	serverSessions map[upstreamMCPID]map[downstreamSessionID]*upstreamSessionState

	// mcpServers tracks the known servers
	// TODO this should be protected or be a sync map
	mcpServers mcpServers

	// toolMapping tracks the unique gateway'ed tool name to its upstream MCP server implementation
	toolMapping map[toolName]*upstreamToolInfo

	// listeningMCPServer returns an actual listening MCP server that federates registered MCP servers
	listeningMCPServer *server.MCPServer

	logger *slog.Logger

	// enforceToolFilter if set will ensure only a filtered list of tools is returned this list is based on the x-authorized-tools trusted header
	enforceToolFilter bool

	// trustedHeadersPublicKey this is the key to verify that a trusted header came from the trusted source (the owner of the private key)
	trustedHeadersPublicKey string
}

// this ensures that mcpBrokerImpl implements the MCPBroker interface
var _ MCPBroker = &mcpBrokerImpl{}

// WithEnforceToolFilter defines enforceToolFilter setting and is intended for use with the NewBroker function
func WithEnforceToolFilter(enforce bool) func(mb *mcpBrokerImpl) {
	return func(mb *mcpBrokerImpl) {
		mb.enforceToolFilter = enforce
	}
}

// WithTrustedHeadersPublicKey defines the public key used to verify signed headers and is intended for use with the NewBroker function
func WithTrustedHeadersPublicKey(key string) func(mb *mcpBrokerImpl) {
	return func(mb *mcpBrokerImpl) {
		mb.trustedHeadersPublicKey = key
	}
}

// NewBroker creates a new MCPBroker accepts optional config functions such as WithEnforceToolFilter
func NewBroker(logger *slog.Logger, opts ...func(*mcpBrokerImpl)) MCPBroker {
	mcpBkr := &mcpBrokerImpl{
		// knownSessionIDs: map[downstreamSessionID]clientStatus{},
		serverSessions: map[upstreamMCPID]map[downstreamSessionID]*upstreamSessionState{},
		mcpServers:     map[upstreamMCPID]*upstreamMCP{},
		toolMapping:    map[toolName]*upstreamToolInfo{},
		logger:         logger,
	}

	for _, option := range opts {
		option(mcpBkr)
	}

	hooks := &server.Hooks{}

	// Enhanced session registration to log gateway session assignment
	hooks.AddOnRegisterSession(func(_ context.Context, session server.ClientSession) {
		// Note that AddOnRegisterSession is for GET, not POST, for a session.
		// https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#listening-for-messages-from-the-server
		slog.Info("Gateway client session connected with session", "gatewaySessionID", session.SessionID())
	})

	hooks.AddOnUnregisterSession(func(_ context.Context, session server.ClientSession) {
		slog.Info("Gateway client session unregister ", "gatewaySessionID", session.SessionID())
	})

	hooks.AddBeforeAny(func(_ context.Context, _ any, method mcp.MCPMethod, _ any) {
		slog.Info("Processing request", "method", method)
	})

	hooks.AddOnError(func(_ context.Context, _ any, method mcp.MCPMethod, _ any, err error) {
		slog.Info("MCP server error", "method", method, "error", err)
	})

	hooks.AddAfterListTools(mcpBkr.FilteredTools)

	mcpBkr.listeningMCPServer = server.NewMCPServer(
		"Kagenti MCP Broker",
		"0.0.1",
		server.WithHooks(hooks),
		server.WithToolCapabilities(true),
	)

	return mcpBkr
}

func (m *mcpBrokerImpl) IsRegistered(id string) bool {
	_, ok := m.mcpServers[upstreamMCPID(id)]
	return ok
}

func (m *mcpBrokerImpl) OnConfigChange(ctx context.Context, conf *config.MCPServersConfig) {
	m.logger.Debug("Broker OnConfigChange called")
	// unregister decommissioned servers
	for serverID := range m.mcpServers {
		if !slices.ContainsFunc(conf.Servers, func(s *config.MCPServer) bool {
			return serverID == upstreamMCPID(s.ID())
		}) {
			if err := m.UnregisterServer(ctx, string(serverID)); err != nil {
				m.logger.Warn("unregister failed ", "server", serverID)
			}
		}
	}
	// ensure new servers registered
	discoveredTools := []mcp.Tool{}
	for _, mcpServer := range conf.Servers {
		m.logger.Info("Registering Server ", "mcpID", mcpServer.ID())

		tools, err := m.RegisterServerWithConfig(ctx, mcpServer)
		if err != nil {
			slog.Warn("Could not register upstream MCP", "upstream", mcpServer.URL, "name", mcpServer.Name, "error", err)
			continue
		}
		if tools != nil {
			discoveredTools = append(discoveredTools, tools...)
		}
		m.logger.Info("Registered Server ", "mcpID", mcpServer.ID())
	}
	m.logger.Debug("OnConfigChange discovered tools ", "total", len(discoveredTools))
	if len(discoveredTools) > 0 {
		m.listeningMCPServer.AddTools(toolsToServerTools(discoveredTools)...)
	}
}

// RegisterServerWithConfig registers an MCP server with full config
func (m *mcpBrokerImpl) RegisterServerWithConfig(ctx context.Context, mcpServer *config.MCPServer) ([]mcp.Tool, error) {
	m.logger.Debug("registering server ", "mcpID", mcpServer.ID())
	// check if configuration changed for already registered server
	if existingUpstream, isRegistered := m.mcpServers[upstreamMCPID(mcpServer.ID())]; isRegistered {

		configChanged := mcpServer.ConfigChanged(existingUpstream.MCPServer)
		if !configChanged {
			m.logger.Debug("mcp server is already registered and up to date", "server ", mcpServer.ID())
			return nil, nil
		}
		// config or credentials changed, unregister and re-register
		m.logger.Info("RegisterServerWithConfig mcp re-registering",
			"server", mcpServer.ID(),
			"mcpURL", mcpServer.URL,
			"configChanged", configChanged)

		if err := m.UnregisterServer(ctx, mcpServer.ID()); err != nil {
			m.logger.Warn("failed to unregister before re-registration", "error", err)
			return nil, err
		}
	}

	m.logger.Info("Registering server", "mcpURL", mcpServer.Name, "prefix", mcpServer.ToolPrefix)
	upstream := &upstreamMCP{
		MCPServer: *mcpServer,
	}

	m.logger.Debug("Registering server adding upstream", "mcp server", mcpServer.ID())
	// Add to map BEFORE discovering tools so createMCPClient can find it
	m.mcpServers[upstreamMCPID(mcpServer.ID())] = upstream
	// TODO this will block
	newTools, err := m.discoverTools(ctx, upstreamMCPID(mcpServer.ID()))
	if err != nil {
		m.logger.Info("Failed to discover tools, will retry with backoff", "mcpID", mcpServer.ID(), "mcpURL", mcpServer.URL, "error", err)
		// start background retry with exponential backoff
		go m.retryDiscovery(context.Background(), upstreamMCPID(mcpServer.ID()))
		return nil, nil // don't return error, allow partial registration
	}
	m.logger.Info("Discovered tools", "serverName", mcpServer.Name, "num tools", len(newTools))
	if len(newTools) > 0 {
		m.listeningMCPServer.AddTools(toolsToServerTools(newTools)...)
	}
	return newTools, nil
}

func (m *mcpBrokerImpl) UnregisterServer(_ context.Context, id string) error {
	m.logger.Info("unregistering mcp server ", "id", id)
	upstream, ok := m.mcpServers[upstreamMCPID(id)]
	if !ok {
		return fmt.Errorf("unknown mcp server %s", id)
	}

	// only close client if it exists (might be nil if discovery failed)
	if upstream.mcpClient != nil {
		err := upstream.mcpClient.Close()
		if err != nil {
			m.logger.Info("Failed to close upstream connection while unregistering",
				"mcpID", id,
				"error", err,
			)
		}
	}

	delete(m.mcpServers, upstreamMCPID(id))

	// Find tools registered to this server
	toolsToDelete := make([]string, 0)
	for toolName, upstreamToolInfo := range m.toolMapping {
		if upstreamToolInfo.id == upstreamMCPID(id) {
			toolsToDelete = append(toolsToDelete, string(toolName))
		}
	}
	m.listeningMCPServer.DeleteTools(toolsToDelete...)

	// Close any connections to the upstream server
	mapping, ok := m.serverSessions[upstreamMCPID(id)]
	if ok {
		for downstreamSessionID, upstreamSessionState := range mapping {
			err := upstreamSessionState.client.Close()
			if err != nil {
				slog.Warn(
					"Could not close upstream session",
					"mcpID",
					id,
					"sessionID",
					downstreamSessionID,
				)
			}
		}
	}

	return nil
}

func (m *mcpBrokerImpl) ToolAnnotations(tool string) (mcp.ToolAnnotation, bool) {
	upstreamToolInfo, ok := m.toolMapping[toolName(tool)]
	if !ok {
		return mcp.ToolAnnotation{}, false
	}
	return upstreamToolInfo.annotations, true
}

// TODO(craig) consider if these should actually just be methods om the upstream type
func (m *mcpBrokerImpl) discoverTools(ctx context.Context, mcpID upstreamMCPID, options ...transport.StreamableHTTPCOption) ([]mcp.Tool, error) {

	upstream, ok := m.mcpServers[mcpID]

	if !ok {
		return nil, fmt.Errorf("discover tools failed no upstream server registered %s", mcpID)
	}

	if upstream.mcpClient == nil {
		// TODO (craig) most of the function require an upstreamMCP object
		// Connection mgmt shouldn't be a part of this function
		// prob functions should just become methods of that object
		//upstreamMCP.Connect()
		//upstreamMCP.ListTools()
		//upstreamMCP.Ping() etc...
		options = append(options, transport.WithContinuousListening())
		var resInit *mcp.InitializeResult
		client, resInit, err := m.createMCPClient(ctx, upstream.ID(), ClientTypeDiscovery, options...)
		if err != nil {
			return nil, fmt.Errorf("failed to create streamable client: %w", err)
		}
		upstream.mcpClient = client
		upstream.initializeResult = resInit
	}

	resTools, err := upstream.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}
	upstream.toolsResult = resTools

	newTools, _ := m.populateToolMapping(upstream, resTools.Tools, nil)

	// TODO probe resources other than tools

	// Keep the tools probe client open and monitor for tool changes
	upstream.mcpClient.OnConnectionLost(func(err error) {
		m.logger.Info("Broker OnConnectionLost, will retry with backoff",
			"err", err,
			"upstream.URL", upstream.URL,
			"sessionID", upstream.mcpClient.GetSessionId())
		// start background retry when connection is lost
		go m.retryDiscovery(context.Background(), mcpID)
	})

	upstream.mcpClient.OnNotification(func(notification mcp.JSONRPCNotification) {
		if notification.Method == "notifications/tools/list_changed" {
			m.logger.Debug("notifications/tools/list_changed received", "mcpid", upstream.ID())
			resTools, err := upstream.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
			if err != nil {
				m.logger.Warn("failed to list tools", "err", err)
			} else {
				m.logger.Info("OnNotification Re-Discovered tools  ", "mcpURL", upstream.URL, "#tools", len(resTools.Tools))
			}

			addedTools, removedTools := diffTools(upstream.toolsResult.Tools, resTools.Tools)

			newlyAddedTools, newlyRemovedToolNames := m.populateToolMapping(upstream, addedTools, removedTools)

			// Add any tools added since the last notification
			if len(newlyAddedTools) > 0 {
				m.logger.Info("OnNotification Adding tools", "mcpURL", upstream.URL, "#tools", len(newlyAddedTools))
				//NOTE this sends a notification to connected clients
				m.listeningMCPServer.AddTools(toolsToServerTools(newlyAddedTools)...)
			}

			// Delete any tools removed since the last notification
			if len(newlyRemovedToolNames) > 0 {
				m.logger.Info("OnNotification Removing tools", "mcpURL", upstream.URL, "newlyRemovedToolNames", newlyRemovedToolNames)
				m.listeningMCPServer.DeleteTools(newlyRemovedToolNames...)
			}

			// Track the current state of tools
			upstream.toolsResult = resTools
		}
	})

	m.logger.Info("OnNotification Re-Discovered tools", "mcpURL", upstream.URL, "#tools", len(resTools.Tools))

	return newTools, err
}

// ConfigureBackOff configures a backoff based on EnvVars
func ConfigureBackOff() wait.Backoff {
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

	return wait.Backoff{
		Duration: baseDelay,
		Factor:   2.0,
		Steps:    maxRetries,
		Cap:      maxDelay,
	}

}

// retryDiscovery attempts to discover tools with exponential backoff
func (m *mcpBrokerImpl) retryDiscovery(ctx context.Context, mcpID upstreamMCPID) {
	// configurable via env vars with sensible defaults
	upstream, ok := m.mcpServers[mcpID]
	if !ok {
		m.logger.Info("retry: upstream has been unregistered", "id", mcpID)
		return
	}
	attempt := 0
	backOff := ConfigureBackOff()
	err := wait.ExponentialBackoffWithContext(ctx, backOff, func(ctx context.Context) (bool, error) {
		attempt++
		m.logger.Info("attempting discovery",
			"id", upstream.ID(),
			"url", upstream.URL,
			"attempt", attempt)

		newTools, err := m.discoverTools(ctx, mcpID)
		if err != nil {
			m.logger.Warn("retry discovery failed",
				"id", upstream.ID(),
				"url", upstream.URL,
				"attempt", attempt,
				"error", err)
			return false, nil
		}

		m.logger.Info("retry discovery succeeded",
			"id", upstream.ID(),
			"url", upstream.URL,
			"attempt", attempt,
			"tools", len(newTools))

		m.listeningMCPServer.AddTools(toolsToServerTools(newTools)...)
		return true, nil
	})
	if err != nil {
		if wait.Interrupted(err) {
			m.logger.Error("max retries exceeded for discovery",
				"id", upstream.ID(),
				"url", upstream.URL,
				"maxRetries", backOff.Steps,
				"error", err)
		} else {
			m.logger.Info("retry discovery cancelled",
				"id", upstream.ID(),
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
			id:          upstreamMCPID(upstream.ID()),
			toolName:    tool.Name,
			annotations: tool.Annotations,
		}
	}
	return retvalAdditions, retvalRemovals
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
		if mcpServer.mcpClient != nil {
			_ = mcpServer.mcpClient.Close()
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
func (m *mcpBrokerImpl) createMCPClient(ctx context.Context, id string, clientType ClientType, options ...transport.StreamableHTTPCOption) (*client.Client, *mcp.InitializeResult, error) {
	upstream, found := m.mcpServers[upstreamMCPID(id)]
	if !found {
		return nil, nil, fmt.Errorf("unable to create client for unknown MCP server %s", id)
	}

	if upstream.Credential != "" {
		options = append(options, transport.WithHTTPHeaders(map[string]string{
			"Authorization": upstream.Credential,
		}))
	}

	httpClient, err := client.NewStreamableHttpClient(upstream.URL, options...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Start the client before initialize to listen for notifications
	err = httpClient.Start(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start streamable client: %w", err)
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
	}
}

func toolsToServerTools(newTools []mcp.Tool) []server.ServerTool {
	tools := make([]server.ServerTool, 0)
	for _, newTool := range newTools {
		tools = append(tools, toolToServerTool(newTool))
	}

	return tools
}

// validateMCPServer validates a single MCP server using existing session data
func (m *mcpBrokerImpl) validateMCPServer(_ context.Context, mcpID, name, toolPrefix string) ServerValidationStatus {
	// Use already-discovered data for registered servers
	upstream, exists := m.mcpServers[upstreamMCPID(mcpID)]
	if !exists {
		m.logger.Warn("Server validation failed: server not registered", "id", mcpID, "name", name)
		return ServerValidationStatus{
			ID:         mcpID,
			Name:       name,
			ToolPrefix: toolPrefix,
			ConnectionStatus: ConnectionStatus{
				IsReachable: false,
				Error:       "Server not registered",
			},
		}
	}
	m.logger.Debug("validateMCPServer: checking MCPServer connection status", "servername", name)
	connectionStatus := m.checkSessionHealth(upstream)

	var protocolValidation ProtocolValidation
	var tools []mcp.Tool

	if upstream.initializeResult != nil {
		protocolValidation = validateProtocol(upstream.initializeResult)
		if !protocolValidation.IsValid {
			m.logger.Warn("validateMCPServer: Protocol validation failed", "id", mcpID,
				"expected", protocolValidation.ExpectedVersion,
				"actual", protocolValidation.SupportedVersion)
		}
	} else {
		m.logger.Warn("validateMCPServer: No initialization result available for validation", "id", mcpID)
		protocolValidation = ProtocolValidation{
			IsValid:         false,
			ExpectedVersion: mcp.LATEST_PROTOCOL_VERSION,
		}
	}

	if upstream.toolsResult != nil {
		tools = upstream.toolsResult.Tools
		if len(tools) == 0 {
			m.logger.Warn("validateMCPServer: Server has no tools available", "id", mcpID)
		}
	} else {
		m.logger.Warn("validateMCPServer: No tools result available for validation", "id", mcpID)
	}

	status := buildValidationStatus(connectionStatus, protocolValidation, tools, mcpID, name, toolPrefix)
	status.ToolConflicts = m.checkToolConflicts(tools, toolPrefix, mcpID)

	if len(status.ToolConflicts) > 0 {
		m.logger.Warn("Tool conflicts detected", "id", mcpID, "conflicts", len(status.ToolConflicts))
	}

	return status
}

// checkSessionHealth checks the health of an existing session and reinitializes if needed
func (m *mcpBrokerImpl) checkSessionHealth(upstream *upstreamMCP) ConnectionStatus {
	if upstream.mcpClient == nil {
		// Create a temporary validation-only session
		return m.validateServerConnectivity(upstream)
	}

	// Test existing session with a quick ping
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := upstream.mcpClient.Ping(ctx)
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
	client, _, err := m.createMCPClient(ctx, upstream.ID(), ClientTypeValidation)
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
		IsValid:          slices.Contains(mcp.ValidProtocolVersions, initResp.ProtocolVersion),
		SupportedVersion: initResp.ProtocolVersion,
		ExpectedVersion:  mcp.LATEST_PROTOCOL_VERSION,
	}
}

// buildValidationStatus constructs a ServerValidationStatus from validation results
func buildValidationStatus(connectionStatus ConnectionStatus, protocolValidation ProtocolValidation, tools []mcp.Tool, ID, name, toolPrefix string) ServerValidationStatus {
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
		ID:                     ID,
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
func (m *mcpBrokerImpl) checkToolConflicts(tools []mcp.Tool, toolPrefix, id string) []ToolConflict {
	var conflicts []ToolConflict

	for _, tool := range tools {
		prefixedToolName := fmt.Sprintf("%s%s", toolPrefix, tool.Name)
		var conflictingServers []string

		for existingToolName, existingToolInfo := range m.toolMapping {
			if string(existingToolName) == prefixedToolName && string(existingToolInfo.id) != id {
				conflictingServers = append(conflictingServers, string(existingToolInfo.id))
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

	for id, upstream := range m.mcpServers {
		status := m.validateMCPServer(ctx, string(id), upstream.Name, upstream.ToolPrefix)
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
