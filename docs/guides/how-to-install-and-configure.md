# Installing and Configuring MCP Gateway
This guide demonstrates how to install and configure MCP Gateway to aggregate multiple Model Context Protocol (MCP) servers behind a single endpoint.

## Prerequisites

- **Gateway API Provider** (Istio) including Gateway API CRDs
- **Some MCP Server** At least 1 MCP server you want to route via the Gateway

## Install via Helm

```shell
helm install mcp-gateway oci://ghcr.io/kagenti/charts/mcp-gateway
```

## Install via kustomize

```shell
kubectl apply -k 'https://github.com/kagenti/mcp-gateway/config/install
```

## Next Steps

- [Configure MCP Gateway listener and route](./configure-mcp-gateway-listener-and-router.md)
- [Configure MCP servers](./configure-mcp-servers.md)
- [Connect to external MCP servers](./external-mcp-server.md)
- [Configure authentication](./authentication.md)
- [Configure authorization](./authorization.md)  
- [Configure virtual MCP servers](./virtual-mcp-servers.md)