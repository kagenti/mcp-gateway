package broker

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestFilteredTools(t *testing.T) {

	var createJWTHeader = func(allowedTools map[string][]string) string {
		keyBytes := []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIEY3QeiP9B9Bm3NHG3SgyiDHcbckwsGsQLKgv4fJxjJWoAoGCCqGSM49
AwEHoUQDQgAE7WdMdvC8hviEAL4wcebqaYbLEtVOVEiyi/nozagw7BaWXmzbOWyy
95gZLirTkhUb1P4Z4lgKLU2rD5NCbGPHAA==
-----END EC PRIVATE KEY-----
`)
		claimPayload, _ := json.Marshal(allowedTools)
		block, _ := pem.Decode(keyBytes)
		token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"allowed-tools": string(claimPayload)})
		parsedKey, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			t.Fatalf("error parsing key for jwt %s", err)
		}
		jwtToken, err := token.SignedString(parsedKey)
		if err != nil {
			t.Fatalf("error signing jwt %s", err)
		}
		return jwtToken
	}

	testCases := []struct {
		Name                 string
		FullToolList         *mcp.ListToolsResult
		AllowedToolsList     map[string][]string
		RegisteredMCPServers mcpServers
		allowfullToolList    bool
		ExpectedTools        []mcp.Tool
	}{
		{
			Name: "test filters tools as expected",
			FullToolList: &mcp.ListToolsResult{Tools: []mcp.Tool{
				{
					Name: "test_tool",
				},
				{
					Name: "test_tool2",
				},
			}},

			RegisteredMCPServers: mcpServers{
				"http://upstream.mcp.cluster.local": &upstreamMCP{
					MCPServer: config.MCPServer{
						Hostname:   "mcp1.server.local",
						ToolPrefix: "test_",
					},
					toolsResult: &mcp.ListToolsResult{
						Tools: []mcp.Tool{
							{
								Name: "tool",
							},
							{
								Name: "tool2",
							},
						},
					},
				},
			},
			AllowedToolsList: map[string][]string{
				"mcp1.server.local": {"tool"},
			},
			allowfullToolList: false,
			ExpectedTools: []mcp.Tool{
				{
					Name: "test_tool",
				},
			},
		},
		{
			Name: "test filters tools with same tool name as expected",
			FullToolList: &mcp.ListToolsResult{Tools: []mcp.Tool{
				{
					Name: "test_tool",
				},
				{
					Name: "test_tool",
				},
			}},

			RegisteredMCPServers: mcpServers{
				"http://upstream.mcp1.cluster.local": &upstreamMCP{
					MCPServer: config.MCPServer{
						Hostname:   "mcp1.server.local",
						ToolPrefix: "test1_",
					},
					toolsResult: &mcp.ListToolsResult{
						Tools: []mcp.Tool{
							{
								Name: "tool",
							},
							{
								Name: "tool2",
							},
						},
					},
				},
				"http://upstream.mcp2.cluster.local": &upstreamMCP{
					MCPServer: config.MCPServer{
						Hostname:   "mcp2.server.local",
						ToolPrefix: "test2_",
					},
					toolsResult: &mcp.ListToolsResult{
						Tools: []mcp.Tool{
							{
								Name: "tool",
							},
							{
								Name: "tool2",
							},
						},
					},
				},
			},
			AllowedToolsList: map[string][]string{
				"mcp1.server.local": {"tool"},
				"mcp2.server.local": {"tool"},
			},
			allowfullToolList: false,
			ExpectedTools: []mcp.Tool{
				{
					Name: "test2_tool",
				},
				{
					Name: "test2_tool",
				},
			},
		},
		{
			Name: "test filters tools returns no tools if none allowed",
			FullToolList: &mcp.ListToolsResult{Tools: []mcp.Tool{
				{
					Name: "test_tool",
				},
				{
					Name: "test_tool",
				},
			}},

			RegisteredMCPServers: mcpServers{
				"http://upstream.mcp1.cluster.local": &upstreamMCP{
					MCPServer: config.MCPServer{
						Hostname:   "mcp1.server.local",
						ToolPrefix: "test1_",
					},
					toolsResult: &mcp.ListToolsResult{
						Tools: []mcp.Tool{
							{
								Name: "tool",
							},
							{
								Name: "tool2",
							},
						},
					},
				},
				"http://upstream.mcp2.cluster.local": &upstreamMCP{
					MCPServer: config.MCPServer{
						Hostname:   "mcp2.server.local",
						ToolPrefix: "test2_",
					},
					toolsResult: &mcp.ListToolsResult{
						Tools: []mcp.Tool{
							{
								Name: "tool",
							},
							{
								Name: "tool2",
							},
						},
					},
				},
			},
			AllowedToolsList:  map[string][]string{},
			allowfullToolList: false,
			ExpectedTools:     []mcp.Tool{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {

			mcpBroker := &mcpBrokerImpl{
				enforceToolFilter: tc.allowfullToolList,
				trustedHeadersPublicKey: `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE7WdMdvC8hviEAL4wcebqaYbLEtVO
VEiyi/nozagw7BaWXmzbOWyy95gZLirTkhUb1P4Z4lgKLU2rD5NCbGPHAA==
-----END PUBLIC KEY-----`,
				logger: slog.Default(),
			}
			headerValue := createJWTHeader(tc.AllowedToolsList)
			fmt.Println("header ", headerValue)
			request := &mcp.ListToolsRequest{
				Header: http.Header{
					authorizedToolsHeader: {string(headerValue)},
				},
			}
			mcpBroker.mcpServers = tc.RegisteredMCPServers
			mcpBroker.FilteredTools(context.TODO(), 1, request, tc.FullToolList)
			for _, et := range tc.ExpectedTools {
				if !slices.ContainsFunc(tc.FullToolList.Tools, func(m mcp.Tool) bool {
					if m.Name == et.Name {
						return true
					}
					return false
				}) {
					t.Fatalf("returned tool list should not contain %s", et.Name)
				}
			}
		})
	}
}
