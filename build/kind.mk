# Kind

KIND_CLUSTER_NAME ?= mcp-gateway

.PHONY: kind-create-cluster
kind-create-cluster: kind # Create the "mcp-gateway" kind cluster.
	$(KIND) create cluster --name $(KIND_CLUSTER_NAME) --config config/kind/cluster.yaml

.PHONY: kind-delete-cluster
kind-delete-cluster: kind # Delete the "mcp-gateway" kind cluster.
	- $(KIND) delete cluster --name $(KIND_CLUSTER_NAME)
