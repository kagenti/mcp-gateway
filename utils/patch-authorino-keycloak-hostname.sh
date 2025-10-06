#!/bin/bash

# Hacks the internal DNS of the Authorino deployment to resolve Keycloak's external test domain name to the IP of MCP gateway – used for demos, do not use in production

gateway_ip=$(kubectl get gateway/mcp-gateway -n gateway-system -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)

if [[ -z "$gateway_ip" ]]; then
  echo "Error: could not determine mcp-gateway IP address. Is the gateway installed and running?" >&2
  exit 1
fi

kubectl patch deployment authorino -n kuadrant-system \
  --type='json' \
  -p="[$(
    cat <<EOF
{
  "op": "add",
  "path": "/spec/template/spec/hostAliases",
  "value": [
    {
      "ip": "$gateway_ip",
      "hostnames": [
        "keycloak.127-0-0-1.sslip.io"
      ]
    }
  ]
}
EOF
  )]"

kubectl wait --for=condition=available --timeout=90s deployment/authorino -n kuadrant-system
