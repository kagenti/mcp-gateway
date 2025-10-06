# Platform detection
OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH := $(shell uname -m | tr '[:upper:]' '[:lower:]')
ifeq ($(ARCH),x86_64)
    ARCH = amd64
endif
ifeq ($(ARCH),aarch64)
    ARCH = arm64
endif

# Use `export BUILD_FLAGS=--load` for Podman
BUILDFLAGS = $(BUILD_FLAGS)

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

# Generate CRDs from Go types
generate-crds: ## Generate CRD manifests from Go types
	controller-gen crd paths="./pkg/apis/..." output:dir=config/crd

# Update Helm chart CRDs from generated ones
update-helm-crds: generate-crds ## Update Helm chart CRDs (run after generate-crds)
	@echo "Copying CRDs to Helm chart..."
	@mkdir -p charts/mcp-gateway/crds
	cp config/crd/mcp.kagenti.com_*.yaml charts/mcp-gateway/crds/
	@echo "✅ Helm chart CRDs updated"

# Generate CRDs and update Helm chart in one step
generate-crds-all: update-helm-crds ## Generate CRDs and update Helm chart
	@echo "✅ All CRDs generated and synchronized"

# Check if CRDs are synchronized between config/crd and charts/
check-crd-sync: ## Check if CRDs are synchronized between config/crd and charts/mcp-gateway/crds
	@echo "Checking CRD synchronization..."
	@if [ ! -d "charts/mcp-gateway/crds" ]; then \
		echo "❌ Helm CRDs directory doesn't exist. Run 'make update-helm-crds'"; \
		exit 1; \
	fi
	@# Only compare actual CRD files, not kustomization.yaml
	@SYNC_ERROR=0; \
	for crd in config/crd/mcp.kagenti.com_*.yaml; do \
		crd_name=$$(basename "$$crd"); \
		if [ ! -f "charts/mcp-gateway/crds/$$crd_name" ]; then \
			echo "❌ Missing CRD in Helm chart: $$crd_name"; \
			SYNC_ERROR=1; \
		elif ! diff "$$crd" "charts/mcp-gateway/crds/$$crd_name" >/dev/null 2>&1; then \
			echo "❌ CRD differs: $$crd_name"; \
			SYNC_ERROR=1; \
		fi; \
	done; \
	if [ $$SYNC_ERROR -eq 1 ]; then \
		echo ""; \
		echo "Run 'make update-helm-crds' to sync, or 'make generate-crds-all' to regenerate and sync"; \
		exit 1; \
	else \
		echo "✅ CRDs are synchronized"; \
	fi

# Install CRD
install-crd: ## Install MCPServer and MCPVirtualServer CRDs
	kubectl apply -f config/crd/mcp.kagenti.com_mcpservers.yaml
	kubectl apply -f config/crd/mcp.kagenti.com_mcpvirtualservers.yaml

# Deploy mcp-gateway components
deploy: install-crd ## Deploy broker/router and controller to mcp-system namespace
	@kubectl create namespace mcp-system --dry-run=client -o yaml | kubectl apply -f -
	@kubectl get secret mcp-config-update-token -n mcp-system &>/dev/null || \
		kubectl create secret generic mcp-config-update-token \
		-n mcp-system \
		--from-literal=token="$$(openssl rand -base64 32)"
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

# Build & load router/broker/controller image into the Kind cluster
build-and-load-image:
	@echo "Building and loading image into Kind cluster..."
	docker build ${BUILDFLAGS} -t ghcr.io/kagenti/mcp-gateway:latest .
	kind load docker-image ghcr.io/kagenti/mcp-gateway:latest --name mcp-gateway
	kubectl rollout restart deployment/mcp-broker-router -n mcp-system 2>/dev/null || true

# Deploy example MCPServer
deploy-example: install-crd ## Deploy example MCPServer resource
	@echo "Waiting for test servers to be ready..."
	@kubectl wait --for=condition=ready pod -n mcp-test -l app=mcp-test-server1 --timeout=60s
	@kubectl wait --for=condition=ready pod -n mcp-test -l app=mcp-test-server2 --timeout=60s
	@kubectl wait --for=condition=ready pod -n mcp-test -l app=mcp-test-server3 --timeout=90s
	@kubectl wait --for=condition=ready pod -n mcp-test -l app=mcp-api-key-server --timeout=60s
	@kubectl wait --for=condition=ready pod -n mcp-test -l app=mcp-custom-path-server --timeout=60s 2>/dev/null || true
	@echo "All test servers ready, deploying MCPServer resources..."
	kubectl apply -f config/samples/mcpserver-test-servers.yaml
	@echo "Waiting for controller to process MCPServer..."
	@sleep 3
	@echo "Restarting broker to ensure all connections..."
	kubectl rollout restart deployment/mcp-broker-router -n mcp-system
	@kubectl rollout status deployment/mcp-broker-router -n mcp-system --timeout=60s

