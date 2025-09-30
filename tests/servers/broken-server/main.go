// Broken-server - Intentionally "bad" MCP server for testing validation failures
// This server is designed to fail validation checks:
// - Uses old protocol version (2024-11-05 instead of 2025-06-18)
// - Has no tools capability (missing required capability)
// - Has problematic transport implementation
// Note: Connection failures are tested by scaling the deployment to 0 replicas

package main

import (
	"bytes"
	"context"
	"flag"
	"io"
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
	"type of failure to simulate: protocol, no-tools, tool-conflicts, 404-tool",
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

	case "404-tool":
		log.Printf("Simulating server with tool that always returns 404...")
		createServerWith404Tool()

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
						"protocolVersion": "2024-11-05",
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

func createServerWith404Tool() {
	// Create a normal MCP server but with a tool that always returns 404 via HTTP handler
	server := mcp.NewServer(&mcp.Implementation{Name: "404-tool-test-server", Version: "1.0.0"}, nil)

	// Add a tool that appears normal but we'll intercept its HTTP responses
	mcp.AddTool(server, &mcp.Tool{
		Name:        "always_404",
		Description: "A tool that always returns HTTP 404 to test router detection",
	}, func(_ context.Context, _ *mcp.ServerSession, _ *mcp.CallToolParamsFor[struct{}]) (*mcp.CallToolResultFor[struct{}], error) {
		return &mcp.CallToolResultFor[struct{}]{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "This tool should never return successfully due to 404 intercept"},
			},
		}, nil
	})

	if *httpAddr != "" {
		log.Printf("404-tool server will listen at %s", *httpAddr)

		// Create the MCP handler
		mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server
		}, nil)

		// Wrap with 404 interceptor for specific tool calls
		handler := &tool404Handler{
			handler: mcpHandler,
		}

		httpServer := &http.Server{
			Addr:              *httpAddr,
			Handler:           handler,
			ReadHeaderTimeout: 3 * time.Second,
		}

		if err := httpServer.ListenAndServe(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	} else {
		log.Printf("404-tool server using stdio - not supported in this version")
	}
}

// tool404Handler intercepts requests for the always_404 tool and returns 404
type tool404Handler struct {
	handler http.Handler
}

func (h *tool404Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if this is a tool call for our 404 tool
	if r.Method == "POST" {
		// Read the body to check if it's calling the always_404 tool
		body, err := io.ReadAll(r.Body)
		if err == nil && bytes.Contains(body, []byte("always_404")) {
			log.Printf("Intercepting always_404 tool call - returning HTTP 404")
			// Restore the body for potential logging
			r.Body = io.NopCloser(bytes.NewReader(body))

			// Return HTTP 404 with session ID header if present
			sessionID := r.Header.Get("mcp-session-id")
			if sessionID != "" {
				w.Header().Set("mcp-session-id", sessionID)
			}
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "Tool not found", "code": 404}`))
			return
		}
		// Restore the body for normal processing
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	// For all other requests, pass through normally
	h.handler.ServeHTTP(w, r)
}
