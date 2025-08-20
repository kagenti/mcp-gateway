package main

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestSayHi(t *testing.T) {
	res, err := sayHi(context.Background(), &mcp.ServerSession{}, &mcp.CallToolParamsFor[hiArgs]{
		Arguments: hiArgs{
			Name: "Fred",
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.NotNil(t, res)
	require.Len(t, res.Content, 1)
	require.IsType(t, &mcp.TextContent{}, res.Content[0])
	require.Equal(t, "Hi Fred", res.Content[0].(*mcp.TextContent).Text)
}

func TestTimeTool(t *testing.T) {
	res, err := timeTool(context.Background(), &mcp.ServerSession{}, &mcp.CallToolParamsFor[struct{}]{})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.NotNil(t, res)
	require.Len(t, res.Content, 1)
	require.IsType(t, &mcp.TextContent{}, res.Content[0])
}

func TestHeadersTool(t *testing.T) {
	res, err := headersTool(context.Background(), &mcp.ServerSession{}, &mcp.CallToolParamsFor[struct{}]{})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.NotNil(t, res)
	require.Len(t, res.Content, 0)
}

func TestSlowTool(t *testing.T) {
	res, err := slowTool(context.Background(), &mcp.ServerSession{}, &mcp.CallToolParamsFor[slowArgs]{})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.NotNil(t, res)
	require.Len(t, res.Content, 0)
}
