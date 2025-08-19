// main implements the CLI for mcp-router
package main

import (
	"log"
	"net"
	"os"

	extProcV3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
)

// ExtProcServer represents an Envoy external processor.
type ExtProcServer struct {
	streaming      bool
	requestHeaders *extProcV3.HttpHeaders // Store headers for later use in body processing
}

// Process handles Envoy external process routing
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

func main() {
	lis, err := net.Listen("tcp", getEnv("SERVER_ADDRESS", "localhost:9002"))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	extProcV3.RegisterExternalProcessorServer(s, &ExtProcServer{})

	log.Println("Ext-proc server starting on port 9002...")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
