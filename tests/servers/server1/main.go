// Based on https://github.com/modelcontextprotocol/go-sdk/blob/5bd02a3c0451110e8e01a56b9fcfeb048c560a92/examples/server/hello/main.go

// A simple MCP server that implements a few tools
// - The "hi" tool from the library sample
// - A "time" tool that returns the current time
// - A "slow" tool that waits N seconds, notifying the client of progress
// - A "headers" tool that returns all HTTP headers it received
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type contextKey string

const (
	// HeadersKey is used to save HTTP headers in request context, for the "headers" tool
	HeadersKey contextKey = "http-headers"
)

var httpAddr = flag.String(
	"http",
	"",
	"if set, use streamable HTTP at this address, instead of stdin/stdout",
)

type hiArgs struct {
	Name string `json:"name" jsonschema:"the name to say hi to"`
}

func sayHi(
	_ context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[hiArgs],
) (*mcp.CallToolResultFor[struct{}], error) {
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Hi " + params.Arguments.Name},
		},
	}, nil
}

// A simple tool that returns the current time
func timeTool(
	_ context.Context,
	_ *mcp.ServerSession,
	_ *mcp.CallToolParamsFor[struct{}],
) (*mcp.CallToolResultFor[struct{}], error) {
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: time.Now().String()},
		},
	}, nil
}

// A simple tool that returns all HTTP headers it received
func headersTool(
	ctx context.Context,
	_ *mcp.ServerSession,
	_ *mcp.CallToolParamsFor[struct{}],
) (*mcp.CallToolResultFor[struct{}], error) {
	content := make([]mcp.Content, 0)
	headers, ok := ctx.Value(HeadersKey).(http.Header)
	if ok {
		for k, v := range headers {
			content = append(content, &mcp.TextContent{Text: fmt.Sprintf("%s: %v", k, v)})
		}
	}

	return &mcp.CallToolResultFor[struct{}]{
		Content: content,
	}, nil
}

type slowArgs struct {
	Seconds int `json:"seconds" jsonschema:"number of seconds to wait"`
}

type addToolArgs struct {
	Name        string `json:"name" jsonschema:"the name of the new tool to add"`
	Description string `json:"description" jsonschema:"the description of the new tool"`
}

type dynamicToolManager struct {
	server *mcp.Server
}

// addTool dynamically adds a new tool to the server and triggers notifications/tools/list_changed
func (m *dynamicToolManager) addTool(
	_ context.Context,
	_ *mcp.ServerSession,
	params *mcp.CallToolParamsFor[addToolArgs],
) (*mcp.CallToolResultFor[struct{}], error) {
	name := params.Arguments.Name
	desc := params.Arguments.Description
	if desc == "" {
		desc = "dynamically added tool"
	}

	mcp.AddTool(m.server, &mcp.Tool{Name: name, Description: desc}, func(
		_ context.Context,
		_ *mcp.ServerSession,
		_ *mcp.CallToolParamsFor[struct{}],
	) (*mcp.CallToolResultFor[struct{}], error) {
		return &mcp.CallToolResultFor[struct{}]{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "I am the dynamically added tool: " + name},
			},
		}, nil
	})

	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Added new tool: %s", name)},
		},
	}, nil
}

// A long-running tool that waits N seconds, notifying the client of progress
func slowTool(
	ctx context.Context,
	ss *mcp.ServerSession,
	params *mcp.CallToolParamsFor[slowArgs],
) (*mcp.CallToolResultFor[struct{}], error) {
	startTime := time.Now()
	fmt.Printf("Slow tool will wait for %d seconds\n", params.Arguments.Seconds)
	for {
		waited := int(time.Since(startTime).Seconds())
		if waited >= params.Arguments.Seconds {
			break
		}

		var progressToken string
		if params.Meta != nil {
			rawProgressToken := params.Meta["progressToken"]
			switch v := rawProgressToken.(type) {
			case string:
				progressToken = v
			case float64:
				progressToken = fmt.Sprintf("%d", int(v))
			default:
				fmt.Printf("Unexpected type for progressToken: %T\n", v)
			}
		}

		if progressToken != "" {
			fmt.Printf("Notify client that we have waited %d seconds\n", waited)
			err := ss.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
				Message:       fmt.Sprintf("Waited %d seconds...", waited),
				ProgressToken: progressToken,
				Progress:      float64(waited),
			})
			if err != nil {
				fmt.Printf("NotifyProgress error: %v\n", err)
			}
		}

		time.Sleep(1 * time.Second)
	}

	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{},
	}, nil
}

