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

The MCP Broker is a backend service configured with other MCP servers that acts as a default MCP server backend for the MCP endpoint.


**Responsibilities**:

- MCP client to backend MCP servers (init and tools/list)
- Session brokering between client and MCP Servers
- Handling the init call and responding with the baseline capabilities, version and service info
- Brokering SSE notifications between client and MCP server
- Creating the aggregated tools/list call
- Validating discovered MCP Servers meet minimum requirements (protocol version and capabilities)



### MCP Discovery Controller

**Overview**

The MCP Discovery Controller is a kubernetes based controller that will watch for resources and discover new MCP servers

**Responsibilities:**

- Watching HTTPRoutes labelled as MCP routes
- Updating the MCP Broker config 
(note as we understand better the different config options we may move to an MCP (CRD) that can target a HTTPRoute to configure it as an MCP route)