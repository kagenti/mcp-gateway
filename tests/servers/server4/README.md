# Server4 - "Bad" Test Server

This is an intentionally problematic MCP server designed to **fail validation checks** for testing the MCP Gateway's error handling and status reporting.

## Purpose

Server4 simulates various failure scenarios to test that the MCP Gateway validation system correctly:

1. **Detects connectivity failures**
2. **Identifies capability mismatches**
3. **Catches protocol version issues**
4. **Reports specific server failures in status**

## Failure Modes

Server4 supports different failure modes via the `FAILURE_MODE` environment variable:

### `protocol` (default)

- Returns **wrong protocol version** (`2024-11-05` instead of `2025-06-18`)
- Should fail `validateProtocolAndTransport()` check
- Status should show: `"mcp-test/mcp-server4-route: connection failed"`

### `no-tools`

- Valid protocol but **missing tools capability**
- Should fail `validateCapabilities()` check
- Server connects but has no tools

### `tool-conflicts`

- **Provides tools with conflicting names**
- Tools: `time`, `slow`, `headers` (same as other test servers)
- Should fail global tool conflict detection
- When combined with same prefixes as other servers, creates naming conflicts

## Usage

```bash
# Build the bad server image
docker build -t ghcr.io/kagenti/mcp-gateway/test-server4:latest .

# Test different failure modes
docker run -e FAILURE_MODE=protocol test-server4:latest
docker run -e FAILURE_MODE=no-tools test-server4:latest
docker run -e FAILURE_MODE=tool-conflicts test-server4:latest
```

## Integration with MCP Gateway Tests

When deployed to Kubernetes, server4 will:

1. **Fail validation checks** based on configured failure mode
2. **Appear in MCPServer status** as a failing server
3. **Trigger error reporting** in controller logs
4. **Show specific failure details** in status messages

This helps verify that the validation system works correctly and provides useful error information to operators.
