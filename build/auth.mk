##@ Auth Examples

.PHONY: oauth-acl-example-setup
oauth-acl-example-setup: ## Setup auth example based on OAuth2 - permissions managed with an external Access Control List provider (requires: make local-env-setup)
	@echo "========================================="
	@echo "Setting up OAuth Example"
	@echo "========================================="
	@echo "Prerequisites: make local-env-setup should be completed"
	@echo ""
	@echo "Step 1/4: Configuring OAuth environment variables..."
	@kubectl set env deployment/mcp-broker-router \
		OAUTH_RESOURCE_NAME="MCP Server" \
		OAUTH_RESOURCE="http://mcp.127-0-0-1.sslip.io:8888/mcp" \
		OAUTH_AUTHORIZATION_SERVERS="http://keycloak.127-0-0-1.sslip.io:8889/realms/mcp" \
		OAUTH_BEARER_METHODS_SUPPORTED="header" \
		OAUTH_SCOPES_SUPPORTED="basic,groups" \
		-n mcp-system
	@echo "✅ OAuth environment variables configured"
	@echo ""
	@echo "Step 2/4: Applying AuthPolicy configurations..."
	@kubectl apply -f ./config/samples/oauth-acl/tools-list-auth.yaml
	@kubectl apply -f ./config/samples/oauth-acl/tools-call-auth.yaml
	@echo "✅ AuthPolicy configurations applied"
	@echo ""
	@echo "Step 3/4: Configuring CORS rules for the OpenID Connect Client Registration endpoint..."
	@kubectl apply -f ./config/keycloak/preflight_envoyfilter.yaml
	@kubectl -n mcp-system apply -k ./config/example-access-control/
	@echo "✅ CORS configured"
	@echo ""
	@echo "Step 4/4: Patch Authorino deployment to resolve external Keycloak host name..."
	@./utils/patch-authorino-keycloak-hostname.sh
	@echo "✅ Authorino deployment patched"
	@echo ""
	@echo "🎉 OAuth example setup complete!"
	@echo ""
	@echo "The mcp-broker now serves OAuth discovery information at:"
	@echo "  /.well-known/oauth-protected-resource"
	@echo ""
	@echo "Next step: Open MCP Inspector with 'make inspect-gateway'"
	@echo "and go through the OAuth flow with credentials: mcp/mcp"

.PHONY: oauth-token-exchange-example-setup
oauth-token-exchange-example-setup: ## Setup auth example based on OAuth2 with Token Exchange automatically handled for the tools/call request – permissions stored in the Keycloak server (requires: make local-env-setup)
	@echo "========================================="
	@echo "Setting up OAuth Example"
	@echo "========================================="
	@echo "Prerequisites: make local-env-setup should be completed"
	@echo ""
	@echo "Step 1/4: Configuring OAuth environment variables..."
	@kubectl set env deployment/mcp-broker-router \
		OAUTH_RESOURCE_NAME="MCP Server" \
		OAUTH_RESOURCE="http://mcp.127-0-0-1.sslip.io:8888/mcp" \
		OAUTH_AUTHORIZATION_SERVERS="http://keycloak.127-0-0-1.sslip.io:8889/realms/mcp" \
		OAUTH_BEARER_METHODS_SUPPORTED="header" \
		OAUTH_SCOPES_SUPPORTED="basic,groups,roles" \
		-n mcp-system
	@echo "✅ OAuth environment variables configured"
	@echo ""
	@echo "Step 2/4: Applying AuthPolicy configurations..."
	@kubectl apply -k ./config/samples/oauth-token-exchange/
	@kubectl patch deployment/mcp-broker-router \
  	-n mcp-system \
  	--type='json' \
  	-p='[{"op": "add", "path": "/spec/template/spec/containers/0/env/-", "value": {"name": "TRUSTED_HEADER_PUBLIC_KEY", "valueFrom": {"secretKeyRef": {"name": "mcp-config-update-token","key": "token"}}}}]'
	@echo "✅ AuthPolicy configurations applied"
	@echo ""
	@echo "Step 3/4: Configuring CORS rules for the OpenID Connect Client Registration endpoint..."
	@kubectl apply -f ./config/keycloak/preflight_envoyfilter.yaml
	@echo "✅ CORS configured"
	@echo ""
	@echo "Step 4/4: Patch Authorino deployment to resolve external Keycloak host name..."
	@./utils/patch-authorino-keycloak-hostname.sh
	@echo "✅ Authorino deployment patched"
	@echo ""
	@echo "🎉 OAuth example setup complete!"
	@echo ""
	@echo "The mcp-broker now serves OAuth discovery information at:"
	@echo "  /.well-known/oauth-protected-resource"
	@echo ""
	@echo "Next step: Open MCP Inspector with 'make inspect-gateway'"
	@echo "and go through the OAuth flow with credentials: mcp/mcp"
