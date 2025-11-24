// Package mcprouter ext proc process
package mcprouter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	extProcV3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/kagenti/mcp-gateway/internal/broker"
	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/kagenti/mcp-gateway/internal/session"
	"github.com/mark3labs/mcp-go/client"
)

var _ config.Observer = &ExtProcServer{}

// SessionCache defines how the router interacts with a store to store and retrieves sessions
type SessionCache interface {
	GetSession(ctx context.Context, key string) (map[string]string, error)
	AddSession(ctx context.Context, key, mcpID, mcpSession string) (bool, error)
	DeleteSessions(ctx context.Context, key ...string) error
	RemoveServerSession(ctx context.Context, key, mcpServerID string) error
	KeyExists(ctx context.Context, key string) (bool, error)
}

// InitForClient defines a function for initializing an MCP server for a client
type InitForClient func(ctx context.Context, gatewayHost, routerKey string, conf *config.MCPServer, passThroughHeaders map[string]string) (*client.Client, error)

// ExtProcServer struct boolean for streaming & Store headers for later use in body processing
type ExtProcServer struct {
	RoutingConfig *config.MCPServersConfig
	JWTManager    *session.JWTManager
	Logger        *slog.Logger
	InitForClient InitForClient
	SessionCache  SessionCache
	//TODO this should not be needed
	Broker broker.MCPBroker
}

// OnConfigChange is used to register the router for config changes
func (s *ExtProcServer) OnConfigChange(_ context.Context, newConfig *config.MCPServersConfig) {
	s.RoutingConfig = newConfig
}

// Process function
func (s *ExtProcServer) Process(stream extProcV3.ExternalProcessor_ProcessServer) error {
	var (
		localRequestHeaders *extProcV3.HttpHeaders
		streaming           = false
		mcpRequest          *MCPRequest
	)
	for {
		req, err := stream.Recv()

		if err != nil {
			s.Logger.Error("[ext_proc] Process: Error receiving request", "error", err)
			return err
		}
		responseBuilder := NewResponse()
		switch r := req.Request.(type) {
		case *extProcV3.ProcessingRequest_RequestHeaders:
			// TODO we are ignoring errors here
			localRequestHeaders = r.RequestHeaders
			responses, _ := s.HandleRequestHeaders(r.RequestHeaders)
			s.Logger.Debug("[ext_proc ] Process: request headers", "local headers ", localRequestHeaders.Headers)
			for _, response := range responses {
				s.Logger.Debug(fmt.Sprintf("Sending header processing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue

		case *extProcV3.ProcessingRequest_RequestBody:
			// default response
			responses := responseBuilder.WithDoNothingResponse(streaming).Build()
			if localRequestHeaders == nil || localRequestHeaders.Headers == nil {
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
				if _, err := mcpRequest.Validate(); err != nil {
					s.Logger.Error("Invalid MCPRequest", "error", err)
					resp := responseBuilder.WithImmediateResponse(400, "invalid mcp request").Build()
					for _, res := range resp {
						if err := stream.Send(res); err != nil {
							s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
							return err
						}
					}
				}
			}
			// override responses with custom handle responses
			// GET /mcp would come through here
			mcpRequest.Headers = localRequestHeaders.Headers
			mcpRequest.Streaming = streaming
			responses = s.RouteMCPRequest(stream.Context(), mcpRequest)
			for _, response := range responses {
				s.Logger.Debug(fmt.Sprintf("Sending MCP body routing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue

		case *extProcV3.ProcessingRequest_ResponseHeaders:
			responses, _ := s.HandleResponseHeaders(stream.Context(), r.ResponseHeaders, localRequestHeaders, mcpRequest)
			for _, response := range responses {
				s.Logger.Debug(fmt.Sprintf("Sending response header processing instructions to Envoy: %+v", response))
				if err := stream.Send(response); err != nil {
					s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
					return err
				}
			}
			continue
		case *extProcV3.ProcessingRequest_ResponseBody:
			// This should never be called as response_body_mode is set to NONE in the EnvoyFilter.
			// If this is called, it indicates a configuration issue or Envoy bug.
			s.Logger.Error("[EXT-PROC] Unexpected response body processing request received",
				"size", len(r.ResponseBody.GetBody()),
				"end_of_stream", r.ResponseBody.GetEndOfStream(),
				"note", "response_body_mode is set to NONE in EnvoyFilter - this should not occur")
			// Return empty response to satisfy the interface
			response := &extProcV3.ProcessingResponse{
				Response: &extProcV3.ProcessingResponse_ResponseBody{
					ResponseBody: &extProcV3.BodyResponse{},
				},
			}
			if err := stream.Send(response); err != nil {
				s.Logger.Error(fmt.Sprintf("Error sending response: %v", err))
				return err
			}
			continue
		}
	}
}
