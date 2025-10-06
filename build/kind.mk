# Kind

KIND_CLUSTER_NAME ?= mcp-gateway

.PHONY: kind-create-cluster
kind-create-cluster: kind # Create the "mcp-gateway" kind cluster.
	@if $(KIND) get clusters | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' already exists, skipping creation"; \
	else \
		echo "Creating Kind cluster '$(KIND_CLUSTER_NAME)' with HTTP port $(KIND_HOST_PORT_HTTP) and HTTPS port $(KIND_HOST_PORT_HTTPS)..."; \
		cat config/kind/cluster.yaml | sed \
			-e 's/hostPort: 8080/hostPort: $(KIND_HOST_PORT_HTTP)/' \
			-e 's/hostPort: 8443/hostPort: $(KIND_HOST_PORT_HTTPS)/' | \
		$(KIND) create cluster --name $(KIND_CLUSTER_NAME) --config -; \
	fi

.PHONY: kind-delete-cluster
kind-delete-cluster: kind # Delete the "mcp-gateway" kind cluster.
	- $(KIND) delete cluster --name $(KIND_CLUSTER_NAME)
