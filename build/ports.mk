# Port configuration for MCP Gateway
# These variables control both Kind cluster ports and local port forwarding

# Kind cluster host ports (what gets exposed on your local machine from Kind)
KIND_HOST_PORT_HTTP ?= 8080
KIND_HOST_PORT_HTTPS ?= 8443

# Local port forwarding ports (for accessing services via kubectl port-forward)
# Gateway ports
GATEWAY_LOCAL_PORT_HTTP ?= 8888
GATEWAY_LOCAL_PORT_HTTPS ?= 8889

# Export for use in shell commands
export KIND_HOST_PORT_HTTP
export KIND_HOST_PORT_HTTPS
export GATEWAY_LOCAL_PORT_HTTP
export GATEWAY_LOCAL_PORT_HTTPS
