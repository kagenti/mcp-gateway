// Broken-server - Intentionally "bad" MCP server for testing validation failures
// This server is designed to fail validation checks:
// - Uses old protocol version (2024-11-05 instead of 2025-06-18)
// - Has no tools capability (missing required capability)
// - Has problematic transport implementation
// Note: Connection failures are tested by scaling the deployment to 0 replicas

package main

import (
	"context"
	"flag"
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

var simulateFailure = flag.String(
	"failure-mode",
	"protocol",
	"type of failure to simulate: protocol, no-tools, tool-conflicts",
)

func main() {
	flag.Parse()

	// Simulate different types of failures based on environment
	failureMode := os.Getenv("FAILURE_MODE")
	if failureMode == "" {
		failureMode = *simulateFailure
	}

	log.Printf("Broken-server starting with failure mode: %s", failureMode)

	switch failureMode {
	case "no-tools":
		log.Printf("Simulating server with no tools capability...")
		createServerWithNoTools()

	case "tool-conflicts":
		log.Printf("Simulating server with conflicting tool names...")
		createServerWithConflictingTools()

	default: // "protocol"
		log.Printf("Simulating server with wrong protocol version...")
		createServerWithWrongProtocol()
	}
}

func createServerWithWrongProtocol() {
	// Create server but we'll intentionally use wrong protocol version

	if *httpAddr != "" {
		log.Printf("Bad server will listen at %s with wrong protocol", *httpAddr)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Broken-server received %s request to %s", r.Method, r.URL.Path)

			if r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)

				badInitResponse := `{
					"jsonrpc": "2.0",
					"id": 1,
					"result": {
						"protocolVersion": "2021-11-05",
						"capabilities": {
							"tools": {}
						},
						"serverInfo": {
							"name": "bad-test-Broken-server",
							"version": "1.0.0"
						}
					}
				}`
				w.Write([]byte(badInitResponse))
				log.Printf("Broken-server sent bad protocol response: 2024-11-05")
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"status": "bad-Broken-server-running"}`))
		})

		server := &http.Server{
			Addr:              *httpAddr,
			Handler:           handler,
			ReadHeaderTimeout: 3 * time.Second,
		}

		log.Printf("Starting HTTP server on %s", *httpAddr)
		if err := server.ListenAndServe(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	} else {
		log.Printf("Bad server using stdio - this will also fail")
		log.Fatal("Broken-server refusing stdio connection")
	}
}

func createServerWithNoTools() {
	// Create a valid MCP server but with NO tools (missing required capability)
	server := mcp.NewServer(&mcp.Implementation{Name: "bad-test-Broken-server-no-tools", Version: "1.0.0"}, nil)

	if *httpAddr != "" {
		log.Printf("No-tools server will listen at %s", *httpAddr)

		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server
		}, nil)

		httpServer := &http.Server{
			Addr:              *httpAddr,
			Handler:           handler,
			ReadHeaderTimeout: 3 * time.Second,
		}

		if err := httpServer.ListenAndServe(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	} else {
		log.Printf("No-tools server using stdio - not supported in this version")
	}
}

// Tool handlers for conflicting tools
func conflictingTimeHandler(_ context.Context, _ *mcp.ServerSession, _ *mcp.CallToolParamsFor[struct{}]) (*mcp.CallToolResultFor[struct{}], error) {
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Broken-server conflicting time: " + time.Now().Format(time.RFC3339)},
		},
	}, nil
}

func conflictingSlowHandler(_ context.Context, _ *mcp.ServerSession, _ *mcp.CallToolParamsFor[struct{}]) (*mcp.CallToolResultFor[struct{}], error) {
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Broken-server slow tool (for conflict testing)"},
		},
	}, nil
}

func conflictingHeadersHandler(_ context.Context, _ *mcp.ServerSession, _ *mcp.CallToolParamsFor[struct{}]) (*mcp.CallToolResultFor[struct{}], error) {
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Broken-server headers tool (for conflict testing)"},
		},
	}, nil
}

func createServerWithConflictingTools() {
	// Create server that provides tools with names that will conflict with other servers
	server := mcp.NewServer(&mcp.Implementation{Name: "conflicting-test-Broken-server", Version: "1.0.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "time",
		Description: "Get current time - CONFLICTING VERSION",
	}, conflictingTimeHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "slow",
		Description: "Slow operation - CONFLICTING VERSION",
	}, conflictingSlowHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "headers",
		Description: "Get headers - CONFLICTING VERSION",
	}, conflictingHeadersHandler)

	if *httpAddr != "" {
		log.Printf("Conflicting tools server will listen at %s", *httpAddr)

		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server
		}, nil)

		httpServer := &http.Server{
			Addr:              *httpAddr,
			Handler:           handler,
			ReadHeaderTimeout: 3 * time.Second,
		}

		if err := httpServer.ListenAndServe(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	} else {
		log.Printf("Conflicting tools server using stdio - not supported in this version")
	}
}
