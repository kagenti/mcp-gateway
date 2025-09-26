# Connecting to External MCP Servers

This guide demonstrates how to connect MCP Gateway to external MCP servers using Gateway API and Istio. We'll use the public GitHub MCP server in this example.

## About the GitHub MCP Server

The GitHub MCP server (https://api.githubcopilot.com/mcp/) provides programmatic access to GitHub functionality through the Model Context Protocol. It exposes 90+ tools for:
- Repository management and queries
- Issue and pull request operations
- Code search and file operations
- GitHub Actions workflow management

More details: https://github.com/github/github-mcp-server

## Prerequisites

**Required Setup:** This guide assumes you've already created a local Kind cluster with MCP Gateway installed:

```bash
make local-env-setup
```

This command creates a Kind cluster and installs:
- Istio service mesh
- Gateway API CRDs
- MCP Gateway controller and broker
- Test MCP servers

If you haven't run this yet, do so before proceeding with this guide.

### GitHub Personal Access Token (PAT)
For this tutorial, you'll need a GitHub Personal Access Token (PAT) with minimal permissions

**For GitHub.com:**
1. Go to https://github.com/settings/tokens/new
2. Select only: `read:user` - Read user profile data
3. Generate the token

**Production Note:** This static PAT approach is for demonstration only. In production, you would use OAuth flows for per-user authentication.

**Set your token as an environment variable:**
```bash
export GITHUB_PAT="ghp_YOUR_GITHUB_TOKEN_HERE"
```

### Network Requirements
- Egress access from your cluster to `api.githubcopilot.com` (port 443)

## Verify Prerequisites

Before proceeding, verify all dependencies are met:

```bash
# Verify MCP Gateway components are deployed
kubectl get pods -n mcp-system
kubectl get gateway -n gateway-system mcp-gateway

# verify egress to GitHub's MCP server from a test pod
kubectl run -it --rm debug --image=nicolaka/netshoot --restart=Never -- \
  curl -I https://api.githubcopilot.com/mcp/

# Test your GitHub token (outside cluster)
curl -H "Authorization: Bearer $GITHUB_PAT" \
  https://api.github.com/user
```

## Overview

To connect to an external MCP server using Istio and Gateway API, we need:
1. A `Gateway` listener for the external hostname (`api.githubcopilot.com`)
2. An `HTTPRoute` that matches the external hostname
3. An `ExternalName` Service pointing to the external service
4. A `ServiceEntry` to define the external service in Istio's service registry
5. A `DestinationRule` for connection policies
6. An `MCPServer` CR to register this MCP server with the gateway
7. A `Secret` containing authentication credentials that the broker/router will use to authenticate with the external MCP server

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
until kubectl logs -n mcp-system deploy/mcp-broker-router --tail=100 | grep -q "Discovered.*github.*94"; do
  echo "Still waiting..."
  sleep 5
done
echo "GitHub tools discovered!"
```

**Note:** With labeled secrets and volume mounts, credentials update automatically without pod restarts.

## Verification

Check that the `MCPServer` is registered:

```bash
kubectl get mcpservers -n mcp-test
```

Check the controller logs to see if it discovered the external server:

```bash
kubectl logs -n mcp-system deployment/mcp-controller | grep github
```

Verify the broker discovered the tools:

```bash
kubectl logs -n mcp-system deployment/mcp-broker-router | grep "Discovered.*tools.*github"
# should show: "Discovered tools mcpURL=https://api.githubcopilot.com:443/mcp #tools=94"
```

Test the integration through the gateway:

```bash
# Open MCP Inspector
make inspect-gateway
```

This will open the inspector at `http://mcp.127-0-0-1.sslip.io:8888/mcp`

**Verify:**

1. Navigate to **Tools** → **List Tools**
   - You should see both test server tools and ~90 GitHub tools prefixed with `github_`

2. Test a GitHub tool:
   - Find and select `github_get_me`
   - Click **Run Tool**
   - Should return your GitHub user information

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
