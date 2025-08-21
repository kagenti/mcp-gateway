// Package mcprouter ext proc process
package mcprouter

import (
	"log"

	extProcV3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
)

// ExtProcServer struct boolean for streaming & Store headers for later use in body processing
type ExtProcServer struct {
	streaming      bool
	requestHeaders *extProcV3.HttpHeaders
}

// Process function
func (s *ExtProcServer) Process(stream extProcV3.ExternalProcessor_ProcessServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			log.Printf("Error receiving request: %v", err)
			return err
		}

		// Log request details
		switch r := req.Request.(type) {
		case *extProcV3.ProcessingRequest_RequestHeaders:
			log.Printf("Request Headers: %+v", r.RequestHeaders.Headers.Headers)
		case *extProcV3.ProcessingRequest_RequestBody:
			log.Printf("Request Body: %s", string(r.RequestBody.Body))
		case *extProcV3.ProcessingRequest_ResponseHeaders:
			log.Printf("Response Headers: %+v", r.ResponseHeaders.Headers.Headers)
		case *extProcV3.ProcessingRequest_ResponseBody:
			log.Printf("Response Body: %s", string(r.ResponseBody.Body))
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
			log.Printf("Error sending response: %v", err)
			return err
		}
	}
}
