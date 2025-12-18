# MCP Conformance Test Server

A reference implementation of an MCP server that implements all features required for conformance testing.

## Source

Original source: https://github.com/modelcontextprotocol/conformance/tree/main/examples/servers/typescript
The included Dockerfile uses this source code as a dependency. See README.md there for more details.

## Configuration
- `PORT`: Server port (default: 3000)

## Build and Run

```bash
podman build --load --tag conformance-server .
# non-default port used intentionally to show how to configure it
podman run -p 9090:9090 --env PORT=9090 conformance-server
```

The server will start on `http://localhost:3000` (or the port specified in `PORT` environment variable).

## Testing the MCP server with the @modelcontextprotocol/inspector

Run `npx @modelcontextprotocol/inspector`
Under the `URL` field, use `http://localhost:3000/mcp`
