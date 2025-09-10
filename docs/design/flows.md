# MCP Gateway Request Flows


Below are some theorised flows. They are likely to adapt and change as we get deeper into the weeds. The idea is to illustrate how it "might" work rather than dictate how it "should" work. 

> note: Some show "no auth" this is to reduce noise and focus on the main flow.

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
        MCP Broker->>MCP Client: set g-session-id
        
```

## Tools/Call (no auth)

```mermaid
sequenceDiagram
        title MCP Tool Call (auth)
        actor MCP Client
        MCP Client->>Gateway: POST /mcp 
        note right of MCP Client: method: tools/call
        Gateway->>MCP Router: POST /mcp
        note left of MCP Router: method: tools/call <br/> gateway-session-id present <br/> payload validated
        MCP Router->>Session Cache: get mcp-session-id from  gateway-session-id/server-name
        Session Cache->>MCP Router: no session found
        MCP Router->>MCP Server: initialize with client headers
        MCP Server->>MCP Router: initialize response OK
        MCP Router->>Session Cache: store mcp-session-id under gateway-session-id/server-name
        MCP Router->>Gateway: set header mcp-session-id
        MCP Router->>Gateway: set header authority: <configured host>
        MCP Router->>Gateway: update body to remove prefix 
        MCP Router->>Gateway: set header x-mcp-tool header 
        Gateway->>MCP Server: Route <configured host> Post /mcp tools/call
        MCP Server->>MCP Client: tools/call response

```

## Discovery

```mermaid
sequenceDiagram
  participant MCP Controller as MCP Controller
  participant Gateway as Gateway
  participant MCP Broker as MCP Broker
  participant MCP Server as MCP Server
  actor MCP Client(s)

  MCP Controller ->> Gateway: watch for new MCPServer resoruces
  note right of MCP Controller:  MCPServer resources <br/> target HTTPRoutes
  MCP Controller ->> MCP Broker: update MCP Router config 
  MCP Controller ->> MCP Router: update MCP Broker config 
  note right of MCP Controller:  This is a configmap mounted as volume MVP
  MCP Broker ->> MCP Server: initialize call
  MCP Server ->> MCP Broker: initialized response
  note right of MCP Broker: Broker validates MCP version <br/> and capabilities meet min requirements
  MCP Broker ->> MCP Server: initialized
  MCP Broker ->> MCP Server: tools/list
  MCP Server ->> MCP Broker: tools/list response
  Note left of MCP Server: tools/list response is cached by <br/> broker under id (name, namespace, prefix)? from configmap <br/> ready for aggregated tools/list
  MCP Broker ->> MCP Server: register for tools/list changed notifications

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


## Auth

Below are some attempts with Auth in the mix. Still need some refinement of these flows

## MCP Gateway request Auth required (initialize for example)

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
        actor MCP Client
        MCP Client->>Gateway: POST /mcp 
        note right of MCP Client: method: tools/call <br/> name: prefix_echo
        Gateway->>MCP Router: POST /mcp
        note left of MCP Router: method: tools/call <br/> name: prefix_echo
        MCP Router->>Gateway: set authority: <prefix>.<host>
        MCP Router->>Gateway: update body to remove prefix 
        MCP Router->>Gateway: set x-mcp-tool header 
        Gateway->>WASM: auth on authority
        WASM->>Authorino: apply auth 
        note right of Authorino: checking JWT and tool name <br/> defined in AuthPolicy
        Authorino->>WASM: 401 WWW-Authenticate 
        note left of Authorino: WWW-Authenticate: Bearer <br/> resource_metadata=<host>/.well-known/oauth-protected-resource/tool/prefix_echo
        note left of Authorino: the response is set in the  AuthPolicy targeting the MCP HTTPRoute <br/> as the owner of the MCP server will know <br/> what that should be . Prefix will need to be included in the resource url
        WASM->>MCP Client: 401 WWW-Authenticate 
        note left of WASM: WWW-Authenticate: Bearer <br/> resource_metadata=<host>/.well-known/oauth-protected-resource/tool/prefix_echo
        MCP Client->>Gateway: .well-known/oauth-protected-resource/tool/prefix_echo
        Gateway->>MCP Router: .well-known/oauth-protected-resource/tool/prefix_echo
        MCP Router->>Gateway: set authority: <prefix>.<host>
        MCP Router->>Gateway: set path: .well-known/oauth-protected-resource/tool/echo
        Gateway->>MCP Server: GET .well-known/oauth-protected-resource/tool/echo
        MCP Server->>MCP Client: responds with resource json with auth server etc
        MCP Client->>Auth Server: Authenticate 
        Auth Server->>MCP Client: Authenticated !!
        MCP Client->>Gateway: Bearer header set POST/mcp
        note right of MCP Client: method: tools/call <br/> name: prefix_echo
        Gateway->>MCP Router: POST /mcp tools/call
        MCP Router->>Gateway: set authority: <prefix>.<host>
        MCP Router->>Gateway: update body to remove prefix 
        MCP Router->>Gateway: set x-mcp-tool header 
        Gateway->>WASM: POST /mcp tools/call
        WASM->>Authorino: Apply Auth
        Authorino->>WASM: 200
        Gateway->>MCP Server: POST /mcp tools/call
        MCP Server->>MCP Client: tools/call response
```        

## MCP Notifications

TODO (recommend scoping to just tools/list_changed) notifications initially.

The GET /mcp request will fall through to the MCP Broker.
MCP broker will see the registered session id and any send any ```tools/list_changed` notifications recieved via its own notifications connection to the backend MCP servers to any registered clients.