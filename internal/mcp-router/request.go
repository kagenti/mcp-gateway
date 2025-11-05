package mcprouter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"errors"

	basepb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	eppb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typepb "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/kagenti/mcp-gateway/internal/config"
)

// ErrInvalidRequest is an error for an invalid request
var ErrInvalidRequest = fmt.Errorf("MCP Request is invalid")

// MCPRequest encapsulates a mcp protocol request to the gateway
type MCPRequest struct {
	ID        *int           `json:"id"`
	JSONRPC   string         `json:"jsonrpc"`
	Method    string         `json:"method"`
	Params    map[string]any `json:"params"`
	SessionID string         `json:"-"`
}

// Validate validates the mcp request
func (mr *MCPRequest) Validate() (bool, error) {
	if mr.JSONRPC != "2.0" {
		return false, errors.Join(ErrInvalidRequest, fmt.Errorf("json rpc version invalid"))
	}
	if mr.Method == "" {
		return false, errors.Join(ErrInvalidRequest, fmt.Errorf("no method set in json rpc payload"))
	}
	if mr.ID == nil && !mr.isNotificationRequest() {
		return false, errors.Join(ErrInvalidRequest, fmt.Errorf("no id set in json rpc payload for none notification method: %s ", mr.Method))
	}

	return true, nil
}

func (mr *MCPRequest) isNotificationRequest() bool {
	return strings.HasPrefix(mr.Method, "notifications")
}

// isToolCall will check if the request is a tool call request
func (mr *MCPRequest) isToolCall() bool {
	return mr.Method == "tools/call"
}

// ToolName returns the tool name in a tools/call request
func (mr *MCPRequest) ToolName() string {
	if !mr.isToolCall() {
		return ""
	}
	tool, ok := mr.Params["name"]
	if !ok {
		return ""
	}
	t, ok := tool.(string)
	if !ok {
		return ""
	}
	return t
}

// ReWriteToolName will allow re-setting the tool name to something different. This is needed to remove prefix
// and set the actual tool name
func (mr *MCPRequest) ReWriteToolName(actualTool string) {
	mr.Params["name"] = actualTool
}

// ToBytes marshals the data ready to send on
func (mr *MCPRequest) ToBytes() ([]byte, error) {
	return json.Marshal(mr)
}

// HandleMCPRequest handles request bodies for MCP requests.
func (s *ExtProcServer) HandleMCPRequest(ctx context.Context, mcpReq *MCPRequest, config *config.MCPServersConfig) []*eppb.ProcessingResponse {
	mcpReq.SessionID = s.getRequestSessionID(s.requestHeaders)
	slog.Debug("Request Handler: Processing request body for MCP requests...", "data", mcpReq)

	headers := NewHeaders()
	headers.WithMCPMethod(mcpReq.Method)
	// Extract tool name for routing
	calculatedResponse := []*eppb.ProcessingResponse{}
	if !mcpReq.isToolCall() {
		headers.WithMCPServerName("mcpBroker")
		slog.Debug(
			"[EXT-PROC] HandleRequestBody None tool call setting method header only" + mcpReq.Method,
		)
		// none tool call set headers
		calculatedResponse = append(calculatedResponse, s.HeaderResponse(headers.Build()))
		return calculatedResponse
	}
	if mcpReq.SessionID == "" {
		slog.Info("No mcp-session-id found in headers")
		return append(calculatedResponse, s.createErrorResponse("No session ID found", 400))
	}

	if _, err := s.JWTManager.Validate(mcpReq.SessionID); err != nil {
		slog.Info("mcp session id is invalid")
		return append(calculatedResponse, s.createErrorResponse("Session Invalid", 404))
	}
	// handle tools call
	toolName := mcpReq.ToolName()
	if toolName == "" {
		slog.Error("[EXT-PROC] HandleRequestBody no tool name set in tools/call")
		return append(calculatedResponse, s.createErrorResponse("no tool name set", 400))
	}

	// TODO prefix here really is the the server id. It is confusing to think of it as both
	slog.Debug("[EXT-PROC] HandleRequestBody", "Tool name:", toolName)
	serverInfo := config.GetServerInfo(toolName)
	if serverInfo == nil {
		slog.Info("Tool name doesn't match any configured server prefix", "tool", toolName)
		// todo should this be a 404??
		return append(calculatedResponse, s.createErrorResponse("not found", 404))
	}
	upstreamToolName := config.StripServerPrefix(toolName)
	headers.WithMCPToolName(upstreamToolName)
	mcpReq.ReWriteToolName(upstreamToolName)
	slog.Info("Stripped tool name", "tool", mcpReq.ToolName())
	headers.WithMCPServerName(serverInfo.Name)
	// Get Helper session ID

	slog.Info("Helper session", "session", mcpReq.SessionID)

	// Use cache to get or create upstream MCP session
	var upstreamSession string
	if s.SessionCache != nil {
		us, err := s.SessionCache.GetOrInit(ctx, serverInfo.Name, serverInfo.URL, mcpReq.SessionID)
		if err != nil {
			slog.Error("Failed to get session from cache", "error", err)
			return append(calculatedResponse, s.createErrorResponse("Session lookup failed", 502))
		}
		upstreamSession = us
		slog.Info("Got session from cache. Setting the session for the upstream mcp server", "session", upstreamSession)
		headers.WithMCPSession(upstreamSession)
	} else {
		slog.Warn("Session cache not configured; proceeding without upstream session")
	}

	headers.WithAuthority(serverInfo.Hostname)

	calculatedResponse, err := s.HeaderAndBodyResponse(headers, mcpReq)
	if err != nil {
		return append(calculatedResponse, s.createErrorResponse("Gateway Error", 500))
	}
	return calculatedResponse
}

