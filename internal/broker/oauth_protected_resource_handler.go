package broker

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

const (
	ENV_OAUTH_RESOURCE_NAME            = "OAUTH_RESOURCE_NAME"
	ENV_OAUTH_RESOURCE                 = "OAUTH_RESOURCE"
	ENV_OAUTH_AUTHORIZATION_SERVERS    = "OAUTH_AUTHORIZATION_SERVERS"
	ENV_OAUTH_BEARER_METHODS_SUPPORTED = "OAUTH_BEARER_METHODS_SUPPORTED"
	ENV_OAUTH_SCOPES_SUPPORTED         = "OAUTH_SCOPES_SUPPORTED"
)

type ProtectedResourceHandler struct {
	Logger *slog.Logger
}

// OAuthProtectedResource represents the OAuth protected resource response
type OAuthProtectedResource struct {
	ResourceName           string   `json:"resource_name"`
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ScopesSupported        []string `json:"scopes_supported"`
}

// getOAuthConfig parses OAuth configuration from environment variables
func getOAuthConfig() *OAuthProtectedResource {
	// Set defaults
	oauthConfig := &OAuthProtectedResource{
		ResourceName:           "MCP Server",
		Resource:               "/mcp",
		AuthorizationServers:   []string{},
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        []string{"basic"},
	}

	// Override with environment variables if provided
	if resourceName := os.Getenv(ENV_OAUTH_RESOURCE_NAME); resourceName != "" {
		oauthConfig.ResourceName = resourceName
	}

	if resource := os.Getenv(ENV_OAUTH_RESOURCE); resource != "" {
		oauthConfig.Resource = resource
	}

	if authServers := os.Getenv(ENV_OAUTH_AUTHORIZATION_SERVERS); authServers != "" {
		// Split by comma and trim whitespace
		servers := strings.Split(authServers, ",")
		oauthConfig.AuthorizationServers = make([]string, len(servers))
		for i, server := range servers {
			oauthConfig.AuthorizationServers[i] = strings.TrimSpace(server)
		}
	}

	if bearerMethods := os.Getenv(ENV_OAUTH_BEARER_METHODS_SUPPORTED); bearerMethods != "" {
		// Split by comma and trim whitespace
		methods := strings.Split(bearerMethods, ",")
		oauthConfig.BearerMethodsSupported = make([]string, len(methods))
		for i, method := range methods {
			oauthConfig.BearerMethodsSupported[i] = strings.TrimSpace(method)
		}
	}

	if scopes := os.Getenv(ENV_OAUTH_SCOPES_SUPPORTED); scopes != "" {
		// Split by comma and trim whitespace
		scopeList := strings.Split(scopes, ",")
		oauthConfig.ScopesSupported = make([]string, len(scopeList))
		for i, scope := range scopeList {
			oauthConfig.ScopesSupported[i] = strings.TrimSpace(scope)
		}
	}

	return oauthConfig
}

// oauthProtectedResourceHandler handles the /.well-known/oauth-protected-resource endpoint
func (prh *ProtectedResourceHandler) ProtectedResourceHandler(w http.ResponseWriter, r *http.Request) {
	prh.Logger.Info("service protected resource endpoint")
	oauthConfig := getOAuthConfig()
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, Origin, X-Requested-With")
	w.Header().Set("Access-Control-Max-Age", "3600")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Handle preflight requests
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Set content type and return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	prh.Logger.Debug("oauth protected resource", "config", oauthConfig)
	if err := json.NewEncoder(w).Encode(oauthConfig); err != nil {
		prh.Logger.Error("Failed to encode OAuth protected resource response", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}
