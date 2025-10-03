## Installation Method 2: Standalone Binary

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
