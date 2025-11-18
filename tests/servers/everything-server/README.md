# MCP Everything Server

This test server uses the official MCP everything server from the Model Context Protocol organization.
It attempts to exercise all the features of the MCP protocol, and implements prompts, tools, resources, sampling, and more.

## Source

Original source: https://github.com/modelcontextprotocol/servers/tree/main/src/everything
The included Dockerfile uses this source code as a dependency.

## Features

- **Tools**: echo, add, longRunningOperation, printEnv, sampleLLM, getTinyImage, annotatedMessage, getResourceReference, startElicitation, structuredContent, listRoots
- **Resources**: 100 test resources (even: plaintext, odd: binary blobs)
- **Prompts**: simple_prompt, complex_prompt, resource_prompt
- **Logging**: Random-leveled log messages every 15 seconds
- **Roots**: Demonstrates MCP roots protocol capability

## Configuration
- `PORT`: Server port (default: 3001)

## Docker Build and Run

```bash
docker build --load --tag everything-server .
docker run -p 9090:9090 --env PORT=9090 everything-server
```


## Testing the MCP server with the @modelcontextprotocol/inspector

Run `npx @modelcontextprotocol/inspector`
Under the `URL` field, use `http://localhost:9090/mcp`
