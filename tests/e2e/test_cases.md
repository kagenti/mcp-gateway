## Test Cases


### [Happy] Test MCPVirtualServer behaves as expected when defined

- When a developer defines a MCPVirtualServer resource and specify the value of the `X-Mcp-Virtualserver` header as the name in the format `namespace/name` where the namespace and name come from the created MCPVirtualServer resource, they should only get the tools specified in the MCPVirtualServer resource when they do a tools/list request to the MCP Gateway host. 


### [Happy] Test tools are filtered down based on x-authorized-tools header

- When the value of the `x-authorized-tools` header is set as a JWT signed by a trusted key to a set of tools, the MCP Gateway should respond with only tools in that list.
