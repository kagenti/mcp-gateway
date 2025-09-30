package broker

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestVirtualServiceHandlerPassthrough(t *testing.T) {
	testFunc := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}

	vsh := NewVirtualServerHandler(
		http.HandlerFunc(testFunc),
		&config.MCPServersConfig{},
		slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// Wrong HTTP Method
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w := httptest.NewRecorder()
	vsh.ServeHTTP(w, req)
	res := w.Result()
	require.Equal(t, 500, res.StatusCode)

	// Missing header
	req = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	w = httptest.NewRecorder()
	vsh.ServeHTTP(w, req)
	res = w.Result()
	require.Equal(t, 500, res.StatusCode)

	// Missing JSON
	req = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Add("X-Mcp-Virtualserver", "dummy-vs")
	w = httptest.NewRecorder()
	vsh.ServeHTTP(w, req)
	res = w.Result()
	require.Equal(t, 500, res.StatusCode)

	// JSON but not tools/list
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(mcp.JSONRPCRequest{})
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPost, "/mcp", &buf)
	req.Header.Add("X-Mcp-Virtualserver", "dummy-vs")
	w = httptest.NewRecorder()
	vsh.ServeHTTP(w, req)
	res = w.Result()
	require.Equal(t, 500, res.StatusCode)
}

func TestVirtualServiceHandlerPayloadError(t *testing.T) {
	respCode := 500
	respBody := ""
	testFunc := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(respCode)
		_, _ = w.Write([]byte(respBody))
	}

	vsh := NewVirtualServerHandler(
		http.HandlerFunc(testFunc),
		&config.MCPServersConfig{},
		slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// Upstream returns a JSON-RPC Error
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(mcp.JSONRPCRequest{
		Request: mcp.Request{
			Method: "tools/list",
		},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/mcp", &buf)
	req.Header.Add("X-Mcp-Virtualserver", "dummy-vs")
	var upstreamBuf bytes.Buffer
	err = json.NewEncoder(&upstreamBuf).Encode(mcp.JSONRPCError{
		Error: mcp.JSONRPCErrorDetails{
			Code: 500,
		},
	})
	require.NoError(t, err)
	respBody = upstreamBuf.String()
	w := httptest.NewRecorder()
	vsh.ServeHTTP(w, req)
	res := w.Result()
	require.Equal(t, 500, res.StatusCode)
}

func TestVirtualServiceHandlerPayloadResponse(t *testing.T) {
	respBody := ""
	testFunc := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(respBody))
	}

	vsh := NewVirtualServerHandler(
		http.HandlerFunc(testFunc),
		&config.MCPServersConfig{
			VirtualServers: []*config.VirtualServer{
				{
					Name:  "dummy-vs",
					Tools: []string{"tool-a", "tool-b"},
				},
			},
		},
		slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// Upstream returns a JSON-RPC response
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(mcp.JSONRPCRequest{
		Request: mcp.Request{
			Method: "tools/list",
		},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/mcp", &buf)
	req.Header.Add("X-Mcp-Virtualserver", "dummy-vs")
	var upstreamBuf2 bytes.Buffer
	upstreamBuf2.Reset()
	err = json.NewEncoder(&upstreamBuf2).Encode(mcp.JSONRPCResponse{
		Result: mcp.ListToolsResult{
			Tools: []mcp.Tool{
				{
					Name: "tool-b",
				},
				{
					Name: "tool-c",
				},
			},
		},
	})
	require.NoError(t, err)
	respBody = upstreamBuf2.String()
	w := httptest.NewRecorder()
	vsh.ServeHTTP(w, req)
	res := w.Result()
	require.Equal(t, 200, res.StatusCode)
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	tr := mcp.JSONRPCResponse{}
	err = json.Unmarshal(data, &tr)
	require.NoError(t, err)
	// Re-encode, so that we can decode in a type-specific fashion
	resultBytes, err := json.Marshal(tr.Result)
	require.NoError(t, err)
	var listToolsResult mcp.ListToolsResult
	err = json.Unmarshal(resultBytes, &listToolsResult)
	require.NoError(t, err)
	require.Len(t, listToolsResult.Tools, 1)
	// "tool-b" is the intersection of the config's tools and the upstream's tools
	require.Equal(t, "tool-b", listToolsResult.Tools[0].Name)
}
