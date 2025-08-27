# mcp-gateway

An Envoy-based MCP Gateway

## Quick Start

```bash
# Setup (automatically installs required tools to ./bin/)
make local-env-setup        # Create Kind cluster with Istio, Gateway API, MetalLB, Keycloak, and Kuadrant

# Local Development (Note: MCP implementation is incomplete - expect 500 errors)
make dev                    # Configure cluster to use local services
make run-mcp-broker-router  # Run combined broker/router (broker on :8080, ext_proc on :50051)
make dev-gateway-forward    # Forward gateway to localhost:8888

# Inspection & Debugging
make info                   # Show setup info and useful commands
make urls                   # Show all service URLs
make status                 # Check component status
make logs                   # Tail gateway logs
make debug-envoy            # Enable debug logging
make inspect-mock           # Open MCP Inspector for mock server

# Services
make keycloak-forward       # Access Keycloak at localhost:8095
make kuadrant-status        # Check Kuadrant operator status

# Cleanup
make dev-stop               # Stop local processes
make local-env-teardown     # Destroy cluster
```

Run `make help` to see all available commands.
