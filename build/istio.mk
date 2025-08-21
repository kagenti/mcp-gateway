# Istio

SAIL_VERSION = 1.26.3
ISTIO_NAMESPACE = istio-system
ISTIO_VERSION = 1.26.3

# istioctl tool
ISTIOCTL = bin/istioctl
$(ISTIOCTL):
	mkdir -p bin
	curl -sL https://istio.io/downloadIstio | ISTIO_VERSION=$(ISTIO_VERSION) TARGET_ARCH=$(ARCH) sh -
	mv istio-$(ISTIO_VERSION)/bin/istioctl bin/
	rm -rf istio-$(ISTIO_VERSION)

istioctl-impl: $(ISTIOCTL)
	@echo "istioctl installed at: $(ISTIOCTL)"
	@echo "Version: $$($(ISTIOCTL) version --remote=false)"

.PHONY: istio-install
istio-install: $(HELM) # Install Istio using Sail operator
	$(HELM) install sail-operator \
		--create-namespace \
		--namespace $(ISTIO_NAMESPACE) \
		--wait \
		--timeout=300s \
		https://github.com/istio-ecosystem/sail-operator/releases/download/$(SAIL_VERSION)/sail-operator-$(SAIL_VERSION).tgz
	kubectl apply -f config/istio/istio.yaml
	kubectl -n $(ISTIO_NAMESPACE) wait --for=condition=Ready istio/default --timeout=300s

.PHONY: istio-uninstall
istio-uninstall: $(HELM) # Uninstall Istio and Sail operator
	- kubectl delete -f config/istio/istio.yaml
	$(HELM) uninstall sail-operator -n $(ISTIO_NAMESPACE)
	- kubectl delete namespace $(ISTIO_NAMESPACE)
