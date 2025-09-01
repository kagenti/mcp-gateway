# MCP Gateway

An Envoy-based gateway for Model Context Protocol (MCP) servers, enabling aggregation and routing of multiple MCP servers behind a single endpoint.

## Architecture

See [./docs/design/overview.md](./docs/design/overview.md)

## Quick Start with mcp-inspector

Set up a local kind cluster with the Broker, Router & Controller running.
These components are built during the make target into a single image and loaded into the cluster.
Also sets up an Istio Gateway API Gateway, and HTTPRoutes for test mcp servers, which are added to the broker/router.

```bash
make local-env-setup
```

Run the mcp-inspector and connect to the gateway (This also port forwards to the gateway)

```bash
make inspect-gateway
```

Open http://localhost:6274/?transport=streamable-http&serverUrl=http://mcp.127-0-0-1.sslip.io:8888/mcp

## Running Modes

### Standalone Mode (File-based)
Uses a YAML configuration file to define MCP servers:

```bash
make run
# Or directly:
./bin/mcp-broker-router --mcp-gateway-config ./config/mcp-system/config.yaml
```

The broker watches the config file for changes and hot-reloads configuration automatically.

### Controller Mode (Kubernetes)
Discovers MCP servers dynamically from Kubernetes Gateway API `HTTPRoute` resources:

```bash
make run-controller
# Or directly:
./bin/mcp-broker-router --controller
```

In controller mode:
- Watches `MCPServer` custom resources
- Discovers servers via `HTTPRoute` references
- Generates aggregated configuration in `ConfigMap`, for use by the broker/router
- Exposes health endpoints on `:8081` and metrics on `:8082`

## Configuration

### Standalone Configuration
Edit `config/mcp-system/config.yaml`:

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

### Kubernetes Configuration  
Create MCPServer resources that reference HTTPRoutes:

```yaml
apiVersion: mcp.kagenti.com/v1alpha1
kind: MCPServer
metadata:
  name: ai-tools
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: weather-route
    toolPrefix: weather_
  - group: gateway.networking.k8s.io
    kind: HTTPRoute  
    name: calendar-route
    toolPrefix: cal_
```

## Command Line Flags

```bash
--mcp-router-address    # gRPC ext_proc address (default: 0.0.0.0:50051)
--mcp-broker-address    # HTTP broker address (default: 0.0.0.0:8080)
--mcp-gateway-config    # Config file path (default: ./config/mcp-system/config.yaml)
--controller            # Enable Kubernetes controller mode
```
