# Troubleshooting MCP Gateway

This guide covers common issues and solutions when working with MCP Gateway.

## Gateway Not Starting

**Check port availability:**

```bash
# Linux/Mac
lsof -i :8080
lsof -i :50051
```

**Verify configuration syntax:**

```bash
# Kubernetes
kubectl get mcpserver -A
kubectl describe mcpserver <name> -n <namespace>

# Standalone
cat config/samples/config.yaml
```

## Backend Servers Not Discovered

**Check controller logs:**

```bash
kubectl logs -n mcp-system deployment/mcp-controller
```

**Verify HTTPRoute exists:**

```bash
kubectl get httproute -A
kubectl describe httproute <route-name> -n <namespace>
```

**Check RBAC permissions:**

```bash
kubectl get clusterrole mcp-controller-role
kubectl get clusterrolebinding mcp-controller-rolebinding
```

## Tools Not Appearing

**Check broker logs:**

```bash
kubectl logs -n mcp-system deployment/mcp-broker-router -c broker | grep "Discovered tools"
```

**Verify backend server is reachable:**

```bash
# From within the cluster
kubectl run -it --rm debug --image=nicolaka/netshoot --restart=Never -- \
  curl http://<backend-service>.<namespace>.svc.cluster.local:<port>/health
```
