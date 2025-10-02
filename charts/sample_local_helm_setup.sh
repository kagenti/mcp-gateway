#!/bin/bash

# Sample local Helm setup script for MCP Gateway
# This script can be run from any directory - it automatically resolves paths
# relative to the repository root.

set -e

# Get the directory of this script and the repository root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Setting up MCP Gateway using Helm chart..."
echo "Repository root: $REPO_ROOT"

kind create cluster --config "$REPO_ROOT/config/kind/cluster.yaml"

kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml

helm repo add istio https://istio-release.storage.googleapis.com/charts
helm repo update
helm install istio-base istio/base -n istio-system --create-namespace --wait
helm install istiod istio/istiod -n istio-system --wait

kubectl apply -k "$REPO_ROOT/config/istio/gateway"
kubectl apply -k "$REPO_ROOT/config/test-servers"

helm install mcp-gateway "$REPO_ROOT/charts/mcp-gateway"
kubectl apply -f "$REPO_ROOT/config/samples/mcpserver-test-servers.yaml"

cat <<EOF | kubectl apply -f -
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: mcp-route
  namespace: mcp-system
spec:
  parentRefs:
    - name: mcp-gateway
      namespace: gateway-system
  hostnames:
    - 'mcp.127-0-0-1.sslip.io'
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /mcp
      filters:
        - type: ResponseHeaderModifier
          responseHeaderModifier:
            add:
              - name: Access-Control-Allow-Origin
                value: "*"
              - name: Access-Control-Allow-Methods
                value: "GET, POST, PUT, DELETE, OPTIONS, HEAD"
              - name: Access-Control-Allow-Headers
                value: "Content-Type, Authorization, Accept, Origin, X-Requested-With"
              - name: Access-Control-Max-Age
                value: "3600"
              - name: Access-Control-Allow-Credentials
                value: "true"
      backendRefs:
        - name: mcp-gateway-broker
          port: 8080
    - matches:
        - path:
            type: PathPrefix
            value: /.well-known/oauth-protected-resource
      backendRefs:
        - name: mcp-gateway-broker
          port: 8080
EOF

echo "Waiting for MCP Gateway pods to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/mcp-gateway-broker-router -n mcp-system
kubectl wait --for=condition=available --timeout=300s deployment/mcp-gateway-controller -n mcp-system

echo "Waiting for Istio gateway pod to be ready..."
kubectl wait --for=condition=ready --timeout=300s pod -l istio=ingressgateway -n gateway-system

echo "Starting port forwarding..."
kubectl -n gateway-system port-forward svc/mcp-gateway-istio 8888:8080 8889:8081 &
PORT_FORWARD_PID=$!

echo "Starting MCP inspector..."
MCP_AUTO_OPEN_ENABLED=false DANGEROUSLY_OMIT_AUTH=true npx @modelcontextprotocol/inspector@latest &
INSPECTOR_PID=$!

sleep 3

echo "================================================================"
echo "Setup complete! 🎉"
echo "================================================================"
echo "Port forwarding: kubectl port-forward active (PID: $PORT_FORWARD_PID)"
echo "MCP Inspector: http://localhost:6274"
echo "Gateway URL: http://mcp.127-0-0-1.sslip.io:8888/mcp"
echo ""
echo "Check status:"
echo "  kubectl get pods -n mcp-system"
echo "  kubectl get pods -n istio-system"
echo "  kubectl get httproute -n mcp-system"
echo ""
echo "Press Ctrl+C to stop port forwarding and cleanup."
echo "================================================================"

open "http://localhost:6274/?transport=streamable-http&serverUrl=http://mcp.127-0-0-1.sslip.io:8888/mcp" 2>/dev/null || echo "Open manually: http://localhost:6274/?transport=streamable-http&serverUrl=http://mcp.127-0-0-1.sslip.io:8888/mcp"

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    kill $PORT_FORWARD_PID 2>/dev/null || true
    kill $INSPECTOR_PID 2>/dev/null || true
    exit 0
}

trap cleanup SIGINT SIGTERM
wait

