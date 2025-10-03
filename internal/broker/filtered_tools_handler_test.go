package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/kagenti/mcp-gateway/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestFilteredTools(t *testing.T) {

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
			}
			headerValue, _ := json.Marshal(tc.AllowedToolsList)
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
