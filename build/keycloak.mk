# Keycloak IdP for development

KEYCLOAK_NAMESPACE = keycloak
KEYCLOAK_ADMIN_USER = admin
KEYCLOAK_ADMIN_PASSWORD = admin

HELM ?= bin/helm

keycloak-install-impl: $(HELM)
	@echo "Installing Keycloak (dev mode)..."
	@-$(HELM) repo add bitnami https://charts.bitnami.com/bitnami 2>/dev/null
	@$(HELM) repo update
	@$(HELM) upgrade --install keycloak bitnami/keycloak \
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
	@echo "Creating HTTPRoute"
	kubectl apply -f config/keycloak/httproute.yaml
	@echo ""
	@echo "Keycloak installed!"
	@echo "Admin credentials: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
	@echo "Run 'make keycloak-forward' to access at http://localhost:8095"

.PHONY: keycloak-uninstall
keycloak-uninstall: $(HELM) # Uninstall Keycloak
	$(HELM) uninstall keycloak -n $(KEYCLOAK_NAMESPACE)
	kubectl delete namespace $(KEYCLOAK_NAMESPACE)

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
	@echo "Admin Console (via port-forward): http://localhost:8095"
	@echo "OIDC Discovery: http://localhost:8095/realms/master/.well-known/openid-configuration"
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

.PHONY: keycloak-setup-mcp-realm
keycloak-setup-mcp-realm: ## Create MCP realm with user and configure client registration
	@echo "========================================="
	@echo "Setting up MCP Realm"
	@echo "========================================="
	@echo "Assuming Keycloak is available at http://keycloak.127-0-0-1.sslip.io:8889"
	@echo "(Run 'make inspect-gateway' or 'make dev-gateway-forward' to enable port forwarding)"
	@echo ""
	@echo "Getting admin access token..."
	@TOKEN=$$(curl -s -X POST "http://keycloak.127-0-0-1.sslip.io:8889/realms/master/protocol/openid-connect/token" \
		-H "Content-Type: application/x-www-form-urlencoded" \
		-d "username=$(KEYCLOAK_ADMIN_USER)" \
		-d "password=$(KEYCLOAK_ADMIN_PASSWORD)" \
		-d "grant_type=password" \
		-d "client_id=admin-cli" \
		2>/dev/null | jq -r '.access_token // empty'); \
	if [ -z "$$TOKEN" ] || [ "$$TOKEN" = "null" ]; then \
		echo "❌ Failed to get access token. Check if:"; \
		echo "  - Keycloak is running and accessible"; \
		echo "  - Port forwarding is active (make inspect-gateway)"; \
		echo "  - Admin credentials are correct: $(KEYCLOAK_ADMIN_USER)/$(KEYCLOAK_ADMIN_PASSWORD)"; \
		exit 1; \
	fi; \
	echo "✅ Successfully obtained access token"; \
	echo ""; \
	echo "Creating MCP realm..."; \
	REALM_RESPONSE=$$(curl -s -w "%{http_code}" -X POST "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"realm":"mcp","enabled":true}'); \
	REALM_CODE=$$(echo "$$REALM_RESPONSE" | tail -c 4); \
	if [ "$$REALM_CODE" = "201" ] || [ "$$REALM_CODE" = "409" ]; then \
		if [ "$$REALM_CODE" = "201" ]; then echo "✅ MCP realm created"; \
		else echo "✅ MCP realm already exists"; fi; \
	else \
		echo "❌ Failed to create MCP realm (HTTP $$REALM_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Creating MCP user..."; \
	USER_RESPONSE=$$(curl -s -w "%{http_code}" -X POST "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/users" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"username":"mcp","email":"mcp@example.com","firstName":"mcp","lastName":"mcp","enabled":true,"emailVerified":true,"credentials":[{"type":"password","value":"mcp","temporary":false}]}'); \
	USER_CODE=$$(echo "$$USER_RESPONSE" | tail -c 4); \
	if [ "$$USER_CODE" = "201" ] || [ "$$USER_CODE" = "409" ]; then \
		if [ "$$USER_CODE" = "201" ]; then echo "✅ MCP user created"; \
		else echo "✅ MCP user already exists"; fi; \
	else \
		echo "❌ Failed to create MCP user (HTTP $$USER_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Removing trusted hosts policy for anonymous client registration..."; \
	COMPONENTS=$$(curl -s -X GET "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/components?name=Trusted%20Hosts" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	COMPONENT_ID=$$(echo "$$COMPONENTS" | jq -r '.[0].id // empty' 2>/dev/null); \
	if [ -z "$$COMPONENT_ID" ] || [ "$$COMPONENT_ID" = "null" ]; then \
		echo "✅ Trusted hosts policy was not present"; \
	else \
		echo "Found trusted hosts component: $$COMPONENT_ID"; \
		DELETE_RESPONSE=$$(curl -s -w "%{http_code}" -X DELETE "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/components/$$COMPONENT_ID" \
			-H "Authorization: Bearer $$TOKEN"); \
		DELETE_CODE=$$(echo "$$DELETE_RESPONSE" | tail -c 4); \
		if [ "$$DELETE_CODE" = "204" ]; then \
			echo "✅ Trusted hosts policy removed"; \
		else \
			echo "❌ Failed to remove trusted hosts policy (HTTP $$DELETE_CODE)"; \
			exit 1; \
		fi; \
	fi; \
	echo ""; \
	echo "🎉 MCP realm setup complete!"; \
	echo ""; \
	echo "Realm: mcp"; \
	echo "User: mcp / mcp"; \
	echo "Email: mcp@example.com"

