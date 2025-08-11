# server1

A simple MCP server based on https://github.com/modelcontextprotocol/go-sdk
with tools for time, HTTP header testing, and slow response testing.

## Test Go binary

go run main.go --http localhost:9090
MCP=http://localhost:9090/mcp

## Build and run Dockerfile

docker build --load --tag mcp-test1 . 
docker run --publish 9091:9090 mcp-test1 /mcp-test-server --http 0.0.0.0:9090
MCP=http://localhost:9091/mcp

## Testing the MCP server with the @modelcontextprotocol/inspector

Run `DANGEROUSLY_OMIT_AUTH=true npx @modelcontextprotocol/inspector`

Note that the 'slow' tool's updates do not appear in the @modelcontextprotocol/inspector window.

## Testing the MCP server with _curl_

First, initialize the server:

```
curl --include -X POST -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" ${MCP} --data '
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2025-03-26",
    "capabilities": {
      "roots": {
        "listChanged": true
      },
      "sampling": {}
    },
    "clientInfo": {
      "name": "ExampleClient",
      "version": "1.0.0"
    }
  }
}
' | tee /tmp/init-response.txt
```

Next, complete the initialization:

```
SESSION_ID=$(cat /tmp/init-response.txt | grep -i mcp-session-id: | sed 's/mcp-session-id: //I' | sed 's/\r//g')
echo SESSION_ID is ${SESSION_ID}
curl -v -X POST -H "mcp-session-id: ${SESSION_ID}" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" ${MCP} --data '
{
  "jsonrpc": "2.0",
  "method": "notifications/initialized"
}
'
```

(Optional) List tools:

```
curl -v ${MCP} -H "mcp-session-id: ${SESSION_ID}" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" --silent --data '
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list"
}
' | grep "data:" | sed 's/data: //' | jq
```

Inspect HTTP headers sent to server:

```
curl -v ${MCP} -H "mcp-session-id: ${SESSION_ID}" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" --silent --data '
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "headers"
  }
}
' | grep "data:" | sed 's/data: //' | jq
```

Make a slow call with progress updates:

```
curl -v ${MCP} -H "mcp-session-id: ${SESSION_ID}" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" --data '
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "slow",
    "arguments": {
      "seconds": 5
    }
  }
}
'
```

Note that these updates do not appear in the @modelcontextprotocol/inspector.
