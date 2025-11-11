#!/bin/bash

set -e

MCP_GATEWAY_HELM_VERSION="${MCP_GATEWAY_HELM_VERSION:-0.4.0}"

MCP_GATEWAY_HOST="${MCP_GATEWAY_HOST:-mcp.apps.$(oc get dns cluster -o jsonpath='{.spec.baseDomain}')}"

MCP_GATEWAY_NAMESPACE="${MCP_GATEWAY_NAMESPACE:-mcp-system}"
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-gateway-system}"

SCRIPT_BASE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

# Check if OpenShift cli tool is installed
command -v oc >/dev/null 2>&1 || { echo >&2 "OpenShift CLI is required but not installed.  Aborting."; exit 1; } 

# Check if Helm is installed
command -v helm >/dev/null 2>&1 || { echo >&2 "Helm is required but not installed.  Aborting."; exit 1; } 

# Install Service Mesh Operator
echo "Installing Service Mesh Operator..."
oc apply -k "$SCRIPT_BASE_DIR/kustomize/service-mesh/operator/base"

# Wait for the Service Mesh Operator to be ready
echo "Waiting for Service Mesh Operator to be ready..."
until kubectl wait crd/istios.sailoperator.io --for condition=established &>/dev/null; do sleep 5; done
until kubectl wait crd/istiocnis.sailoperator.io --for condition=established &>/dev/null; do sleep 5; done

# Install Service Mesh Instance
echo "Installing Service Mesh Instance..."
oc apply -k "$SCRIPT_BASE_DIR/kustomize/service-mesh/instance/base"

# Install Connectivity Link Operator
echo "Installing Connectivity Link Operator..."
oc apply -k "$SCRIPT_BASE_DIR/kustomize/connectivity-link/operator/base"

# Wait for the Connectivity Link Operator to be ready
echo "Waiting for Connectivity Link Operator to be ready..."
until kubectl wait crd/kuadrants.kuadrant.io --for condition=established &>/dev/null; do sleep 5; done

# Install Connectivity Link Instance
echo "Installing Connectivity Link Instance..."
oc apply -k "$SCRIPT_BASE_DIR/kustomize/connectivity-link/instance/base"

# Install MCP Gateway using Helm
echo "Installing MCP Gateway using Helm..."
helm upgrade -i mcp-gateway -n $MCP_GATEWAY_NAMESPACE --create-namespace oci://ghcr.io/kagenti/charts/mcp-gateway --version $MCP_GATEWAY_HELM_VERSION

# Configure MCP Gateway Ingress using Helm
echo "Configuring MCP Gateway Ingress using Helm..."
helm upgrade -i mcp-gateway-ingress -n $GATEWAY_NAMESPACE --create-namespace "$SCRIPT_BASE_DIR/charts/mcp-gateway-ingress" \
  --set mcpGateway.host="$MCP_GATEWAY_HOST"

echo
echo "MCP Gateway deployment completed successfully."
echo "Access the MCP Gateway at: https://$MCP_GATEWAY_HOST/mcp"
echo
