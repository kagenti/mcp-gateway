# Installing and Configuring MCP Gateway

This guide demonstrates how to install and configure MCP Gateway to aggregate multiple Model Context Protocol (MCP) servers behind a single endpoint.

## About MCP Gateway

MCP Gateway acts as a unified entry point for multiple MCP servers, providing:
- Tool aggregation from multiple backend MCP servers
- Automatic service discovery via Kubernetes resources
- Dynamic routing based on tool prefixes
- Support for both standalone and Kubernetes deployments

## Prerequisites

Before proceeding, ensure you have the required tools installed:

**For standalone deployment:**
- Go 1.21 or later

**For Kubernetes deployment:**
- kubectl configured for your cluster
- Docker (for building images)

**For local development:**
- kind (Kubernetes in Docker)
- Docker and Docker Compose


## Installation Method 1: Local Development Environment

This method creates a complete local testing environment with all dependencies. **Recommended for getting started.**

### Step 1: Clone the Repository

```bash
git clone https://github.com/kagenti/mcp-gateway.git
cd mcp-gateway
```

### Step 2: Create Local Cluster

Run the automated setup:

```bash
make local-env-setup
```

This command automatically:
- Creates a Kind cluster
- Installs Istio service mesh
- Installs Gateway API CRDs
- Deploys MCP Gateway components (controller, broker, router)
- Deploys test MCP servers
- Configures routing and gateway resources

### Step 3: Verify Installation

Check that all components are running:

```bash
# Check MCP Gateway components
kubectl get pods -n mcp-system

# Check test MCP servers
kubectl get pods -n mcp-test

# Check gateway resources
kubectl get gateway -n gateway-system

# View overall status
make status
```

**Expected output:**
- Pods in `mcp-system` namespace: `mcp-broker-router` and `mcp-controller` (Running)
- Pods in `mcp-test` namespace: Test server pods (Running)
- Gateway in `gateway-system` namespace: `mcp-gateway` (Programmed)

### Step 4: Test the Gateway

Open the MCP Inspector to interact with your gateway:

```bash
make inspect-gateway
```

This opens a web interface at `http://localhost:6274/?transport=streamable-http&serverUrl=http://mcp.127-0-0-1.sslip.io:8888/mcp`

**Verify:**
1. Navigate to **Tools** → **List Tools**
2. You should see tools from all test servers with prefixes (`test_`, `test2_`, etc.)
3. Try calling a tool like `test_hi` to verify routing works

## Installation Method 2: Kubernetes Deployment

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

## Installation Method 3: Standalone Binary

Use this method to run MCP Gateway as a single binary with file-based configuration (no Kubernetes required).

### Step 1: Clone and Build

```bash
git clone https://github.com/kagenti/mcp-gateway.git
cd mcp-gateway
make build
```

### Step 2: Create Configuration File

Create `config/samples/config.yaml`:

```yaml
servers:
  - name: weather-service
    url: http://weather.example.com:8080
    hostname: weather.example.com
    enabled: true
    toolPrefix: "weather_"
  - name: calendar-service
    url: http://calendar.example.com:8080
    hostname: calendar.example.com
    enabled: true
    toolPrefix: "cal_"
```

**Configuration fields:**
- `name`: Identifier for the server
- `url`: Full URL to the MCP server endpoint
- `hostname`: Used for routing decisions
- `enabled`: Whether to include this server
- `toolPrefix`: Prefix added to all tools from this server

### Step 3: Start the Gateway

```bash
make run
```

**Or run the binary directly:**

```bash
./bin/mcp-broker-router --config=config/samples/config.yaml
```

The gateway starts with:
- HTTP broker listening on `0.0.0.0:8080`
- gRPC router listening on `0.0.0.0:50051`

### Step 4: Verify Standalone Installation

```bash
# Check health endpoint
curl http://localhost:8080/health

# List available tools
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```

## Troubleshooting

### Gateway Not Starting

**Check port availability:**

```bash
# Linux/Mac
lsof -i :8080
lsof -i :50051
```

**Verify configuration syntax:**

```bash
# Kubernetes
kubectl get mcpserver -A
kubectl describe mcpserver <name> -n <namespace>

# Standalone
cat config/samples/config.yaml
```

### Backend Servers Not Discovered

**Check controller logs:**

```bash
kubectl logs -n mcp-system deployment/mcp-controller
```

**Verify HTTPRoute exists:**

```bash
kubectl get httproute -A
kubectl describe httproute <route-name> -n <namespace>
```

**Check RBAC permissions:**

```bash
kubectl get clusterrole mcp-controller-role
kubectl get clusterrolebinding mcp-controller-rolebinding
```

### Tools Not Appearing

**Check broker logs:**

```bash
kubectl logs -n mcp-system deployment/mcp-broker-router -c broker | grep "Discovered tools"
```

**Verify backend server is reachable:**

```bash
# From within the cluster
kubectl run -it --rm debug --image=nicolaka/netshoot --restart=Never -- \
  curl http://<backend-service>.<namespace>.svc.cluster.local:<port>/health
```

## Next Steps

- [Connect to external MCP servers](./external-mcp-server.md)
- [Understand the architecture](./understanding-mcp-gateway-architecture.md)
- Configure rate limiting with Kuadrant