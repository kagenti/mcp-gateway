# Installing and Configuring MCP Gateway

This guide demonstrates how to install and configure MCP Gateway to aggregate multiple Model Context Protocol (MCP) servers behind a single endpoint.

## Prerequisites

### Kubernetes Cluster Options

MCP Gateway runs on Kubernetes and integrates with Gateway API and Istio. You should be familiar with:
- **Kubernetes** - Basic kubectl and YAML knowledge
- **Gateway API** - Kubernetes standard for traffic routing
- **Istio** - Service mesh and Gateway API provider

**Choose your setup approach:**

**Option A: Quick Start (5 minutes)**
- Want to try MCP Gateway immediately with minimal setup
- Automated script handles everything for you
- Perfect for evaluation and testing
- **[Quick Start Guide](./quick-start.md)**

**Option B: Existing Cluster**
- You have a Kubernetes cluster with Gateway API CRDs and Istio installed
- Ready to deploy MCP Gateway immediately

**Option C: Local Development Cluster**  
- Set up a local Kind cluster with all prerequisites
- See: [Kind Cluster Setup Guide](./kind-cluster-setup.md)

### MCP Servers

You'll need at least one MCP server to route through the gateway. This can be:
- Internal services running in your cluster
- External MCP servers (like GitHub's MCP API)
- Test servers for evaluation

## Installation Methods

### Method 1: Helm (Recommended)

Install from GitHub Container Registry:

```bash
helm install mcp-gateway oci://ghcr.io/kagenti/charts/mcp-gateway
```

This automatically installs:
- MCP Gateway components (broker, router, controller)
- Required CRDs and RBAC
- EnvoyFilter for Istio integration

### Method 2: Kustomize

Install using Kubernetes manifests:

```bash
kubectl apply -k 'https://github.com/kagenti/mcp-gateway/config/install?ref=main'
```

This provides the same components as Helm but with less configuration flexibility.

### Method 3: Standalone Installation (Advanced)

For non-Kubernetes deployments or advanced use cases, see [Standalone Installation Guide](./binary-install.md).

**Note:** This method is not fully supported and requires manual configuration of routing and service discovery. Also note that most guides lean into the kuberentes
based setup, leveraging various CRDs and kubectl commands.

## Post-Installation Configuration

After installation, you'll need to configure the gateway and connect your MCP servers:

1. **[Configure Gateway Listener and Route](./configure-mcp-gateway-listener-and-router.md)** - Set up traffic routing
2. **[Configure MCP Servers](./configure-mcp-servers.md)** - Connect internal MCP servers  
3. **[Connect External MCP Servers](./external-mcp-server.md)** - Connect to external APIs

## Optional Configuration

- **[Authentication](./authentication.md)** - Configure OAuth-based authentication
- **[Authorization](./authorization.md)** - Set up fine-grained access control
- **[Virtual MCP Servers](./virtual-mcp-servers.md)** - Create focused tool collections