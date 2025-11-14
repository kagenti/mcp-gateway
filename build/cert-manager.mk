# cert-manager

CERT_MANAGER_VERSION := 1.16.2

.PHONY: cert-manager-install-impl
cert-manager-install-impl:
	@kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v$(CERT_MANAGER_VERSION)/cert-manager.yaml
	@kubectl wait --namespace cert-manager --for=condition=available deployment/cert-manager --timeout=120s
	@kubectl wait --namespace cert-manager --for=condition=available deployment/cert-manager-cainjector --timeout=120s
	@kubectl wait --namespace cert-manager --for=condition=available deployment/cert-manager-webhook --timeout=120s
