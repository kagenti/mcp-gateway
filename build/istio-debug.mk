# Istio Debugging

# Show all clusters registered in the gateway
istio-clusters-impl: $(ISTIOCTL)
	@GATEWAY_POD=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.name}' 2>/dev/null); \
	GATEWAY_NS=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.namespace}' 2>/dev/null); \
	if [ -z "$$GATEWAY_POD" ]; then \
		echo "Error: No Istio gateway pod found"; \
		exit 1; \
	fi; \
	echo "Clusters registered in $$GATEWAY_POD:"; \
	bin/istioctl proxy-config cluster $$GATEWAY_POD.$$GATEWAY_NS

# Show listeners in the gateway
.PHONY: istio-listeners
istio-listeners: $(ISTIOCTL) # Show all listeners in the gateway
	@GATEWAY_POD=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.name}' 2>/dev/null); \
	GATEWAY_NS=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.namespace}' 2>/dev/null); \
	if [ -z "$$GATEWAY_POD" ]; then \
		echo "Error: No Istio gateway pod found"; \
		exit 1; \
	fi; \
	echo "Listeners in $$GATEWAY_POD:"; \
	bin/istioctl proxy-config listener $$GATEWAY_POD.$$GATEWAY_NS

# Show routes in the gateway
.PHONY: istio-routes
istio-routes: $(ISTIOCTL) # Show all routes in the gateway
	@GATEWAY_POD=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.name}' 2>/dev/null); \
	GATEWAY_NS=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.namespace}' 2>/dev/null); \
	if [ -z "$$GATEWAY_POD" ]; then \
		echo "Error: No Istio gateway pod found"; \
		exit 1; \
	fi; \
	echo "Routes in $$GATEWAY_POD:"; \
	bin/istioctl proxy-config route $$GATEWAY_POD.$$GATEWAY_NS

# Show endpoints in the gateway
.PHONY: istio-endpoints
istio-endpoints: $(ISTIOCTL) # Show all endpoints in the gateway
	@GATEWAY_POD=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.name}' 2>/dev/null); \
	GATEWAY_NS=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.namespace}' 2>/dev/null); \
	if [ -z "$$GATEWAY_POD" ]; then \
		echo "Error: No Istio gateway pod found"; \
		exit 1; \
	fi; \
	echo "Endpoints in $$GATEWAY_POD:"; \
	bin/istioctl proxy-config endpoint $$GATEWAY_POD.$$GATEWAY_NS

# Show all proxy configs at once
istio-config-impl: $(ISTIOCTL)
	@GATEWAY_POD=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.name}' 2>/dev/null); \
	GATEWAY_NS=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.namespace}' 2>/dev/null); \
	if [ -z "$$GATEWAY_POD" ]; then \
		echo "Error: No Istio gateway pod found"; \
		exit 1; \
	fi; \
	echo "All proxy configs for $$GATEWAY_POD:"; \
	bin/istioctl proxy-config all $$GATEWAY_POD.$$GATEWAY_NS

# Analyze Istio configuration for issues
.PHONY: istio-analyze
istio-analyze: $(ISTIOCTL) # Analyze Istio configuration for potential issues
	bin/istioctl analyze --all-namespaces

# Show Istio dashboard
.PHONY: istio-dashboard
istio-dashboard: $(ISTIOCTL) # Open Istio dashboard (Kiali)
	@echo "Opening Istio dashboard..."
	@echo "If Kiali is not installed, install it with: istioctl dashboard kiali"
	bin/istioctl dashboard envoy $$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.name}').$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.namespace}')

# Check external services (like host.docker.internal)
.PHONY: istio-external
istio-external: $(ISTIOCTL) # Show external services configuration
	@echo "External services (ServiceEntry resources):"
	@kubectl get serviceentry -A
	@echo ""
	@echo "Checking for host.docker.internal cluster:"
	@GATEWAY_POD=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.name}' 2>/dev/null); \
	GATEWAY_NS=$$(kubectl get pods -A -l istio=ingressgateway -o jsonpath='{.items[0].metadata.namespace}' 2>/dev/null); \
	bin/istioctl proxy-config cluster $$GATEWAY_POD.$$GATEWAY_NS | grep -E "(host.docker|9002|8080)" || echo "No host.docker.internal clusters found"
