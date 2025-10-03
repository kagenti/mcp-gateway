## Kubernetes Deployment

Use this method to deploy MCP Gateway to an existing Kubernetes cluster.

### Prerequisites

Ensure your cluster has:
- Istio service mesh installed
- Gateway API CRDs installed

```bash
# Install Istio (if not already installed)
Follow instructions at https://istio.io/latest/docs/setup/getting-started/

# Install Gateway API CRDs (if not already installed)
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml
```

### Step 1: Install MCP Gateway CRDs

```bash
make install-crd
```

This installs the `MCPServer` custom resource definition.

### Step 2: Deploy MCP Gateway Components

```bash
make deploy
```

This creates:
- `mcp-system` namespace
- MCP broker and router deployment
- MCP controller deployment
- Required RBAC permissions (ServiceAccounts, Roles, RoleBindings)

### Step 3: Create a Gateway Resource

Create a Gateway that routes to MCP Gateway:

```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: mcp-gateway
  namespace: gateway-system
spec:
  gatewayClassName: istio
  listeners:
  - name: http
    hostname: "mcp.example.com"
    port: 8080
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: All
EOF
```

### Step 4: Create HTTPRoute to MCP Gateway Service

```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: mcp-gateway-route
  namespace: mcp-system
spec:
  parentRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: mcp-gateway
    namespace: gateway-system
  hostnames:
  - "mcp.example.com"
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /mcp
    backendRefs:
    - name: mcp-broker-router
      port: 8080
EOF
```

### Step 5: Configure MCP Servers

Create an `MCPServer` resource for each backend MCP server:

```bash
kubectl apply -f - <<EOF
apiVersion: mcp.kagenti.com/v1alpha1
kind: MCPServer
metadata:
  name: my-mcp-server
  namespace: mcp-system
spec:
  toolPrefix: myapp_
  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: my-service-route
    namespace: mcp-system
EOF
```

The controller will:
- Discover the HTTPRoute referenced in `targetRef`
- Extract the backend service URL
- Update the broker/router configuration
- Enable routing for tools with the `myapp_` prefix

### Step 6: Verify Deployment

```bash
# Check MCPServer status
kubectl get mcpserver -A

# Check controller discovered the server
kubectl logs -n mcp-system deployment/mcp-controller

# Check broker connected to the server
kubectl logs -n mcp-system deployment/mcp-broker-router -c broker | grep "Discovered tools"
```
