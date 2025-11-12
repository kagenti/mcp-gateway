# Kubernetes MCP Server

This guide demonstrates how to add the [Kubernetes MCP server](https://github.com/containers/kubernetes-mcp-server) as an external MCP server behind the MCP Gateway.

The Kubernetes MCP server runs on the local machine alongside a Kubernetes Kind cluster that has the MCP Gateway stack deployed to it.

This demo is part of a 3-use case series:

1. [No auth](#use-case-1-no-auth) ✅
2. [Simple auth](#use-case-2-simple-auth)
3. [Cross-domain auth](#use-case-3-cross-domain-auth)

<br/>

## Use case 1: No auth

In this use case, the Kubernetes MCP server does not handle authentication for the client. It uses the access from the current `.kube/config` file to connect to the known Kubernetes clusters when tools are called.

### ❶ Setup a local cluster with the MCP Gateway stack

Clone the MCP Gateway repo (so you have the tooling to easily setup a local dev/test environment):

```sh
git clone git@github.com:kagenti/mcp-gateway.git && cd mcp-gateway
```

Create a local cluster with MCP Gateway:

```sh
make local-env-setup
```

### ❷ Run the Kubernetes MCP server locally

Clone the Kubernetes MCP server repo:

```sh
git clone git@github.com:containers/kubernetes-mcp-server.git && cd kubernetes-mcp-server
```

Build a fresh new version of the MCP server:

```sh
make build
```

Run the MCP server on port 9999:

```sh
./kubernetes-mcp-server --port 9999
```

> **Note:** The command above will hold the shell. Start a new session to run the next steps.

### ❸ Register the MCP server with the MCP Gateway

Add a dedicated gateway listener for the MCP server:

```sh
kubectl patch gateway mcp-gateway -n gateway-system --type json -p='[
  {
    "op": "add",
    "path": "/spec/listeners/-",
    "value": {
      "name": "kubernetes-mcp",
      "hostname": "host.containers.internal",
      "port": 9999,
      "protocol": "HTTP",
      "allowedRoutes": {
        "namespaces": {
          "from": "All"
        }
      }
    }
  }
]'
```

Create a route, external service and `MCPServer` custom resource:

```sh
kubectl apply -n mcp-test -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: kubernetes-mcp
spec:
  parentRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: mcp-gateway
    namespace: gateway-system
  hostnames:
  - host.containers.internal
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /mcp
    backendRefs:
    - group: ""
      kind: Service
      name: kubernetes-mcp-external
      port: 9999
---
apiVersion: v1
kind: Service
metadata:
  name: kubernetes-mcp-external
spec:
  type: ExternalName
  externalName: host.containers.internal
  ports:
  - name: http
    port: 9999
    targetPort: 9999
    protocol: TCP
    appProtocol: http
---
apiVersion: networking.istio.io/v1beta1
kind: ServiceEntry
metadata:
  name: kubernetes-mcp
  namespace: mcp-test
spec:
  hosts:
  - host.containers.internal
  ports:
  - number: 9999
    name: https
    protocol: HTTP
  location: MESH_EXTERNAL
  resolution: DNS
---
apiVersion: mcp.kagenti.com/v1alpha1
kind: MCPServer
metadata:
  name: kubernetes-mcp-server
spec:
  toolPrefix: kube_
  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: kubernetes-mcp
EOF
```

### ❹ Try the MCP server behind the gateway

From the MCP gateway repo, run the following command to launch the MCP Inspector on your default web browser:

```sh
make inspect-gateway
```

In the MCP Inspector, click _Connect_:

![MCP Inspector](images/mcp-inspector-1.png)

List the available MCP tools:

![MCP Inspector Connected](images/mcp-inspector-2-connected.png)

Notice the multiple MCP tools from various test MCP servers behind the gateway, including tools from the Kubernetes MCP server (prefixed with `kube_`):

![MCP Inspector Tools](images/mcp-inspector-3-tools.png)

Select Kubernetes MCP server's `namespaces_list` tool:

![Kubernetes MCP server namespaces tool](images/mcp-inspector-4-kube-namespaces.png)

Call the tool:

![Kubernetes MCP server namespaces tool success](images/mcp-inspector-5-kube-namespaces-success.png)

<br/>

## Use case 2: Simple auth

This use case handles simple authentication with token passthrough from the Kubernetes MCP server to the Kubernetes clusters.

The accessible Kubernetes API servers accept same-domain OIDC access tokens, which are obtained by the MCP gateway on behalf of the MCP clients via Standard OAuth2 Token Exchange.

-- TODO --

<br/>

## Use case 3: Cross-domain auth

This use case handles token exchange across different auth domains so multiple Kubernetes clusters can be called by the MCP server, with each cluster deployed to its own OIDC domain.

-- TODO --
