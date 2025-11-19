# server2

A simple MCP server based on https://github.com/modelcontextprotocol/go-sdk
with tools for time, HTTP header testing, and slow response testing.

## Test Go binary

```bash
MCP_TRANSPORT=http PORT=9091 go run main.go
MCP=http://localhost:9091/mcp
```

## Build and run Dockerfile

```bash
docker build --load --tag mcp-test2 .
docker run --publish 9091:9090 --env MCP_TRANSPORT=http --env PORT=9090 mcp-test2
MCP=http://localhost:9091/mcp
```

## Testing the MCP server with the @modelcontextprotocol/inspector

Run `DANGEROUSLY_OMIT_AUTH=true npx @modelcontextprotocol/inspector`

## Testing the MCP server with _curl_

First, initialize the server:

```bash
curl --include -X POST -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" ${MCP} --data '
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2025-06-18",
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

```bash
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

```bash
curl -v ${MCP} -H "mcp-session-id: ${SESSION_ID}" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" --silent --data '
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list"
}
' | jq
```

Inspect HTTP headers sent to server:

```bash
curl -v ${MCP} -H "mcp-session-id: ${SESSION_ID}" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" --silent --data '
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "headers"
  }
}
' | jq
```

Make a slow call with progress updates:

```bash
curl -v ${MCP} -H "mcp-session-id: ${SESSION_ID}" -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" --data '
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "slow",
    "arguments": {
      "seconds": 5
    },
    "_meta": {
      "progressToken": "abc123"
    }
  }
}
'
```

### Security options

To require a bearer token, define `EXPECTED_AUTH`

- `EXPECTED_AUTH="Bearer 1234" MCP_TRANSPORT=http PORT=9091 go run main.go`
- `docker run --publish 9091:9090 --EXPECTED_AUTH="Bearer 1234" --env MCP_TRANSPORT=http --env PORT=9090 mcp-test2`

Then, before Connecting in the MCP Inspector console, expand _Authorization_ and set the Bearer Token value to `1234`.  (Note that the MCP inspector prepends this with "Bearer ", so the value set in the MCP Inspector UI doesn't match exactly with what the test MCP server receives).
