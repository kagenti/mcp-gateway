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

// ServerInfo contains routing information for an MCP server
// type ServerInfo struct {
// 	ServerName       string
// 	Hostname         string
// 	ToolPrefix       string
// 	URL              string
// 	CredentialEnvVar string
// }

// func validateJSONRPC(data map[string]any) bool {
// 	// Check if this is a JSON-RPC request
// 	jsonrpcVal, ok := data["jsonrpc"]
// 	if !ok {
// 		return false
// 	}

// 	jsonrpcStr, ok := jsonrpcVal.(string)
// 	if !ok || jsonrpcStr != "2.0" {
// 		return false
// 	}
// 	return true
// }

// func extractMCPMethod(data map[string]any) string {
// 	if !validateJSONRPC(data) {
// 		return ""
// 	}
// 	methodVal, ok := data["method"]
// 	if !ok {
// 		return ""
// 	}

// 	methodStr, ok := methodVal.(string)
// 	if !ok {
// 		return ""
// 	}
// 	return methodStr
// }

// // extractMCPToolName safely extracts the tool name from MCP tool call request
// func extractMCPToolName(data map[string]any) string {
// 	if !validateJSONRPC(data) {
// 		return ""
// 	}
// 	// Extract method field and check if it's tools/call
// 	methodVal, ok := data["method"]
// 	if !ok {
// 		return ""
// 	}

// 	methodStr, ok := methodVal.(string)
// 	if !ok {
// 		return ""
// 	}

// 	if methodStr != "tools/call" {
// 		return ""
// 	}

// 	// Extract params
// 	paramsVal, ok := data["params"]
// 	if !ok {
// 		slog.Error("[EXT-PROC] MCP tool call missing params field")
// 		return ""
// 	}

// 	paramsMap, ok := paramsVal.(map[string]interface{})
// 	if !ok {
// 		slog.Error("[EXT-PROC] MCP tool call params is not an object")
// 		return ""
// 	}

// 	// Extract tool name
// 	nameVal, ok := paramsMap["name"]
// 	if !ok {
// 		slog.Error("[EXT-PROC] MCP tool call missing name field in params")
// 		return ""
// 	}

// 	nameStr, ok := nameVal.(string)
// 	if !ok {
// 		slog.Error("[EXT-PROC] MCP tool call name is not a string")
// 		return ""
// 	}

// 	return nameStr
// }

// func getServerInfo(toolName string, config *config.MCPServersConfig) *ServerInfo {
// 	if config == nil {
// 		return nil
// 	}

// 	// find server by prefix
// 	for _, server := range config.Servers {
// 		if server.Enabled && strings.HasPrefix(toolName, server.ToolPrefix) {
// 			slog.Info("[EXT-PROC] Found matching server",
// 				"toolName", toolName,
// 				"serverPrefix", server.ToolPrefix,
// 				"serverName", server.Name)
// 			return &ServerInfo{
// 				ServerName:       server.Name,
// 				Hostname:         server.Hostname,
// 				ToolPrefix:       server.ToolPrefix,
// 				URL:              server.URL,
// 				CredentialEnvVar: server.CredentialEnvVar,
// 			}
// 		}
// 	}

// 	slog.Info("Tool name doesn't match any configured server prefix", "tool", toolName)
// 	return nil
// }

// extractSessionFromContext extracts mcp-session-id from the stored request headers

// ErrInvalidRequest is an error for an invalid request
var ErrInvalidRequest = fmt.Errorf("MCP Request is invalid")

// MCPRequest encapsulates a mcp protocol request to the gateway
type MCPRequest struct {
	ID        int            `json:"id"`
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

	return true, nil
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

// ToBytes mashels the data ready to send on
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
	// handle tools call
	toolName := mcpReq.ToolName()
	if toolName == "" {
		slog.Error("[EXT-PROC] HandleRequestBody no tool name set in tools/call")
		return append(calculatedResponse, s.createErrorResponse("no tool name set", 400))
	}
	headers.WithMCPToolName(toolName)
	// TODO prefix here really is the the server id. It is confusing to think of it as both
	slog.Debug("[EXT-PROC] HandleRequestBody", "Tool name:", toolName)
	serverInfo := config.GetServerInfo(toolName)
	if serverInfo == nil {
		slog.Info("Tool name doesn't match any configured server prefix", "tool", toolName)
		// todo should this be a 404??
		return append(calculatedResponse, s.createErrorResponse("not found", 404))
	}

	mcpReq.ReWriteToolName(config.StripServerPrefix(toolName))
	slog.Info("Stripped tool name", "tool", mcpReq.ToolName())
	headers.WithMCPServerName(serverInfo.Name)
	// Get Helper session ID
	if mcpReq.SessionID == "" {
		slog.Info("No mcp-session-id found in headers")
		return append(calculatedResponse, s.createErrorResponse("No session ID found", 400))
	}

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
	if serverInfo.Credential() != "" {
		headers.WithAuth(serverInfo.Credential())
	}

	calculatedResponse, err := s.HeaderAndBodyResponse(headers, mcpReq)
	if err != nil {
		return append(calculatedResponse, s.createErrorResponse("Gateway Error", 500))
	}
	return calculatedResponse
}

