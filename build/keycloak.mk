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

keycloak-forward-impl:
	@echo "Forwarding Keycloak to http://localhost:8095"
	@echo "Login: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
	kubectl port-forward -n $(KEYCLOAK_NAMESPACE) svc/keycloak 8095:80

keycloak-status-impl:
	@if kubectl get svc -n $(KEYCLOAK_NAMESPACE) keycloak >/dev/null 2>&1; then \
		echo "========================================"; \
		echo "Keycloak Status"; \
		echo "========================================"; \
		echo ""; \
		echo "Status: Installed"; \
		echo ""; \
		echo "Admin Console:"; \
		echo "  URL: http://localhost:8095 (run: make keycloak-forward)"; \
		echo "  Username: $(KEYCLOAK_ADMIN_USER)"; \
		echo "  Password: $(KEYCLOAK_ADMIN_PASSWORD)"; \
		echo ""; \
		echo "OIDC Endpoints:"; \
		echo "  Discovery: http://localhost:8095/realms/master/.well-known/openid-configuration"; \
		echo "  Token:     http://localhost:8095/realms/master/protocol/openid-connect/token"; \
		echo "  Authorize: http://localhost:8095/realms/master/protocol/openid-connect/auth"; \
		echo "  UserInfo:  http://localhost:8095/realms/master/protocol/openid-connect/userinfo"; \
		echo "  JWKS:      http://localhost:8095/realms/master/protocol/openid-connect/certs"; \
		echo ""; \
		echo "Test Client Configuration:"; \
		echo "  Client ID: mcp-gateway"; \
		echo "  Root URL: http://localhost:$(GATEWAY_LOCAL_PORT_HTTP_MCP)"; \
		echo "  Valid Redirect URIs: http://localhost:$(GATEWAY_LOCAL_PORT_HTTP_MCP)/*"; \
		echo "  Web Origins: +"; \
		echo ""; \
		echo "========================================"; \
	else \
		echo "Keycloak is not installed. Run: make keycloak-install"; \
	fi

.PHONY: keycloak-url
keycloak-url: # Get Keycloak URLs
	@echo "=== Keycloak URLs ==="
	@echo "Admin Console (via port-forward): http://localhost:8095"
	@echo "OIDC Discovery: http://localhost:8095/realms/master/.well-known/openid-configuration"
	@echo ""
	@echo "To access: make keycloak-forward"
	@echo "Credentials: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
