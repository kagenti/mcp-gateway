package mcprouter

import (
	"fmt"
	"log/slog"
	"strings"

	basepb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	eppb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
)

// extractHelperSessionFromBackend extracts the helper session ID from a backend session ID
// Returns empty string if not a backend session ID
func extractHelperSessionFromBackend(_ string) string {
	// TODO: check known server session prefixes
	return ""
}

// HandleResponseHeaders handles response headers for session ID reverse mapping
func (s *ExtProcServer) HandleResponseHeaders(
	headers *eppb.HttpHeaders) ([]*eppb.ProcessingResponse, error) {
	slog.Info("[EXT-PROC] Processing response headers for session mapping...")

	if headers == nil || headers.Headers == nil {
		slog.Info("[EXT-PROC] No response headers to process")
		return []*eppb.ProcessingResponse{
			{
				Response: &eppb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &eppb.HeadersResponse{},
				},
			},
		}, nil
	}

	// Check for HTTP 404 status indicating invalid session
	var httpStatus string
	var mcpSessionID string
	for _, header := range headers.Headers.Headers {
		if header.Key == ":status" {
			httpStatus = string(header.RawValue)
		}
		if strings.ToLower(header.Key) == "mcp-session-id" {
			mcpSessionID = string(header.RawValue)
		}
	}

	// Handle 404 responses from MCP servers
	if httpStatus == "404" {
		slog.Info("[EXT-PROC] Detected HTTP 404 from MCP server, invalidating router cache",
			"sessionID", mcpSessionID)

		if s.SessionCache != nil && mcpSessionID != "" {
			s.SessionCache.InvalidateByMCPSessionID(mcpSessionID)
			slog.Info("[EXT-PROC] Router cache invalidated for invalid MCP session",
				"invalidMcpSession", mcpSessionID)
		}
	}


	if mcpSessionID == "" {
		slog.Info("[EXT-PROC] No mcp-session-id in response headers")
		return []*eppb.ProcessingResponse{
			{
				Response: &eppb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &eppb.HeadersResponse{},
				},
			},
		}, nil
	}

	slog.Info(fmt.Sprintf("[EXT-PROC] Response backend session: %s", mcpSessionID))

	// Check if this is a backend session that needs mapping back to helper session
	helperSession := extractHelperSessionFromBackend(mcpSessionID)
	if helperSession == "" {
		// Not a backend session ID, leave as-is
		slog.Info("[EXT-PROC] Session ID doesn't need reverse mapping")
		return []*eppb.ProcessingResponse{
			{
				Response: &eppb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &eppb.HeadersResponse{},
				},
			},
		}, nil
	}

	slog.Info(
		fmt.Sprintf("[EXT-PROC] Mapping backend session back to helper session: %s", helperSession),
	)

	// Return response with updated session header
	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_ResponseHeaders{
				ResponseHeaders: &eppb.HeadersResponse{
					Response: &eppb.CommonResponse{
						HeaderMutation: &eppb.HeaderMutation{
							SetHeaders: []*basepb.HeaderValueOption{
								{
									Header: &basepb.HeaderValue{
										Key:      "mcp-session-id",
										RawValue: []byte(helperSession),
									},
								},
							},
						},
					},
				},
			},
		},
	}, nil
}

// HandleResponseBody handles response bodies.
func (s *ExtProcServer) HandleResponseBody(
	body *eppb.HttpBody) ([]*eppb.ProcessingResponse, error) {
	slog.Info(fmt.Sprintf("[EXT-PROC] Processing response body... (size: %d, end_of_stream: %t)",
		len(body.GetBody()), body.GetEndOfStream()))

	// slog the response body content if it's not too large
	if len(body.GetBody()) > 0 && len(body.GetBody()) < 1000 {
		slog.Info(fmt.Sprintf("[EXT-PROC] Response body content: %s", string(body.GetBody())))
	}

	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_ResponseBody{
				ResponseBody: &eppb.BodyResponse{},
			},
		},
	}, nil
}

// HandleResponseTrailers handles response trailers.
func (s *ExtProcServer) HandleResponseTrailers(
	_ *eppb.HttpTrailers,
) ([]*eppb.ProcessingResponse, error) {
	slog.Info("[EXT-PROC] Processing response trailers...")

	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_ResponseTrailers{
				ResponseTrailers: &eppb.TrailersResponse{},
			},
		},
	}, nil
}
