# Kind

KIND_CLUSTER_NAME ?= mcp-gateway

.PHONY: kind-create-cluster
kind-create-cluster: kind # Create the "mcp-gateway" kind cluster.
	@./utils/generate-placeholder-ca.sh
	@# Set KIND provider for podman
	@if echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	if $(KIND) get clusters | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' already exists, skipping creation"; \
	else \
		echo "Creating Kind cluster '$(KIND_CLUSTER_NAME)' with MCP_GATEWAY port $(KIND_HOST_PORT_MCP_GATEWAY) and KEYCLOAK port $(KIND_HOST_PORT_KEYCLOAK)..."; \
		cat config/kind/cluster.yaml | sed \
			-e 's/hostPort: 8001/hostPort: $(KIND_HOST_PORT_MCP_GATEWAY)/' \
			-e 's/hostPort: 8002/hostPort: $(KIND_HOST_PORT_KEYCLOAK)/' | \
		$(KIND) create cluster --name $(KIND_CLUSTER_NAME) --config -; \
	fi

.PHONY: kind-delete-cluster
kind-delete-cluster: kind # Delete the "mcp-gateway" kind cluster.
	@# Set KIND provider for podman
	@if echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)
