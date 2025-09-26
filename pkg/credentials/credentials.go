// Package credentials reads from mounted secrets
package credentials

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MountPath is the standard mount path for credentials
	MountPath = "/etc/mcp-credentials"
)

// Get reads credential from mounted secret file
func Get(name string) string {
	credPath := filepath.Join(MountPath, name)
	data, err := os.ReadFile(credPath) //nolint:gosec // reading kubernetes mounted secrets
	if err != nil {
		if !os.IsNotExist(err) {
			// log non-enoent errors
			slog.Debug("Failed to read credential file", "path", credPath, "error", err)
		}
		return "" // empty if not found
	}
	return strings.TrimSpace(string(data))
}
