//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/gomega"
)

// MakeMCPRequestWithSession makes an MCP request and returns the response and session ID
func MakeMCPRequestWithSession(url string, payload map[string]interface{}) (map[string]interface{}, string) {
	jsonData, err := json.Marshal(payload)
	Expect(err).ToNot(HaveOccurred())

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	Expect(err).ToNot(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// get session id from response header
	sessionID := resp.Header.Get("Mcp-Session-Id")

	body, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())

	// just parse as JSON - broker returns plain JSON, not SSE
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	Expect(err).ToNot(HaveOccurred())

	return result, sessionID
}

// MakeMCPRequestWithSessionID makes an MCP request with an existing session ID
func MakeMCPRequestWithSessionID(url string, payload map[string]interface{}, sessionID string) map[string]interface{} {
	jsonData, err := json.Marshal(payload)
	Expect(err).ToNot(HaveOccurred())

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	Expect(err).ToNot(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Mcp-Session-Id", sessionID)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())

	// just parse as JSON - broker returns plain JSON, not SSE
	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	Expect(err).ToNot(HaveOccurred())

	return result
}
