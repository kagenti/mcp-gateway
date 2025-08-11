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
