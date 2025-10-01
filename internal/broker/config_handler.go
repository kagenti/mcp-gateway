package broker

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/kagenti/mcp-gateway/internal/config"
	"sigs.k8s.io/yaml"
)

// ConfigUpdateHandler handles dynamic configuration updates via HTTP
// Each broker instance receives the same config from the controller via the service
type ConfigUpdateHandler struct {
	config    *config.MCPServersConfig
	authToken string
	logger    *slog.Logger
}

// NewConfigUpdateHandler creates a new config update handler
func NewConfigUpdateHandler(cfg *config.MCPServersConfig, authToken string, logger *slog.Logger) *ConfigUpdateHandler {
	if cfg == nil {
		panic("cfg cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &ConfigUpdateHandler{
		config:    cfg,
		authToken: authToken,
		logger:    logger,
	}
}

// UpdateConfig handles config update requests
func (h *ConfigUpdateHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	// method check handled by mux with "POST /config" pattern
	if h.authToken != "" {
		token := r.Header.Get("Authorization")
		expectedToken := "Bearer " + h.authToken
		if token != expectedToken {
			h.logger.Warn("unauthorized config update attempt")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read request body", "error", err)
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	var configData struct {
		Servers        []*config.MCPServer     `yaml:"servers"`
		VirtualServers []*config.VirtualServer `yaml:"virtualServers"`
	}

	err = yaml.Unmarshal(body, &configData)
	if err != nil {
		h.logger.Error("failed to parse config", "error", err)
		http.Error(w, "Invalid YAML format", http.StatusBadRequest)
		return
	}

	h.config.Servers = configData.Servers
	// Only update VirtualServers if they were provided in the config
	if configData.VirtualServers != nil {
		h.config.VirtualServers = configData.VirtualServers
	}

	h.config.Notify(r.Context())

	h.logger.Info("configuration updated via API", "serverCount", len(configData.Servers), "virtualServerCount", len(configData.VirtualServers))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Configuration updated with %d servers", len(configData.Servers)),
	}
	_ = json.NewEncoder(w).Encode(response)
}
