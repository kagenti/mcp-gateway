package mcprouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	eppb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
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

// HandleRequestHeaders handles request headers minimally.
func (s *ExtProcServer) HandleRequestHeaders(
	headers *eppb.HttpHeaders,
) ([]*eppb.ProcessingResponse, error) {
	s.Logger.Info("Request Handler: HandleRequestHeaders called", "streaming", s.streaming)
	requestHeaders := NewHeaders()
	response := NewResponse()
	s.Logger.Info("HandleRequestHeaders ", "request headers", headers.Headers)
	requestHeaders.WithAuthority(s.RoutingConfig.MCPGatewayHostname)
	return response.WithRequestHeadersReponse(requestHeaders.Build()).Build(), nil
}

// HandleMCPRequest handles request bodies for MCP requests.
func (s *ExtProcServer) HandleMCPRequest(ctx context.Context, mcpReq *MCPRequest, config *config.MCPServersConfig) []*eppb.ProcessingResponse {
	mcpReq.SessionID = getSingleValueHeader(s.requestHeaders.Headers, sessionHeader)
	s.Logger.Debug("HandleMCPRequest: Processing request body for MCP requests...", "data", mcpReq)

	headers := NewHeaders()
	headers.WithMCPMethod(mcpReq.Method)
	// Extract tool name for routing
	calculatedResponse := NewResponse()
	if !mcpReq.isToolCall() {
		s.Logger.Debug("HandleMCPRequest not a tool call", "request headers", s.requestHeaders.Headers)

		headers.WithMCPServerName("mcpBroker")
		s.Logger.Debug(
			"[EXT-PROC] HandleRequestBody None tool call setting method header only" + mcpReq.Method,
		)
		// none tool call set headers
		calculatedResponse.WithRequestBodyHeadersResponse(headers.Build())
		// calculatedResponse = append(calculatedResponse, s.HeaderBodyResponse(headers.Build()))
		return calculatedResponse.Build()
	}
	// handle tools call
	toolName := mcpReq.ToolName()
	if toolName == "" {
		s.Logger.Error("[EXT-PROC] HandleRequestBody no tool name set in tools/call")
		calculatedResponse.WithImmediateResponse(400, "no tool name set")
		return calculatedResponse.Build()
	}

	// Get tool annotations from broker and set headers
	if s.Broker != nil {
		if annotations, hasAnnotations := s.Broker.ToolAnnotations(toolName); hasAnnotations {
			// build header value (e.g. readOnly=true,destructive=false,openWorld=true)
			var parts []string
			push := func(key string, val *bool) {
				if val == nil {
					parts = append(parts, fmt.Sprintf("%s=unspecified", key))
				} else if *val {
					parts = append(parts, fmt.Sprintf("%s=true", key))
				} else {
					parts = append(parts, fmt.Sprintf("%s=false", key))
				}
			}

			push("readOnly", annotations.ReadOnlyHint)
			push("destructive", annotations.DestructiveHint)
			push("idempotent", annotations.IdempotentHint)
			push("openWorld", annotations.OpenWorldHint)

			hintsHeader := strings.Join(parts, ",")
			headers.WithToolAnnotations(hintsHeader)
		}
	}

	// TODO prefix here really is the the server id. It is confusing to think of it as both
	s.Logger.Debug("[EXT-PROC] HandleRequestBody", "Tool name:", toolName)
	serverInfo := config.GetServerInfo(toolName)
	if serverInfo == nil {
		s.Logger.Info("Tool name doesn't match any configured server prefix", "tool", toolName)
		// todo should this be a 404??
		calculatedResponse.WithImmediateResponse(404, "not found")
		return calculatedResponse.Build()
	}
	upstreamToolName := config.StripServerPrefix(toolName)
	headers.WithMCPToolName(upstreamToolName)
	mcpReq.ReWriteToolName(upstreamToolName)
	s.Logger.Info("Stripped tool name", "tool", mcpReq.ToolName())
	headers.WithMCPServerName(serverInfo.Name)
	// Get Helper session ID
	if mcpReq.SessionID == "" {
		s.Logger.Info("No mcp-session-id found in headers")
		calculatedResponse.WithImmediateResponse(400, "no session ID found")
		return calculatedResponse.Build()
	}

	s.Logger.Debug("Helper session", "session", mcpReq.SessionID)

	// Use cache to get or create upstream MCP session
	if s.SessionCache != nil {
		upstreamSession, err := s.SessionCache.GetOrInit(ctx, serverInfo.Name, serverInfo.URL, mcpReq.SessionID)
		if err != nil {
			s.Logger.Error("Failed to get session from cache", "error", err)
			calculatedResponse.WithImmediateResponse(502, "failed to look up session")
			return calculatedResponse.Build()
		}
		s.Logger.Info("Got session from cache. Setting the session for the upstream mcp server", "session", upstreamSession)
		headers.WithMCPSession(upstreamSession)
	} else {
		s.Logger.Warn("Session cache not configured; proceeding without upstream session")
	}
	// prepare request for MCP Backend
	body, err := mcpReq.ToBytes()
	if err != nil {
		s.Logger.Error("failed to marshal body to bytes ", "error ", err)
		calculatedResponse.WithImmediateResponse(500, "internal error")
		return calculatedResponse.Build()
	}
	// reset the host name now we have identifed the correct tool and backend
	headers.WithAuthority(serverInfo.Hostname)
	// ensure our contnent length has been reset
	headers.WithContentLength(len(body))
	if s.streaming {
		s.Logger.Debug("returning streaming response")
		calculatedResponse.WithStreamingResponse(headers.Build(), body)
		return calculatedResponse.Build()
	}
	s.Logger.Debug("returning none streaming response")
	calculatedResponse.WithRequestBodyHeadersAndBodyReponse(headers.Build(), body)
	return calculatedResponse.Build()
}
