# MCP Server Configuration

This guide covers configuring MCP servers to be discovered and routed by MCP Gateway.

## Prerequisites

- MCP Gateway installed and configured
- Gateway and HTTPRoute configured for MCP Gateway
- An existing MCP server running in your cluster

## Overview

To connect an MCP server to MCP Gateway, you need:
1. An HTTPRoute that routes to your MCP server
2. An MCPServer resource that references the HTTPRoute

## Step 1: Create HTTPRoute for Your MCP Server

Create an HTTPRoute that routes to your MCP server:

```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: mcp-api-key-server-route
  namespace: mcp-test
  labels:
    mcp-server: 'true'
spec:
  parentRefs:
    - name: mcp-gateway
      namespace: gateway-system
  hostnames:
    - 'api-key-server.mcp.local'  # Internal routing hostname
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: mcp-api-key-server  # Your MCP server service name
          port: 9090                # Your MCP server port
EOF
```

## Step 2: Create MCPServer Resource

Create an MCPServer resource that references the HTTPRoute:

```bash
kubectl apply -f - <<EOF
apiVersion: mcp.kagenti.com/v1alpha1
kind: MCPServer
metadata:
  name: my-mcp-server
  namespace: mcp-system
spec:
  toolPrefix: "myserver_"
  targetRef:
    group: "gateway.networking.k8s.io"
    kind: "HTTPRoute"
    name: "mcp-api-key-server-route"  # The name and namespace of your MCP Server HTTPRoute
    namespace: "mcp-test"
EOF
```

## Step 3: Verify Configuration

Check that the MCPServer was created and discovered:

```bash
# Check MCPServer status
kubectl get mcpserver -A

# Check controller logs
kubectl logs -n mcp-system deployment/mcp-gateway-controller

# Check broker logs for tool discovery
kubectl logs -n mcp-system deployment/mcp-gateway-broker-router | grep "Discovered tools"
```

## Step 4: Test Tool Discovery

Verify that your MCP server tools are now available through the gateway:

```bash
# Step 1: Initialize MCP session and capture session ID
# Use -D to dump headers to a file, then read the session ID
curl -s -D /tmp/mcp_headers -X POST http://mcp.127-0-0-1.sslip.io:8888/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0.0"}}}'

# Extract the MCP session ID from response headers
SESSION_ID=$(grep -i "mcp-session-id:" /tmp/mcp_headers | cut -d' ' -f2 | tr -d '\r')

echo "MCP Session ID: $SESSION_ID"

# Step 2: List tools using the session ID
curl -X POST http://mcp.127-0-0-1.sslip.io:8888/mcp \
  -H "Content-Type: application/json" \
  -H "mcp-session-id: $SESSION_ID" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}'

# Clean up
rm -f /tmp/mcp_headers
```

You should now see your MCP server tools in the response, prefixed with your configured `toolPrefix` (e.g., `myserver_`).

## Next Steps

Once you have MCP servers configured, you can explore advanced features:

- **[Virtual MCP Servers](./virtual-mcp-servers.md)** - Create focused tool collections
- **[Authentication](./authentication.md)** - Configure OAuth-based security
- **[Authorization](./authorization.md)** - Set up fine-grained access control
