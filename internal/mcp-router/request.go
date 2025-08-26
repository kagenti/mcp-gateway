package mcprouter

import (
	"context"
	"encoding/json"
	"fmt"
	"slog/slog"
	"strings"


	basepb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	eppb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typepb "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

const (
	toolHeader    = "x-mcp-toolname"
	serverHeader  = "x-mcp-server"
	sessionHeader = "mcp-session-id"
	authorityHeader = ":authority"
)


// Config represents the server configuration 
type Config struct {
	LogLevel string   `yaml:"logLevel"`
	Port     int      `yaml:"port"`
	Servers  []Server `yaml:"servers"`
}

type Server struct {
	Name       string `yaml:"name"`
	URL        string `yaml:"url"`
	Hostname   string `yaml:"hostname"`
	ToolPrefix string `yaml:"toolPrefix"`
	Enabled    bool   `yaml:"enabled"`
}

// extractMCPToolName safely extracts the tool name from MCP tool call request
func extractMCPToolName(data map[string]any) string {
	// Check if this is a JSON-RPC request
	jsonrpcVal, ok := data["jsonrpc"]
	if !ok {
		return ""
	}

	jsonrpcStr, ok := jsonrpcVal.(string)
	if !ok || jsonrpcStr != "2.0" {
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

// determines which server to route to based on tool name prefix
// e.g. if the toolName is 'server1-echo', return 'server1'
// NOTE: The prefix and route target aren't necessarily the same value.
// The prefix could be any string that maps to a server name.
func getRouteTargetFromTool(toolName string) string {
	routeTarget := strings.Split(toolName, "_")[0]
	if routeTarget == "" {
		slog.Error("Route Target not found")
		return ""
		//TODO look at exit out
	}
	return routeTarget
}

// Returns the stripped tool name and whether stripping was needed
// e.g. server1-echo returns echo, true
func stripServerPrefix(toolName string) (string, bool) {
	routeTarget := getRouteTargetFromTool(toolName)
	strippedToolName := strings.TrimPrefix(toolName, routeTarget+"_")
	slog.Info(" Stripped tool name: %s", strippedToolName)
	return strippedToolName, true
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

func getAuthority(config *Config, toolName string) (hostname string) {
// for testing
	if config == nil {
		config = getDefaultConfig()
		slog.Info("Using default config")
	}else{
		slog.Info("Using provided config")
	}
	
	slog.Info("Checking tool '%s' against %d servers", toolName, len(config.Servers))
	
	for _, s := range config.Servers {
		if s.Enabled && s.ToolPrefix != "" && strings.HasPrefix(toolName, s.ToolPrefix) {
			slog.Info("Found matching server: %s -> %s", s.ToolPrefix, s.Hostname)
			return s.Hostname
		}
	}
	
	slog.Warn("No matching server found for tool: '%s'", toolName)
	return ""
}

// getDefaultConfig returns a default configuration for testing
func getDefaultConfig() *Config {
	return &Config{
		LogLevel: "info",
		Port:     8080,
		Servers: []Server{
			{
				Name:       "mcp-system/weather-route",
				URL:        "http://weather-service.mcp-system.svc.cluster.local:8080",
				Hostname:   "weather.example.com",
				ToolPrefix: "weather_",
				Enabled:    true,
			},
			{
				Name:       "mcp-system/calendar-route",
				URL:        "http://calendar-service.mcp-system.svc.cluster.local:8080",
				Hostname:   "calendar.example.com",
				ToolPrefix: "cal_",
				Enabled:    true,
			},
			{
				Name:       "kb-system/kbase-route",
				URL:        "http://kbase-service.kb-system.svc.cluster.local:8080",
				Hostname:   "kbase.example.com",
				ToolPrefix: "ai_",
				Enabled:    true,
			},
		},
	}
}



// HandleRequestBody handles request bodies for MCP tool calls.
func (s *ExtProcServer) HandleRequestBody(ctx context.Context, data map[string]any,config *Config) ([]*eppb.ProcessingResponse, error) {
	slog.Info(" Processing request body for MCP tool calls...")
	// slog.Println("FULL RESPONSE BODY", data)
	// Extract tool name - only process tools/call
	toolName := extractMCPToolName(data)
	if toolName == "" {
		slog.Debug(
			"[EXT-PROC] HandleRequestBody No MCP tool name found or not tools/call, continuing to helper",
		)
		return s.createEmptyBodyResponse(), nil
	}

	slog.Debug("[EXT-PROC] HandleRequestBody", "Tool name:", toolName)

	// Determine routing based on tool prefix
	routeTarget := getRouteTargetFromTool(toolName)
	if routeTarget == "" {
		slog.Info(
			"[EXT-PROC] HandleRequestBody Tool name  doesn't match any server prefix, continuing to helper",
			"tool",
			toolName,
		)
		return s.createEmptyBodyResponse(), nil
	}

	slog.Info(" Routing to: %s", routeTarget)
	authority := getAuthority(config, toolName)
	slog.Info("AUTHORITY: '%s' for tool: '%s'", authority, toolName)
	 	 


	// Strip server prefix from tool name and modify request body
	strippedToolName, _ := stripServerPrefix(toolName)
	slog.Info(" Stripped tool name: %s", strippedToolName)

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
		slog.Info(" No mcp-session-id found in headers")
		return s.createErrorResponse("No session ID found", 400), nil
	}

	slog.Info(" Helper session: %s", helperSession)

	// Use broker to exchange for upstream MCP session
	var upstreamSession string
// 	if s.Broker != nil {
// 		us, err := s.Broker.ExchangeSession(ctx, authority, routeTarget, gatewaySession)
// 		if err != nil {
// 			slog.Errorf(" Failed session exchange via broker: %v", err)
// 			return s.createErrorResponse("Session exchange failed", 502), nil
// 		}
// 		upstreamSession = us
// 		slog.Infof(" Upstream session from broker: %s", upstreamSession)
// 	} else {
// 		slog.Warn(" Broker not configured; proceeding without upstream session")
// 	}

	return s.createRoutingResponse(toolName, requestBodyBytes, routeTarget, authority, upstreamSession), nil
}

// createRoutingResponse creates a response with routing headers and session mapping
func (s *ExtProcServer) createRoutingResponse(toolName string, bodyBytes []byte, routeTarget,authority string, backendSession string,) []*eppb.ProcessingResponse {

	headers := []*basepb.HeaderValueOption{
		{
			Header: &basepb.HeaderValue{
				Key:      toolHeader,
				RawValue: []byte(toolName),
			},
		},
		{
			Header: &basepb.HeaderValue{
				Key:      serverHeader,
				RawValue: []byte(routeTarget),
			},
		},
		{
			Header: &basepb.HeaderValue{
				Key:      authorityHeader,
				RawValue: []byte(authority),
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

	// Update content-length header to match the modified body
	contentLength := fmt.Sprintf("%d", len(bodyBytes))
	headers = append(headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      "content-length",
			RawValue: []byte(contentLength),
		},
	})

	if s.streaming {
		slog.Info("  Using streaming mode - returning header response first")
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
		slog.Info(
			" Completed MCP processing with routing to %s (streaming)",
			routeTarget,
		)
		return ret
	}

	// For non-streaming: Set headers in RequestBody response with ClearRouteCache
	slog.Info("  Using non-streaming mode - setting headers in body response")
	slog.Info(" Completed MCP processing with routing to %s", routeTarget)
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
	slog.Errorf("Returning %d error: %s", statusCode, message)

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
	slog.Info("HandleRequestHeaders called - streaming: %v", s.streaming)
	if headers != nil && headers.Headers != nil {
		for _, header := range headers.Headers.Headers {
			if strings.ToLower(header.Key) == "content-type" ||
				strings.ToLower(header.Key) == "mcp-session-id" {
				slog.Info("Header: %s = %s", header.Key, string(header.RawValue))
			}
		}
	}
	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_RequestHeaders{
				RequestHeaders: &eppb.HeadersResponse{},
			},
		},
	}, nil
}
