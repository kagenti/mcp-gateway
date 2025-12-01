
# Test MCP servers

This folder contains MCP server implementations that are used for testing the MCP Gateway.

| Name               | Library | Features |
|--------------------|-----------------------|----------|
| api-key-server     | github.com/modelcontextprotocol/go-sdk       | Auth middleware for Authorization header |
| broken-server      | github.com/modelcontextprotocol/go-sdk       | Simulates tool conflicts, no tools, and wrong MCP protocol version |
| conformance-server | github.com/modelcontextprotocol/typescript-sdk| [Conformance test server](https://github.com/modelcontextprotocol/conformance) from ModelContextProtocol conformance repository|
| custom-path-server | github.com/modelcontextprotocol/go-sdk       | HTTP endpoint /v1/special/mcp |
| server1            | github.com/modelcontextprotocol/go-sdk       | |
| server2            | github.com/mark3labs/mcp-go                  | |
| server3            | [fastmcp](https://pypi.org/project/fastmcp/) | |
| everything-server  | github.com/modelcontextprotocol/typescript-sdk| [Everything MCP server](https://github.com/modelcontextprotocol/servers/tree/main/src/everything) from ModelContextProtocol servers repository|
