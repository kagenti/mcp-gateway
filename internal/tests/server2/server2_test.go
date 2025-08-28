package server2

import (
	"context"
	"net/http"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestHello(t *testing.T) {
	res, err := helloHandler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.True(t, res.IsError)

	res, err = helloHandler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{
				"name": "Fred",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.IsType(t, mcp.TextContent{}, res.Content[0])
	require.Equal(t, "Hello, Fred!", res.Content[0].(mcp.TextContent).Text)
}

func TestTime(t *testing.T) {
	res, err := timeHandler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.IsType(t, mcp.TextContent{}, res.Content[0])
}

func TestHeaders(t *testing.T) {
	res, err := headersToolHandler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, res.Content, 0)

	res, err = headersToolHandler(context.Background(), mcp.CallToolRequest{
		Header: http.Header{
			"Authorization": []string{"bearer 1234"},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, res.Content, 1)
	require.IsType(t, &mcp.TextContent{}, res.Content[0])
	require.Equal(t, "Authorization: [bearer 1234]", res.Content[0].(*mcp.TextContent).Text)
}

func TestSlow(t *testing.T) {
	res, err := slowHandler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.True(t, res.IsError)

	res, err = slowHandler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{
				// Note that although seconds is an int, it is passed
				// as 0 by the MCP inspector.  This test follows the same
				// convention.  TODO: Verify the libraries are following the spec.
				"seconds": "0",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.IsType(t, mcp.TextContent{}, res.Content[0])
}

func TestAuth1234(t *testing.T) {
	_, err := auth1234ToolHandler(context.Background(), mcp.CallToolRequest{})
	require.Error(t, err)

	res, err := auth1234ToolHandler(context.Background(), mcp.CallToolRequest{
		Header: http.Header{
			"Authorization": []string{"bearer 1234"},
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.IsType(t, mcp.TextContent{}, res.Content[0])
}

func TestRunStreamableServer(t *testing.T) {
	startFunc, shutdownFunc, err := RunServer("http", "8085")
	require.NoError(t, err)
	require.NotNil(t, startFunc)
	require.NotNil(t, shutdownFunc)
}
