// test server with custom path /v1/special/mcp for testing custom path functionality
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var httpAddr = flag.String(
	"http",
	"",
	"if set, use streamable HTTP at this address, instead of stdin/stdout",
)

type echoArgs struct {
	Message string `json:"message" jsonschema:"the message to echo back"`
}

func echoTool(
	_ context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[echoArgs],
) (*mcp.CallToolResultFor[struct{}], error) {
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Echo from custom path: %s", params.Arguments.Message)},
		},
	}, nil
}

func pathInfoTool(
	_ context.Context,
	_ *mcp.ServerSession,
	_ *mcp.CallToolParamsFor[struct{}],
) (*mcp.CallToolResultFor[struct{}], error) {
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "This server is configured at /v1/special/mcp"},
		},
	}, nil
}

func timestampTool(
	_ context.Context,
	_ *mcp.ServerSession,
	_ *mcp.CallToolParamsFor[struct{}],
) (*mcp.CallToolResultFor[struct{}], error) {
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Timestamp: %s", time.Now().Format(time.RFC3339))},
		},
	}, nil
}

func main() {
	flag.Parse()

	// create mcp server using new API
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-custom-path-server",
		Version: "0.0.1",
	}, nil)

	// add tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo_custom",
		Description: "Echo a message from the custom path server",
	}, echoTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "path_info",
		Description: "Get information about this server's custom path",
	}, pathInfoTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "timestamp",
		Description: "Get current timestamp from custom path server",
	}, timestampTool)

	if *httpAddr != "" {
		// create streamable http handler
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server
		}, nil)

		// create custom mux to serve at specific path
		mux := http.NewServeMux()

		// serve mcp at custom path /v1/special/mcp
		mux.Handle("/v1/special/mcp", handler)

		// health check at root
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, "Custom path server with MCP at /v1/special/mcp\n")
			} else {
				fmt.Fprintf(w, "Custom path server NOT FOUND")
				http.NotFound(w, r)
			}
		})

		log.Printf("MCP custom-path-server listening at %s with custom path /v1/special/mcp", *httpAddr)
		httpServer := &http.Server{
			Addr:              *httpAddr,
			Handler:           mux,
			ReadHeaderTimeout: 3 * time.Second,
		}
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	} else {
		log.Printf("MCP custom-path-server using stdio")
		t := mcp.NewLoggingTransport(mcp.NewStdioTransport(), os.Stderr)
		if err := server.Run(context.Background(), t); err != nil {
			log.Fatalf("Error running server: %v", err)
		}
	}
}
