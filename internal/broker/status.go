package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type ServerValidationStatus struct {
	URL                    string                 `json:"url"`
	Name                   string                 `json:"name"`
	ToolPrefix             string                 `json:"toolPrefix"`
	ConnectionStatus       ConnectionStatus       `json:"connectionStatus"`
	ProtocolValidation     ProtocolValidation     `json:"protocolValidation"`
	CapabilitiesValidation CapabilitiesValidation `json:"capabilitiesValidation"`
	ToolConflicts          []ToolConflict         `json:"toolConflicts"`
	LastValidated          time.Time              `json:"lastValidated"`
}

type ConnectionStatus struct {
	IsReachable    bool   `json:"isReachable"`
	Error          string `json:"error,omitempty"`
	HTTPStatusCode int    `json:"httpStatusCode,omitempty"`
}

type ProtocolValidation struct {
	IsValid          bool   `json:"isValid"`
	SupportedVersion string `json:"supportedVersion"`
	ExpectedVersion  string `json:"expectedVersion"`
}

type CapabilitiesValidation struct {
	IsValid             bool     `json:"isValid"`
	HasToolCapabilities bool     `json:"hasToolCapabilities"`
	ToolCount           int      `json:"toolCount"`
	MissingCapabilities []string `json:"missingCapabilities"`
}

type ToolConflict struct {
	ToolName      string   `json:"toolName"`
	PrefixedName  string   `json:"prefixedName"`
	ConflictsWith []string `json:"conflictsWith"`
}

type StatusResponse struct {
	Servers          []ServerValidationStatus `json:"servers"`
	OverallValid     bool                     `json:"overallValid"`
	TotalServers     int                      `json:"totalServers"`
	HealthyServers   int                      `json:"healthyServers"`
	UnHealthyServers int                      `json:"unHealthyServers"`
	ToolConflicts    int                      `json:"toolConflicts"`
	Timestamp        time.Time                `json:"timestamp"`
}

// StatusHandler handles HTTP requests to the status endpoint
type StatusHandler struct {
	broker MCPBroker
	logger slog.Logger
}

func NewStatusHandler(broker MCPBroker, logger slog.Logger) *StatusHandler {
	return &StatusHandler{
		broker: broker,
		logger: logger,
	}
}

// ServeHTTP implements http.Handler interface
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.setResponseHeaders(w, r)
	
	switch r.Method {
	case http.MethodGet:
		h.handleGetStatus(w, r)
	default:
		h.sendErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed. Supported methods: GET")
	}
}

func (h *StatusHandler) setResponseHeaders(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func (h *StatusHandler) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse URL path to check for specific server request
	path := strings.TrimPrefix(r.URL.Path, "/status")
	if path != "" && path != "/" {
		// Remove leading slash and extract server name
		serverName := strings.TrimPrefix(path, "/")
		if serverName != "" {
			h.handleSingleServerByName(w, ctx, serverName)
			return
		}
	}

	// Check if requesting specific server status via query parameter (legacy support)
	serverURL := r.URL.Query().Get("server")
	if serverURL != "" {
		h.handleSingleServerStatus(w, r, ctx, serverURL)
		return
	}

	response := h.broker.ValidateAllServers(ctx)
	h.sendJSONResponse(w, http.StatusOK, response)
}

func (h *StatusHandler) handleSingleServerByName(w http.ResponseWriter, ctx context.Context, serverName string) {
	statusResponse := h.broker.ValidateAllServers(ctx)

	var serverStatus *ServerValidationStatus
	for _, server := range statusResponse.Servers {
		if server.Name == serverName {
			serverStatus = &server
			break
		}
		if strings.Contains(server.Name, "/") {
			parts := strings.Split(server.Name, "/")
			if len(parts) == 2 && parts[1] == serverName {
				serverStatus = &server
				break
			}
		}
	}

	if serverStatus == nil {
		h.sendErrorResponse(w, http.StatusNotFound, fmt.Sprintf("Server '%s' not found", serverName))
		return
	}

	h.logger.Info("Retrieved status for specific server", "serverName", serverName)
	h.sendJSONResponse(w, http.StatusOK, serverStatus)
}

// handleSingleServerStatus handles requests for a specific server's status
func (h *StatusHandler) handleSingleServerStatus(w http.ResponseWriter, _ *http.Request, ctx context.Context, serverURL string) {
	brokerImpl, ok := h.broker.(*mcpBrokerImpl)
	if !ok {
		h.sendErrorResponse(w, http.StatusInternalServerError, "Invalid broker implementation")
		return
	}

	// Find the server in our registered servers
	upstream, exists := brokerImpl.mcpServers[upstreamMCPURL(serverURL)]
	if !exists {
		h.sendErrorResponse(w, http.StatusNotFound, fmt.Sprintf("Server %s is not registered", serverURL))
		return
	}

	status := brokerImpl.validateMCPServer(ctx, serverURL, upstream.Name, upstream.ToolPrefix)
	h.sendJSONResponse(w, http.StatusOK, status)
}

func (h *StatusHandler) sendJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (h *StatusHandler) sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := map[string]string{"error": message}
	h.sendJSONResponse(w, statusCode, response)
}
