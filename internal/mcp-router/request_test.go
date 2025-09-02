package mcprouter

import (
	"context"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	eppb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/kagenti/mcp-gateway/internal/broker"
	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/stretchr/testify/require"
)

func TestExtractMCPToolName(t *testing.T) {

	testcases := []struct {
		input  map[string]any
		output string
	}{
		{
			// No version
			input:  map[string]any{},
			output: "",
		},
		{
			// Unsupported version
			input: map[string]any{
				"jsonrpc": "1.9",
				"method":  "update",
			},
			output: "",
		},
		{
			// Numeric version
			input: map[string]any{
				"jsonrpc": 2.0,
				"method":  "tools/call",
			},
			output: "",
		},
		{
			// No method
			input: map[string]any{
				"jsonrpc": "2.0",
			},
			output: "",
		},
		{
			// Boolean method
			input: map[string]any{
				"jsonrpc": "2.0",
				"method":  false,
			},
			output: "",
		},
		{
			// non-MCP method
			input: map[string]any{
				"jsonrpc": "2.0",
				"method":  "update",
			},
			output: "",
		},
		{
			// no name param method
			input: map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
			},
			output: "",
		},
		{
			// invalid params
			input: map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params":  "dummy",
			},
			output: "",
		},
		{
			// no name param method
			input: map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params":  map[string]any{},
			},
			output: "",
		},
		{
			// invalid name
			input: map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]any{
					"name": 2.0,
				},
			},
			output: "",
		},
		{
			// invalid name
			input: map[string]any{
				"jsonrpc": "2.0",
				"method":  "tools/call",
				"params": map[string]any{
					"name": "dummy",
				},
			},
			output: "dummy",
		},
	}

	for _, testcase := range testcases {
		tool := extractMCPToolName(testcase.input)
		require.Equal(t, testcase.output, tool)
	}
}

func TestGetServerInfo(t *testing.T) {
	serverInfo := getServerInfo("", nil)
	require.Nil(t, serverInfo)

	serverInfo = getServerInfo("dummy", nil)
	require.Nil(t, serverInfo)

	serverInfo = getServerInfo("s_dummy", nil)
	require.Nil(t, serverInfo)

	serverInfo = getServerInfo("s_dummy", &config.MCPServersConfig{})
	require.Nil(t, serverInfo)

	serverInfo = getServerInfo("s_dummy", &config.MCPServersConfig{
		Servers: []*config.MCPServer{},
	})
	require.Nil(t, serverInfo)

	serverInfo = getServerInfo("s_dummy", &config.MCPServersConfig{
		Servers: []*config.MCPServer{
			{},
		},
	})
	require.Nil(t, serverInfo)

	serverInfo = getServerInfo("s_dummy", &config.MCPServersConfig{
		Servers: []*config.MCPServer{
			{
				Name:       "dummy",
				URL:        "http://localhost:8080/mcp",
				ToolPrefix: "s_",
				Enabled:    true,
				Hostname:   "localhost",
			},
		},
	})
	require.NotNil(t, serverInfo)
	require.Equal(t, "localhost", serverInfo.Hostname)
	require.Equal(t, "dummy", serverInfo.ServerName)
	require.Equal(t, "s_", serverInfo.ToolPrefix)
	require.Equal(t, "http://localhost:8080/mcp", serverInfo.URL)
}

func TestHandleRequestBody(t *testing.T) {
	server := &ExtProcServer{
		MCPConfig: &config.MCPServersConfig{
			Servers: []*config.MCPServer{
				{
					Name:       "dummy",
					URL:        "http://localhost:8080/mcp",
					ToolPrefix: "s_",
					Enabled:    true,
					Hostname:   "localhost",
				},
			},
		},
		Broker: broker.NewBroker(),
	}

	data := map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "s_mytool",
		},
	}
	cfg := &config.MCPServersConfig{
		Servers: []*config.MCPServer{
			{
				Name:       "dummy",
				URL:        "http://localhost:8080/mcp",
				ToolPrefix: "s_",
				Enabled:    true,
				Hostname:   "localhost",
			},
		},
	}

	var err error
	var resp []*eppb.ProcessingResponse

	// Inject a request session ID for testing
	server.requestHeaders = &eppb.HttpHeaders{
		Headers: &corev3.HeaderMap{
			Headers: []*corev3.HeaderValue{
				{
					Key:      "mcp-session-id",
					RawValue: []byte("123"),
				},
			},
		},
	}
	resp, err = server.HandleRequestBody(context.Background(), data, cfg)
	require.NoError(t, err)
	require.Len(t, resp, 1)
	require.IsType(t, &eppb.ProcessingResponse_RequestBody{}, resp[0].Response)
	rb := resp[0].Response.(*eppb.ProcessingResponse_RequestBody)
	require.NotNil(t, rb.RequestBody.Response)
	require.Len(t, rb.RequestBody.Response.HeaderMutation.SetHeaders, 3)
	require.Equal(t, "x-mcp-toolname", rb.RequestBody.Response.HeaderMutation.SetHeaders[0].Header.Key)
	require.Equal(t, []uint8("s_mytool"), rb.RequestBody.Response.HeaderMutation.SetHeaders[0].Header.RawValue)
	require.Equal(t, ":authority", rb.RequestBody.Response.HeaderMutation.SetHeaders[1].Header.Key)
	require.Equal(t, []uint8("localhost"), rb.RequestBody.Response.HeaderMutation.SetHeaders[1].Header.RawValue)
	require.Equal(t, "content-length", rb.RequestBody.Response.HeaderMutation.SetHeaders[2].Header.Key)
	require.Equal(t, []uint8("66"), rb.RequestBody.Response.HeaderMutation.SetHeaders[2].Header.RawValue)
	require.Equal(t,
		[]byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"mytool"}}`),
		rb.RequestBody.Response.BodyMutation.GetBody())
}
