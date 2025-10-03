# Authentication Configuration

This guide covers configuring authentication for MCP Gateway using the Model Context Protocol (MCP) authorization specification.

## Overview

MCP Gateway implements the [MCP Authorization specification](https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization) which is based on OAuth 2.1. When authentication is enabled, the MCP Gateway broker acts as an OAuth 2.1 resource server, requiring valid access tokens for protected requests.

Key concepts:
- **OAuth 2.1 Resource Server**: MCP Gateway validates access tokens issued by your identity provider
- **WWW-Authenticate Response**: Returns 401 with authorization server discovery information
- **Protected Resource Metadata**: Exposes OAuth configuration at `/.well-known/oauth-protected-resource`
- **Dynamic Client Registration**: Supports automatic client registration for MCP clients

## Prerequisites

- MCP Gateway installed and configured
- Identity provider supporting OAuth 2.1 (this guide uses Keycloak)
- [Kuadrant operator](https://docs.kuadrant.io/1.2.x/install-helm/) installed
- [Node.js and npm](https://nodejs.org/en/download/) installed (for MCP Inspector testing)

**Note**: This guide demonstrates authentication using Kuadrant's AuthPolicy, but MCP Gateway supports any Istio/Gateway API compatible authentication mechanism.

## Step 1: Deploy Identity Provider

Deploy Keycloak as your OAuth 2.1 authorization server:

```bash
# Install Keycloak
kubectl apply -f https://raw.githubusercontent.com/kagenti/mcp-gateway/main/config/keycloak/deployment.yaml
kubectl apply -f https://raw.githubusercontent.com/kagenti/mcp-gateway/main/config/keycloak/httproute.yaml

# Wait for Keycloak to be ready
kubectl wait --for=condition=ready pod -l app=keycloak -n keycloak --timeout=120s

# Apply CORS preflight fix for Keycloak OIDC client registration
# This works around a known Keycloak bug: https://github.com/keycloak/keycloak/issues/39629
kubectl apply -f https://raw.githubusercontent.com/kagenti/mcp-gateway/refs/heads/main/config/keycloak/preflight_envoyfilter.yaml
```

Create the MCP realm and test user. This sets up a dedicated OAuth realm for MCP Gateway with proper OIDC configuration and dynamic client registration support:

```bash
# Get admin access token from Keycloak
TOKEN=$(curl -s -X POST "http://keycloak.127-0-0-1.sslip.io:8889/realms/master/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "username=admin" \
  -d "password=admin" \
  -d "grant_type=password" \
  -d "client_id=admin-cli" | jq -r '.access_token')

# Create MCP realm
curl -X POST "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"realm":"mcp","enabled":true}'

# Update MCP realm token settings
curl -X PUT "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"realm":"mcp","enabled":true,"ssoSessionIdleTimeout":1800,"accessTokenLifespan":1800}'

# Create test user 'mcp' with password 'mcp'
curl -X POST "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/users" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"mcp","email":"mcp@example.com","firstName":"mcp","lastName":"mcp","enabled":true,"emailVerified":true,"credentials":[{"type":"password","value":"mcp","temporary":false}]}'

# Create accounting group
curl -X POST "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/groups" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"accounting"}'

# Add mcp user to accounting group
# First get the user ID
USER_ID=$(curl -s -X GET "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/users?username=mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json" | jq -r '.[0].id')

# Get the group ID
GROUP_ID=$(curl -s -X GET "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/groups" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json" | jq -r '.[] | select(.name == "accounting") | .id')

# Add user to group
curl -X PUT "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/users/$USER_ID/groups/$GROUP_ID" \
  -H "Authorization: Bearer $TOKEN"

# Create groups client scope
curl -X POST "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/client-scopes" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"groups","protocol":"openid-connect","attributes":{"display.on.consent.screen":"false","include.in.token.scope":"true"}}'

# Get the client scope ID
SCOPE_ID=$(curl -s -X GET "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/client-scopes" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json" | jq -r '.[] | select(.name == "groups") | .id')

# Add groups mapper to client scope
curl -X POST "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/client-scopes/$SCOPE_ID/protocol-mappers/models" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"groups","protocol":"openid-connect","protocolMapper":"oidc-group-membership-mapper","config":{"claim.name":"groups","full.path":"false","id.token.claim":"true","access.token.claim":"true","userinfo.token.claim":"true"}}'

# Add groups client scope to realm's optional client scopes
curl -X PUT "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/default-optional-client-scopes/$SCOPE_ID" \
  -H "Authorization: Bearer $TOKEN"

# (FOR DEVELOPMENT ONLY) Remove trusted hosts policy for anonymous client registration
COMPONENT_ID=$(curl -s -X GET "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/components?name=Trusted%20Hosts" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json" | jq -r '.[0].id // empty' 2>/dev/null)

if [ -n "$COMPONENT_ID" ] && [ "$COMPONENT_ID" != "null" ]; then
  curl -X DELETE "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/components/$COMPONENT_ID" \
    -H "Authorization: Bearer $TOKEN"
  echo "Trusted hosts policy removed"
else
  echo "No trusted hosts policy found (already removed or not present)"
fi
```

**What this setup creates:**
- **MCP Realm**: Dedicated realm for MCP Gateway authentication
- **Test User**: User 'mcp' with password 'mcp' for testing
- **Accounting Group**: Group for authorization testing (user is added to this group)
- **Groups Client Scope**: Enables group membership claims in JWT tokens
- **Token Settings**: 30-minute session timeout and access token lifetime
- **Anonymous Client Registration**: Removes trusted hosts policy to allow dynamic client registration from any host (For development only. Not recommended for production)

**Why this setup is needed:**
- **Dedicated Realm**: Isolates MCP authentication from other applications
- **Dynamic Client Registration**: Allows MCP clients to automatically register without manual setup
- **Group Membership**: Enables authorization based on user groups (used in authorization guide)
- **OIDC Configuration**: Enables proper JWT token issuance with required claims

## Step 2: Configure MCP Gateway OAuth Environment

Configure the MCP Gateway broker to respond with OAuth discovery information:

```bash
kubectl set env deployment/mcp-gateway-broker-router \
  OAUTH_RESOURCE_NAME="MCP Server" \
  OAUTH_RESOURCE="http://mcp.127-0-0-1.sslip.io:8888/mcp" \
  OAUTH_AUTHORIZATION_SERVERS="http://keycloak.127-0-0-1.sslip.io:8889/realms/mcp" \
  OAUTH_BEARER_METHODS_SUPPORTED="header" \
  OAUTH_SCOPES_SUPPORTED="basic,groups" \
  -n mcp-system
```

**Environment Variables Explained:**

- `OAUTH_RESOURCE_NAME`: Human-readable name for this resource server
- `OAUTH_RESOURCE`: Canonical URI of the MCP server (used for token audience validation)  
- `OAUTH_AUTHORIZATION_SERVERS`: Authorization server URL for client discovery
- `OAUTH_BEARER_METHODS_SUPPORTED`: Supported bearer token methods (header, body, query)
- `OAUTH_SCOPES_SUPPORTED`: OAuth scopes this resource server understands

## Step 3: Configure AuthPolicy for Authentication

Apply the authentication policy that validates JWT tokens:

```bash
kubectl apply -f - <<EOF
apiVersion: kuadrant.io/v1
kind: AuthPolicy
metadata:
  name: mcp-auth-policy
  namespace: gateway-system
spec:
  targetRef:
    group: gateway.networking.k8s.io
    kind: Gateway
    name: mcp-gateway
    sectionName: mcp
  defaults:
    when:
      - predicate: "!request.path.contains('/.well-known')"
    rules:
      authentication:
        'keycloak':
          jwt:
            issuerUrl: http://keycloak.keycloak.svc.cluster.local/realms/mcp
      response:
        unauthenticated:
          code: 401
          headers:
            'WWW-Authenticate':
              value: Bearer resource_metadata=http://mcp.127-0-0-1.sslip.io:8888/.well-known/oauth-protected-resource/mcp
          body:
            value: |
              {
                "error": "Unauthorized",
                "message": "Authentication required."
              }
EOF
```

**Key Configuration Points:**

- **JWT Validation**: Validates tokens against Keycloak's OIDC issuer
- **Discovery Exclusion**: Allows unauthenticated access to `/.well-known` endpoints
- **WWW-Authenticate Header**: Points clients to OAuth discovery metadata
- **Standard Response**: Returns 401 with proper OAuth error format

## Step 4: Verify OAuth Discovery

Test that the broker now serves OAuth discovery information:

```bash
# Check the protected resource metadata endpoint
curl http://mcp.127-0-0-1.sslip.io:8888/.well-known/oauth-protected-resource

# Should return OAuth 2.0 Protected Resource Metadata like:
# {
#   "resource_name": "MCP Server",
#   "resource": "http://mcp.127-0-0-1.sslip.io:8888/mcp",
#   "authorization_servers": [
#     "http://keycloak.127-0-0-1.sslip.io:8889/realms/mcp"
#   ],
#   "bearer_methods_supported": [
#     "header"
#   ],
#   "scopes_supported": [
#     "basic",
#     "groups"
#   ]
# }
```

Test that protected endpoints now require authentication:

```bash
# This should return 401 with WWW-Authenticate header
curl -v http://mcp.127-0-0-1.sslip.io:8888/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```

You should get a response like this:

```bash
{
  "error": "Unauthorized",
  "message": "Authentication required."
}
```

## Step 5: Test Authentication Flow

Use the MCP Inspector to test the complete OAuth flow. You'll need to set up port forwarding to access the gateway through your local browser:

```bash
# Start port forwarding to the Istio gateway
kubectl -n gateway-system port-forward svc/mcp-gateway-istio 8888:8080 &
PORT_FORWARD_PID=$!

# Start MCP Inspector (requires Node.js/npm)
npx @modelcontextprotocol/inspector@latest &
INSPECTOR_PID=$!

# Wait a moment for services to start
sleep 3

# Open MCP Inspector with the gateway URL
open "http://localhost:6274/?transport=streamable-http&serverUrl=http://mcp.127-0-0-1.sslip.io:8888/mcp"
```

**What this does:**
- **Port Forwarding**: Makes the Istio gateway accessible on localhost:8888
- **MCP Inspector**: Launches the official MCP debugging tool
- **Auto-Configuration**: Pre-configures the inspector to connect to your gateway

**To stop the services later:**
```bash
kill $PORT_FORWARD_PID $INSPECTOR_PID
```

The MCP Inspector will:
1. Detect the 401 response and WWW-Authenticate header
2. Retrieve authorization server metadata from `/.well-known/oauth-protected-resource`
3. Perform dynamic client registration (if supported)
4. Redirect to Keycloak for user authentication
5. Exchange authorization code for access token
6. Use the access token for subsequent MCP requests

**Test Credentials**: `mcp` / `mcp`

## Alternative Authentication Methods

While this guide uses Kuadrant AuthPolicy with Keycloak, MCP Gateway supports any Istio/Gateway API compatible authentication mechanism including other identity providers and authentication methods.

## Next Steps

With authentication configured, you can proceed to:
- **[Authorization Configuration](./authorization.md)** - Control which users can access specific tools
- **[External MCP Servers](./external-mcp-server.md)** - Connect authenticated external services