func promptHi(
	_ context.Context,
	_ *mcp.ServerSession,
	params *mcp.GetPromptParams,
) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Code review prompt",
		Messages: []*mcp.PromptMessage{
			{
				Role:    "user",
				Content: &mcp.TextContent{Text: "Say hi to " + params.Arguments["name"]},
			},
		},
	}, nil
}

func main() {
	flag.Parse()

	server := mcp.NewServer(&mcp.Implementation{Name: "test mcp server 1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "greet", Description: "say hi"}, sayHi)
	mcp.AddTool(server, &mcp.Tool{Name: "time", Description: "get current time", Annotations: &mcp.ToolAnnotations{Title: "time"}}, timeTool)
	mcp.AddTool(server, &mcp.Tool{Name: "slow", Description: "delay N seconds"}, slowTool)
	mcp.AddTool(server, &mcp.Tool{Name: "headers", Description: "get headers"}, headersTool)

	toolManager := &dynamicToolManager{server: server}
	mcp.AddTool(server, &mcp.Tool{Name: "add_tool", Description: "dynamically add a new tool (triggers notifications/tools/list_changed)", Annotations: &mcp.ToolAnnotations{Title: "add"}}, toolManager.addTool)

	server.AddPrompt(&mcp.Prompt{Name: "greet"}, promptHi)

	server.AddResource(&mcp.Resource{
		Name:     "info",
		MIMEType: "text/plain",
		URI:      "embedded:info",
	}, handleEmbeddedResource)

	if *httpAddr != "" {
		server.AddReceivingMiddleware(rpcPrintMiddleware)
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server
		}, nil)

		// Create a new HTTP handler that records headers
		handler2 := mcpRecordHeaders{
			Handler: handler,
		}

		log.Printf("MCP handler will listen at %s", *httpAddr)
		server := &http.Server{
			Addr:              *httpAddr,
			Handler:           handler2,
			ReadHeaderTimeout: 3 * time.Second,
		}
		_ = server.ListenAndServe()
	} else {
		log.Printf("MCP handler use stdio")
		t := mcp.NewLoggingTransport(mcp.NewStdioTransport(), os.Stderr)
		if err := server.Run(context.Background(), t); err != nil {
			log.Printf("Server failed: %v", err)
		}
	}
}

type mcpRecordHeaders struct {
	Handler http.Handler
}

// ServeHTTP implements http.Handler.
func (m mcpRecordHeaders) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Save the headers in the request context
	newReq := req.WithContext(context.WithValue(req.Context(),
		HeadersKey, req.Header))
	m.Handler.ServeHTTP(rw, newReq)
}

// Simple middleware that just prints the method and parameters
func rpcPrintMiddleware(
	next mcp.MethodHandler[*mcp.ServerSession],
) mcp.MethodHandler[*mcp.ServerSession] {
	return func(
		ctx context.Context,
		session *mcp.ServerSession,
		method string,
		params mcp.Params,
	) (mcp.Result, error) {
		fmt.Printf("MCP method started: method=%s, session_id=%s, params=%v\n",
			method,
			session.ID(),
			params,
		)

		result, err := next(ctx, session, method, params)
		return result, err
	}
}

var embeddedResources = map[string]string{
	"info": "This is the hello example server.",
}

func handleEmbeddedResource(
	_ context.Context,
	_ *mcp.ServerSession,
	params *mcp.ReadResourceParams,
) (*mcp.ReadResourceResult, error) {
	u, err := url.Parse(params.URI)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "embedded" {
		return nil, fmt.Errorf("wrong scheme: %q", u.Scheme)
	}
	key := u.Opaque
	text, ok := embeddedResources[key]
	if !ok {
		return nil, fmt.Errorf("no embedded resource named %q", key)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{URI: params.URI, MIMEType: "text/plain", Text: text},
		},
	}, nil
}
