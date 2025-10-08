# Connecting to External MCP Servers

This guide demonstrates how to connect MCP Gateway to external MCP servers using Gateway API and Istio. We'll use the public GitHub MCP server as an example.

## Prerequisites

- MCP Gateway installed and configured
- Gateway and HTTPRoute configured for MCP Gateway  
- Gateway API Provider (Istio) with ServiceEntry and DestinationRule support
- Network egress access to external MCP server
- Authentication credentials for the external server (if required)

## About the GitHub MCP Server

The GitHub MCP server (https://api.githubcopilot.com/mcp/) provides programmatic access to GitHub functionality through the Model Context Protocol. It exposes 90+ tools for repository management, issues, pull requests, and code operations.

For this example, you'll need a GitHub Personal Access Token with `read:user` permissions. Get one at https://github.com/settings/tokens/new

```bash
export GITHUB_PAT="ghp_YOUR_GITHUB_TOKEN_HERE"
```

## Overview

To connect to an external MCP server, you need:
1. Gateway listener for the external hostname
2. HTTPRoute that routes to an ExternalName Service  
3. ServiceEntry to define the external service in Istio
4. DestinationRule for connection policies
5. MCPServer resource to register with MCP Gateway
6. Secret with authentication credentials

## Step 1: Add External Hostname to Gateway

The `Gateway` needs a listener for the external hostname:

```bash
# Add the GitHub listener to the Gateway using kubectl patch
kubectl patch gateway mcp-gateway -n gateway-system --type json -p='[
  {
    "op": "add",
    "path": "/spec/listeners/-",
    "value": {
      "name": "github-external",
      "hostname": "api.githubcopilot.com",
      "port": 8080,
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

## Step 2: Create HTTPRoute for External Service

Create an `HTTPRoute` that matches the external hostname and routes to the `ExternalName` Service:

```bash
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: github-mcp-external
  namespace: mcp-test
spec:
  parentRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: mcp-gateway
    namespace: gateway-system
  hostnames:
  - api.githubcopilot.com  # must match the Gateway listener hostname
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /mcp
    backendRefs:
    - group: ""
      kind: Service
      name: api-githubcopilot-com
      namespace: mcp-test
      port: 443
EOF
```

## Step 3: Create ExternalName Service

Create a `Service` that represents the external endpoint:

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: api-githubcopilot-com
  namespace: mcp-test
spec:
  type: ExternalName
  externalName: api.githubcopilot.com
  ports:
  - name: https
    port: 443
    targetPort: 443
    protocol: TCP
    appProtocol: https
EOF
```

## Step 4: Create ServiceEntry for GitHub MCP API

The `ServiceEntry` tells Istio about the external service:

```bash
kubectl apply -f - <<EOF
apiVersion: networking.istio.io/v1beta1
kind: ServiceEntry
metadata:
  name: github-mcp-external
  namespace: mcp-test
spec:
  hosts:
  - api.githubcopilot.com
  ports:
  - number: 443
    name: https
    protocol: HTTPS
  location: MESH_EXTERNAL
  resolution: DNS
EOF
```

## Step 5: Create DestinationRule

Configure how Istio connects to the external service via a `DestinationRule`:

```bash
kubectl apply -f - <<EOF
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: github-mcp-external
  namespace: mcp-test
spec:
  host: api.githubcopilot.com
  trafficPolicy:
    connectionPool:
      tcp:
        maxConnections: 10
      http:
        http1MaxPendingRequests: 10
        h2UpgradePolicy: UPGRADE
    tls:
      mode: SIMPLE
      sni: api.githubcopilot.com
EOF
```

## Step 6: Create Secret with Authentication

Create a secret containing your GitHub PAT token with the Bearer prefix. The required label enables the controller to watch for credential changes:

```bash
# Create the secret with Bearer prefix and required label
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: github-token
  namespace: mcp-test
  labels:
    mcp.kagenti.com/credential: "true"  # Required label for credential secrets
type: Opaque
stringData:
  token: "Bearer $GITHUB_PAT"
EOF
```

**Important:** The `mcp.kagenti.com/credential=true` label is **required** for all credential secrets. Without this label:
- The secret will not be watched for changes
- The MCPServer will fail validation with an error in its status
- Automatic credential updates will not work

## Step 7: Create the MCPServer Resource

Create the `MCPServer` resource that registers the GitHub MCP server with the gateway:

```bash
kubectl apply -f - <<EOF
apiVersion: mcp.kagenti.com/v1alpha1
kind: MCPServer
metadata:
  name: github
  namespace: mcp-test
spec:
  toolPrefix: github_
  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: github-mcp-external
  credentialRef:
    name: github-token
    key: token
EOF
```

## Step 8: Wait for Configuration Sync

Wait for the configuration to sync to the broker (this typically takes 10-15 seconds with volume-mounted credentials):

```bash
# Wait for GitHub tools to be discovered
echo "Waiting for GitHub tools to be discovered..."
until kubectl logs -n mcp-system deploy/mcp-broker-router | grep "Discovered.*tools.*github"; do
  echo "Still waiting..."
  sleep 5
done
echo "GitHub tools discovered!"
```

**Note:** With labeled secrets and volume mounts, credentials update automatically without pod restarts.

## Verification

Check that the MCPServer is registered:

```bash
# Check MCPServer status
kubectl get mcpservers -n mcp-test

# Check controller logs
kubectl logs -n mcp-system deployment/mcp-controller | grep github

# Check broker tool discovery
kubectl logs -n mcp-system deployment/mcp-broker-router | grep "Discovered.*tools.*github"
```

## Test Integration

Test the external MCP server through the gateway:

```bash
# Test tools/list through the gateway
curl -X POST http://mcp.127-0-0-1.sslip.io:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```

You should see GitHub tools prefixed with `github_` in the response, along with any other configured MCP servers.

## Cleanup

When done, cleanup things:

```bash
# Delete the MCPServer and Istio resources
kubectl delete mcpserver github -n mcp-test
kubectl delete httproute github-mcp-external -n mcp-test
kubectl delete service api-githubcopilot-com -n mcp-test
kubectl delete serviceentry github-mcp-external -n mcp-test
kubectl delete destinationrule github-mcp-external -n mcp-test
kubectl delete secret github-token -n mcp-test

# Remove the listener from the Gateway
kubectl get gateway mcp-gateway -n gateway-system -o json | \
  jq 'del(.spec.listeners[] | select(.name == "github-external"))' | \
  kubectl apply -f -
```
