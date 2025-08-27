// Package mcprouter ext proc process
package mcprouter

import (
	"log/slog"

	extProcV3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/kagenti/mcp-gateway/internal/config"
)

// ExtProcServer struct boolean for streaming & Store headers for later use in body processing
type ExtProcServer struct {
	MCPConfig      *config.MCPServersConfig
	streaming      bool
	requestHeaders *extProcV3.HttpHeaders
	Logger         *slog.Logger
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
			s.Logger.Debug("[ext_proc] Process:", "Request Headers", r.RequestHeaders.Headers.Headers)
		case *extProcV3.ProcessingRequest_RequestBody:
			s.Logger.Debug("[ext_proc] Process:", "Request Body", string(r.RequestBody.Body))
		case *extProcV3.ProcessingRequest_ResponseHeaders:
			s.Logger.Debug("[ext_proc] Process:", "Response Headers", r.ResponseHeaders.Headers.Headers)
		case *extProcV3.ProcessingRequest_ResponseBody:
			s.Logger.Debug("[ext_proc] Process:", "Response Body", string(r.ResponseBody.Body))
		}

		// Send simple response to continue processing
		resp := &extProcV3.ProcessingResponse{
			Response: &extProcV3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &extProcV3.HeadersResponse{
					Response: &extProcV3.CommonResponse{
						Status: extProcV3.CommonResponse_CONTINUE,
					},
				},
			},
		}

		if err := stream.Send(resp); err != nil {
			s.Logger.Error("Error sending response", "error", err)
			return err
		}
	}
}
