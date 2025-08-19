# Tools

KIND = bin/kind
KIND_VERSION = v0.23.0
$(KIND):
	GOBIN=$(PWD)/bin go install sigs.k8s.io/kind@$(KIND_VERSION)

.PHONY: kind
kind: $(KIND) # Download kind locally if necessary.

HELM = bin/helm
HELM_VERSION = v3.15.2
$(HELM):
	mkdir -p bin
	curl -fsSL https://get.helm.sh/helm-$(HELM_VERSION)-$(OS)-$(ARCH).tar.gz | tar -xz -C bin --strip-components=1 $(OS)-$(ARCH)/helm

.PHONY: helm
helm: $(HELM) # Download helm locally if necessary.

YQ = bin/yq
YQ_VERSION = v4.44.2
$(YQ):
	GOBIN=$(PWD)/bin go install github.com/mikefarah/yq/v4@$(YQ_VERSION)

.PHONY: yq
yq: $(YQ) # Download yq locally if necessary.

KUSTOMIZE = bin/kustomize
KUSTOMIZE_VERSION = v4.5.7
$(KUSTOMIZE):
	GOBIN=$(PWD)/bin go install sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION)

.PHONY: kustomize
kustomize: $(KUSTOMIZE) # Download kustomize locally if necessary.
