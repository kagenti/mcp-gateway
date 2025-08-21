# Inspection & URLs

# URLs for services
urls-impl:
	@echo "=== MCP Gateway URLs ==="
	@echo ""
	@echo "Gateway (via port-forward):"
	@echo "  http://localhost:8888"
	@echo "  Host header required: mcp.example.com"
	@echo ""
	@echo "Local Services:"
	@echo "  Broker: http://localhost:8080"
	@echo "  Router: grpc://localhost:9002"
	@echo ""
	@echo "Mock MCP Server (via port-forward):"
	@echo "  http://localhost:8081/mcp"
	@echo ""
	@echo "Test commands:"
	@echo "  curl -H 'Host: mcp.example.com' http://localhost:8888/"
	@echo "  curl http://localhost:8080/"

# Open MCP Inspector for the broker
.PHONY: inspect-broker
inspect-broker: ## Open MCP Inspector for local broker
	@echo "Opening MCP Inspector for broker at http://localhost:8080/mcp"
	@echo ""
	@npx @modelcontextprotocol/inspector http://localhost:8080/mcp

# Open MCP Inspector for mock server implementation
inspect-mock-impl:
	@echo "Ensuring port-forward to mock server..."
	@-pkill -f "kubectl.*port-forward.*mcp-test" || true
	@kubectl -n mcp-server port-forward svc/mcp-test 8081:8081 > /dev/null 2>&1 &
	@sleep 2
	@echo "Opening MCP Inspector for mock server at http://localhost:8081/mcp"
	@echo ""
	@npx @modelcontextprotocol/inspector http://localhost:8081/mcp

# Open MCP Inspector for gateway (broker via gateway)
.PHONY: inspect-gateway
inspect-gateway: # Open MCP Inspector for broker via gateway
	@echo "Ensuring port-forward to gateway..."
	@-pkill -f "kubectl.*port-forward.*mcp-gateway" || true
	@kubectl -n gateway-system port-forward svc/mcp-gateway-istio 8888:80 > /dev/null 2>&1 &
	@sleep 2
	@echo "Opening MCP Inspector for gateway at http://localhost:8888/mcp"
	@echo "Note: This connects to the broker through the full gateway stack"
	@echo ""
	@npx @modelcontextprotocol/inspector http://localhost:8888/mcp --header "Host: mcp.example.com"

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
