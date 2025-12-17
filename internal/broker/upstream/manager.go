/*
Package upstream is a package for managing upstream MCP servers
*/
package upstream

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AddToolsFunc is a callback function for adding tools to the gateway server
type AddToolsFunc func(tools ...server.ServerTool)

// RemoveToolsFunc is a callback function for removing tools from the gateway server by name
type RemoveToolsFunc func(tools ...string)

const (
	notificationToolsListChanged = "notifications/tools/list_changed"
)

// ServerValidationStatus contains the validation results for an upstream MCP server
type ServerValidationStatus struct {
	ID                     string                 `json:"id"`
	Name                   string                 `json:"name"`
	ToolPrefix             string                 `json:"toolPrefix"`
	ConnectionStatus       ConnectionStatus       `json:"connectionStatus"`
	ProtocolValidation     ProtocolValidation     `json:"protocolValidation"`
	CapabilitiesValidation CapabilitiesValidation `json:"capabilitiesValidation"`
	ToolConflicts          []ToolConflict         `json:"toolConflicts"`
	LastValidated          time.Time              `json:"lastValidated"`
}

// ConnectionStatus represents the connection health of an MCP server
type ConnectionStatus struct {
	IsReachable bool   `json:"isReachable"`
	Error       string `json:"error,omitempty"`
}

// ProtocolValidation represents the MCP protocol version validation results
type ProtocolValidation struct {
	IsValid          bool   `json:"isValid"`
	SupportedVersion string `json:"supportedVersion"`
	ExpectedVersion  string `json:"expectedVersion"`
}

// CapabilitiesValidation represents the capabilities validation results
type CapabilitiesValidation struct {
	IsValid             bool     `json:"isValid"`
	HasToolCapabilities bool     `json:"hasToolCapabilities"`
	ToolCount           int      `json:"toolCount"`
	MissingCapabilities []string `json:"missingCapabilities"`
}

// ToolConflict represents a tool name conflict between servers
type ToolConflict struct {
	ToolName      string   `json:"toolName"`
	PrefixedName  string   `json:"prefixedName"`
	ConflictsWith []string `json:"conflictsWith"`
}

// MCPManager manages a single backend MCPServer for the broker. It is the only thing that should be connecting to the MCP Server for the broker. External client connections are  not handled by this Manager and are not handled by the broker. It handles tools updates, disconnection, liveness checks for the broker and updating the overall status that is available from the /status endpoint. It is responsible for adding and removing tools to the broker to make them available via the gateway. It periodically pings and will fetch new tools based on notifications. It is intended to be long lived and have 1:1 relationship with a backend MCP server.
type MCPManager struct {
	UpstreamMCP     *MCPServer
	ticker          *time.Ticker // ticker allows for us to continue to probe and retry the backend
	addToolsFunc    AddToolsFunc
	removeToolsFunc RemoveToolsFunc
	// serverTools contains the managed MCP's tools with prefixed names. It is these that are externally available via the gateway
	serverTools []server.ServerTool
	// tools is the original set from MCP server with no prefix
	tools     []mcp.Tool
	logger    *slog.Logger
	toolsLock sync.RWMutex   // protects tools, serverTools
	stopOnce  sync.Once      // ensures Stop() is only executed once
	wg        sync.WaitGroup // tracks the Start() goroutine
	done      chan struct{}  // triggers the exit of the select and routine
}

// NewUpstreamMCPManager creates a new MCPManager for managing a single upstream MCP server.
// The addTools and removeTools callbacks are used to update the gateway's tool registry.
func NewUpstreamMCPManager(upstream *MCPServer, addTools AddToolsFunc, removeTools RemoveToolsFunc, logger *slog.Logger) *MCPManager {

	return &MCPManager{
		UpstreamMCP:     upstream,
		addToolsFunc:    addTools,
		removeToolsFunc: removeTools,
		logger:          logger.With("sub-component", "mcp manager"),
		serverTools:     []server.ServerTool{},
		tools:           []mcp.Tool{},
		done:            make(chan struct{}, 1),
	}
}

// MCPName returns the name of the upstream MCP server being managed
func (man *MCPManager) MCPName() string {
	return man.UpstreamMCP.Name
}

// Start begins the management loop for the upstream MCP server. It connects to
// the server, discovers tools, and periodically validates the connection. It also
// registers notification callbacks to handle tool list changes. This method blocks
// until Stop is called or the context is cancelled.
func (man *MCPManager) Start(ctx context.Context) {
	man.wg.Add(1)
	// this will be called once Stop is called
	defer man.wg.Done()

	// TODO make configurable
	man.ticker = time.NewTicker(time.Minute * 5)

	man.manage(ctx)

	man.registerCallbacks(ctx)

	for {
		select {
		case <-ctx.Done():
			man.Stop()
		case <-man.ticker.C:
			man.manage(ctx)
		case <-man.done:
			man.logger.Debug("shutting down manager for mcp ", "server id", man.UpstreamMCP.ID())
			return
		}
	}
}

