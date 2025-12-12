# MCP Gateway Vision

MCP Gateway enables platform teams to expose MCP servers with production-grade infrastructure, using Gateway API and Envoy.

## Approach

**Move fast, align with standards.** MCP Gateway implements functionality early while upstream projects refine standards. As those standards mature, we adopt them.

We actively engage and/or integrate with:

- **[Gateway API](https://gateway-api.sigs.k8s.io/)** - declarative routing and infrastructure
- **[AI Gateway Working Group](https://github.com/kubernetes-sigs/wg-ai-gateway)** - payload processing, Backend resources
- **[Kube Agentic Networking](https://github.com/kubernetes-sigs/kube-agentic-networking)** - AccessPolicy, agent-to-tool communication standards
- **[Kuadrant](https://kuadrant.io/)** - auth and rate limiting policies; we explore agentic-specific policy patterns here first
- **[Envoy](https://www.envoyproxy.io/)** - routing and payload processing; upstream work on [JSON-RPC/MCP support](https://github.com/envoyproxy/envoy/issues/39174) may replace our ext_proc
- **[Istio](https://istio.io/)** - Gateway API provider with evolving agent-oriented features

## Principles

- **Envoy first** - the core router and broker work directly with Envoy; no Kubernetes required
- **Kubernetes adds convenience** - Gateway API and CRDs provide declarative management on top of the Envoy foundation
- **Bring your own policies** - expose metadata for external policy engines
- **Federate tools** - aggregate multiple MCP servers behind a single endpoint

## Outcome

When standards firm up, MCP Gateway adopts them. Until then, teams get working infrastructure today.

### What does this mean in practice?

Note, these are just examples at the time of writing, and are not intended to be goals.

*Example 1:* The MCP Gateway project will provide a kubernetes CRD, like MCPServer, that represents a running MCP Server somewhere.
This CRD may eventually evolve into or be replaced by a 'Backend' resource that is aligned with the outcome of the ai-gatweay working group or agentic-networking sub-project. This Backend resource may get implemented in the Gateway API provider, Istio, in time.

*Example 2:* The MCP Gateway project has some very specific Kuadrant AuthPolicy examples around tool permissions based on integrating with Keycloak. The MCP Gateway project will provide a KeycloakToolRoleMappingPolicy that wraps an AuthPolicy, abstracting the detailed rules configuration required to parse and iterate tools, checking against keycloak role mappings.

*Example 3:* The MCP Gateway project includes an ext-proc component that parses MCP requests, hoisting information like the tool being called into headers. It also provides MCP server multiplexing by way of tool prefixing. In time, this multiplexing may be a feature available from Envoy proxy. We would look to leverage that feature in Envoy proxy instead of our own ext-proc at that time.
