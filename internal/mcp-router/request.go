package mcprouter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	basepb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	eppb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typepb "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/kagenti/mcp-gateway/internal/config"
)

const (
	toolHeader      = "x-mcp-toolname"
	toolIDHeader    = "x-mcp-toolid" // internally used only
	methodHeader    = "x-mcp-method" // internally used only
	sessionHeader   = "mcp-session-id"
	authorityHeader = ":authority"
)

// ServerInfo contains routing information for an MCP server
type ServerInfo struct {
	ServerName       string
	Hostname         string
	ToolPrefix       string
	URL              string
	CredentialEnvVar string
}

func validateJSONRPC(data map[string]any) bool {
	// Check if this is a JSON-RPC request
	jsonrpcVal, ok := data["jsonrpc"]
	if !ok {
		return false
	}

	jsonrpcStr, ok := jsonrpcVal.(string)
	if !ok || jsonrpcStr != "2.0" {
		return false
	}
	return true
}

func extractMCPMethod(data map[string]any) string {
	if !validateJSONRPC(data) {
		return ""
	}
	methodVal, ok := data["method"]
	if !ok {
		return ""
	}

	methodStr, ok := methodVal.(string)
	if !ok {
		return ""
	}
	return methodStr
}

// extractMCPToolName safely extracts the tool name from MCP tool call request
func extractMCPToolName(data map[string]any) string {
	if !validateJSONRPC(data) {
		return ""
	}
	// Extract method field and check if it's tools/call
	methodVal, ok := data["method"]
	if !ok {
		return ""
	}

	methodStr, ok := methodVal.(string)
	if !ok {
		return ""
	}

	if methodStr != "tools/call" {
		return ""
	}

	// Extract params
	paramsVal, ok := data["params"]
	if !ok {
		slog.Error("[EXT-PROC] MCP tool call missing params field")
		return ""
	}

	paramsMap, ok := paramsVal.(map[string]interface{})
	if !ok {
		slog.Error("[EXT-PROC] MCP tool call params is not an object")
		return ""
	}

	// Extract tool name
	nameVal, ok := paramsMap["name"]
	if !ok {
		slog.Error("[EXT-PROC] MCP tool call missing name field in params")
		return ""
	}

	nameStr, ok := nameVal.(string)
	if !ok {
		slog.Error("[EXT-PROC] MCP tool call name is not a string")
		return ""
	}

	return nameStr
}

func getServerInfo(toolName string, config *config.MCPServersConfig) *ServerInfo {
	if config == nil {
		return nil
	}

	// find server by prefix
	for _, server := range config.Servers {
		if server.Enabled && strings.HasPrefix(toolName, server.ToolPrefix) {
			slog.Info("[EXT-PROC] Found matching server",
				"toolName", toolName,
				"serverPrefix", server.ToolPrefix,
				"serverName", server.Name)
			return &ServerInfo{
				ServerName:       server.Name,
				Hostname:         server.Hostname,
				ToolPrefix:       server.ToolPrefix,
				URL:              server.URL,
				CredentialEnvVar: server.CredentialEnvVar,
			}
		}
	}

	slog.Info("Tool name doesn't match any configured server prefix", "tool", toolName)
	return nil
}

// Returns the stripped tool name and whether stripping was needed
func stripServerPrefix(toolName string, config *config.MCPServersConfig) (string, bool) {
	if config == nil {
		return toolName, false
	}

	// strip matching prefix
	for _, server := range config.Servers {
		if server.Enabled && strings.HasPrefix(toolName, server.ToolPrefix) {
			strippedToolName := strings.TrimPrefix(toolName, server.ToolPrefix)
			slog.Info("Stripped tool name", "tool", strippedToolName, "originalPrefix", server.ToolPrefix)
			return strippedToolName, true
		}
	}

	return toolName, false
}

