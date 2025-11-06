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
		s.Logger.Info("No mcp session id found for", "serverName", serverName, "gateway session", gwSessionID)
		sessionID, err := s.Broker.CreateSession(ctx, authority)
		if err != nil {
			return "", fmt.Errorf("failed to create session: %w", err)
		}
		s.Logger.Info("Created MCP session ", "sessionID", sessionID, "server", serverName, "host", authority)
		return sessionID, nil
	})
}

// Process function
func (s *ExtProcServer) Process(stream extProcV3.ExternalProcessor_ProcessServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			s.Logger.Error("[ext_proc] Process: Error receiving request", "error", err)
			return err
		}
		responseBuilder := NewResponse()
		// Log request details
		switch r := req.Request.(type) {
		case *extProcV3.ProcessingRequest_RequestHeaders:
			// TODO we are ignoring errors here
			responses, _ := s.HandleRequestHeaders(r.RequestHeaders)
			// Store the processed request headers for later use in body processing
			s.requestHeaders = r.RequestHeaders
			s.Logger.Debug("post request headers handler ", "headers", s.requestHeaders)
			for _, response := range responses {
				s.Logger.Info(fmt.Sprintf("Sending header processing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue

		case *extProcV3.ProcessingRequest_RequestBody:
			mcpRequest := &MCPRequest{}
			// default response
			responses := responseBuilder.WithDoNothingResponse(s.streaming).Build()
			if s.requestHeaders == nil || s.requestHeaders.Headers == nil {
				s.Logger.Error("Error no request headers present. Exiting ")
				for _, response := range responses {
					if err := stream.Send(response); err != nil {
						s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
						return fmt.Errorf("no request headers present")
					}
				}
			}

			if len(r.RequestBody.Body) > 0 {
				if err := json.Unmarshal(r.RequestBody.Body, &mcpRequest); err != nil {
					s.Logger.Error(fmt.Sprintf("Error unmarshalling request body: %v", err))
					for _, response := range responses {
						if err := stream.Send(response); err != nil {
							s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
							return err
						}
					}
				}
			}
			if _, err := mcpRequest.Validate(); err != nil {
				s.Logger.Error(fmt.Sprintf("Error request is not valid MCPRequest: %v", err))
				for _, response := range responses {
					if err := stream.Send(response); err != nil {
						s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
						return err
					}
				}
			}
			// override responses with custom handle responses
			responses = s.HandleMCPRequest(stream.Context(), mcpRequest, s.RoutingConfig)
			for _, response := range responses {
				s.Logger.Info(fmt.Sprintf("Sending MCP body routing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue

		case *extProcV3.ProcessingRequest_ResponseHeaders:
			responses, _ := s.HandleResponseHeaders(r.ResponseHeaders)
			for _, response := range responses {
				s.Logger.Info(fmt.Sprintf("Sending response header processing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue
		case *extProcV3.ProcessingRequest_ResponseBody:
			responses, _ := s.HandleResponseBody(r.ResponseBody)
			for _, response := range responses {
				s.Logger.Info(fmt.Sprintf("Sending response body processing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue
		}
	}
}
