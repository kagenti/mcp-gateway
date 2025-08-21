# Deploy

.PHONY: deploy-gateway
deploy-gateway: $(KUSTOMIZE) # Deploy the MCP gateway with Istio
	$(KUSTOMIZE) build config/istio/gateway | kubectl apply -f -
	kubectl apply -f config/istio/envoyfilter.yaml

.PHONY: undeploy-gateway
undeploy-gateway: $(KUSTOMIZE) # Remove the MCP gateway
	- $(KUSTOMIZE) build config/istio/gateway | kubectl delete -f -
	- kubectl delete -f config/istio/envoyfilter.yaml

.PHONY: deploy-namespaces
deploy-namespaces: # Create MCP namespaces
	kubectl apply -f config/mcp-system/namespace.yaml
	kubectl apply -f config/istio/gateway/namespace.yaml

.PHONY: deploy-mcp-router
deploy-mcp-router: # Deploy MCP router to cluster
	@echo "TODO: Add deployment manifest for mcp-router"

.PHONY: deploy-mcp-broker
deploy-mcp-broker: # Deploy MCP broker to cluster
	@echo "TODO: Add deployment manifest for mcp-broker"

.PHONY: deploy-mock-mcp
deploy-mock-mcp: $(KUSTOMIZE) ## Deploy mock MCP server for testing
	$(KUSTOMIZE) build config/mock-mcp | kubectl apply -f -

.PHONY: undeploy-mock-mcp
undeploy-mock-mcp: $(KUSTOMIZE) # Remove mock MCP server
	- $(KUSTOMIZE) build config/mock-mcp | kubectl delete -f -
