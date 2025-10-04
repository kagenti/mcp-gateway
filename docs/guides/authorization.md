# Authorization Configuration

This guide covers configuring fine-grained authorization and access control for MCP Gateway, building on the authentication setup.

## Overview

Authorization in MCP Gateway controls which authenticated users can access specific MCP tools. This guide demonstrates using Kuadrant's AuthPolicy with Common Expression Language (CEL) to implement role-based access control.

Key concepts:
- **Tool-Level Authorization**: Control access to individual MCP tools
- **Group-Based Access**: Use OAuth groups/roles for permission decisions
- **External ACL**: Fetch access control lists from external services
- **CEL Expressions**: Define complex authorization logic using Common Expression Language

## Prerequisites

- [Authentication Configuration](./authentication.md) completed
- Identity provider configured to include group/role claims in tokens
- [Node.js and npm](https://nodejs.org/en/download/) installed (for MCP Inspector testing)

**Note**: This guide demonstrates authorization using Kuadrant's AuthPolicy, but MCP Gateway supports any Istio/Gateway API compatible authorization mechanism.

## Understanding the Authorization Flow

1. **Authentication**: User authenticates and receives JWT token with group claims
2. **Tool Request**: Client makes MCP tool call (e.g., `tools/call`)
3. **Metadata Fetch**: AuthPolicy fetches ACL from external endpoint
4. **Authorization Check**: CEL expression evaluates user groups against tool permissions
5. **Access Decision**: Allow or deny based on evaluation result

## Step 1: Deploy Access Control Service

Deploy a service that provides access control information:

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: acl-config
  namespace: mcp-system
data:
  config.json: |
    {
        "acls": [
            {
                "id": "server1.mcp.local",
                "access": {
                    "accounting": [
                        "test_headers"
                    ],
                    "developers": [
                        "test_hello_world"
                    ]
                }
            },
            {
                "id": "server2.mcp.local", 
                "access": {
                    "accounting": [
                        "test2_headers"
                    ]
                }
            }
        ]
    }
    
    # NOTE: This is an example ACL configuration. Replace with your own:
    # - "id" should match your MCP server hostnames
    # - "access" groups should match your Keycloak groups
    # - Tool names should match your actual MCP server tools
    # Use 'curl http://mcp.127-0-0-1.sslip.io:8888/mcp' with tools/list to discover available tools
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: acl-config
  namespace: mcp-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: acl-config
  template:
    metadata:
      labels:
        app: acl-config
    spec:
      containers:
      - name: acl-server
        image: nginx:alpine
        ports:
        - containerPort: 80
        volumeMounts:
        - name: config
          mountPath: /usr/share/nginx/html
        command: ["/bin/sh"]
        args: ["-c", "cp /usr/share/nginx/html/config.json /usr/share/nginx/html/config/server1.mcp.local && cp /usr/share/nginx/html/config.json /usr/share/nginx/html/config/server2.mcp.local && nginx -g 'daemon off;'"]
      volumes:
      - name: config
        configMap:
          name: acl-config
---
apiVersion: v1
kind: Service
metadata:
  name: acl-config
  namespace: mcp-system
spec:
  selector:
    app: acl-config
  ports:
  - port: 8181
    targetPort: 80
EOF
```

**ACL Structure Explained:**

- `id`: Server identifier (matches hostnames in HTTPRoutes)
- `access`: Maps groups to allowed tools
- `accounting` group: Can use `test_headers` tool
- `developers` group: Can use `test_hello_world` tool

## Step 2: Configure Tool-Level Authorization

Apply an AuthPolicy that enforces tool-level access control:

```bash
kubectl apply -f - <<EOF
apiVersion: kuadrant.io/v1
kind: AuthPolicy
metadata:
  name: mcp-tool-auth-policy
  namespace: gateway-system
spec:
  targetRef:
    group: gateway.networking.k8s.io
    kind: Gateway
    name: mcp-gateway
    sectionName: mcps  # Targets the MCP server listener
  defaults:
    rules:
      metadata:
        rbac:
          http:
            urlExpression: '"http://acl-config.mcp-system.svc.cluster.local:8181/config/"+request.headers[":authority"]'
      authentication:
        'keycloak':
          jwt:
            issuerUrl: http://keycloak.keycloak.svc.cluster.local/realms/mcp
      authorization:
        'allow-tool-call':
          patternMatching:
            patterns:
              - predicate: auth.identity.groups.exists(g, request.headers['x-mcp-toolname'] in auth.metadata.rbac.access[g])
      response:
        unauthorized:
          code: 403
          body:
            value: |
              {
                "error": "Forbidden", 
                "message": "MCP Tool Access denied. Insufficient permissions for this tool."
              }
        unauthenticated:
          code: 401
          headers:
            'WWW-Authenticate':
              value: Bearer resource_metadata=http://mcp.127-0-0-1.sslip.io:8888/.well-known/oauth-protected-resource/mcp
          body:
            value: |
              {
                "error": "Unauthorized",
                "message": "MCP Tool Access denied. Authentication required."
              }
EOF
```

**Key Configuration Explained:**

- **Metadata Fetch**: `urlExpression` builds URL using request hostname to fetch relevant ACL
- **Authentication**: Same JWT validation as basic auth policy
- **Authorization Logic**: CEL expression checks if user's groups allow access to the requested tool
- **CEL Breakdown**: `auth.identity.groups.exists(g, request.headers['x-mcp-toolname'] in auth.metadata.rbac.access[g])`
  - `auth.identity.groups`: User's groups from JWT token
  - `exists(g, ...)`: Checks if any group `g` satisfies the condition
  - `request.headers['x-mcp-toolname']`: Tool name from MCP request header
  - `auth.metadata.rbac.access[g]`: List of allowed tools for group `g` from ACL

## Step 3: Test Authorization

**Note**: The authentication guide already created the `accounting` group, added the `mcp` user to it, and configured group claims in JWT tokens. No additional Keycloak configuration is needed.

Test that authorization now controls tool access by setting up the MCP Inspector with port forwarding:

```bash
# Start port forwarding to the Istio gateway
kubectl -n gateway-system port-forward svc/mcp-gateway-istio 8888:8080 &
PORT_FORWARD_PID=$!

# Start MCP Inspector (requires Node.js/npm)
npx @modelcontextprotocol/inspector@latest &
INSPECTOR_PID=$!

# Wait for services to start
sleep 3

# Open MCP Inspector with the gateway URL
open "http://localhost:6274/?transport=streamable-http&serverUrl=http://mcp.127-0-0-1.sslip.io:8888/mcp"
```

**What this accomplishes:**
- **Gateway Access**: Makes the MCP Gateway accessible through your local browser
- **Authentication Testing**: Allows you to test the complete OAuth + authorization flow
- **Tool Verification**: Lets you verify which tools are accessible based on user groups

**Test Scenarios:**

1. **Login as mcp/mcp** (has both `accounting` and `developers` groups)
2. **Try allowed tools**:
   - `test_hello_world` - Should work (developers group)
   - `test_headers` - Should work (accounting group)
3. **Try restricted tools**:
   - Tools not in the ACL - Should return 403 Forbidden


## Alternative Authorization Mechanisms

While this guide uses Kuadrant AuthPolicy, MCP Gateway supports various authorization approaches including other policy engines, built-in Istio authorization, and Gateway API policy extensions.

## Monitoring and Observability

Monitor authorization decisions:

```bash
# Check AuthPolicy status
kubectl get authpolicy -A

# View authorization logs
kubectl logs -n kuadrant-system -l app=authorino
```

## Next Steps

With authorization configured, you can:
- **[External MCP Servers](./external-mcp-server.md)** - Apply auth to external services
- **[Virtual MCP Servers](./virtual-mcp-servers.md)** - Compose auth across multiple servers
- **[Troubleshooting](./troubleshooting.md)** - Debug auth and authz issues
