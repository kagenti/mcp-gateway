// Based on sample https://github.com/mark3labs/mcp-go/blob/93935261086dda133e3e4b6447304e24deb56a21/www/docs/pages/servers/basics.mdx

// A simple MCP server that implements a few tools
// - The "hello_world" tool from the library sample
// - A "time" tool that returns the current time
// - A "slow" tool that waits N seconds, notifying the client of progress
// - A "headers" tool that returns all HTTP headers it received
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type contextKey string

const (
	// HeadersKey saves HTTP headers in request context
	HeadersKey contextKey = "http-headers"
)

type mcpRecordHeaders struct {
	Handler http.Handler
}

func main() {
	hooks := &server.Hooks{}

	// Add session lifecycle hooks
	hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		log.Printf("Client %s connected", session.SessionID())
	})

	hooks.AddOnUnregisterSession(func(ctx context.Context, session server.ClientSession) {
		log.Printf("Client %s disconnected", session.SessionID())
	})

	// Add request hooks
	hooks.AddBeforeAny(func(ctx context.Context, id any, method mcp.MCPMethod, message any) {
		log.Printf("Processing %s request", method)
	})

	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		log.Printf("Error in %s: %v", method, err)
	})

	// Create a new MCP server
	s := server.NewMCPServer(
		"Demo 🚀",
		"1.0.0",
		server.WithHooks(hooks),
		server.WithToolCapabilities(true), // @@@ false?
	)

	// Add tool
	tool := mcp.NewTool("hello_world",
		mcp.WithDescription("Say hello to someone"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the person to greet"),
		),
	)

	// Add tool handler
	s.AddTool(tool, helloHandler)

	// Add time handler
	s.AddTool(mcp.NewTool("time",
		mcp.WithDescription("Get the current time"),
	), timeHandler)

	// Add headers handler
	s.AddTool(mcp.NewTool("headers",
		mcp.WithDescription("get HTTP headers"),
	), headersToolHandler)

	// Add slow handler
	s.AddTool(mcp.NewTool("slow",
		mcp.WithDescription("Delay for N seconds"),
		mcp.WithString("seconds",
			mcp.Required(),
			mcp.Description("number of seconds to wait"),
		),
	), slowHandler)

	// Start the stdio server
	// if err := server.ServeStdio(s); err != nil {
	// 	fmt.Printf("Server error: %v\n", err)
	// }

	// Choose transport based on environment
	transport := os.Getenv("MCP_TRANSPORT")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	var err error
	switch transport {
	case "http":
		fmt.Printf("Serving HTTPStreamable on http://localhost:%s/mcp\n", port)

		// Define the HTTP server with interceptor to record HTTP headers
		mux := http.NewServeMux()
		httpServer := &http.Server{
			Addr: ":" + port,
			Handler: mcpRecordHeaders{
				Handler: mux,
			},
		}

		streamableHttpServer := server.NewStreamableHTTPServer(s, server.WithStreamableHTTPServer(httpServer))
		mux.Handle("/mcp", streamableHttpServer)

		err = streamableHttpServer.Start(":" + port)
	case "sse":
		fmt.Printf("Serving SSE on http://localhost:%s\n", port)
		sseServer := server.NewSSEServer(s)
		err = sseServer.Start(":" + port)
	default:
		fmt.Print("Serving on stdio")
		err = server.ServeStdio(s)
	}
	if err != nil {
		fmt.Printf("Server error: %v\n", err)
		return
	}

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println("Shutting down server...")
	// s.Shutdown()

	fmt.Print("Server completed\n")
}

func helloHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
}

func timeHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(time.Now().String()), nil
}

func headersToolHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	content := make([]mcp.Content, 0)
	headers, ok := ctx.Value(HeadersKey).(http.Header)
	if ok {
		for k, v := range headers {
			content = append(content, &mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf("%s: %v", k, v),
			})
		}
	}

	return &mcp.CallToolResult{
		Content: content}, nil
}

func slowHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	seconds, err := request.RequireInt("seconds")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	var progressToken mcp.ProgressToken = nil
	if request.Params.Meta != nil {
		progressToken = request.Params.Meta.ProgressToken
	}
	server := server.ServerFromContext(ctx)

	startTime := time.Now()
	fmt.Printf("Slow tool will wait for %d seconds\n", seconds)
	for {
		waited := int(time.Since(startTime).Seconds())
		if waited >= seconds {
			break
		}

		if progressToken != nil {
			fmt.Printf("Notify client that we have waited %d seconds\n", waited)
			msg := fmt.Sprintf("Waited %d seconds...", waited)
			err := server.SendNotificationToClient(ctx, "notifications/progress", map[string]any{
				"progress":      waited,
				"progressToken": progressToken,
				"message":       msg,
			})
			if err != nil {
				fmt.Printf("NotifyProgress error: %v\n", err)
			}
		}

		time.Sleep(1 * time.Second)
	}

	return mcp.NewToolResultText("done"), nil
}

// ServeHTTP implements http.Handler.
func (m mcpRecordHeaders) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Save the headers in the request context
	newReq := req.WithContext(context.WithValue(req.Context(),
		HeadersKey, req.Header))
	m.Handler.ServeHTTP(rw, newReq)
}
