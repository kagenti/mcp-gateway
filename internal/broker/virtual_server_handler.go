package broker

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// VirtualServerHandler wraps an MCP handler to provide virtual server tool filtering
type VirtualServerHandler struct {
	mcpHandler http.Handler
	config     *config.MCPServersConfig
	logger     *slog.Logger
}

// NewVirtualServerHandler creates a new virtual server handler
func NewVirtualServerHandler(mcpHandler http.Handler, config *config.MCPServersConfig, logger *slog.Logger) *VirtualServerHandler {
	return &VirtualServerHandler{
		mcpHandler: mcpHandler,
		config:     config,
		logger:     logger,
	}
}

// ServeHTTP implements http.Handler interface
func (h *VirtualServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only intercept POST requests (MCP requests) for tools/list
	if r.Method != http.MethodPost {
		h.mcpHandler.ServeHTTP(w, r)
		return
	}

	// Check for x-mcp-virtualserver header
	virtualServerHeader := r.Header.Get("x-mcp-virtualserver")
	if virtualServerHeader == "" {
		// No virtual server specified, pass through without filtering
		h.mcpHandler.ServeHTTP(w, r)
		return
	}

	// Read the request body to check if it's a tools/list request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read request body", "error", err)
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}

	// Parse the JSON-RPC request
	var jsonRPCRequest mcp.JSONRPCRequest
	if err := json.Unmarshal(body, &jsonRPCRequest); err != nil {
		h.logger.Debug("Failed to parse JSON-RPC request, passing through", "error", err)
		// If we can't parse it, just pass it through
		r.Body = io.NopCloser(bytes.NewReader(body))
		h.mcpHandler.ServeHTTP(w, r)
		return
	}

	// Check if this is a tools/list request
	if jsonRPCRequest.Method != "tools/list" {
		// Not a tools/list request, pass through
		r.Body = io.NopCloser(bytes.NewReader(body))
		h.mcpHandler.ServeHTTP(w, r)
		return
	}

	h.logger.Debug("Processing tools/list request with virtual server filtering",
		"virtualServer", virtualServerHeader)

	// Handle the tools/list request with virtual server filtering
	h.handleToolsListWithFiltering(w, r, jsonRPCRequest, virtualServerHeader, body)
}

// handleToolsListWithFiltering handles tools/list requests with virtual server filtering
func (h *VirtualServerHandler) handleToolsListWithFiltering(
	w http.ResponseWriter,
	r *http.Request,
	jsonRPCRequest mcp.JSONRPCRequest,
	virtualServerName string,
	originalBody []byte,
) {
	// First, get the full list of tools by calling the original handler
	r.Body = io.NopCloser(bytes.NewReader(originalBody))

	// Create a custom ResponseWriter to capture the response
	responseCapture := &responseWriter{
		header: make(http.Header),
		body:   &bytes.Buffer{},
	}

	h.mcpHandler.ServeHTTP(responseCapture, r)

	// Check if this is an error response first
	var jsonRPCError mcp.JSONRPCError
	if err := json.Unmarshal(responseCapture.body.Bytes(), &jsonRPCError); err == nil && jsonRPCError.Error.Code != 0 {
		// Pass through error responses
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(responseCapture.statusCode)
		w.Write(responseCapture.body.Bytes())
		return
	}

	// Parse the response as a successful response
	var toolsResponse mcp.JSONRPCResponse
	if err := json.Unmarshal(responseCapture.body.Bytes(), &toolsResponse); err != nil {
		h.logger.Error("Failed to parse tools/list response", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Parse the result as ListToolsResult
	var listToolsResult mcp.ListToolsResult
	resultBytes, err := json.Marshal(toolsResponse.Result)
	if err != nil {
		h.logger.Error("Failed to marshal tools result", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := json.Unmarshal(resultBytes, &listToolsResult); err != nil {
		h.logger.Error("Failed to unmarshal tools result", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Filter tools based on virtual server configuration
	filteredTools := h.filterToolsForVirtualServer(listToolsResult.Tools, virtualServerName)

	// Create the filtered response
	filteredResult := mcp.ListToolsResult{
		Tools: filteredTools,
	}

	filteredResponse := mcp.JSONRPCResponse{
		JSONRPC: toolsResponse.JSONRPC,
		ID:      toolsResponse.ID,
		Result:  filteredResult,
	}

	// Send the filtered response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(filteredResponse); err != nil {
		h.logger.Error("Failed to encode filtered response", "error", err)
		return
	}

	h.logger.Debug("Sent filtered tools/list response",
		"virtualServer", virtualServerName,
		"originalToolCount", len(listToolsResult.Tools),
		"filteredToolCount", len(filteredTools))
}

// filterToolsForVirtualServer filters tools based on virtual server configuration
func (h *VirtualServerHandler) filterToolsForVirtualServer(tools []mcp.Tool, virtualServerName string) []mcp.Tool {
	// Find the virtual server configuration
	var virtualServer *config.VirtualServer
	for _, vs := range h.config.VirtualServers {
		if vs.Name == virtualServerName {
			virtualServer = vs
			break
		}
	}

	// If virtual server not found, return empty list
	if virtualServer == nil {
		h.logger.Warn("Virtual server not found", "virtualServer", virtualServerName)
		return []mcp.Tool{}
	}

	// Filter tools to only include those specified in the virtual server
	var filteredTools []mcp.Tool
	for _, tool := range tools {
		if slices.Contains(virtualServer.Tools, tool.Name) {
			filteredTools = append(filteredTools, tool)
		}
	}

	return filteredTools
}

// responseWriter is a custom ResponseWriter to capture responses
type responseWriter struct {
	header     http.Header
	body       *bytes.Buffer
	statusCode int
}

func (rw *responseWriter) Header() http.Header {
	return rw.header
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	return rw.body.Write(data)
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
}