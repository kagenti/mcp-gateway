# MCP Gateway

An Envoy-based gateway for Model Context Protocol (MCP) servers, enabling aggregation and routing of multiple MCP servers behind a single endpoint.

## Architecture

See [./docs/design/overview.md](./docs/design/overview.md)

## Design Principles

### Envoy First
The core router and broker work directly with Envoy. No Kubernetes required - you can run this with plain Envoy config if you want.

### Kubernetes Adds Convenience
Running in Kubernetes gets you:
- **MCPServer CRD** - manage servers declaratively
- **HTTPRoute integration** - automatic backend discovery
- **Controller** - watches for changes and updates config in the broker & router

### Bring Your Own Policies
The router sets metadata on requests that any Envoy filter can use. We use Kuadrant in our examples for auth and rate limiting, but you can plug in whatever you want - custom ext_authz, WASM modules, or any other Envoy-compatible policy engine.

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

## Example OAuth setup

After running the Quick start above, configure OAuth by setting environment variables for the mcp-broker and applying an AuthPolicy that validates tokens on the /mcp endpoint.

Configure the mcp-broker with OAuth environment variables:

```bash
# Configure OAuth discovery endpoint via environment variables
export OAUTH_RESOURCE_NAME="MCP Server"
export OAUTH_RESOURCE="http://mcp.127-0-0-1.sslip.io:8888/mcp"  
export OAUTH_AUTHORIZATION_SERVERS="http://keycloak.127-0-0-1.sslip.io:8889/realms/mcp"
export OAUTH_BEARER_METHODS_SUPPORTED="header"
export OAUTH_SCOPES_SUPPORTED="basic"

# Restart the broker to pick up the new configuration
kubectl set env deployment/mcp-broker-router \
  OAUTH_RESOURCE_NAME="$OAUTH_RESOURCE_NAME" \
  OAUTH_RESOURCE="$OAUTH_RESOURCE" \
  OAUTH_AUTHORIZATION_SERVERS="$OAUTH_AUTHORIZATION_SERVERS" \
  OAUTH_BEARER_METHODS_SUPPORTED="$OAUTH_BEARER_METHODS_SUPPORTED" \
  OAUTH_SCOPES_SUPPORTED="$OAUTH_SCOPES_SUPPORTED" \
  -n mcp-system

# Apply AuthPolicy for token validation
kubectl apply -f ./config/mcp-system/authpolicy.yaml
```

The mcp-broker now serves OAuth discovery information at `/.well-known/oauth-protected-resource`.

### Keycloak Setup

Set up a new 'mcp' realm in keycloak with user/pass mcp/mcp:

```bash
make keycloak-setup-mcp-realm
```

Finally, open the mcp-inspector at http://localhost:6274/?transport=streamable-http&serverUrl=http://mcp.127-0-0-1.sslip.io:8888/mcp and go through the OAuth flow.

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

#### MCPServer Resource

The `MCPServer` is a Kubernetes Custom Resource that defines a collection of MCP (Model Context Protocol) servers to be aggregated by the gateway. It enables discovery and federation of tools from multiple backend MCP servers through Gateway API `HTTPRoute` references, providing a declarative way to configure which MCP servers should be accessible through the gateway.

Each `MCPServer` resource:
- References one or more HTTPRoutes that point to backend MCP services
- Configures tool prefixes to avoid naming conflicts when federating tools
- Enables the controller to automatically discover and configure the broker with available MCP servers
- Maintains status conditions to indicate whether the servers are successfully discovered, valid and ready

Create `MCPServer` resources that reference HTTPRoutes:

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

### OAuth Configuration

The mcp-broker supports configurable OAuth protected resource discovery through environment variables. When configured, the broker serves OAuth discovery information at `/.well-known/oauth-protected-resource`.

| Environment Variable | Description | Default | Example |
|---------------------|-------------|---------|---------|
| `OAUTH_RESOURCE_NAME` | Human-readable name for the protected resource | `"MCP Server"` | `"My MCP Gateway"` |
| `OAUTH_RESOURCE` | URL of the protected MCP endpoint | `"/mcp"` | `"http://mcp.example.com/mcp"` |
| `OAUTH_AUTHORIZATION_SERVERS` | Comma-separated list of authorization server URLs | `[]` (empty) | `"http://keycloak.example.com/realms/mcp,http://auth.example.com"` |
| `OAUTH_BEARER_METHODS_SUPPORTED` | Comma-separated list of bearer token methods | `["header"]` | `"header,query"` |
| `OAUTH_SCOPES_SUPPORTED` | Comma-separated list of supported scopes | `["basic"]` | `"basic,read,write"` |

**Example configuration:**

```bash
export OAUTH_RESOURCE_NAME="Production MCP Server"
export OAUTH_RESOURCE="https://mcp.example.com/mcp"
export OAUTH_AUTHORIZATION_SERVERS="https://keycloak.example.com/realms/mcp"
export OAUTH_BEARER_METHODS_SUPPORTED="header"
export OAUTH_SCOPES_SUPPORTED="basic,read,write"
```

**Response format:**

The endpoint returns a JSON response following the OAuth Protected Resource discovery specification:

```json
{
  "resource_name": "Production MCP Server",
  "resource": "https://mcp.example.com/mcp", 
  "authorization_servers": [
    "https://keycloak.example.com/realms/mcp"
  ],
  "bearer_methods_supported": ["header"],
  "scopes_supported": ["basic", "read", "write"]
}
```
