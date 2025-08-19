package main

import (
	"log"
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
func (s *ExtProcServer) HandleResponseHeaders(headers *eppb.HttpHeaders) ([]*eppb.ProcessingResponse, error) {
	log.Println("[EXT-PROC] Processing response headers for session mapping...")

	if headers == nil || headers.Headers == nil {
		log.Println("[EXT-PROC] No response headers to process")
		return []*eppb.ProcessingResponse{
			{
				Response: &eppb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &eppb.HeadersResponse{},
				},
			},
		}, nil
	}

	// Look for mcp-session-id header that needs reverse mapping
	var mcpSessionID string
	for _, header := range headers.Headers.Headers {
		if strings.ToLower(header.Key) == "mcp-session-id" {
			mcpSessionID = string(header.RawValue)
			break
		}
	}

	if mcpSessionID == "" {
		log.Println("[EXT-PROC] No mcp-session-id in response headers")
		return []*eppb.ProcessingResponse{
			{
				Response: &eppb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &eppb.HeadersResponse{},
				},
			},
		}, nil
	}

	log.Printf("[EXT-PROC] Response backend session: %s", mcpSessionID)

	// Check if this is a backend session that needs mapping back to helper session
	helperSession := extractHelperSessionFromBackend(mcpSessionID)
	if helperSession == "" {
		// Not a backend session ID, leave as-is
		log.Println("[EXT-PROC] Session ID doesn't need reverse mapping")
		return []*eppb.ProcessingResponse{
			{
				Response: &eppb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &eppb.HeadersResponse{},
				},
			},
		}, nil
	}

	log.Printf("[EXT-PROC] Mapping backend session back to helper session: %s", helperSession)

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
func (s *ExtProcServer) HandleResponseBody(body *eppb.HttpBody) ([]*eppb.ProcessingResponse, error) {
	log.Printf("[EXT-PROC] Processing response body... (size: %d, end_of_stream: %t)",
		len(body.GetBody()), body.GetEndOfStream())

	// Log the response body content if it's not too large
	if len(body.GetBody()) > 0 && len(body.GetBody()) < 1000 {
		log.Printf("[EXT-PROC] Response body content: %s", string(body.GetBody()))
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
func (s *ExtProcServer) HandleResponseTrailers(_ *eppb.HttpTrailers) ([]*eppb.ProcessingResponse, error) {
	log.Println("[EXT-PROC] Processing response trailers...")

	return []*eppb.ProcessingResponse{
		{
			Response: &eppb.ProcessingResponse_ResponseTrailers{
				ResponseTrailers: &eppb.TrailersResponse{},
			},
		},
	}, nil
}
