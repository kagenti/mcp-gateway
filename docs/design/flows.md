# MCP Gateway Request Flows


Below are some theorized flows. They are likely to adapt and change as we get deeper into the weeds. The idea is to illustrate how it "might" work rather than dictate how it "should" work. 

> note: Some show "no auth" this is to reduce noise and focus on the main flow.

## MCP Server Registration

For detailed information on how MCP server registration works, including the MCPManager lifecycle and configuration change handling, see the [server registration design documentation](./server-registration.md).

## Initialize:

```mermaid
sequenceDiagram
        title MCP Initialize Request Flow (no auth)
        actor MCP Client
        MCP Client->>Gateway: POST /mcp init
        Gateway->>MCP Router: POST /mcp init
        MCP Router->>Gateway: no routing needed
        Gateway->>MCP Broker: POST /mcp init
        note right of MCP Broker: MCP Broker is the default backend for /mcp
        MCP Broker->>MCP Client: set mcp-session-id
```

## Aggregated Tools/List (no auth)

```mermaid
sequenceDiagram
  actor MCP Client
  participant Gateway as Gateway
  participant MCP Router as MCP Router
  participant MCP Broker as MCP Broker
  
  MCP Client->>Gateway: tools/list (with auth bearer token)
  Gateway->>MCP Router: tools/list
  MCP Router->>Gateway: nothing to do
  Gateway->>MCP Broker: tools/list
  MCP Broker->>MCP Client: aggregated tools/list response
  note left of MCP Broker: list is built via discovery phase
```

## Tools/Call (no auth)

```mermaid
sequenceDiagram
        title MCP Tool Call (auth)
        actor MCP Client
        MCP Client->>Gateway: POST /mcp 
        note right of MCP Client: method: tools/call
        Gateway->>MCP Router: POST /mcp
        note left of MCP Router: method: tools/call <br/> gateway mcp-session-id present <br/> payload validated
        MCP Router->>Session Cache: get backend mcp-session-id based ok key  gateway-session-id/server-name
        Session Cache->>MCP Router: no session found
        MCP Router->>Gateway: initialize with client headers via gateway to ensure additional auth applied
        Gateway->>MCP Server: initialize 
        MCP Server->>MCP Router: initialize response OK
        MCP Router->>Session Cache: store mcp-session-id under gateway-session-id/server-name
        MCP Router->>Gateway: set header mcp-session-id
        MCP Router->>Gateway: set header authority: <configured host>
        MCP Router->>Gateway: update body to remove prefix 
        MCP Router->>Gateway: set header x-mcp-tool header 
        Gateway->>MCP Server: Route <configured host> Post /mcp tools/call
        MCP Server->>MCP Client: tools/call response
```

## Auth

Below are some attempts with Auth in the mix. Still need some refinement of these flows

## MCP Gateway Request Authentication

```mermaid
sequenceDiagram
        title MCP Initialize Request Flow (auth)
        actor MCP Client
        MCP Client->>Gateway: POST /mcp init
        Gateway->>MCP Router: POST /mcp init
        MCP Router->>Gateway: no routing needed
        Gateway->>WASM: POST /mcp init
        WASM->>Authorino: Apply Auth
        Authorino->>MCP Client: 401 WWW-Authenticate with resource meta-data
        note left of Authorino: WWW-Authenticate: Bearer <br/> resource_metadata=<host>/.well-known/oauth-protected-resource/mcp
        MCP Client->>Gateway: GET /.well-known/oauth-protected-resource/mcp
        MCP Router->>Gateway: no routing needed
        Gateway->>MCP Broker: GET /.well-known/oauth-protected-resource/mcp
        MCP Broker->>MCP Client: responds with resource json with configured auth server etc
        MCP Client->>Auth Server: register
        MCP Client->>Auth Server: authenticate
        Auth Server->>MCP Client: authenticated !
        MCP Client->>Gateway: Bearer header set POST/mcp init
        Gateway->>MCP Router: POST /mcp init
        MCP Router->>Gateway: no routing needed
        Gateway->>WASM: POST /mcp init
        WASM->>Authorino: Apply Auth
        Authorino->>WASM: 200
        Gateway->>MCP Broker: POST /mcp init
        MCP Broker->>MCP Client: init response 200
```        


## MCP Server Tool Call with Auth

```mermaid
sequenceDiagram
        title MCP Tool Call (auth)
        MCPClient->>Gateway: POST /mcp 
        note right of MCPClient: method: tools/call <br/> name: prefix_echo
        Gateway->>MCPRouter: POST /mcp
        note left of MCPRouter: method: tools/call <br/> name: prefix_echo
        MCPRouter->>Gateway: set authority: <prefix>.<host>
        MCPRouter->>Gateway: update body to remove prefix 
        MCPRouter->>Gateway: set x-mcp-tool header 
        Gateway->>WASM: auth on authority
        WASM->>Authorino: apply auth 
        note right of Authorino: checking JWT and tool name <br/> defined in AuthPolicy
        Authorino->>WASM: 401 WWW-Authenticate 
        note left of Authorino: WWW-Authenticate: Bearer <br/> resource_metadata=<host>/.well-known/oauth-protected-resource/mcp
        WASM->>MCPClient: 401 WWW-Authenticate 
        note left of WASM: WWW-Authenticate: Bearer <br/> resource_metadata=<host>/.well-known/oauth-protected-resource/mcp
        MCPClient->>Gateway: .well-known/oauth-protected-resource/mcp
        Gateway->>MCPRouter: .well-known/oauth-protected-resource/mcp
        Gateway->>MCPBroker: .well-known/oauth-protected-resource/mcp
        MCPBroker->>MCPClient: auth metadata response
        MCPClient->>Auth-Server: Authenticate (dynamic client reg etc) 
        Auth-Server->>MCPClient: Authenticated !!
        MCPClient->>Gateway: Bearer header set POST/mcp
        note right of MCPClient: method: tools/call <br/> name: prefix_echo
        Gateway->>MCPRouter: POST /mcp tools/call
        MCPRouter->>Gateway: set authority: <prefix>.<host>
        MCPRouter->>Gateway: update body to remove prefix set headers etc
        Gateway->>WASM: POST /mcp tools/call
        WASM->>Authorino: Apply Auth
        Authorino->>WASM: OK
        Gateway->>MCPServer: POST /mcp tools/call
        MCPServer->>MCPClient: tools/call response
```        

## MCP Notifications

For detailed information on how notifications work in the MCP Gateway, see the [notifications design documentation](./notifications.md).
