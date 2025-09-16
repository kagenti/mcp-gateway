# Inspection & URLs

# URLs for services
urls-impl:
	@echo "=== MCP Gateway URLs ==="
	@echo ""
	@echo "Gateway (via port-forward):"
	@echo "  http://mcp.127-0-0-1.sslip.io:8888"
	@echo ""
	@echo "Local Services:"
	@echo "  Broker: http://localhost:8080"
	@echo "  Router: grpc://localhost:9002"
	@echo ""
	@echo "Mock MCP Server (via port-forward):"
	@echo "  http://localhost:8081/mcp"
	@echo ""
	@echo "Test commands:"
	@echo "  curl http://mcp.127-0-0-1.sslip.io:8888/"
	@echo "  curl http://localhost:8080/"

# Deprecated - use inspect-gateway instead
.PHONY: inspect-broker
inspect-broker: inspect-gateway

# Generic template for inspecting MCP servers
# Args: $(1) = server name, $(2) = service name, $(3) = local port, $(4) = tools description, $(5) = extra notes
define inspect-server-template
	@echo "Setting up port-forward to $(1)..."
	@kubectl -n mcp-test port-forward svc/$(2) $(3):9090 > /dev/null 2>&1 & \
		PF_PID=$$$$!; \
		trap "echo '\nCleaning up...'; kill $$$$PF_PID 2>/dev/null || true; exit" INT TERM; \
		sleep 2; \
		echo "Opening MCP Inspector for $(1) at http://localhost:$(3)/mcp"; \
		echo "Available tools: $(4)"; \
		$(if $(5),echo "$(5)";) \
		echo ""; \
		MCP_AUTO_OPEN_ENABLED=false DANGEROUSLY_OMIT_AUTH=true npx @modelcontextprotocol/inspector@latest & \
		sleep 2; \
		open "http://localhost:6274/?transport=streamable-http&serverUrl=http://localhost:$(3)/mcp"; \
		echo "Press Ctrl+C to stop and cleanup"; \
		wait; \
		kill $$$$PF_PID 2>/dev/null || true
endef

.PHONY: inspect-server1
inspect-server1: ## Open MCP Inspector for test server 1
	$(call inspect-server-template,test server 1,mcp-test-server1,9090,hi time slow headers)

.PHONY: inspect-server2
inspect-server2: ## Open MCP Inspector for test server 2
	$(call inspect-server-template,test server 2,mcp-test-server2,9091,similar to server1 different implementation)

.PHONY: inspect-server3
inspect-server3: ## Open MCP Inspector for test server 3
	$(call inspect-server-template,test server 3,mcp-test-server3,9092,time add dozen pi get_weather slow)

.PHONY: inspect-server4
inspect-server4: ## Open MCP Inspector for test server 4 (requires auth)
	$(call inspect-server-template,test server 4,mcp-test-server4,9093,similar to server2 but requires authentication,NOTE: This server requires Bearer token authentication)

# Legacy alias for compatibility
inspect-mock-impl: inspect-server1

# Open MCP Inspector for gateway (broker via gateway)
.PHONY: inspect-gateway
inspect-gateway: ## Open MCP Inspector for the gateway
	@echo "Setting up port-forward to gateway..."
	@-pkill -f "kubectl.*port-forward.*mcp-gateway-istio" || true
	@kubectl -n gateway-system port-forward svc/mcp-gateway-istio 8888:8080 8889:8081 > /dev/null 2>&1 & \
		PF_PID=$$!; \
		trap "echo '\nCleaning up...'; kill $$PF_PID 2>/dev/null || true; exit" INT TERM; \
		sleep 2; \
		echo "Opening MCP Inspector for gateway"; \
		echo "URL: http://mcp.127-0-0-1.sslip.io:8888/mcp"; \
		echo ""; \
		MCP_AUTO_OPEN_ENABLED=false DANGEROUSLY_OMIT_AUTH=true npx @modelcontextprotocol/inspector@latest & \
		sleep 2; \
		open "http://localhost:6274/?transport=streamable-http&serverUrl=http://mcp.127-0-0-1.sslip.io:8888/mcp"; \
		echo "Press Ctrl+C to stop and cleanup"; \
		wait; \
		kill $$PF_PID 2>/dev/null || true

# Show status of all MCP components implementation
status-impl:
	@echo "=== Cluster Components ==="
	@kubectl get pods -n istio-system | grep -E "(istiod|sail)" || echo "Istio: Not found"
	@kubectl get pods -n gateway-system | grep gateway || echo "Gateway: Not found"
	@kubectl get pods -n mcp-system 2>/dev/null || echo "MCP System: No pods"
	@kubectl get pods -n mcp-server 2>/dev/null || echo "Mock MCP: No pods"
	@echo ""
	@echo "=== Local Processes ==="
	@lsof -i :8080 | grep LISTEN | head -1 || echo "Broker: Not running (port 8080)"
	@lsof -i :9002 | grep LISTEN | head -1 || echo "Router: Not running (port 9002)"
	@echo ""
	@echo "=== Port Forwards ==="
	@ps aux | grep -E "kubectl.*port-forward" | grep -v grep || echo "No active port-forwards"
