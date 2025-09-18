# MCP API Key Test Server

Minimal MCP server for testing API key authentication in MCP Gateway.

## Features
- Validates Authorization headers (Bearer tokens/API keys)
- Returns 401 for invalid/missing credentials
- Single `hello_world` tool for testing authenticated access

## Configuration
- `PORT`: Server port (default: 9090)
- `EXPECTED_AUTH`: Expected Authorization header (e.g., "Bearer <api-key>")

## Usage
Deployed in Kubernetes with API key `Bearer test-api-key-secret-token`
