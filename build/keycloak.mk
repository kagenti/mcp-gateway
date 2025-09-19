# Keycloak IdP for development

KEYCLOAK_NAMESPACE = keycloak
KEYCLOAK_ADMIN_USER = admin
KEYCLOAK_ADMIN_PASSWORD = admin

keycloak-install-impl:
	@echo "Installing Keycloak (dev mode using official image)..."
	@echo "Note: Using kubectl deployment due to Helm chart issues"
	@kubectl apply -f config/keycloak/deployment.yaml
	@echo "Waiting for Keycloak to be ready..."
	@kubectl wait --for=condition=ready pod -l app=keycloak -n $(KEYCLOAK_NAMESPACE) --timeout=120s || true
	@echo ""
	@echo "Creating HTTPRoute"
	@kubectl apply -f config/keycloak/httproute.yaml
	@echo ""
	@echo "Keycloak installed!"
	@echo "Admin credentials: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
	@echo "Run 'make keycloak-forward' to access at http://localhost:8095"

.PHONY: keycloak-uninstall
keycloak-uninstall: # Uninstall Keycloak
	@kubectl delete -f config/keycloak/httproute.yaml 2>/dev/null || true
	@kubectl delete -f config/keycloak/deployment.yaml 2>/dev/null || true

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
	echo "Updating MCP realm token settings..."; \
	REALM_UPDATE_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X PUT "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"realm":"mcp","enabled":true,"ssoSessionIdleTimeout":1800,"accessTokenLifespan":1800}'); \
	REALM_UPDATE_CODE=$$(echo "$$REALM_UPDATE_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	REALM_UPDATE_BODY=$$(echo "$$REALM_UPDATE_RESPONSE" | sed 's/HTTPCODE:[0-9]*$$//'); \
	if [ "$$REALM_UPDATE_CODE" = "204" ]; then \
		echo "✅ MCP realm token settings updated (session idle timeout: 30 minutes)"; \
	else \
		echo "❌ Failed to update MCP realm settings (HTTP $$REALM_UPDATE_CODE)"; \
		echo "Response body: $$REALM_UPDATE_BODY"; \
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
	echo "Creating accounting group..."; \
	GROUP_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/groups" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"name":"accounting"}'); \
	GROUP_CODE=$$(echo "$$GROUP_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	GROUP_BODY=$$(echo "$$GROUP_RESPONSE" | sed 's/HTTPCODE:[0-9]*$$//'); \
	if [ "$$GROUP_CODE" = "201" ] || [ "$$GROUP_CODE" = "409" ]; then \
		if [ "$$GROUP_CODE" = "201" ]; then echo "✅ Accounting group created"; \
		else echo "✅ Accounting group already exists"; fi; \
	else \
		echo "❌ Failed to create accounting group (HTTP $$GROUP_CODE)"; \
		echo "Response body: $$GROUP_BODY"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Adding mcp user to accounting group..."; \
	USERS_LIST=$$(curl -s -X GET "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/users?username=mcp" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	USER_ID=$$(echo "$$USERS_LIST" | jq -r '.[0].id // empty' 2>/dev/null); \
	if [ -z "$$USER_ID" ] || [ "$$USER_ID" = "null" ]; then \
		echo "❌ Failed to find mcp user"; \
		exit 1; \
	fi; \
	GROUPS_LIST=$$(curl -s -X GET "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/groups" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	GROUP_ID=$$(echo "$$GROUPS_LIST" | jq -r '.[] | select(.name == "accounting") | .id // empty' 2>/dev/null); \
	if [ -z "$$GROUP_ID" ] || [ "$$GROUP_ID" = "null" ]; then \
		echo "❌ Failed to find accounting group"; \
		exit 1; \
	fi; \
	ADD_USER_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X PUT "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/users/$$USER_ID/groups/$$GROUP_ID" \
		-H "Authorization: Bearer $$TOKEN"); \
	ADD_USER_CODE=$$(echo "$$ADD_USER_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$ADD_USER_CODE" = "204" ]; then \
		echo "✅ MCP user added to accounting group"; \
	else \
		echo "❌ Failed to add user to group (HTTP $$ADD_USER_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Creating groups client scope..."; \
	echo "Request payload: {\"name\":\"groups\",\"protocol\":\"openid-connect\",\"attributes\":{\"display.on.consent.screen\":\"false\",\"include.in.token.scope\":\"true\"}}"; \
	SCOPE_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"name":"groups","protocol":"openid-connect","attributes":{"display.on.consent.screen":"false","include.in.token.scope":"true"}}'); \
	SCOPE_CODE=$$(echo "$$SCOPE_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	SCOPE_BODY=$$(echo "$$SCOPE_RESPONSE" | sed 's/HTTPCODE:[0-9]*$$//'); \
	echo "Response code: $$SCOPE_CODE"; \
	echo "Response body: $$SCOPE_BODY"; \
	if [ "$$SCOPE_CODE" = "201" ] || [ "$$SCOPE_CODE" = "409" ]; then \
		if [ "$$SCOPE_CODE" = "201" ]; then echo "✅ Groups client scope created"; \
		else echo "✅ Groups client scope already exists"; fi; \
	else \
		echo "❌ Failed to create groups client scope (HTTP $$SCOPE_CODE)"; \
		echo "Response body: $$SCOPE_BODY"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Reading back groups client scope for debugging..."; \
	SCOPES_LIST=$$(curl -s -X GET "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	SCOPE_DETAILS=$$(echo "$$SCOPES_LIST" | jq '.[] | select(.name == "groups")' 2>/dev/null); \
	echo "Client scope details:"; \
	echo "$$SCOPE_DETAILS"; \
	echo ""; \
	echo "Adding groups mapper to client scope..."; \
	SCOPES_LIST=$$(curl -s -X GET "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	SCOPE_ID=$$(echo "$$SCOPES_LIST" | jq -r '.[] | select(.name == "groups") | .id // empty' 2>/dev/null); \
	if [ -z "$$SCOPE_ID" ] || [ "$$SCOPE_ID" = "null" ]; then \
		echo "❌ Failed to find groups client scope"; \
		exit 1; \
	fi; \
	MAPPER_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/client-scopes/$$SCOPE_ID/protocol-mappers/models" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"name":"groups","protocol":"openid-connect","protocolMapper":"oidc-group-membership-mapper","config":{"claim.name":"groups","full.path":"false","id.token.claim":"true","access.token.claim":"true","userinfo.token.claim":"true"}}'); \
	MAPPER_CODE=$$(echo "$$MAPPER_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	MAPPER_BODY=$$(echo "$$MAPPER_RESPONSE" | sed 's/HTTPCODE:[0-9]*$$//'); \
	if [ "$$MAPPER_CODE" = "201" ] || [ "$$MAPPER_CODE" = "409" ]; then \
		if [ "$$MAPPER_CODE" = "201" ]; then echo "✅ Groups mapper added to client scope"; \
		else echo "✅ Groups mapper already exists"; fi; \
	else \
		echo "❌ Failed to create groups mapper (HTTP $$MAPPER_CODE)"; \
		echo "Response body: $$MAPPER_BODY"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Adding groups client scope to realm's optional client scopes..."; \
	ADD_OPTIONAL_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X PUT "http://keycloak.127-0-0-1.sslip.io:8889/admin/realms/mcp/default-optional-client-scopes/$$SCOPE_ID" \
		-H "Authorization: Bearer $$TOKEN"); \
	ADD_OPTIONAL_CODE=$$(echo "$$ADD_OPTIONAL_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	ADD_OPTIONAL_BODY=$$(echo "$$ADD_OPTIONAL_RESPONSE" | sed 's/HTTPCODE:[0-9]*$$//'); \
	if [ "$$ADD_OPTIONAL_CODE" = "204" ] || [ "$$ADD_OPTIONAL_CODE" = "409" ]; then \
		if [ "$$ADD_OPTIONAL_CODE" = "204" ]; then echo "✅ Groups client scope added to realm's optional client scopes"; \
		else echo "✅ Groups client scope already in realm's optional client scopes"; fi; \
	else \
		echo "❌ Failed to add client scope to realm's optional scopes (HTTP $$ADD_OPTIONAL_CODE)"; \
		echo "Response body: $$ADD_OPTIONAL_BODY"; \
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
	echo "Email: mcp@example.com"; \
	echo "Group: accounting (with mcp user as member)"; \
	echo "Client Scope: groups (optional, with Group Membership mapper)"; \
	echo "Session idle timeout: 15 minutes"

