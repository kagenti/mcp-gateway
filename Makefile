# Platform detection
OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH := $(shell uname -m | tr '[:upper:]' '[:lower:]')
ifeq ($(ARCH),x86_64)
    ARCH = amd64
endif
ifeq ($(ARCH),aarch64)
    ARCH = arm64
endif


LOG_LEVEL ?= -4

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: build clean mcp-broker-router

# Build the combined broker and router 
mcp-broker-router:
	go build -o bin/mcp-broker-router ./cmd/mcp-broker-router

# Build all binaries
build: mcp-broker-router

# Clean build artifacts
clean:
	rm -rf bin/

# Run the broker/router (standalone mode)
run: mcp-broker-router
	./bin/mcp-broker-router --log-level=${LOG_LEVEL}

# Run the broker and router with debug logging (alias for backwards compatibility)
run-mcp-broker-router: run

# Run in controller mode (discovers MCP servers from Kubernetes)
run-controller: mcp-broker-router
	./bin/mcp-broker-router --controller --log-level=${LOG_LEVEL}

# Install CRD
install-crd: ## Install MCPGateway CRD
	kubectl apply -f config/crd/mcpgateway.yaml

# Deploy mcp-gateway components
deploy: install-crd ## Deploy broker/router and controller to mcp-system namespace
	kubectl apply -k config/mcp-system/

# Deploy only the broker/router
deploy-broker: install-crd ## Deploy only the broker/router (without controller)
	kubectl apply -f config/mcp-system/namespace.yaml
	kubectl apply -f config/mcp-system/rbac.yaml
	kubectl apply -f config/mcp-system/service.yaml
	kubectl apply -f config/mcp-system/deployment-broker.yaml
	kubectl apply -k config/mcp-system/ --dry-run=client -o yaml | kubectl apply -f - -l app=mcp-gateway

# Deploy only the controller
deploy-controller: install-crd ## Deploy only the controller
	kubectl apply -f config/mcp-system/namespace.yaml
	kubectl apply -f config/mcp-system/rbac.yaml
	kubectl apply -f config/mcp-system/deployment-controller.yaml

# Deploy example MCPGateway
deploy-example: install-crd ## Deploy example MCPGateway resource
	kubectl apply -f config/samples/mcpgateway-calendar.yaml

# Build and push container image
docker-build: ## Build container image locally
	docker build -t mcp-gateway:local .

# Build multi-platform image
docker-buildx: ## Build multi-platform container image
	docker buildx build --platform linux/amd64,linux/arm64 -t mcp-gateway:local .

# Download dependencies
deps:
	go mod download

# Update dependencies
update:
	go mod tidy
	go get -u ./...

# Lint

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: golangci-lint
golangci-lint:
	golangci-lint run ./...

.PHONY: lint
lint: fmt vet golangci-lint

test-unit:
	echo "blah blah blah test"

.PHONY: tools
tools: ## Install all required tools (kind, helm, kustomize, yq, istioctl) to ./bin/
	@echo "Checking and installing required tools to ./bin/ ..."
	@if [ -f bin/kind ]; then echo "[OK] kind already installed"; else echo "Installing kind..."; $(MAKE) -s kind; fi
	@if [ -f bin/helm ]; then echo "[OK] helm already installed"; else echo "Installing helm..."; $(MAKE) -s helm; fi
	@if [ -f bin/kustomize ]; then echo "[OK] kustomize already installed"; else echo "Installing kustomize..."; $(MAKE) -s kustomize; fi
	@if [ -f bin/yq ]; then echo "[OK] yq already installed"; else echo "Installing yq..."; $(MAKE) -s yq; fi
	@if [ -f bin/istioctl ]; then echo "[OK] istioctl already installed"; else echo "Installing istioctl..."; $(MAKE) -s istioctl; fi
	@echo "All tools ready!"

.PHONY: local-env-setup
local-env-setup: ## Setup local Kind cluster with Istio, Gateway API, MetalLB, Keycloak, and Kuadrant
	@echo "========================================="
	@echo "Starting MCP Gateway Environment Setup"
	@echo "========================================="
	$(MAKE) tools
	$(MAKE) kind-delete-cluster
	$(MAKE) kind-create-cluster
	$(MAKE) gateway-api-install
	$(MAKE) istio-install
	$(MAKE) metallb-install
	$(MAKE) deploy-namespaces
	$(MAKE) deploy-gateway
	$(MAKE) keycloak-install
	$(MAKE) kuadrant-install

.PHONY: local-env-teardown
local-env-teardown: ## Tear down the local Kind cluster
	$(MAKE) kind-delete-cluster

.PHONY: dev
dev: ## Setup cluster for local development (binaries run on host)
	$(MAKE) dev-setup
	@echo ""
	@echo "Ready for local development! Run these in separate terminals:"
	@echo "  1. make run-mcp-broker-router"
	@echo "  2. make dev-gateway-forward"
	@echo ""
	@echo "Then test with: make dev-test"

##@ Getting Started

.PHONY: info
info: ## Show quick setup info and useful commands
	@$(MAKE) -s -f build/info.mk info-impl

##@ Inspection

.PHONY: urls
urls: ## Show all available service URLs
	@$(MAKE) -s -f build/inspect.mk urls-impl

.PHONY: status
status: ## Show status of all MCP components
	@$(MAKE) -s -f build/inspect.mk status-impl

.PHONY: inspect-mock
inspect-mock: ## Open MCP Inspector for mock server
	@$(MAKE) -s -f build/inspect.mk inspect-mock-impl

##@ Tools

.PHONY: istioctl
istioctl: ## Download and install istioctl
	@$(MAKE) -s -f build/istio.mk istioctl-impl

.PHONY: keycloak-install
keycloak-install: ## Install Keycloak IdP for development
	@$(MAKE) -s -f build/keycloak.mk keycloak-install-impl

.PHONY: keycloak-forward
keycloak-forward: ## Port forward Keycloak to localhost:8090
	@$(MAKE) -s -f build/keycloak.mk keycloak-forward-impl

.PHONY: keycloak-status
keycloak-status: ## Show Keycloak URLs, credentials, and OIDC endpoints
	@$(MAKE) -s -f build/keycloak.mk keycloak-status-impl

.PHONY: kuadrant-install
kuadrant-install: ## Install Kuadrant operator for API gateway policies
	@$(MAKE) -s -f build/kuadrant.mk kuadrant-install-impl

.PHONY: kuadrant-status
kuadrant-status: ## Show Kuadrant operator status and available CRDs
	@$(MAKE) -s -f build/kuadrant.mk kuadrant-status-impl

.PHONY: kuadrant-configure
kuadrant-configure: ## Apply Kuadrant configuration from config/kuadrant
	@$(MAKE) -s -f build/kuadrant.mk kuadrant-configure-impl

##@ Debug

.PHONY: debug-envoy
debug-envoy: ## Enable debug logging for Istio gateway
	@$(MAKE) -s -f build/debug.mk debug-envoy-impl

.PHONY: istio-clusters
istio-clusters: ## Show all registered clusters in the gateway
	@$(MAKE) -s -f build/istio-debug.mk istio-clusters-impl

.PHONY: istio-config
istio-config: ## Show all proxy configurations
	@$(MAKE) -s -f build/istio-debug.mk istio-config-impl

.PHONY: debug-envoy-off
debug-envoy-off: ## Disable debug logging for Istio gateway
	@$(MAKE) -s -f build/debug.mk debug-envoy-off-impl

.PHONY: logs
logs: ## Tail Istio gateway logs
	@$(MAKE) -s -f build/debug.mk debug-logs-gateway-impl

-include build/*.mk
