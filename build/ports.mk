# Port configuration for MCP Gateway
# These variables control both Kind cluster ports and local port forwarding

# Kind cluster host ports (what gets exposed on your local machine from Kind)
KIND_HOST_PORT_MCP_GATEWAY ?= 8001
KIND_HOST_PORT_KEYCLOAK ?= 8002

# Export for use in shell commands
export KIND_HOST_PORT_MCP_GATEWAY
export KIND_HOST_PORT_KEYCLOAK
