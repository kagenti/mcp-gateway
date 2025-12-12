## Tool Prefix Matching

### Problem

MCP Gateway routes tool calls to backend MCP servers based on tool name prefixes. Each MCPServer resource defines a `toolPrefix` field that the router uses to match incoming tool calls against registered backends.

Current routing implementation checks MCP servers sequentially (in undefined order) until finding a prefix match. This creates ambiguity when:

- Multiple MCP servers share the same `toolPrefix`
- An MCP server has an empty `toolPrefix` (matches all tool names)

An empty prefix acts as a wildcard, matching any tool name. If checked before prefixed servers, it incorrectly captures tool calls intended for specific backends. This breaks the prefix-based routing model.

**Example Failure:**
```
MCPServer A: toolPrefix="" (empty)
MCPServer B: toolPrefix="test_"

Tool call: "test_tool_from_B"
Current behavior: Routes to A if A is checked before B (wrong)
Expected behavior: Always routes to B (correct)
```

### Solution

Enforce single empty-prefix server and prioritize prefix matching:

1. **Validation**: Only one MCPServer with an empty toolPrefix is allowed. Additional MCPServers with empty prefix fail validation with clear error message.

> Alternative approach: a stricter validation rule that prevents the creation of any MCPServer CRs with duplicate `toolPrefix` values. This would prevent both types of routing ambiguity.

> Note: currently only one MCP Gateway instance per cluster is supported thus the MCPServer prefix needs to be unique cluster-wide for the routing to work reliably. Once multiple MCP Gateway instances are supported this can be relaxed so that the prefix is unique per namespace (MCP Gateway instance).

2. **Router Ordering**: Router checks prefixed servers first, falling back to empty-prefix server only when no prefix matches.

3. **Status Reporting**: MCPServer status indicates when validation fails due to conflicting prefix.

### Implementation Details

**Controller Changes:**
- Validate at reconciliation time that only one MCPServer defines empty `toolPrefix`
- Set appropriate status conditions on conflicting resources

**Router Changes:**
- Check prefixed servers first using longest-prefix-match algorithm
- Use empty-prefix server as fallback when no prefix matches
- Refactor current logic where the information about the server including its prefix is already available (stored in [serverInfo](https://github.com/kagenti/mcp-gateway/blob/8665eaaa9ba666a961e0d9424c8549d1356ecf30/internal/mcp-router/request_handlers.go#L192) object) but it is not used, instead a complex [StripServerPrefix](https://github.com/kagenti/mcp-gateway/blob/8665eaaa9ba666a961e0d9424c8549d1356ecf30/internal/mcp-router/request_handlers.go#L226) method is called to retrieve the information again.

### Limitations

This solution does not address tool name collisions where:
- Empty-prefix server exposes tool `test_tool`
- Prefixed server has `toolPrefix="test_"`

Router will route to prefixed server, resulting in "tool not found" error instead of routing to empty-prefix server.

**Rationale**: Allowing prefix override would break the prefix-based routing model. Users must design tool names to avoid conflicts, or use distinct prefixes for all servers.
