# Virtual MCP Servers Configuration

This guide covers configuring virtual MCP servers to create focused, curated tool collections from your aggregated MCP servers.

## Overview

Virtual MCP servers solve a common problem when using MCP Gateway: while aggregating all your MCP tools centrally provides excellent benefits for authentication, authorization, and configuration management, it can overwhelm LLMs and AI agents with too many tools to choose from.

**Why use virtual MCP servers:**
- **Focused Tool Sets**: Create specialized collections of tools for specific use cases
- **Improved AI Performance**: Reduce cognitive load on LLMs by presenting fewer, more relevant tools
- **Domain-Specific Interfaces**: Group tools by function (e.g., "development tools", "data analysis tools")
- **Simplified Discovery**: Make it easier for users and agents to find the right tools
- **Layered Access Control**: Combine with authorization policies for fine-grained access management

Virtual servers work by filtering the complete tool list based on a curated selection, accessed via HTTP headers.

## Prerequisites

- MCP Gateway installed and configured
- [MCP servers configured](./configure-mcp-servers.md) with tools available
- [kubectl](https://kubernetes.io/docs/tasks/tools/) installed

## Understanding Virtual Servers

A virtual MCP server is defined by an `MCPVirtualServer` custom resource that specifies:
- **Tool Selection**: Which tools from the aggregated pool to expose
- **Description**: Human-readable description of the virtual server's purpose
- **Access Method**: Accessed via `X-Mcp-Virtualserver` header with `namespace/name` format

When a client includes the virtual server header, MCP Gateway filters responses to only include the specified tools.

## Step 1: Create Virtual Server Definitions

Create virtual servers for different use cases using tools from your configured MCP servers:

### Development Tools Virtual Server

```bash
kubectl apply -f - <<EOF
apiVersion: mcp.kagenti.com/v1alpha1
kind: MCPVirtualServer
metadata:
  name: dev-tools
  namespace: mcp-system
spec:
  description: "Development and debugging tools"
  tools:
  - test_hello_world      # Example: replace with your actual tool names
  - test_headers
  - github_get_me
  - github_list_repos
EOF
```

### Data Analysis Virtual Server

```bash
kubectl apply -f - <<EOF
apiVersion: mcp.kagenti.com/v1alpha1
kind: MCPVirtualServer
metadata:
  name: data-tools
  namespace: mcp-system
spec:
  description: "Data analysis and reporting tools"
  tools:
  - test2_time            # Example: replace with your actual tool names
  - test3_dozen
  - github_get_repo_stats
EOF
```

**Important**: Replace the example tool names above with actual tools from your configured MCP servers. To see available tools, run:

```bash
curl -X POST http://mcp.127-0-0-1.sslip.io:8888/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}' | jq '.result.tools[].name'
```

**Configuration Explained:**
- `name`: Identifier for the virtual server (used in header)
- `namespace`: Kubernetes namespace (typically `mcp-system`)
- `description`: Human-readable purpose description
- `tools`: Array of tool names to include (must match exactly the tool names from your configured MCP servers)

## Step 2: Verify Virtual Server Creation

Check that your virtual servers were created successfully:

```bash
# List all virtual servers
kubectl get mcpvirtualserver -A
```

## Step 3: Test Virtual Server Access

Test your virtual servers using curl with the appropriate header:

### Test Development Tools Virtual Server

```bash
# Request tools from the dev-tools virtual server
curl -X POST http://mcp.127-0-0-1.sslip.io:8888/mcp \
  -H "Content-Type: application/json" \
  -H "X-Mcp-Virtualserver: mcp-system/dev-tools" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```

**Expected Response**: Only tools specified in the `dev-tools` virtual server (the example tools you configured)

### Test Data Analysis Virtual Server

```bash
# Request tools from the data-tools virtual server
curl -X POST http://mcp.127-0-0-1.sslip.io:8888/mcp \
  -H "Content-Type: application/json" \
  -H "X-Mcp-Virtualserver: mcp-system/data-tools" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```

**Expected Response**: Only tools specified in the `data-tools` virtual server (the example tools you configured)

### Test Without Virtual Server Header

```bash
# Request all available tools (no filtering)
curl -X POST http://mcp.127-0-0-1.sslip.io:8888/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```

**Expected Response**: All tools from all configured MCP servers

## Step 4: Use with MCP Inspector

You can also test virtual servers using the MCP Inspector by setting the virtual server header. The MCP Inspector allows you to configure custom headers for testing different virtual server configurations.

## Combining with Authorization

Virtual servers work excellently with [authorization policies](./authorization.md). You can:

1. **Use virtual servers** to create focused tool collections
2. **Use authorization policies** to control who can access which virtual servers
3. **Layer both approaches** for fine-grained access control

Example authorization flow:
- **Virtual Server**: `dev-tools` contains development-related tools
- **Authorization Policy**: Only users in `developers` group can access `dev-tools` virtual server
- **Result**: Developers see only development tools they're authorized to use

## Management and Updates

### Update Virtual Server Tool Lists

```bash
# Update an existing virtual server (replace with your actual tool names)
kubectl patch mcpvirtualserver dev-tools -n mcp-system --type='merge' -p='
{
  "spec": {
    "tools": [
      "your_actual_tool_1",
      "your_actual_tool_2", 
      "your_actual_tool_3",
      "your_new_tool"
    ]
  }
}'
```

### Remove Virtual Servers

```bash
# Delete a virtual server
kubectl delete mcpvirtualserver dev-tools -n mcp-system
```

## Verification and Monitoring

### Check Virtual Server Status

```bash
# View all virtual servers and their configurations
kubectl get mcpvirtualserver -A -o yaml

# Check controller logs for virtual server processing
kubectl logs -n mcp-system deployment/mcp-gateway-controller | grep -i virtual
```

### Validate Tool Availability

```bash
# Verify that tools specified in virtual servers actually exist
# First get all available tools
curl -X POST http://mcp.127-0-0-1.sslip.io:8888/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}' | jq '.result.tools[].name'

# Then compare with your virtual server configurations
kubectl get mcpvirtualserver -A -o jsonpath='{range .items[*]}{.metadata.name}: {.spec.tools}{"\n"}{end}'
```

## Next Steps

With virtual MCP servers configured, you can:
- **[Configure Authorization](./authorization.md)** - Add access control to virtual servers
- **[External MCP Servers](./external-mcp-server.md)** - Include external tools in virtual servers
