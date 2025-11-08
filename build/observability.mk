
##@ Observability Setup

.PHONY: observability-setup
observability-setup: ## Setup observability environment with Loki and Perses
	@echo "========================================="
	@echo "Setting up Observability Environment"
	@echo "========================================="
	@echo "Step 1/4: Deploying Loki..."
	@kubectl apply -f https://raw.githubusercontent.com/grafana/loki/main/production/kubernetes/loki-simple-scalable.yaml
	@echo "✅ Loki deployed"
	@echo ""
	@echo "Step 2/4: Configuring mcp-router to send logs to Loki..."
	@kubectl set env deployment/mcp-broker-router \
		LOGGING_BACKEND="loki" \
		LOKI_URL="http://loki:3100" \
		-n mcp-system
	@echo "✅ mcp-router configured to send logs to Loki"
	@echo ""
	@echo "Step 3/4: Deploying Perses dashboard..."
	@kubectl apply -f https://raw.githubusercontent.com/perses/perses/main/deploy/perses.yaml
	@kubectl apply -f ./config/perses/mcp-router-dashboard.yaml
	@echo "✅ Perses dashboard deployed"
	@echo ""
	@echo "Step 4/4: Verifying setup..."
	@kubectl get pods -n mcp-system
	@echo "✅ Observability environment setup complete"
	@echo ""
	@echo "🎉 You can now view logs in Loki and the dashboard in Perses!"
	@echo "Loki URL: http://<LOKI_IP>:3100"
	@echo "Perses Dashboard URL: http://<PERSES_IP>:8080"
