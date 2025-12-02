// Package credentials reads from mounted secrets
package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MountPath is the standard mount path for credentials
	MountPath = "/etc/mcp-credentials"
)

// Get reads credential from mounted secret file
func Get(name string) (string, error) {
	if name == "" {
		return "", nil
	}
	credPath := filepath.Join(MountPath, name)
	data, err := os.ReadFile(credPath) //nolint:gosec // reading kubernetes mounted secrets
	if err != nil {
		return "", fmt.Errorf("failed to read credential from file %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
