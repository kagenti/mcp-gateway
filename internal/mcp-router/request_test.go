package mcprouter

import (
	"context"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"k8s.io/utils/ptr"

	"errors"
	"log/slog"
	"os"
	"testing"

	eppb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"github.com/kagenti/mcp-gateway/internal/broker"
	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/stretchr/testify/require"
)

func TestMCPRequestValid(t *testing.T) {

	testCases := []struct {
		Name      string
		Input     *MCPRequest
		ExpectErr error
	}{
		{
			Name: "test with valid request",
			Input: &MCPRequest{
				JSONRPC: "2.0",
				Method:  "initialize",
				Params:  map[string]any{},
				ID:      ptr.To(2),
			},
			ExpectErr: nil,
		},
		{
			Name: "test with invalid version",
			Input: &MCPRequest{
				JSONRPC: "1.0",
				Method:  "initialize",
				Params:  map[string]any{},
				ID:      ptr.To(2),
			},
			ExpectErr: ErrInvalidRequest,
		},
		{
			Name: "test with invalid method",
			Input: &MCPRequest{
				JSONRPC: "2.0",
				Method:  "",
				Params:  map[string]any{},
				ID:      ptr.To(2),
			},
			ExpectErr: ErrInvalidRequest,
		},
		{
			Name: "test with missing id ",
			Input: &MCPRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params:  map[string]any{},
			},
			ExpectErr: ErrInvalidRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			valid, err := tc.Input.Validate()
			if tc.ExpectErr != nil {
				if errors.Is(tc.ExpectErr, err) {
					t.Fatalf("expected an error but got none")
				}
				if valid {
					t.Fatalf("mcp request should not have been marked valid")
				}
			} else {
				if !valid {
					t.Fatalf("expected the mcp request to be valid")
				}
			}

		})
	}
}

func TestMCPRequestToolName(t *testing.T) {
	testCases := []struct {
		Name       string
		Input      *MCPRequest
		ExpectTool string
	}{
		{
			Name: "test with valid request",
			Input: &MCPRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params: map[string]any{
					"name": "test_tool",
				},
			},
			ExpectTool: "test_tool",
		},
		{
			Name: "test with no tool",
			Input: &MCPRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params: map[string]any{
					"name": "",
				},
			},
			ExpectTool: "",
		},
		{
			Name: "test with not a tool call",
			Input: &MCPRequest{
				JSONRPC: "2.0",
				Method:  "intialise",
				Params: map[string]any{
					"name": "test",
				},
			},
			ExpectTool: "",
		},
		{
			Name: "test with not a tool call",
			Input: &MCPRequest{
				JSONRPC: "2.0",
				Method:  "intialise",
				Params: map[string]any{
					"name": 2,
				},
			},
			ExpectTool: "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.Input.ToolName() != tc.ExpectTool {
				t.Fatalf("expected mcp request tool call to have tool %s but got %s", tc.ExpectTool, tc.Input.ToolName())
			}
		})
	}
}

func TestHandleRequestBody(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	server := &ExtProcServer{
		RoutingConfig: &config.MCPServersConfig{
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
		Broker: broker.NewBroker(logger, broker.Opts{}),
	}

	data := &MCPRequest{
		ID:      ptr.To(0),
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: map[string]any{
			"name":  "s_mytool",
			"other": "other",
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
	resp = server.HandleMCPRequest(context.Background(), data, cfg)
	require.Len(t, resp, 1)
	require.IsType(t, &eppb.ProcessingResponse_RequestBody{}, resp[0].Response)
	rb := resp[0].Response.(*eppb.ProcessingResponse_RequestBody)
	require.NotNil(t, rb.RequestBody.Response)
	require.Len(t, rb.RequestBody.Response.HeaderMutation.SetHeaders, 5)
	require.Equal(t, "x-mcp-method", rb.RequestBody.Response.HeaderMutation.SetHeaders[0].Header.Key)
	require.Equal(t, []uint8("tools/call"), rb.RequestBody.Response.HeaderMutation.SetHeaders[0].Header.RawValue)
	require.Equal(t, "x-mcp-toolname", rb.RequestBody.Response.HeaderMutation.SetHeaders[1].Header.Key)
	require.Equal(t, []uint8("mytool"), rb.RequestBody.Response.HeaderMutation.SetHeaders[1].Header.RawValue)
	require.Equal(t, "x-mcp-servername", rb.RequestBody.Response.HeaderMutation.SetHeaders[2].Header.Key)
	require.Equal(t, []uint8("dummy"), rb.RequestBody.Response.HeaderMutation.SetHeaders[2].Header.RawValue)
	require.Equal(t, ":authority", rb.RequestBody.Response.HeaderMutation.SetHeaders[3].Header.Key)
	require.Equal(t, []uint8("localhost"), rb.RequestBody.Response.HeaderMutation.SetHeaders[3].Header.RawValue)
	require.Equal(t, "content-length", rb.RequestBody.Response.HeaderMutation.SetHeaders[4].Header.Key)

	require.Equal(t,
		`{"id":0,"jsonrpc":"2.0","method":"tools/call","params":{"name":"mytool","other":"other"}}`,
		string(rb.RequestBody.Response.BodyMutation.GetBody()))
}
