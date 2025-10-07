# server3

A simple MCP server based on https://github.com/jlowin/fastmcp
with tools for time and slow response testing.

## Test Python

```bash
fastmcp run server.py --transport http
```

## Build and run Dockerfile

```bash
docker build --load --tag mcp-test3 .
docker run --publish 9093:9090 mcp-test3
```

## Testing the MCP server with the @modelcontextprotocol/inspector

Run `DANGEROUSLY_OMIT_AUTH=true npx @modelcontextprotocol/inspector`
