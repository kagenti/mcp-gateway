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
)

// ExtProcServer struct boolean for streaming & Store headers for later use in body processing
type ExtProcServer struct {
	MCPConfig      *config.MCPServersConfig
	Broker         broker.MCPBroker
	SessionCache   *cache.Cache
	streaming      bool
	requestHeaders *extProcV3.HttpHeaders
	Logger         *slog.Logger
}

// SetupSessionCache initializes the session cache with broker's real MCP initialization logic
func (s *ExtProcServer) SetupSessionCache() {
	s.SessionCache = cache.New(func(
		ctx context.Context,
		serverName string,
		authority string,
		gwSessionID string,
	) (string, error) {
		s.Logger.Info(
			"Creating real MCP session via broker createUpstreamSession",
			"server", serverName,
			"authority", authority,
			"gwSession", gwSessionID,
		)

		// Use the provided authority directly (no config lookup needed)
		if authority == "" {
			return "", fmt.Errorf("no authority provided for server: %s", serverName)
		}

		// Use broker's CreateSession method
		sessionID, err := s.Broker.CreateSession(ctx, authority)
		if err != nil {
			return "", fmt.Errorf("failed to create session via broker: %w", err)
		}
		s.Logger.Info(
			"Created real MCP session via broker",
			"sessionID", sessionID,
			"server", serverName,
			"authority", authority,
		)
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

		// Log request details
		switch r := req.Request.(type) {
		case *extProcV3.ProcessingRequest_RequestHeaders:
			// Store headers for later use in body processing
			s.requestHeaders = r.RequestHeaders
			responses, _ := s.HandleRequestHeaders(s.requestHeaders)
			for _, response := range responses {
				slog.Info(fmt.Sprintf("Outgoing processing response: %+v", response))
				if err := stream.Send(response); err != nil {
					slog.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			// slog.Info("Request Headers: %+v", r.RequestHeaders)

		case *extProcV3.ProcessingRequest_RequestBody:
			var data map[string]any
			if len(r.RequestBody.Body) > 0 {
				if err := json.Unmarshal(r.RequestBody.Body, &data); err != nil {
					slog.Error(fmt.Sprintf("Error unmarshalling request body: %v", err))
				}
			}
			if data == nil {
				for _, response := range s.createEmptyBodyResponse() {
					slog.Info(fmt.Sprintf("Outgoing processing response: %+v", response))
					if err := stream.Send(response); err != nil {
						slog.Error(fmt.Sprintf("Error sending response: %v", err))
						return err
					}
				}
				continue
			}
			responses, _ := s.HandleRequestBody(stream.Context(), data, s.MCPConfig)
			for _, response := range responses {
				slog.Info(fmt.Sprintf("Outgoing processing response: %+v", response))
				if err := stream.Send(response); err != nil {
					slog.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
		case *extProcV3.ProcessingRequest_ResponseHeaders:
			responses, _ := s.HandleResponseHeaders(r.ResponseHeaders)
			for _, response := range responses {
				slog.Info(fmt.Sprintf("Outgoing processing response: %+v", response))
				if err := stream.Send(response); err != nil {
					slog.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
		case *extProcV3.ProcessingRequest_ResponseBody:
			responses, _ := s.HandleResponseBody(r.ResponseBody)
			for _, response := range responses {
				slog.Info(fmt.Sprintf("Outgoing processing response: %+v", response))
				if err := stream.Send(response); err != nil {
					slog.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
		}
	}
}