# Build test server Docker images
build-test-servers: ## Build test server Docker images locally
	@echo "Building test server images..."
	cd tests/servers/server1 && docker build ${BUILDFLAGS} -t ghcr.io/kagenti/mcp-gateway/test-server1:latest .
	cd tests/servers/server2 && docker build ${BUILDFLAGS} -t ghcr.io/kagenti/mcp-gateway/test-server2:latest .
	cd tests/servers/server3 && docker build ${BUILDFLAGS} -t ghcr.io/kagenti/mcp-gateway/test-server3:latest .
	cd tests/servers/api-key-server && docker build ${BUILDFLAGS} -t ghcr.io/kagenti/mcp-gateway/test-api-key-server:latest .
	cd tests/servers/broken-server && docker build ${BUILDFLAGS} -t ghcr.io/kagenti/mcp-gateway/test-broken-server:latest .
	cd tests/servers/custom-path-server && docker build ${BUILDFLAGS} -t ghcr.io/kagenti/mcp-gateway/test-custom-path-server:latest .

# Load test server images into Kind cluster
kind-load-test-servers: build-test-servers ## Load test server images into Kind cluster
	@echo "Loading test server images into Kind cluster..."
	kind load docker-image ghcr.io/kagenti/mcp-gateway/test-server1:latest --name mcp-gateway
	kind load docker-image ghcr.io/kagenti/mcp-gateway/test-server2:latest --name mcp-gateway
	kind load docker-image ghcr.io/kagenti/mcp-gateway/test-server3:latest --name mcp-gateway
	kind load docker-image ghcr.io/kagenti/mcp-gateway/test-api-key-server:latest --name mcp-gateway
	kind load docker-image ghcr.io/kagenti/mcp-gateway/test-broken-server:latest --name mcp-gateway
	kind load docker-image ghcr.io/kagenti/mcp-gateway/test-custom-path-server:latest --name mcp-gateway

# Deploy test servers
deploy-test-servers: kind-load-test-servers ## Deploy test MCP servers for local testing
	@echo "Deploying test MCP servers..."
	kubectl apply -k config/test-servers/

# Build and push container image
docker-build: ## Build container image locally
	docker build ${BUILDFLAGS} -t mcp-gateway:local .

# Common reload steps
define reload-image
	@docker tag mcp-gateway:local ghcr.io/kagenti/mcp-gateway:latest
	@kind load docker-image ghcr.io/kagenti/mcp-gateway:latest --name mcp-gateway
endef

.PHONY: reload-controller
reload-controller: build docker-build ## Build, load to Kind, and restart controller
	$(call reload-image)
	@kubectl rollout restart -n mcp-system deployment/mcp-controller
	@kubectl rollout status -n mcp-system deployment/mcp-controller --timeout=60s

.PHONY: reload-broker
reload-broker: build docker-build ## Build, load to Kind, and restart broker
	$(call reload-image)
	@kubectl rollout restart -n mcp-system deployment/mcp-broker-router
	@kubectl rollout status -n mcp-system deployment/mcp-broker-router --timeout=60s

.PHONY: reload
reload: build docker-build ## Build, load to Kind, and restart both controller and broker
	$(call reload-image)
	@kubectl rollout restart -n mcp-system deployment/mcp-controller deployment/mcp-broker-router
	@kubectl rollout status -n mcp-system deployment/mcp-controller --timeout=60s
	@kubectl rollout status -n mcp-system deployment/mcp-broker-router --timeout=60s

##@ E2E Testing

# E2E test targets are in build/e2e.mk

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
	goimports -w .

.PHONY: vet
vet:
	go vet ./...

.PHONY: golangci-lint
golangci-lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	elif [ -f bin/golangci-lint ]; then \
		bin/golangci-lint run ./...; \
	else \
		$(MAKE) golangci-lint-bin && bin/golangci-lint run ./...; \
	fi

.PHONY: lint
lint: check-gofmt check-goimports check-newlines fmt vet golangci-lint ## Run all linting and style checks
	@echo "All lint checks passed!"

# Code style checks
.PHONY: check-style
check-style: check-gofmt check-goimports check-newlines

.PHONY: check-gofmt
check-gofmt:
	@echo "Checking gofmt..."
	@if [ -n "$$(gofmt -s -l . | grep -v '^vendor/' | grep -v '\.deepcopy\.go')" ]; then \
		echo "Files need gofmt -s:"; \
		gofmt -s -l . | grep -v '^vendor/' | grep -v '\.deepcopy\.go'; \
		echo "Run 'make fmt' to fix."; \
		exit 1; \
	fi

.PHONY: check-goimports
check-goimports:
	@echo "Checking goimports..."
	@if [ -n "$$(goimports -l . | grep -v '^vendor/' | grep -v '\.deepcopy\.go')" ]; then \
		echo "Files need goimports:"; \
		goimports -l . | grep -v '^vendor/' | grep -v '\.deepcopy\.go'; \
		echo "Run 'make fmt' to fix."; \
		exit 1; \
	fi

