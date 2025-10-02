# minimal MCP Gateway installation

This directory provides a minimal installation of MCP Gateway with just the core components.

## prerequisites

- kubernetes cluster (1.28+)
- **gateway API CRDs installed** (required!)
  ```bash
  kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml
  ```
- gateway controller (istio, envoy gateway, etc.)
- kubectl configured

**note:** the controller will crash-loop until gateway API CRDs are present

## what gets installed

- MCP Gateway CRDs (`MCPServer`)
- MCP broker/router deployment
- MCP controller deployment
- RBAC (service accounts, roles, bindings)
- services (mcp-broker-router, mcp-config)
- basic HTTPRoute for the broker

## what you need to provide

- gateway resource (your own gateway instance)
- authentication/authorization (optional - kuadrant, keycloak, etc.)
- TLS certificates (optional)
- MCP server deployments

## installation

### from GitHub (recommended)

```bash
kubectl apply -k 'https://github.com/kagenti/mcp-gateway/config/install?ref=main'
```

or a specific version tag:

```bash
kubectl apply -k 'https://github.com/kagenti/mcp-gateway/config/install?ref=v0.1.0'
```

**note:** quotes are required in zsh to prevent globbing on the `?` character

### local development

```bash
git clone https://github.com/kagenti/mcp-gateway
cd mcp-gateway
kubectl apply -k config/install
```

## verify installation

```bash
# check namespace created
kubectl get namespace mcp-system

# check deployments
kubectl get deployments -n mcp-system

# check CRDs
kubectl get crd mcpservers.mcp.kagenti.com
```

## next steps

1. create a gateway resource that the HTTPRoute can attach to
2. deploy your MCP servers
3. create MCPServer resources to register them
4. (optional) configure authentication via AuthPolicy

## example gateway

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: mcp-gateway
  namespace: mcp-system
spec:
  gatewayClassName: istio  # or your gateway class
  listeners:
  - name: http
    protocol: HTTP
    port: 8080
    allowedRoutes:
      namespaces:
        from: All
```

## example MCPServer

```yaml
apiVersion: mcp.kagenti.com/v1alpha1
kind: MCPServer
metadata:
  name: my-mcp-server
  namespace: mcp-test
spec:
  toolPrefix: myserver_
  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: my-mcp-route
```

## uninstall

```bash
kubectl delete -k 'https://github.com/kagenti/mcp-gateway/config/install?ref=main'
```
