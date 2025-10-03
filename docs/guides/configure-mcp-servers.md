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
    name: "mcp-api-key-server-route"
    namespace: "mcp-test"
EOF
```

## Step 3: Verify Configuration

Check that the MCPServer was created and discovered:

```bash
# Check MCPServer status
kubectl get mcpserver -A

# Check controller logs
kubectl logs -n mcp-system deployment/mcp-controller | grep "my-mcp-server"

# Check broker logs for tool discovery
kubectl logs -n mcp-system deployment/mcp-broker-router | grep "Discovered tools"
```

## Step 4: Test Tool Discovery

Verify that your MCP server tools are now available through the gateway:

```bash
# Test tools/list through the gateway
curl -X POST http://mcp.example.com:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Host: mcp.example.com" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```

You should now see your MCP server tools in the response, prefixed with your configured `toolPrefix` (e.g., `myserver_`).

The controller will:
- Discover the HTTPRoute referenced in `targetRef`
- Extract the backend service URL
- Update the broker configuration with the new server
- Enable routing for tools with the specified prefix