// extractSessionFromContext extracts mcp-session-id from the stored request headers
func (s *ExtProcServer) extractSessionFromContext(_ context.Context) string {
	if s.requestHeaders == nil || s.requestHeaders.Headers == nil {
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

// HandleRequestBody handles request bodies for MCP requests.
func (s *ExtProcServer) HandleRequestBody(ctx context.Context, data map[string]any, config *config.MCPServersConfig) ([]*eppb.ProcessingResponse, error) {
	slog.Info("Request Handler: Processing request body for MCP requests...", "data", data)

	// Extract tool name for routing

	toolName := extractMCPToolName(data)
	mcpMethod := extractMCPMethod(data)
	if toolName == "" && mcpMethod == "" {
		slog.Debug(
			"[EXT-PROC] HandleRequestBody No tool name or  method found in request",
		)
		return s.createEmptyBodyResponse(), nil
	}

	if toolName == "" && mcpMethod != "" {
		slog.Debug(
			"[EXT-PROC] HandleRequestBody None tool call setting method header only" + mcpMethod,
		)
		// none tool call set headers
		return s.createCommonHeaders(
			mcpMethod,
		), nil
	}

	serverInfo := getServerInfo(toolName, config)
	if serverInfo == nil {
		slog.Info("Tool name doesn't match any configured server prefix", "tool", toolName)
		return s.createEmptyBodyResponse(), nil
	}

	slog.Debug("[EXT-PROC] HandleRequestBody", "Tool name:", toolName)

	// Get hostname for routing based on tool prefix
	url := serverInfo.URL
	if url == "" {
		slog.Info(
			"[EXT-PROC] HandleRequestBody Tool name doesn't match any configured server prefix",
			"tool",
			toolName,
		)
		return s.createEmptyBodyResponse(), nil
	}
	hostname := serverInfo.Hostname
	if hostname == "" {
		slog.Info(
			"[EXT-PROC] HandleRequestBody Tool name doesn't match any configured server prefix",
			"tool",
			toolName,
		)
		return s.createEmptyBodyResponse(), nil
	}

	serverName := serverInfo.ServerName

	// Strip server prefix from tool name and modify request body
	strippedToolName, _ := stripServerPrefix(toolName, config)
	slog.Info("Stripped tool name", "tool", strippedToolName)

	// Create modified request body with stripped tool name
	modifiedData := make(map[string]any)
	for k, v := range data {
		modifiedData[k] = v
	}

	if params, ok := modifiedData["params"].(map[string]interface{}); ok {
		params["name"] = strippedToolName
		slog.Debug(
			"[EXT-PROC] HandleRequestBody Updated tool in request body",
			"toolname",
			strippedToolName,
		)
	}

	requestBodyBytes, err := json.Marshal(modifiedData)
	if err != nil {
		slog.Error(
			"[EXT-PROC] HandleRequestBody Failed to marshal modified request body:",
			"error",
			err,
		)
		return s.createEmptyBodyResponse(), nil
	}

	// Get Helper session ID
	helperSession := s.extractSessionFromContext(ctx)
	if helperSession == "" {
		slog.Info("No mcp-session-id found in headers")
		return s.createErrorResponse("No session ID found", 400), nil
	}

	slog.Info("Helper session", "session", helperSession)

	// Use cache to get or create upstream MCP session
	var upstreamSession string
	if s.SessionCache != nil {
		us, err := s.SessionCache.GetOrInit(ctx, serverName, url, helperSession)
		if err != nil {
			slog.Error("Failed to get session from cache", "error", err)
			return s.createErrorResponse("Session lookup failed", 502), nil
		}
		upstreamSession = us
		slog.Info("Got session from cache", "session", upstreamSession)
	} else {
		slog.Warn("Session cache not configured; proceeding without upstream session")
	}

	return s.createRoutingResponse(
		toolName, mcpMethod, requestBodyBytes, hostname, serverName, upstreamSession, serverInfo,
	), nil
}

func (s *ExtProcServer) createCommonHeaders(method string) []*eppb.ProcessingResponse {
	headers := []*basepb.HeaderValueOption{
		{
			Header: &basepb.HeaderValue{
				Key:      methodHeader,
				RawValue: []byte(method),
			},
		},
	}
	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_RequestBody{
				RequestBody: &eppb.BodyResponse{
					Response: &eppb.CommonResponse{
						HeaderMutation: &eppb.HeaderMutation{
							SetHeaders: headers,
						},
					},
				},
			},
		},
	}
}