// Stop gracefully shuts down the manager. It stops the ticker, removes all tools
// from the gateway, disconnects from the upstream server, and waits for the Start
// goroutine to complete. Safe to call multiple times.
func (man *MCPManager) Stop() {
	man.stopOnce.Do(func() {
		if man.ticker != nil {
			man.ticker.Stop()
		}
		man.removeTools()
		if err := man.UpstreamMCP.Disconnect(); err != nil {
			man.logger.Error(" manager stopping for ", "server id", man.UpstreamMCP.ID(), "error", err)
		}
		close(man.done)
		man.logger.Debug("stopped manager for ", "server id", man.UpstreamMCP.ID())
	})
	man.wg.Wait()
}

func (man *MCPManager) registerCallbacks(ctx context.Context) {
	man.UpstreamMCP.OnNotification(func(notification mcp.JSONRPCNotification) {
		if notification.Method == notificationToolsListChanged {
			man.logger.Debug(" received notification", "server", man.UpstreamMCP.ID(), "notification", notification)
			man.setTools(ctx)
			return
		}
	})

	man.UpstreamMCP.OnConnectionLost(func(err error) {
		// just logging for visibility as will be re-connected on next tick
		man.logger.Error("connection lost to ", "server id", man.UpstreamMCP.ID(), "error", err)
	})
}

func (man *MCPManager) manage(ctx context.Context) {
	man.logger.Debug("manage ", "upstream mcp", man.UpstreamMCP.ID())

	validationStatus := man.Validate(ctx)
	// TODO allow configuring connection attempts before removing tools
	if !validationStatus.ConnectionStatus.IsReachable {
		man.logger.Error("upstream is un-reachable. Removing tools from gateway", "err", validationStatus.ConnectionStatus.Error)
		if man.removeToolsFunc != nil {
			man.logger.Debug("tools list change remove all tools", "sever id", man.UpstreamMCP.ID())
			man.removeTools()
		}
		return
	}

	if len(man.tools) == 0 {
		man.logger.Debug("fetching tools fresh ", "serverid ", man.UpstreamMCP.ID())
		man.setTools(ctx)
	}
}

func (man *MCPManager) setTools(ctx context.Context) {
	man.logger.Debug("fetching tools fresh ", "serverid ", man.UpstreamMCP.ID())
	current, err := man.getTools(ctx)
	if err != nil {
		man.logger.Error("failed to get tools", "server id", man.UpstreamMCP.ID())
		return
	}
	// build prefixed server tools from the fetched tools
	newServerTools := make([]server.ServerTool, 0, len(current))
	for _, tool := range current {
		prefixedTool := tool
		newServerTools = append(newServerTools, man.toolToServerTool(prefixedTool))
	}

	man.toolsLock.Lock()
	man.tools = current
	man.serverTools = newServerTools
	man.toolsLock.Unlock()

	if len(newServerTools) > 0 {
		man.logger.Info("tools list change add via manage", "server id", man.UpstreamMCP.ID(), "total", len(current))
		man.addToolsFunc(man.serverTools...)
	}
}

