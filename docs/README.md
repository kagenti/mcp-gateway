# MCP Gateway Documentation

## Get Started

**Want to try MCP Gateway?** Start here: **[Installation Guide](./guides/how-to-install-and-configure.md)**

This guide walks you through setting up MCP Gateway on Kubernetes and connecting your first MCP servers.

## Guides and Tutorials

The [`./guides/`](./guides/) folder contains step-by-step instructions for installing, configuring, and using MCP Gateway:

**Essential Setup:**
- [Installation and Configuration](./guides/how-to-install-and-configure.md) - Get MCP Gateway running
- [Configure Gateway Routing](./guides/configure-mcp-gateway-listener-and-router.md) - Set up traffic routing
- [Configure MCP Servers](./guides/configure-mcp-servers.md) - Connect internal servers
- [External MCP Servers](./guides/external-mcp-server.md) - Connect to external APIs

**Advanced Features:**
- [Authentication](./guides/authentication.md) - OAuth-based security
- [Authorization](./guides/authorization.md) - Fine-grained access control
- [Virtual MCP Servers](./guides/virtual-mcp-servers.md) - Focused tool collections
- [Standalone Installation](./guides/binary-install.md) - Non-Kubernetes deployment
- [Troubleshooting](./guides/troubleshooting.md) - Common issues and solutions

## Architecture and Design

The [`./design/`](./design/) folder explains the architecture, design principles, and component responsibilities:

- [Architecture Overview](./design/overview.md) - High-level system design
- [Authentication Design](./design/auth-phase-1.md) - Security architecture
- [Routing Design](./design/routing.md) - Traffic routing concepts
- [Request Flows](./design/flows.md) - How requests are processed

## Development and Internals

The [`./dev/`](./dev/) folder covers code architecture and development workflows:

- [Understanding Architecture](./dev/understanding-mcp-gateway-architecture.md) - Explore running system components
