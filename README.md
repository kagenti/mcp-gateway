# mcp-gateway

An Envoy-based MCP Gateway

## Quick Start

```bash
# Setup (automatically installs required tools to ./bin/)
make local-env-setup    # Create a Kind cluster with Istio, Gateway API, MetalLB, Keycloak, and Kuadrant

# Local Development (Note: MCP implementation is incomplete - expect 500 errors)
make dev                # Configure cluster to use local services
make run-router         # Run router locally (ext_proc on port 50051, debug HTTP on port 9002)
make run-broker         # Run broker locally (port 8080)
make dev-gateway-forward # Forward gateway to localhost:8888

# Inspection
make urls               # Show all service URLs
make status             # Show status of all components
make logs               # Tail Istio gateway logs
make debug-envoy        # Enable debug logging

# Cleanup
make dev-stop          # Stop local processes
make local-env-teardown # Destroy cluster
```

Run `make help` to see all available commands.