// Validate checks the connection and capabilities of the upstream MCP server.
// It attempts to connect if not already connected, pings the server, and validates
// the protocol version and tool capabilities. Returns a ServerValidationStatus
// containing the results.
func (man *MCPManager) Validate(ctx context.Context) ServerValidationStatus {
	s := ServerValidationStatus{
		ID:                     string(man.UpstreamMCP.ID()),
		Name:                   man.UpstreamMCP.Name,
		ToolPrefix:             man.UpstreamMCP.ToolPrefix,
		ConnectionStatus:       ConnectionStatus{},
		ProtocolValidation:     ProtocolValidation{},
		CapabilitiesValidation: CapabilitiesValidation{},
		LastValidated:          time.Now(),
	}
	s.ProtocolValidation.ExpectedVersion = strings.Join(mcp.ValidProtocolVersions, ",")
	s.ProtocolValidation.ExpectedVersion = strings.Join(mcp.ValidProtocolVersions, ",")
	s.ConnectionStatus.IsReachable = false
	if man.UpstreamMCP.Client == nil {
		if err := man.UpstreamMCP.Connect(ctx); err != nil {
			man.logger.Error("mcp manager failed to connect to server ", "server id", man.UpstreamMCP.ID(), "error", err)
			s.ConnectionStatus.Error = err.Error()
		}
		s.ConnectionStatus.IsReachable = true
	}
	// always ping to verify connection if client exists
	if man.UpstreamMCP.Client != nil {
		if err := man.UpstreamMCP.Ping(ctx); err != nil {
			man.logger.Error("mcp manager failed to ping to server ", "server id", man.UpstreamMCP.ID(), "error", err)
			s.ConnectionStatus.Error = err.Error()
			s.ConnectionStatus.IsReachable = false
		} else {
			s.ConnectionStatus.IsReachable = true
		}
	}

	s.ProtocolValidation.IsValid = false
	if man.UpstreamMCP.init != nil {
		s.ProtocolValidation.IsValid = slices.Contains(mcp.ValidProtocolVersions, man.UpstreamMCP.init.ProtocolVersion)
	}
	s.CapabilitiesValidation.HasToolCapabilities = false
	s.CapabilitiesValidation.IsValid = false
	if man.UpstreamMCP.init != nil && man.UpstreamMCP.init.Capabilities.Tools != nil {
		s.CapabilitiesValidation.HasToolCapabilities = true
		s.CapabilitiesValidation.IsValid = true
		s.CapabilitiesValidation.ToolCount = len(man.serverTools)
	}
	return s
}

func (man *MCPManager) getTools(ctx context.Context) ([]mcp.Tool, error) {
	res, err := man.UpstreamMCP.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// GetManagedTools returns a copy of all tools discovered from the upstream server.
// The returned tools have their original names without the gateway prefix.
func (man *MCPManager) GetManagedTools() []mcp.Tool {
	man.toolsLock.RLock()
	defer man.toolsLock.RUnlock()
	// return a copy to avoid races
	result := make([]mcp.Tool, len(man.tools))
	copy(result, man.tools)
	return result
}

// SetToolsForTesting sets the tools directly for testing purposes.
// This bypasses the normal tool discovery flow and should only be used in tests.
// TODO look to remove the need for this
func (man *MCPManager) SetToolsForTesting(tools []mcp.Tool) {
	man.toolsLock.Lock()
	defer man.toolsLock.Unlock()
	man.tools = tools
}

// GetManagedTool returns a copy of a specific tool by its original name (without prefix).
// Returns nil if the tool is not found.
func (man *MCPManager) GetManagedTool(name string) *mcp.Tool {
	man.toolsLock.RLock()
	defer man.toolsLock.RUnlock()
	for _, t := range man.tools {
		if t.Name == name {
			copyTool := t
			return &copyTool
		}
	}
	return nil
}

func (man *MCPManager) removeTools() {
	man.toolsLock.Lock()
	toolsToRemove := make([]string, 0, len(man.serverTools))
	for _, tool := range man.serverTools {
		toolsToRemove = append(toolsToRemove, tool.Tool.Name)
	}
	man.serverTools = nil
	man.tools = nil
	man.toolsLock.Unlock()
	man.logger.Debug("tools list change remove all tools ", "server id", man.UpstreamMCP.ID())
	man.removeToolsFunc(toolsToRemove...)

}

func (man *MCPManager) toolToServerTool(newTool mcp.Tool) server.ServerTool {
	newTool.Name = prefixedName(man.UpstreamMCP, newTool.Name)
	return server.ServerTool{
		Tool: newTool,
		Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultError("Kagenti MCP Broker doesn't forward tool calls"), nil
		},
	}
}

func (man *MCPManager) diffTools(oldTools, newTools []mcp.Tool) ([]server.ServerTool, []string) {
	oldToolMap := make(map[string]mcp.Tool)
	for _, oldTool := range oldTools {
		oldToolMap[oldTool.Name] = oldTool
	}

	newToolMap := make(map[string]mcp.Tool)
	for _, newTool := range newTools {
		newToolMap[newTool.Name] = newTool
	}

	addedTools := make([]server.ServerTool, 0)
	for _, newTool := range newToolMap {
		_, ok := oldToolMap[newTool.Name]
		if !ok {
			addedTools = append(addedTools, man.toolToServerTool(newTool))
		}
	}

	removedTools := make([]string, 0)
	for _, oldTool := range oldToolMap {
		_, ok := newToolMap[oldTool.Name]
		if !ok {
			removedTools = append(removedTools, oldTool.Name)
		}
	}

	return addedTools, removedTools
}

func prefixedName(up *MCPServer, tool string) string {
	return fmt.Sprintf("%s%s", up.ToolPrefix, tool)
}
