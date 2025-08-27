// Based on sample https://github.com/mark3labs/mcp-go/blob/93935261086dda133e3e4b6447304e24deb56a21/www/docs/pages/servers/basics.mdx

// Package server2 implements a simple MCP server that implements a few tools
// - The "hello_world" tool from the library sample
// - A "time" tool that returns the current time
// - A "slow" tool that waits N seconds, notifying the client of progress
// - A "headers" tool that returns all HTTP headers it received
package server2

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// StartupFunc is used for functions that will start a server and block until it is finished
type StartupFunc func() error

// ShutdownFunc is used for functions that stop running servers
type ShutdownFunc func() error

// RunServer create a server that can be started and stopped
func RunServer(transport, port string) (StartupFunc, ShutdownFunc, error) {

	hooks := &server.Hooks{}

	// Note that AddOnRegisterSession is for GET, not POST, for a session.
	// https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#listening-for-messages-from-the-server
	hooks.AddOnRegisterSession(func(_ context.Context, session server.ClientSession) {
		log.Printf("Client %s connected", session.SessionID())
	})

	hooks.AddOnUnregisterSession(func(_ context.Context, session server.ClientSession) {
		log.Printf("Client %s disconnected", session.SessionID())
	})

	// Add request hooks
	hooks.AddBeforeAny(func(_ context.Context, _ any, method mcp.MCPMethod, _ any) {
		log.Printf("Processing %s request", method)
	})

	hooks.AddOnError(func(_ context.Context, _ any, method mcp.MCPMethod, _ any, err error) {
		log.Printf("Error in %s: %v", method, err)
	})

	// Create a new MCP server
	s := server.NewMCPServer(
		"Demo rocket",
		"1.0.0",
		server.WithHooks(hooks),
		server.WithToolCapabilities(true),
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

	// Add auth1234 handler
	s.AddTool(mcp.NewTool("auth1234",
		mcp.WithDescription("check authorization header"),
	), auth1234ToolHandler)

	// Add slow handler
	s.AddTool(mcp.NewTool("slow",
		mcp.WithDescription("Delay for N seconds"),
		mcp.WithString("seconds",
			mcp.Required(),
			mcp.Description("number of seconds to wait"),
		),
	), slowHandler)

	if port == "" {
		port = "8080"
	}

	switch transport {
	case "http":
		// Define the HTTP server with interceptor to record HTTP headers
		mux := http.NewServeMux()
		httpServer := &http.Server{
			Addr:              ":" + port,
			Handler:           mux,
			ReadHeaderTimeout: 3 * time.Second,
		}

		streamableHTTPServer := server.NewStreamableHTTPServer(
			s,
			server.WithStreamableHTTPServer(httpServer),
		)
		mux.Handle("/mcp", streamableHTTPServer)

		// For testing session ID invalidation
		mux.HandleFunc("/admin/forget", forgetFuncFactory(s))

		return func() error {
				fmt.Printf("Serving HTTPStreamable on http://localhost:%s/mcp\n", port)

				return streamableHTTPServer.Start(":" + port)
			}, func() error {
				// We use a timeout because the MCP inspector holds the port open
				shutdownCtx, shutdownRelease := context.WithTimeout(
					context.Background(),
					1*time.Second,
				)
				defer shutdownRelease()
				return streamableHTTPServer.Shutdown(shutdownCtx)
			}, nil
	case "sse":
		fmt.Printf("Serving SSE on http://localhost:%s\n", port)
		sseServer := server.NewSSEServer(s)

		return func() error {
				return sseServer.Start(":" + port)
			}, func() error {
				return sseServer.Shutdown(context.TODO())
			}, nil
	default:
		fmt.Print("Serving on stdio")
		return func() error {
				return server.ServeStdio(s)
			}, func() error {
				return nil
			}, nil
	}
}

func helloHandler(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
}

func timeHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(time.Now().String()), nil
}

func headersToolHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	content := make([]mcp.Content, 0)
	for k, v := range req.Header {
		content = append(content, &mcp.TextContent{
			Type: "text",
			Text: fmt.Sprintf("%s: %v", k, v),
		})
	}

	return &mcp.CallToolResult{
		Content: content}, nil
}

func auth1234ToolHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	auth := strings.ToLower(req.Header.Get("Authorization"))
	if auth != "bearer 1234" {
		return nil, fmt.Errorf("requires Authorization: bearer 1234, got %q", auth)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Text: "Success!",
			},
		},
	}, nil
}

func slowHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	seconds, err := request.RequireInt("seconds")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	var progressToken mcp.ProgressToken
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

func forgetFuncFactory(mcpServer *server.MCPServer) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failure: %v", err), http.StatusInternalServerError)
			return
		}

		sessionID := string(body)

		// We can't check if the client exists
		log.Printf("Client %s will be forcibly disconnected (if it exists)", sessionID)
		mcpServer.UnregisterSession(req.Context(), sessionID)
	}
}
