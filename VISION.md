# MCP Gateway Vision

## Problem Statement

Platform teams want to expose MCP servers to agents safely, but today this requires bespoke routing, auth, policy, and observability per server. There is no standard entrypoint, no shared policy model, and no production-grade way to operate MCP at scale.

## Solution

MCP Gateway is a production-grade MCP entrypoint that lets platform teams operate many MCP servers with shared routing, policy, and observability, while tracking upstream standards instead of inventing new ones.
MCP Gateway composes MCP support out of Envoy’s routing and extension mechanisms, rather than introducing a separate MCP-specific proxy layer.

## Who is this for?

* Platform / infra teams running MCP servers at scale
* Teams standardising agent-to-tool access across orgs
* Not intended as a lightweight dev-only proxy

## Non-Goals

* Not replacing Envoy AI Gateway or its broader AI gateway capabilities

## Approach

**Move pragmatically, align with standards.** MCP Gateway implements functionality early while upstream projects refine standards. As those standards mature, we adopt them.
It exists to de-risk early adoption.

We actively engage and/or integrate with:

- **[Gateway API](https://gateway-api.sigs.k8s.io/)** - declarative routing and infrastructure
- **[AI Gateway Working Group](https://github.com/kubernetes-sigs/wg-ai-gateway)** - payload processing, Backend resources
- **[Kube Agentic Networking](https://github.com/kubernetes-sigs/kube-agentic-networking)** - AccessPolicy, agent-to-tool communication standards
- **[Kuadrant](https://kuadrant.io/)** - auth and rate limiting policies; we explore agentic-specific policy patterns here first
- **[Envoy](https://www.envoyproxy.io/)** - routing and payload processing; upstream work on [JSON-RPC/MCP support](https://github.com/envoyproxy/envoy/issues/39174) may replace our ext_proc
- **[Istio](https://istio.io/)** - Gateway API provider with evolving agent-oriented features
- **[Agentic AI Foundation](https://www.linuxfoundation.org/press/linux-foundation-announces-the-formation-of-the-agentic-ai-foundation)** - an open foundation to ensure agentic AI evolves transparently and collaboratively

## Principles

- **Single Entrypoint** - a single MCP entrypoint to many MCP servers
- **Envoy first** - the core router and broker work directly with Envoy; no Kubernetes required
- **Kubernetes adds convenience** - Gateway API and CRDs provide declarative management on top of the Envoy foundation
- **Bring your own policies** - expose metadata for external policy engines

## Outcome

As standards stabilise, MCP Gateway adopts them or steps aside. Until then, teams get usable, production-grade infrastructure today.

### What does this mean in practice?

Note, these are just examples at the time of writing, and are not intended to be goals.

*Example 1:* The MCP Gateway project will provide a Kubernetes CRD, like MCPServer, that represents a running MCP Server somewhere.
This CRD is expected to converge with or be replaced by a 'Backend' resource that is aligned with the outcome of the ai-gatweay working group or agentic-networking sub-project. This Backend resource may get implemented in the Gateway API provider, Istio, in time.

*Example 2:* The MCP Gateway project has some very specific Kuadrant AuthPolicy examples around tool permissions based on integrating with Keycloak. The MCP Gateway project will provide a KeycloakToolRoleMappingPolicy that wraps an AuthPolicy, abstracting the detailed rules configuration required to parse and iterate tools, checking against Keycloak role mappings.

*Example 3:* The MCP Gateway project includes an ext-proc component that parses MCP requests, hoisting information like the tool being called into headers. It also provides MCP server multiplexing by way of tool prefixing. In time, this multiplexing may be a feature available from Envoy proxy. We would look to leverage that feature in Envoy proxy instead of our own ext-proc at that time.

## Why not use...

### ...standalone MCP Servers? (aka. the case for an MCP Gateway, in general)

- Each MCP server must independently implement auth, policy, observability, and security hardening  
- Agents must integrate with multiple endpoints instead of a single, stable entrypoint  
- Limitations around multi-tenancy and shared governance across teams  
- Scaling the number of servers scales operational complexity linearly

### ...Solo AgentGateway?

- Introduces a new MCP-aware proxy with its own data plane and lifecycle
- MCP routing, policy, and semantics are implemented outside native Envoy mechanisms
- Long-term evolution tied to AgentGateway and kgateway roadmaps

### ...Envoy AI Gateway?

- Typically requires adopting Envoy AI Gateway *and* Envoy Gateway as the Gateway API provider
- MCP support is implemented via a Go-based MCP proxy, limiting reuse by downstream extensions (for example, auth and policy)
- Running the MCP data plane standalone requires static configuration and introduces coupling to AI Gateway–specific MCP semantics

### ...mcp-context-forge?

- Not Gateway API–native; does not integrate with standard Kubernetes ingress/routing primitives
- Not Kubernetes-native; operates outside cluster-level traffic and policy models
- Introduces a separate control and data plane rather than composing with Envoy
- Implemented in Python, making it less aligned with Envoy and Kubernetes native gateway ecosystems