// getRequestSessionID gets the session id from the request headers
func (s *ExtProcServer) getRequestSessionID(requestHeaders *eppb.HttpHeaders) string {
	if requestHeaders == nil || s.requestHeaders.Headers == nil {
		return ""
	}

	// Extract mcp-session-id from stored headers
	for _, header := range s.requestHeaders.Headers.Headers {
		if strings.ToLower(header.Key) == "mcp-session-id" {
			return string(header.RawValue)
		}
	}

	return ""
}

// HeaderResponse will build the headers response for sending to envoy
func (s *ExtProcServer) HeaderResponse(headers []*basepb.HeaderValueOption) *eppb.ProcessingResponse {
	return &eppb.ProcessingResponse{
		Response: &eppb.ProcessingResponse_RequestBody{
			RequestBody: &eppb.BodyResponse{
				Response: &eppb.CommonResponse{
					ClearRouteCache: true,
					HeaderMutation: &eppb.HeaderMutation{
						SetHeaders: headers,
					},
				},
			},
		},
	}
}

// HeaderAndBodyResponse will build the headers and body response to send back to envoy
func (s *ExtProcServer) HeaderAndBodyResponse(headers *HeadersBuilder, req *MCPRequest) ([]*eppb.ProcessingResponse, error) {
	slog.Info("HeaderAndBodyResponse")
	body, err := req.ToBytes()
	if err != nil {
		return nil, fmt.Errorf("failed to convert modified mcp request to bytes ")
	}
	headers.WithContentLength(len(body))
	// headers all set
	if s.streaming {
		headersResp := s.HeaderResponse(headers.Build())
		slog.Info("Using streaming mode - returning header response first")
		resp := addStreamedBodyResponse([]*eppb.ProcessingResponse{headersResp}, body)
		slog.Info("Completed MCP processing with routing (streaming)")
		return resp, nil
	}
	slog.Info("not streaming request")
	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_RequestBody{
				RequestBody: &eppb.BodyResponse{
					Response: &eppb.CommonResponse{
						// Necessary so that the new headers are used in the routing decision.
						ClearRouteCache: true,
						HeaderMutation: &eppb.HeaderMutation{
							SetHeaders: headers.Build(),
						},
						BodyMutation: &eppb.BodyMutation{
							Mutation: &eppb.BodyMutation_Body{
								Body: body,
							},
						},
					},
				},
			},
		},
	}, nil
}

func addStreamedBodyResponse(
	responses []*eppb.ProcessingResponse,
	requestBodyBytes []byte,
) []*eppb.ProcessingResponse {
	return append(responses, &eppb.ProcessingResponse{
		Response: &eppb.ProcessingResponse_RequestBody{
			RequestBody: &eppb.BodyResponse{
				Response: &eppb.CommonResponse{
					BodyMutation: &eppb.BodyMutation{
						Mutation: &eppb.BodyMutation_StreamedResponse{
							StreamedResponse: &eppb.StreamedBodyResponse{
								Body:        requestBodyBytes,
								EndOfStream: true,
							},
						},
					},
				},
			},
		},
	})
}

// createEmptyBodyResponse creates a response that doesn't modify the request
func (s *ExtProcServer) doNothing() *eppb.ProcessingResponse {
	if s.streaming {
		return &eppb.ProcessingResponse{
			Response: &eppb.ProcessingResponse_RequestHeaders{
				RequestHeaders: &eppb.HeadersResponse{},
			},
		}
	}

	return &eppb.ProcessingResponse{
		Response: &eppb.ProcessingResponse_RequestBody{
			RequestBody: &eppb.BodyResponse{},
		},
	}

}

// createErrorResponse creates an immediate error response with the specified status code
func (s *ExtProcServer) createErrorResponse(
	message string,
	statusCode int32,
) *eppb.ProcessingResponse {
	slog.Error("Returning error", "statusCode", statusCode, "message", message)

	return &eppb.ProcessingResponse{
		Response: &eppb.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &eppb.ImmediateResponse{
				Status: &typepb.HttpStatus{
					Code: typepb.StatusCode(statusCode),
				},
				Body:    []byte(message),
				Details: fmt.Sprintf("ext-proc error: %s", message),
			},
		},
	}

}

// HandleRequestHeaders handles request headers minimally.
func (s *ExtProcServer) HandleRequestHeaders(
	headers *eppb.HttpHeaders,
) ([]*eppb.ProcessingResponse, error) {
	slog.Info("Request Handler: HandleRequestHeaders called", "streaming", s.streaming)
	if headers != nil && headers.Headers != nil {
		for _, header := range headers.Headers.Headers {
			if strings.ToLower(header.Key) == "content-type" ||
				strings.ToLower(header.Key) == "mcp-session-id" {
				slog.Info("Header", "key", header.Key, "value", string(header.RawValue))
			}
		}
	}
	// TODO change processing mode to not receive body if not interested in request based on headers
	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_RequestHeaders{
				RequestHeaders: &eppb.HeadersResponse{},
			},
		},
	}, nil
}