// extractSessionFromContext extracts mcp-session-id from the stored request headers
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
	headersResp := s.HeaderResponse(headers.Build())
	if s.streaming {
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
	// res := headersResp.Response.(*eppb.ProcessingResponse_RequestBody)
	// res.RequestBody.Response.BodyMutation = &eppb.BodyMutation{
	// 	Mutation: &eppb.BodyMutation_Body{
	// 		Body: body,
	// 	},
	// }
	// headersResp.Response = res
	// slog.Info("not streaming request", "sending", headersResp)
	// return []*eppb.ProcessingResponse{headersResp}, nil
}

// func (s *ExtProcServer) createCommonHeaders(method string) []*eppb.ProcessingResponse {
// 	headers := []*basepb.HeaderValueOption{
// 		{
// 			Header: &basepb.HeaderValue{
// 				Key:      methodHeader,
// 				RawValue: []byte(method),
// 			},
// 		},
// 	}
// 	return []*eppb.ProcessingResponse{
// 		{
// 			Response: &eppb.ProcessingResponse_RequestBody{
// 				RequestBody: &eppb.BodyResponse{
// 					Response: &eppb.CommonResponse{
// 						HeaderMutation: &eppb.HeaderMutation{
// 							SetHeaders: headers,
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}
// }

// // TODO this needs refactor
// // createRoutingResponse creates a response with routing headers and session mapping
// func (s *ExtProcServer) createRoutingResponse(
// 	toolName string,
// 	method string,
// 	bodyBytes []byte,
// 	hostname, serverName, backendSession string,
// 	serverInfo *ServerInfo,
// ) []*eppb.ProcessingResponse {

// 	headers := []*basepb.HeaderValueOption{
// 		{
// 			Header: &basepb.HeaderValue{
// 				Key:      toolHeader,
// 				RawValue: []byte(toolName),
// 			},
// 		},
// 		{
// 			Header: &basepb.HeaderValue{
// 				Key:      authorityHeader,
// 				RawValue: []byte(hostname),
// 			},
// 		},
// 		{
// 			Header: &basepb.HeaderValue{
// 				Key:      methodHeader,
// 				RawValue: []byte(method),
// 			},
// 		},
// 	}

// 	// extract path from URL and set :path header if it's not the default /mcp
// 	if serverInfo != nil && serverInfo.URL != "" {
// 		parsedURL, err := url.Parse(serverInfo.URL)
// 		if err == nil && parsedURL.Path != "" && parsedURL.Path != "/mcp" {
// 			slog.Info("Setting custom path header", "path", parsedURL.Path, "server", serverName)
// 			headers = append(headers, &basepb.HeaderValueOption{
// 				Header: &basepb.HeaderValue{
// 					Key:      ":path",
// 					RawValue: []byte(parsedURL.Path),
// 				},
// 			})
// 		}
// 	}

// 	// Add backend session header if we have one
// 	if backendSession != "" {
// 		headers = append(headers, &basepb.HeaderValueOption{
// 			Header: &basepb.HeaderValue{
// 				Key:      sessionHeader,
// 				RawValue: []byte(backendSession),
// 			},
// 		})
// 	}

// 	// add auth header if needed
// 	if serverInfo != nil && serverInfo.CredentialEnvVar != "" {
// 		authValue := os.Getenv(serverInfo.CredentialEnvVar)
// 		if authValue != "" {
// 			slog.Info("Adding Authorization header for routing",
// 				"server", serverName,
// 				"credentialEnvVar", serverInfo.CredentialEnvVar)
// 			headers = append(headers, &basepb.HeaderValueOption{
// 				Header: &basepb.HeaderValue{
// 					Key:      "authorization",
// 					RawValue: []byte(authValue),
// 				},
// 			})
// 		}
// 	}

// 	// Update content-length header to match the modified body
// 	contentLength := fmt.Sprintf("%d", len(bodyBytes))
// 	headers = append(headers, &basepb.HeaderValueOption{
// 		Header: &basepb.HeaderValue{
// 			Key:      "content-length",
// 			RawValue: []byte(contentLength),
// 		},
// 	})

// 	if s.streaming {
// 		slog.Info("Using streaming mode - returning header response first")
// 		ret := []*eppb.ProcessingResponse{
// 			{
// 				Response: &eppb.ProcessingResponse_RequestHeaders{
// 					RequestHeaders: &eppb.HeadersResponse{
// 						Response: &eppb.CommonResponse{
// 							ClearRouteCache: true,
// 							HeaderMutation: &eppb.HeaderMutation{
// 								SetHeaders: headers,
// 							},
// 						},
// 					},
// 				},
// 			},
// 		}
// 		ret = addStreamedBodyResponse(ret, bodyBytes)
// 		slog.Info("Completed MCP processing with routing (streaming)", "target", serverName)
// 		return ret
// 	}

// 	// For non-streaming: Set headers in RequestBody response with ClearRouteCache
// 	slog.Info("Using non-streaming mode - setting headers in body response")
// 	slog.Info("Completed MCP processing with routing", "target", serverName)
// 	return []*eppb.ProcessingResponse{
// 		{
// 			Response: &eppb.ProcessingResponse_RequestBody{
// 				RequestBody: &eppb.BodyResponse{
// 					Response: &eppb.CommonResponse{
// 						// Necessary so that the new headers are used in the routing decision.
// 						ClearRouteCache: true,
// 						HeaderMutation: &eppb.HeaderMutation{
// 							SetHeaders: headers,
// 						},
// 						BodyMutation: &eppb.BodyMutation{
// 							Mutation: &eppb.BodyMutation_Body{
// 								Body: bodyBytes,
// 							},
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}
// }

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