.PHONY: check-newlines
check-newlines:
	@set -e; \
	echo "Checking for missing EOF newlines..."; \
	LINT_FILES=$$(git ls-files | \
		git check-attr --stdin linguist-generated | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		git check-attr --stdin linguist-vendored  | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		grep -Ev '^(third_party/|.github|docs/)' | \
		grep -v '\.ai$$' | \
		grep -v '\.svg$$'); \
	FAIL=0; \
	for x in $$LINT_FILES; do \
		if [ -f "$$x" ]; then \
			if [ -s "$$x" ] && [ -n "$$(tail -c 1 "$$x")" ]; then \
				echo "Missing newline at end of file: $$x"; \
				FAIL=1; \
			fi; \
		fi; \
	done; \
	exit $$FAIL

.PHONY: fix-newlines
fix-newlines:
	@echo "Fixing missing EOF newlines..."
	@LINT_FILES=$$(git ls-files | \
		git check-attr --stdin linguist-generated | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		git check-attr --stdin linguist-vendored  | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		grep -Ev '^(third_party/|.github|docs/)' | \
		grep -v '\.ai$$' | \
		grep -v '\.svg$$'); \
	for x in $$LINT_FILES; do \
		if [ -f "$$x" ]; then \
			if [ -s "$$x" ] && [ -n "$$(tail -c 1 "$$x")" ]; then \
				echo "" >> "$$x"; \
				echo "Fixed: $$x"; \
			fi; \
		fi; \
	done


test-unit:
	go test ./...

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
local-env-setup: ## Setup complete local demo environment with Kind, Istio, MCP Gateway, and test servers
	@echo "========================================="
	@echo "Starting MCP Gateway Environment Setup"
	@echo "========================================="
	$(MAKE) tools
	$(MAKE) kind-create-cluster
	$(MAKE) build-and-load-image
	$(MAKE) gateway-api-install
	$(MAKE) istio-install
	$(MAKE) metallb-install
	$(MAKE) deploy-namespaces
	$(MAKE) deploy-gateway
	$(MAKE) keycloak-install
	$(MAKE) kuadrant-install
	$(MAKE) deploy
	$(MAKE) deploy-test-servers
	$(MAKE) deploy-example

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


##@ Tools

.PHONY: istioctl
istioctl: ## Download and install istioctl
	@$(MAKE) -s -f build/istio.mk istioctl-impl

.PHONY: keycloak-install
keycloak-install: ## Install Keycloak IdP for development
	@echo "Installing Keycloak - using official image with H2 database"
	@$(MAKE) -s -f build/keycloak.mk keycloak-install-impl

.PHONY: keycloak-forward
keycloak-forward: ## Port forward Keycloak to localhost:8090
	@$(MAKE) -s -f build/keycloak.mk keycloak-forward-impl

.PHONY: keycloak-status
keycloak-status: ## Show Keycloak URLs, credentials, and OIDC endpoints
	@$(MAKE) -s -f build/keycloak.mk keycloak-status-impl

.PHONY: oauth-example-setup
oauth-example-setup: ## Complete OAuth example setup (requires: make local-env-setup)
	@echo "========================================="
	@echo "Setting up OAuth Example"
	@echo "========================================="
	@echo "Prerequisites: make local-env-setup should be completed"
	@echo ""
	@echo "Step 1: Setting up Keycloak realm..."
	@$(MAKE) -s -f build/keycloak.mk keycloak-setup-mcp-realm
	@echo ""
	@echo "Step 2: Configuring mcp-broker with OAuth environment variables..."
	@kubectl set env deployment/mcp-broker-router \
		OAUTH_RESOURCE_NAME="MCP Server" \
		OAUTH_RESOURCE="http://mcp.127-0-0-1.sslip.io:8888/mcp" \
		OAUTH_AUTHORIZATION_SERVERS="http://keycloak.127-0-0-1.sslip.io:8889/realms/mcp" \
		OAUTH_BEARER_METHODS_SUPPORTED="header" \
		OAUTH_SCOPES_SUPPORTED="basic,groups" \
		-n mcp-system
	@echo "✅ OAuth environment variables configured"
	@echo ""
	@echo "Step 3: Applying AuthPolicy configurations..."
	@kubectl apply -f ./config/mcp-system/authpolicy.yaml
	@kubectl apply -f ./config/mcp-system/tool-call-auth.yaml
	@echo "✅ AuthPolicy configurations applied"
	@echo ""
	@echo "Step 4: Applying additional OAuth configurations..."
	@kubectl apply -f ./config/keycloak/preflight_envoyfilter.yaml
	@kubectl -n mcp-system apply -k ./config/example-access-control/
	@echo "✅ Additional configurations applied"
	@echo ""
	@echo "🎉 OAuth example setup complete!"
	@echo ""
	@echo "The mcp-broker now serves OAuth discovery information at:"
	@echo "  /.well-known/oauth-protected-resource"
	@echo ""
	@echo "Next step: Open MCP Inspector with 'make inspect-gateway'"
	@echo "and go through the OAuth flow with credentials: mcp/mcp"

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

.PHONY: testwithcoverage
testwithcoverage:
	go test ./... -coverprofile=coverage.out

.PHONY: coverage
coverage: testwithcoverage
	@echo "test coverage: $(shell go tool cover -func coverage.out | grep total | awk '{print substr($$3, 1, length($$3)-1)}')"

.PHONY: htmlcov
htmlcov: coverage
	go tool cover -html=coverage.out
