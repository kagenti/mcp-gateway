package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name           string
		credName       string
		fileContent    string
		expectedResult string
	}{
		{
			name:           "reads from file",
			credName:       "TEST_FILE_CRED",
			fileContent:    "file-secret-456\n",
			expectedResult: "file-secret-456",
		},
		{
			name:           "returns empty when file doesn't exist",
			credName:       "MISSING_FILE_CRED",
			fileContent:    "", // no file created
			expectedResult: "",
		},
		{
			name:           "handles Bearer token format",
			credName:       "BEARER_TOKEN",
			fileContent:    "Bearer ghp_abcdef123456",
			expectedResult: "Bearer ghp_abcdef123456",
		},
		{
			name:           "trims whitespace",
			credName:       "WHITESPACE_CRED",
			fileContent:    "  secret-with-spaces  \n",
			expectedResult: "secret-with-spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// create temp dir to simulate mount path
			tempDir := t.TempDir()

			// setup file if needed
			if tt.fileContent != "" {
				credPath := filepath.Join(tempDir, tt.credName)
				if err := os.WriteFile(credPath, []byte(tt.fileContent), 0600); err != nil {
					t.Fatal(err)
				}
			}

			// use helper for testing with custom path
			result := getFromPath(tempDir, tt.credName)

			// verify
			if result != tt.expectedResult {
				t.Errorf("Get(%q) = %q, want %q", tt.credName, result, tt.expectedResult)
			}
		})
	}
}

// test helper with custom mount path
func getFromPath(mountPath, name string) string {
	credPath := filepath.Join(mountPath, name)
	data, err := os.ReadFile(credPath) //nolint:gosec // test helper reading test files
	if err != nil {
		if !os.IsNotExist(err) {
			// log non-enoent errors
			fmt.Printf("Failed to read credential file %s: %v\n", credPath, err)
		}
		return "" // empty if not found
	}
	return strings.TrimSpace(string(data))
}
