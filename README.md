# MCP Gateway

An Envoy-based gateway for Model Context Protocol (MCP) servers, enabling aggregation and routing of multiple MCP servers behind a single endpoint.

## Architecture

MCP Gateway uses a single unified binary (`mcp-broker-router`) that combines:
- **Broker** (port 8080): Manages sessions and aggregates tools from multiple MCP servers
- **Router** (port 50051): Envoy external processor for parsing and routing MCP requests  
- **Controller** (optional): Kubernetes controller for dynamic MCP server discovery for generating broker/router configuration dynamically

## Quick Start

```bash
# Setup (automatically installs required tools to ./bin/)
make local-env-setup        # Create Kind cluster with Istio, Gateway API, MetalLB, Keycloak, Kuadrant & mcp-gateway

# Running the Gateway

## Standalone Mode (file-based config)
make run                    # Uses config/mcp-system/config.yaml

## Controller Mode (Kubernetes-based discovery)  
make install-crd            # Install MCPGateway CRD
make run-controller         # Run with controller enabled

# Local Development
make dev                    # Configure cluster to use local services
make dev-gateway-forward    # Forward gateway to localhost:8888
make dev-test               # Test the gateway

# Inspection & Debugging
make info                   # Show setup info and useful commands
make urls                   # Show all service URLs
make status                 # Check component status
make logs                   # Tail gateway logs
make debug-envoy            # Enable debug logging
make inspect-mock           # Open MCP Inspector for mock server

# Services
make keycloak-forward       # Access Keycloak at localhost:8095
make kuadrant-status        # Check Kuadrant operator status

# Cleanup
make dev-stop               # Stop local processes
make local-env-teardown     # Destroy cluster
```

Run `make help` to see all available commands.

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
- Watches `MCPGateway` custom resources
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
port: 8080
bindAddr: "0.0.0.0"
logLevel: info
```

### Kubernetes Configuration  
Create MCPGateway resources that reference HTTPRoutes:

```yaml
apiVersion: mcp.kagenti.com/v1alpha1
kind: MCPGateway
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

## Development

See [CLAUDE.md](CLAUDE.md) for detailed development documentation.

### Building

```bash
make build               # Build the unified binary
make clean               # Clean build artifacts
```

### Code Quality

```bash
make lint                # Run all linters
make fmt                 # Format code
make vet                 # Run go vet
```
