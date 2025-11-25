# Keycloak IdP for development

KEYCLOAK_NAMESPACE = keycloak
KEYCLOAK_ADMIN_USER = admin
KEYCLOAK_ADMIN_PASSWORD = admin

keycloak-install-impl:
	@echo "Installing Keycloak (dev mode using official image)..."
	@echo "Note: Using kubectl deployment due to Helm chart issues"
	@# Create namespace if it doesn't exist
	@kubectl create namespace $(KEYCLOAK_NAMESPACE) 2>/dev/null || echo "Namespace $(KEYCLOAK_NAMESPACE) already exists"
	@kubectl apply -f config/keycloak/realm-import.yaml
	@kubectl apply -f config/keycloak/deployment.yaml
	@echo "Waiting for Keycloak to be ready..."
	@kubectl wait --for=condition=ready pod -l app=keycloak -n $(KEYCLOAK_NAMESPACE) --timeout=120s || true
	@echo ""
	@echo "Creating HTTPRoute"
	@kubectl apply -f config/keycloak/httproute.yaml
	@echo ""
	@echo "🎉 Keycloak installed with bootstrapped MCP realm!"
	@echo ""
	@echo "Run 'make keycloak-forward' to access at http://localhost:8095"
	@echo "Admin credentials: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
	@echo ""
	@echo "Realm: mcp"
	@echo "User: mcp / mcp"
	@echo "Email: mcp@example.com"
	@echo "Group: accounting (with mcp user as member)"
	@echo "Client Scope: groups (optional, with Group Membership mapper)"
	@echo "Session idle timeout: 15 minutes"
	@echo ""

.PHONY: keycloak-uninstall
keycloak-uninstall: # Uninstall Keycloak
	@kubectl delete -f config/keycloak/httproute.yaml 2>/dev/null || true
	@kubectl delete -f config/keycloak/deployment.yaml 2>/dev/null || true
	@kubectl delete -f config/keycloak/realm-import.yaml 2>/dev/null || true
	@kubectl delete namespace $(KEYCLOAK_NAMESPACE) 2>/dev/null || true

keycloak-status-impl:
	@if kubectl get svc -n $(KEYCLOAK_NAMESPACE) keycloak >/dev/null 2>&1; then \
		echo "========================================"; \
		echo "Keycloak Status"; \
		echo "========================================"; \
		echo ""; \
		echo "Status: Installed"; \
		echo ""; \
		echo "Admin Console:"; \
		echo "  URL: http://keycloak.127-0-0-1.sslip.io:$(KIND_HOST_PORT_KEYCLOAK)"; \
		echo "  Username: $(KEYCLOAK_ADMIN_USER)"; \
		echo "  Password: $(KEYCLOAK_ADMIN_PASSWORD)"; \
		echo ""; \
		echo "OIDC Endpoints:"; \
		echo "  Discovery: http://keycloak.127-0-0-1.sslip.io:$(KIND_HOST_PORT_KEYCLOAK)/realms/master/.well-known/openid-configuration"; \
		echo "  Token:     http://keycloak.127-0-0-1.sslip.io:$(KIND_HOST_PORT_KEYCLOAK)/realms/master/protocol/openid-connect/token"; \
		echo "  Authorize: http://keycloak.127-0-0-1.sslip.io:$(KIND_HOST_PORT_KEYCLOAK)/realms/master/protocol/openid-connect/auth"; \
		echo "  UserInfo:  http://keycloak.127-0-0-1.sslip.io:$(KIND_HOST_PORT_KEYCLOAK)/realms/master/protocol/openid-connect/userinfo"; \
		echo "  JWKS:      http://keycloak.127-0-0-1.sslip.io:$(KIND_HOST_PORT_KEYCLOAK)/realms/master/protocol/openid-connect/certs"; \
		echo ""; \
		echo "Test Client Configuration:"; \
		echo "  Client ID: mcp-gateway"; \
		echo "  Root URL: http://mcp.127-0-0-1.sslip.io:$(KIND_HOST_PORT_MCP_GATEWAY)"; \
		echo "  Valid Redirect URIs: http://mcp.127-0-0-1.sslip.io:$(KIND_HOST_PORT_MCP_GATEWAY)/*"; \
		echo "  Web Origins: +"; \
		echo ""; \
		echo "========================================"; \
	else \
		echo "Keycloak is not installed. Run: make keycloak-install"; \
	fi

.PHONY: keycloak-url
keycloak-url: # Get Keycloak URLs
	@echo "=== Keycloak URLs ==="
	@echo "Admin Console: http://keycloak.127-0-0-1.sslip.io:$(KIND_HOST_PORT_KEYCLOAK)"
	@echo "OIDC Discovery: http://keycloak.127-0-0-1.sslip.io:$(KIND_HOST_PORT_KEYCLOAK)/realms/master/.well-known/openid-configuration"
	@echo ""
	@echo "Credentials: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