// TODO this needs refactor
// createRoutingResponse creates a response with routing headers and session mapping
func (s *ExtProcServer) createRoutingResponse(
	toolName string,
	method string,
	bodyBytes []byte,
	hostname, serverName, backendSession string,
	serverInfo *ServerInfo,
) []*eppb.ProcessingResponse {

	headers := []*basepb.HeaderValueOption{
		{
			Header: &basepb.HeaderValue{
				Key:      toolHeader,
				RawValue: []byte(toolName),
			},
		},
		{
			Header: &basepb.HeaderValue{
				Key:      toolIDHeader,
				RawValue: []byte(serverInfo.ToolPrefix),
			},
		},
		{
			Header: &basepb.HeaderValue{
				Key:      methodHeader,
				RawValue: []byte(method),
			},
		},
		{
			Header: &basepb.HeaderValue{
				Key:      authorityHeader,
				RawValue: []byte(hostname),
			},
		},
	}

	// Add backend session header if we have one
	if backendSession != "" {
		headers = append(headers, &basepb.HeaderValueOption{
			Header: &basepb.HeaderValue{
				Key:      sessionHeader,
				RawValue: []byte(backendSession),
			},
		})
	}

	// add auth header if needed
	if serverInfo != nil && serverInfo.CredentialEnvVar != "" {
		authValue := os.Getenv(serverInfo.CredentialEnvVar)
		if authValue != "" {
			slog.Info("Adding Authorization header for routing",
				"server", serverName,
				"credentialEnvVar", serverInfo.CredentialEnvVar)
			headers = append(headers, &basepb.HeaderValueOption{
				Header: &basepb.HeaderValue{
					Key:      "authorization",
					RawValue: []byte(authValue),
				},
			})
		}
	}

	// Update content-length header to match the modified body
	contentLength := fmt.Sprintf("%d", len(bodyBytes))
	headers = append(headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      "content-length",
			RawValue: []byte(contentLength),
		},
	})

	if s.streaming {
		slog.Info("Using streaming mode - returning header response first")
		ret := []*eppb.ProcessingResponse{
			{
				Response: &eppb.ProcessingResponse_RequestHeaders{
					RequestHeaders: &eppb.HeadersResponse{
						Response: &eppb.CommonResponse{
							ClearRouteCache: true,
							HeaderMutation: &eppb.HeaderMutation{
								SetHeaders: headers,
							},
						},
					},
				},
			},
		}
		ret = addStreamedBodyResponse(ret, bodyBytes)
		slog.Info("Completed MCP processing with routing (streaming)", "target", serverName)
		return ret
	}

	// For non-streaming: Set headers in RequestBody response with ClearRouteCache
	slog.Info("Using non-streaming mode - setting headers in body response")
	slog.Info("Completed MCP processing with routing", "target", serverName)
	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_RequestBody{
				RequestBody: &eppb.BodyResponse{
					Response: &eppb.CommonResponse{
						// Necessary so that the new headers are used in the routing decision.
						ClearRouteCache: true,
						HeaderMutation: &eppb.HeaderMutation{
							SetHeaders: headers,
						},
						BodyMutation: &eppb.BodyMutation{
							Mutation: &eppb.BodyMutation_Body{
								Body: bodyBytes,
							},
						},
					},
				},
			},
		},
	}
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
func (s *ExtProcServer) createEmptyBodyResponse() []*eppb.ProcessingResponse {
	if s.streaming {
		return []*eppb.ProcessingResponse{
			{
				Response: &eppb.ProcessingResponse_RequestHeaders{
					RequestHeaders: &eppb.HeadersResponse{},
				},
			},
		}
	}

	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_RequestBody{
				RequestBody: &eppb.BodyResponse{},
			},
		},
	}
}

// createErrorResponse creates an immediate error response with the specified status code
func (s *ExtProcServer) createErrorResponse(
	message string,
	statusCode int32,
) []*eppb.ProcessingResponse {
	slog.Error("Returning error", "statusCode", statusCode, "message", message)

	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_ImmediateResponse{
				ImmediateResponse: &eppb.ImmediateResponse{
					Status: &typepb.HttpStatus{
						Code: typepb.StatusCode(statusCode),
					},
					Body:    []byte(message),
					Details: fmt.Sprintf("ext-proc error: %s", message),
				},
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
	// TODO change proccessing mode to not receive body if not interested in request based on headers
	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_RequestHeaders{
				RequestHeaders: &eppb.HeadersResponse{},
			},
		},
	}, nil
}
