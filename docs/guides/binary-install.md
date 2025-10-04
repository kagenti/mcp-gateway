## Installation Method 2: Standalone Binary

Use this method to run MCP Gateway as a single binary with file-based configuration (no Kubernetes required).

### Prerequisites

- [Go](https://golang.org/doc/install) installed (for building from source)
- [Git](https://git-scm.com/downloads) installed

### Step 1: Download and Build

```bash
# Clone the repository
git clone https://github.com/kagenti/mcp-gateway.git
cd mcp-gateway

# Build the binary
go build -o bin/mcp-broker-router ./cmd/mcp-broker-router
```

**Why build from source**: The binary installation method requires building from source as pre-built binaries are not currently distributed.

### Step 2: Create Configuration File

Create `config/samples/config.yaml`:

```yaml
servers:
  - name: weather-service
    url: http://weather.example.com:8080
    hostname: weather.example.com
    enabled: true
    toolPrefix: "weather_"
  - name: calendar-service
    url: http://calendar.example.com:8080
    hostname: calendar.example.com
    enabled: true
    toolPrefix: "cal_"
```

**Configuration fields:**
- `name`: Identifier for the server
- `url`: Full URL to the MCP server endpoint
- `hostname`: Used for routing decisions
- `enabled`: Whether to include this server
- `toolPrefix`: Prefix added to all tools from this server

### Step 3: Start the Gateway

```bash
# Run the binary with your configuration
./bin/mcp-broker-router --config=config/samples/config.yaml --log-level=-4
```

**Configuration options:**
- `--config`: Path to your YAML configuration file
- `--log-level`: Logging verbosity (-4 for debug, 0 for info, 4 for errors only)

The gateway starts with:
- HTTP broker listening on `0.0.0.0:8080`
- gRPC router listening on `0.0.0.0:50051`

### Step 4: Verify Standalone Installation

```bash
# Check health endpoint
curl http://localhost:8080/health

# List available tools
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}'
```
