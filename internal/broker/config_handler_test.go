package broker

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/stretchr/testify/require"
)

func TestConfigUpdateHandlerInvalidConfig(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()

	_ = NewConfigUpdateHandler(nil, "", slog.New(slog.NewTextHandler(os.Stdout, nil)))
	t.Errorf("NewConfigUpdateHandler did not panic for nil cfg")
}

func TestConfigUpdateHandlerInvalidLogger(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()

	_ = NewConfigUpdateHandler(&config.MCPServersConfig{}, "", nil)
	t.Errorf("NewConfigUpdateHandler did not panic for nil logger")
}

func TestConfigUpdateHandlerNoAuth(t *testing.T) {
	cuh := NewConfigUpdateHandler(&config.MCPServersConfig{}, "", slog.New(slog.NewTextHandler(os.Stdout, nil)))
	req := httptest.NewRequest(http.MethodPost, "/config", nil)
	w := httptest.NewRecorder()
	cuh.UpdateConfig(w, req)
	res := w.Result()
	require.Equal(t, 200, res.StatusCode)

	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	m := make(map[string]interface{})
	err = json.Unmarshal(data, &m)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{
		"success": true,
		"message": "Configuration updated with 0 servers",
	}, m)
}

func TestConfigUpdateHandlerAuth(t *testing.T) {
	cuh := NewConfigUpdateHandler(&config.MCPServersConfig{}, "my-bearer-token", slog.New(slog.NewTextHandler(os.Stdout, nil)))
	req := httptest.NewRequest(http.MethodPost, "/config", nil)
	w := httptest.NewRecorder()
	cuh.UpdateConfig(w, req)
	res := w.Result()
	require.Equal(t, 401, res.StatusCode)

	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, "Unauthorized\n", string(data))
}
