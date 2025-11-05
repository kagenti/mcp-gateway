// Package mcprouter ext proc process
package mcprouter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	// "github.com/kagenti/mcp-gateway/internal/config"

	extProcV3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/kagenti/mcp-gateway/internal/broker"
	"github.com/kagenti/mcp-gateway/internal/cache"
	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/kagenti/mcp-gateway/internal/session"
)

var _ config.Observer = &ExtProcServer{}

// ExtProcServer struct boolean for streaming & Store headers for later use in body processing
type ExtProcServer struct {
	RoutingConfig  *config.MCPServersConfig
	Broker         broker.MCPBroker
	SessionCache   *cache.Cache
	JWTManager     *session.JWTManager
	Logger         *slog.Logger
	streaming      bool
	requestHeaders *extProcV3.HttpHeaders
}

// OnConfigChange is used to register the router for config changes
func (s *ExtProcServer) OnConfigChange(_ context.Context, newConfig *config.MCPServersConfig) {
	s.RoutingConfig = newConfig
}

// SetupSessionCache initializes the session cache with broker's real MCP initialization logic
func (s *ExtProcServer) SetupSessionCache() {
	s.SessionCache = cache.New(func(
		ctx context.Context,
		serverName string,
		authority string,
		gwSessionID string,
	) (string, error) {

		// Checks if the authority is provided
		if authority == "" {
			return "", fmt.Errorf("no authority provided for server: %s", serverName)
		}

		// Creates a MCP session
		slog.Info("No mcp session id found for", "serverName", serverName, "gateway session", gwSessionID)
		sessionID, err := s.Broker.CreateSession(ctx, authority)
		if err != nil {
			return "", fmt.Errorf("failed to create session: %w", err)
		}
		slog.Info("Created MCP session ", "sessionID", sessionID, "server", serverName, "host", authority)
		return sessionID, nil
	})
}

// Process function
func (s *ExtProcServer) Process(stream extProcV3.ExternalProcessor_ProcessServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			slog.Error("[ext_proc] Process: Error receiving request", "error", err)
			return err
		}

		// Log request details
		switch r := req.Request.(type) {
		case *extProcV3.ProcessingRequest_RequestHeaders:
			// Store headers for later use in body processing
			s.requestHeaders = r.RequestHeaders
			responses, _ := s.HandleRequestHeaders(s.requestHeaders)
			for _, response := range responses {
				slog.Info(fmt.Sprintf("Sending header processing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					slog.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue

		case *extProcV3.ProcessingRequest_RequestBody:
			var mcpRequest = &MCPRequest{}
			var responses []*extProcV3.ProcessingResponse
			if s.requestHeaders == nil || s.requestHeaders.Headers == nil {
				slog.Error(fmt.Sprintf("Error unmarshalling request body: %v", err))
				if err := stream.Send(s.doNothing()); err != nil {
					slog.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}

			if len(r.RequestBody.Body) > 0 {
				if err := json.Unmarshal(r.RequestBody.Body, &mcpRequest); err != nil {
					slog.Error(fmt.Sprintf("Error unmarshalling request body: %v", err))
					if err := stream.Send(s.doNothing()); err != nil {
						slog.Error(fmt.Sprintf("Error sending response: %v", err))
						return err
					}
				}
			}
			if _, err := mcpRequest.Validate(); err != nil {
				slog.Error(fmt.Sprintf("Error request is not valid MCPRequest: %v", err))
				if err := stream.Send(s.doNothing()); err != nil {
					slog.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}

			}

			responses = s.HandleMCPRequest(stream.Context(), mcpRequest, s.RoutingConfig)
			for _, response := range responses {
				slog.Info(fmt.Sprintf("Sending MCP routing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					slog.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue

		case *extProcV3.ProcessingRequest_ResponseHeaders:
			responses, _ := s.HandleResponseHeaders(r.ResponseHeaders)
			for _, response := range responses {
				slog.Info(fmt.Sprintf("Sending response header processing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					slog.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue
		case *extProcV3.ProcessingRequest_ResponseBody:
			responses, _ := s.HandleResponseBody(r.ResponseBody)
			for _, response := range responses {
				slog.Info(fmt.Sprintf("Sending response body processing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					slog.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue
		}
	}

}
