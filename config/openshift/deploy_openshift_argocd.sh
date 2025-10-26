#!/bin/bash

set -e

SCRIPT_BASE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

MCP_GATEWAY_HOST="${MCP_GATEWAY_HOST:-mcp.apps.$(oc get dns cluster -o jsonpath='{.spec.baseDomain}')}"

# Check if OpenShift cli tool is installed
command -v oc >/dev/null 2>&1 || { echo >&2 "OpenShift CLI is required but not installed.  Aborting."; exit 1; } 


echo "Deploying MCP Gateway and dependent services using Argo CD..."
oc apply -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: mcp-gateway-argocd
  namespace: openshift-gitops
spec:
  destination:
    namespace: openshift-gitops
    server: 'https://kubernetes.default.svc'
  project: default
  source:
    path: config/openshift/kustomize/argocd/apps
    repoURL: 'https://github.com/kagenti/mcp-gateway.git'
    targetRevision: main
    kustomize:
      patches:
        - patch: |-
            - op: replace
              path: /spec/source/helm
              value:
                parameters:
                  - name: mcpGateway.host
                    value: "$MCP_GATEWAY_HOST"
          target:
            kind: Application
            name: mcp-gateway-ingress
  syncPolicy:
    automated:
      enabled: true
      selfHeal: true
EOF

echo
echo "MCP Gateway deployment using Argo CD completed successfully."
echo "Access the MCP Gateway at: https://$MCP_GATEWAY_HOST/mcp"
echo
