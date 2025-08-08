## MCP Gateway Components


### MCP Router

**Overview:**

The MCP router is an envoy focused ext_proc component that is capable of parsing the MCP protocol and using it to set headers to force correct routing of the request to the correct MCP server.

**Resposibilities:**

- Parsing the MCP JSONRPC body
- Setting the x-destination-mcp header
- Ending the request if invalid payload
- watching for 404 reponses from MCP servers and calling to MCP Broker to invalidate session.


### MCP Broker

**Overview:**

The MCP Broker is a backend service that acts as a default MCP server backend for the MCP endpoint.

**Responsibilities**:

- Session brokering between client and MCP Servers
- Handling the init call and responding with the baseline capabilities, version and service info
- Brokering SSE notifications between client and MCP server
- Serving the aggregated tools/list call