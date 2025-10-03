package broker

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"

	"github.com/mark3labs/mcp-go/mcp"
)

// authorizedToolsHeader is a header expected to be set by a trused external source.
// (TODO)it must be encoded as a JWT token
var authorizedToolsHeader = http.CanonicalHeaderKey("x-authorized-tools")

// FilterTools will reduce the tool set down to those passed based the authorization lay via the x-authorized-tools head
func (broker *mcpBrokerImpl) FilteredTools(_ context.Context, id any, mcpReq *mcp.ListToolsRequest, mcpRes *mcp.ListToolsResult) {
	slog.Debug("FilteredTools called", "result", mcpRes.Tools)
	filteredTools := []mcp.Tool{}
	var allowedToolsValue string
	if len(mcpReq.Header[authorizedToolsHeader]) > 0 {
		allowedToolsValue = mcpReq.Header[authorizedToolsHeader][0]
	}
	if allowedToolsValue == "" {
		slog.Debug("filter tools no x-authorized-tools header present", "enforceToolFilter ", broker.enforceToolFilter)
		if broker.enforceToolFilter {
			slog.Debug("full list not allowed")
			mcpRes.Tools = filteredTools
			return
		}
		return
	}
	slog.Debug("filtering tools based on header", "value", allowedToolsValue)
	authorizedTools := map[string][]string{}
	if err := json.Unmarshal([]byte(allowedToolsValue), &authorizedTools); err != nil {
		slog.Error("failed to unmarshall authorized tools header returning empty tool set", "error", err)
		mcpRes.Tools = filteredTools
		return
	}

	for server, allowedToolNames := range authorizedTools {
		slog.Debug("checking tools for server ", "server", server, "allowed tools for server", allowedToolNames, "mcpServers", broker.mcpServers)
		// we key off the host so have to iterate for now
		upstreamServer := broker.mcpServers.findByHost(server)
		if upstreamServer == nil {
			slog.Debug("failed to find registered upstream ", "server", server)
			continue
		}
		if upstreamServer.Hostname != server {
			continue
		}

		if upstreamServer.toolsResult == nil {
			slog.Debug("no tools registered for upstream server", "server", upstreamServer.Hostname)
			continue
		}
		slog.Debug("upstream server found ", "upstream ", upstreamServer, "tools", upstreamServer.toolsResult.Tools)
		for _, upstreamTool := range upstreamServer.toolsResult.Tools {
			slog.Debug("checking access ", "tool", upstreamTool.Name, "against", allowedToolNames)
			if slices.Contains(allowedToolNames, upstreamTool.Name) {
				slog.Debug("access granted to", "tool", upstreamTool)
				upstreamTool.Name = string(upstreamServer.prefixedName(upstreamTool.Name))
				filteredTools = append(filteredTools, upstreamTool)
			}
		}
	}
	mcpRes.Tools = filteredTools
}
