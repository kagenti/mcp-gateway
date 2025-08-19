# Gateway API

.PHONY: gateway-api-install
gateway-api-install: $(KUSTOMIZE) # Install Gateway API CRDs
	$(KUSTOMIZE) build config/gateway-api | kubectl apply -f -

.PHONY: gateway-api-uninstall
gateway-api-uninstall: $(KUSTOMIZE) # Uninstall Gateway API CRDs
	$(KUSTOMIZE) build config/gateway-api | kubectl delete -f -
