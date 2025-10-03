#!/bin/bash

# Sample local Helm setup script for MCP Gateway
# This script sets up a complete MCP Gateway environment using remote resources

set -e

# Allow specifying a different GitHub org/user and branch via environment variables
GITHUB_ORG=${MCP_GATEWAY_ORG:-kagenti}
BRANCH=${MCP_GATEWAY_BRANCH:-main}
echo "Using GitHub org: $GITHUB_ORG"
echo "Using branch: $BRANCH"

echo "Setting up MCP Gateway using Helm chart..."

# Create Kind cluster with inline configuration
echo "Creating Kind cluster..."
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 8080
    protocol: TCP
  - containerPort: 443
    hostPort: 8443
    protocol: TCP
EOF

kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml

helm repo add istio https://istio-release.storage.googleapis.com/charts
helm repo update
helm install istio-base istio/base -n istio-system --create-namespace --wait
helm install istiod istio/istiod -n istio-system --wait

kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/istio/gateway/namespace.yaml
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/istio/gateway/gateway.yaml -n gateway-system
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/test-servers/namespace.yaml
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/test-servers/server1-deployment.yaml -n mcp-test
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/test-servers/server1-service.yaml -n mcp-test
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/test-servers/server1-httproute.yaml -n mcp-test
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/test-servers/server1-httproute-ext.yaml -n mcp-test
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/test-servers/server2-deployment.yaml -n mcp-test
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/test-servers/server2-service.yaml -n mcp-test
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/test-servers/server2-httproute.yaml -n mcp-test
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/test-servers/server2-httproute-ext.yaml -n mcp-test

helm install mcp-gateway oci://ghcr.io/kagenti/charts/mcp-gateway
kubectl apply -f https://raw.githubusercontent.com/$GITHUB_ORG/mcp-gateway/$BRANCH/config/samples/mcpserver-test-servers.yaml

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

