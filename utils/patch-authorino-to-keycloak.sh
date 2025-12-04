#!/bin/bash

# Patches the Authorino deployment to resolve Keycloak's external test domain name to the MCP gateway IP
# and accept its TLS certificate – used for demos, do not use in production

echo "Patching Authorino to trust Keycloak's TLS certificate..."

kubectl create configmap mcp-gateway-keycloak-cert -n kuadrant-system --from-file=keycloak.crt=./out/certs/ca.crt

kubectl patch authorino authorino -n kuadrant-system --type merge -p '
{
  "spec": {
    "volumes": {
      "items": [
        {
          "name": "keycloak-cert",
          "mountPath": "/etc/ssl/certs",
          "configMaps": [
            "mcp-gateway-keycloak-cert"
          ],
          "items": [
            {
              "key": "keycloak.crt",
              "path": "keycloak.crt"
            }
          ]
        }
      ]
    }
  }
}'

echo "Patching Authorino deployment to resolve Keycloak's host name to MCP gateway IP..."

export GATEWAY_IP=$(kubectl get gateway/mcp-gateway -n gateway-system -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)

if [[ -z "$GATEWAY_IP" ]]; then
  echo "Error: could not determine mcp-gateway IP address. Is the gateway installed and running?" >&2
  exit 1
fi

kubectl patch deployment authorino -n kuadrant-system --type='json' -p="$(cat config/keycloak/patch-hostaliases.json | envsubst)"

kubectl wait --for=condition=available --timeout=90s deployment/authorino -n kuadrant-system
