# Keycloak IdP for development

KEYCLOAK_NAMESPACE = keycloak
KEYCLOAK_ADMIN_USER = admin
KEYCLOAK_ADMIN_PASSWORD = admin

HELM ?= bin/helm

keycloak-install-impl: $(HELM)
	@echo "Installing Keycloak (dev mode)..."
	@-$(HELM) repo add bitnami https://charts.bitnami.com/bitnami 2>/dev/null
	@$(HELM) repo update
	@$(HELM) install keycloak bitnami/keycloak \
		--create-namespace \
		--namespace $(KEYCLOAK_NAMESPACE) \
		--set auth.adminUser=$(KEYCLOAK_ADMIN_USER) \
		--set auth.adminPassword=$(KEYCLOAK_ADMIN_PASSWORD) \
		--set production=false \
		--set proxy=edge \
		--set postgresql.enabled=true \
		--set postgresql.auth.postgresPassword=postgres \
		--set postgresql.auth.database=keycloak \
		--set replicaCount=1 \
		--wait --timeout=300s
	@echo ""
	@echo "Keycloak installed!"
	@echo "Admin credentials: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
	@echo "Run 'make keycloak-forward' to access at http://localhost:8090"

.PHONY: keycloak-uninstall
keycloak-uninstall: $(HELM) # Uninstall Keycloak
	$(HELM) uninstall keycloak -n $(KEYCLOAK_NAMESPACE)
	kubectl delete namespace $(KEYCLOAK_NAMESPACE)

keycloak-forward-impl:
	@echo "Forwarding Keycloak to http://localhost:8090"
	@echo "Login: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
	kubectl port-forward -n $(KEYCLOAK_NAMESPACE) svc/keycloak 8090:80

keycloak-status-impl:
	@if kubectl get svc -n $(KEYCLOAK_NAMESPACE) keycloak >/dev/null 2>&1; then \
		echo "========================================"; \
		echo "Keycloak Status"; \
		echo "========================================"; \
		echo ""; \
		echo "Status: Installed"; \
		echo ""; \
		echo "Admin Console:"; \
		echo "  URL: http://localhost:8090 (run: make keycloak-forward)"; \
		echo "  Username: $(KEYCLOAK_ADMIN_USER)"; \
		echo "  Password: $(KEYCLOAK_ADMIN_PASSWORD)"; \
		echo ""; \
		echo "OIDC Endpoints:"; \
		echo "  Discovery: http://localhost:8090/realms/master/.well-known/openid-configuration"; \
		echo "  Token:     http://localhost:8090/realms/master/protocol/openid-connect/token"; \
		echo "  Authorize: http://localhost:8090/realms/master/protocol/openid-connect/auth"; \
		echo "  UserInfo:  http://localhost:8090/realms/master/protocol/openid-connect/userinfo"; \
		echo "  JWKS:      http://localhost:8090/realms/master/protocol/openid-connect/certs"; \
		echo ""; \
		echo "Test Client Configuration:"; \
		echo "  Client ID: mcp-gateway"; \
		echo "  Root URL: http://localhost:8888"; \
		echo "  Valid Redirect URIs: http://localhost:8888/*"; \
		echo "  Web Origins: +"; \
		echo ""; \
		echo "Note: Create the test client manually in the Keycloak admin console"; \
		echo "========================================"; \
	else \
		echo "Keycloak is not installed. Run: make keycloak-install"; \
	fi

.PHONY: keycloak-url
keycloak-url: # Get Keycloak URLs
	@echo "=== Keycloak URLs ==="
	@echo "Admin Console (via port-forward): http://localhost:8090"
	@echo "OIDC Discovery: http://localhost:8090/realms/master/.well-known/openid-configuration"
	@echo ""
	@echo "To access: make keycloak-forward"
	@echo "Credentials: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"

.PHONY: keycloak-create-client
keycloak-create-client: # Create a test OIDC client for MCP
	@echo "Creating test client in Keycloak..."
	@echo "First, port-forward Keycloak and login to admin console"
	@echo "Then create a client with:"
	@echo "  - Client ID: mcp-gateway"
	@echo "  - Client Protocol: openid-connect"
	@echo "  - Root URL: http://localhost:8888"
	@echo "  - Valid Redirect URIs: http://localhost:8888/*"
	@echo "  - Web Origins: +"
