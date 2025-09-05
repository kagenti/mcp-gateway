// Server4 - Intentionally "bad" MCP server for testing validation failures
// This server is designed to fail all validation checks:
// - Uses old protocol version (2024-11-05 instead of 2025-06-18)
// - Has no tools capability (missing required capability)
// - Simulates connection issues
// - Has problematic transport implementation

package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var httpAddr = flag.String(
	"http",
	"",
	"if set, use streamable HTTP at this address, instead of stdin/stdout",
)

var simulateFailure = flag.String(
	"failure-mode",
	"protocol",
	"type of failure to simulate: protocol, no-tools, connection, crash",
)

func main() {
	flag.Parse()

	// Simulate different types of failures based on environment
	failureMode := os.Getenv("FAILURE_MODE")
	if failureMode == "" {
		failureMode = *simulateFailure
	}

	log.Printf("Server4 starting with failure mode: %s", failureMode)

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
			log.Printf("Server4 received %s request to %s", r.Method, r.URL.Path)

			if r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)

				badInitResponse := `{
					"jsonrpc": "2.0",
					"id": 1,
					"result": {
						"protocolVersion": "2024-11-05",
						"capabilities": {
							"tools": {}
						},
						"serverInfo": {
							"name": "bad-test-server4",
							"version": "1.0.0"
						}
					}
				}`
				w.Write([]byte(badInitResponse))
				log.Printf("Server4 sent bad protocol response: 2024-11-05")
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"status": "bad-server4-running"}`))
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
		log.Fatal("Server4 refusing stdio connection")
	}
}

func createServerWithNoTools() {
	// Create a valid MCP server but with NO tools (missing required capability)
	s := server.NewMCPServer(
		"bad-test-server4-no-tools",
		"1.0.0",
	)

	if *httpAddr != "" {
		log.Printf("No-tools server will listen at %s", *httpAddr)

		mux := http.NewServeMux()
		httpServer := &http.Server{
			Addr:              *httpAddr,
			Handler:           mux,
			ReadHeaderTimeout: 3 * time.Second,
		}

		streamableHTTPServer := server.NewStreamableHTTPServer(s, server.WithStreamableHTTPServer(httpServer))
		mux.Handle("/mcp", streamableHTTPServer)

		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Server4 received %s request to %s", r.Method, r.URL.Path)
			w.WriteHeader(200)
			w.Write([]byte("no-tools-server-healthy"))
		})

		if err := streamableHTTPServer.Start(*httpAddr); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	} else {
		log.Printf("No-tools server using stdio - not supported in this version")
	}
}

// Tool handlers for conflicting tools
func conflictingTimeHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("Server4 conflicting time: " + time.Now().Format(time.RFC3339)), nil
}

func conflictingSlowHandler(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("Server4 slow tool (for conflict testing)"), nil
}

func conflictingHeadersHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("Server4 headers tool (for conflict testing)"), nil
}

func createServerWithConflictingTools() {
	// Create server that provides tools with names that will conflict with other servers

	s := server.NewMCPServer(
		"conflicting-test-server4",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	s.AddTool(mcp.NewTool("time", mcp.WithDescription("Get current time - CONFLICTING VERSION")), conflictingTimeHandler)
	s.AddTool(mcp.NewTool("slow", mcp.WithDescription("Slow operation - CONFLICTING VERSION")), conflictingSlowHandler)
	s.AddTool(mcp.NewTool("headers", mcp.WithDescription("Get headers - CONFLICTING VERSION")), conflictingHeadersHandler)

	if *httpAddr != "" {
		log.Printf("Conflicting tools server will listen at %s", *httpAddr)

		mux := http.NewServeMux()
		httpServer := &http.Server{
			Addr:              *httpAddr,
			Handler:           mux,
			ReadHeaderTimeout: 3 * time.Second,
		}

		streamableHTTPServer := server.NewStreamableHTTPServer(
			s,
			server.WithStreamableHTTPServer(httpServer),
		)
		mux.Handle("/mcp", streamableHTTPServer)

		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Server4 received %s request to %s", r.Method, r.URL.Path)
			w.WriteHeader(200)
			w.Write([]byte("conflicting-tools-server-healthy"))
		})

		if err := streamableHTTPServer.Start(*httpAddr); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	} else {
		log.Printf("Conflicting tools server using stdio - not supported in this version")
	}
}
