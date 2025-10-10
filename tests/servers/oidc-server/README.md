# MCP OIDC Test Server

Minimal MCP server for testing OpenID Connect (OIDC) authentication in MCP Gateway.

## Features
- Validates Authorization headers for valid Bearer tokens issued by a configured OIDC provider
- Returns 401 for invalid/missing credentials
- Single `hello_world` tool for testing authenticated access

## Configuration
- `PORT`: Server port (default: 9090)
- `ISSUER_URL`: Issuer URL of the OpenID Connect provider
- `EXPECTED_AUD`: Expected `aud` claim of the ID tokens
- `EXPECTED_ADMIN_TOKEN`: (Optional) API key for admin (backdoor) access. If omitted or set to an empty, disables this authentication method.

## Usage
Deploy in Kubernetes pointing to an auth server that implements OpenID Connect (see `ISSUER_URL` config).

### Authentication

#### OIDC authentication (default)

Requests must send `Authorization: Bearer <valid-token-issued-by-the-oidc-provider>`.

#### API key authentication (for admin/backdoor access)

If `EXPECTED_ADMIN_TOKEN` is configured to a non-empty value, requests can opt for this method to authenticate by sending `Authorization: APIKEY ${EXPECTED_ADMIN_TOKEN}`.

This can be an option for control-plane actors (e.g. MCP gateway) to access the server if otherwise unable to obtain a short-lived bearer token with the trusted OIDC server.
