# Local Development

# Update EnvoyFilter to point to local router
.PHONY: dev-envoyfilter
dev-envoyfilter: # Configure EnvoyFilter to use local MCP router
	@echo "Updating EnvoyFilter to use host.docker.internal:50051..."
	kubectl apply -f config/dev/envoyfilter.yaml
	kubectl apply -f config/dev/serviceentry.yaml

# Create a service that points to host for broker
.PHONY: dev-broker-service
dev-broker-service: # Create service pointing to local MCP broker
	@echo "Creating service pointing to host.docker.internal:8080..."
	kubectl apply -f config/dev/broker-service.yaml

# Setup for local development
.PHONY: dev-setup
dev-setup: dev-envoyfilter dev-broker-service # Configure cluster to use local services
	@echo "Cluster configured for local development!"
	@echo "Now run:"
	@echo "  make run-router   # In one terminal"
	@echo "  make run-broker   # In another terminal"

# Reset to in-cluster configuration
.PHONY: dev-reset
dev-reset: # Reset to in-cluster service configuration
	kubectl apply -f config/istio/envoyfilter.yaml
	@echo "Reset to in-cluster configuration"

# Port forward to access the gateway locally
.PHONY: dev-gateway-forward
dev-gateway-forward: ## Port forward the gateway to localhost:8888
	@echo "Forwarding gateway to localhost:8888..."
	@echo "You can now access the gateway at http://localhost:8888"
	@echo "Try: curl -H 'Host: mcp.example.com' http://localhost:8888"
	kubectl -n gateway-system port-forward svc/mcp-gateway-istio 8888:80

# Watch logs from the gateway
.PHONY: dev-logs-gateway
dev-logs-gateway: # Watch logs from the Istio gateway
	kubectl -n gateway-system logs -f deployment/mcp-gateway-istio

# Test the MCP flow
.PHONY: dev-test
dev-test: # Test MCP request through the gateway
	@echo "Testing MCP request through gateway..."
	curl -X POST \
		-H "Host: mcp.example.com" \
		-H "Content-Type: application/json" \
		-d '{"jsonrpc":"2.0","method":"initialize","params":{},"id":1}' \
		http://localhost:8888/mcp

# Clean up port forwards
.PHONY: dev-stop-forward
dev-stop-forward: # Stop any kubectl port-forward processes
	@echo "Stopping kubectl port-forward processes..."
	-pkill -f "kubectl.*port-forward" || true
	@echo "Port forwards stopped"

# Stop all local development processes
.PHONY: dev-stop
dev-stop: dev-stop-forward # Stop all local dev processes (port-forwards, router, broker)
	@echo "Stopping local mcp-router and mcp-broker..."
	-pkill -f "mcp-router" || true
	-pkill -f "mcp-broker" || true
	@echo "All local development processes stopped"
