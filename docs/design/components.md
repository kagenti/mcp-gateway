## MCP Gateway Components


### MCP Router

**Overview:**

The MCP router is an envoy focused ext_proc component that is capable of parsing the MCP protocol and using it to set headers to force correct routing of the request to the correct MCP server. It is mostly involved with specific tools call requests.

**Resposibilities:**

- Parsing the MCP JSONRPC body
- Setting the key request headers x-destination-mcp,x-mcp-tool,mcp-session-id
- Ending the request if invalid payload
- Watching for 404 reponses from MCP servers and invalidate session via session store.
- Handling session initialization and storage on behalf of a requesting  MCP client during a tools/call


### MCP Broker

**Overview:**

The MCP Broker is a backend service configured with other MCP servers that acts as a default MCP server backend for the MCP endpoint.


**Responsibilities**:

- General MCP Backend 
- Acts as a client to backend MCP servers (init and tools/list)
- Handles the init call and responding with the baseline capabilities, version and service info
- Brokering SSE notifications between client and MCP server
- Creating the aggregated tools/list call
- Validating discovered MCP Servers meet minimum requirements (protocol version and capabilities)



### MCP Discovery Controller

**Overview**

The MCP Discovery Controller is a kubernetes based controller that will watch for resources and discover new MCP servers

**Responsibilities:**

- Watching MCPServer resources labelled as MCP routes
- Reconciling a config from the HTTPRoute targeted and the MCPServer resource
- Updating the MCP Broker and MCP Router config (configmap) based on discovered MCPServer resources and the HTTPRoutes it targets.