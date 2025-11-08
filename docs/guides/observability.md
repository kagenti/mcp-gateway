# Observability Guide

## Adding observability stack

```Makefile
make observability-setup
```

This will setup Loki, Grafana Alloy, and Grafana Dashboard in your cluster.

## Adding Grafana Dashboard

1. Go to Dashboards, create -> add visualization.
2. Set the type to "Bar Chart".
3. Add four queries using the code editor

```logql
# Query A
sum by (destructive) (
  count_over_time({namespace="mcp-system"} |= `x-mcp-annotation-hints` 
  | regexp `x-mcp-annotation-hints\\"[^\"]*raw_value:\\\"(?P<annotation_hints>[^\\\"]+)\\\"` 
  | regexp `"readOnly=(?P<readOnly>[^,]+),destructive=(?P<destructive>[^,]+),idempotent=(?P<idempotent>[^,]+),openWorld=(?P<openWorld>[^\"]+)"` 
  | destructive = "true" [$__auto])
)

# Query B
sum by (readOnly) (
  count_over_time({namespace="mcp-system"} |= `x-mcp-annotation-hints` 
  | regexp `x-mcp-annotation-hints\\"[^\"]*raw_value:\\\"(?P<annotation_hints>[^\\\"]+)\\\"` 
  | regexp `"readOnly=(?P<readOnly>[^,]+),destructive=(?P<destructive>[^,]+),idempotent=(?P<idempotent>[^,]+),openWorld=(?P<openWorld>[^\"]+)"` 
  | readOnly = "false" [$__auto])
)

# Query C
sum by (idempotent) (
  count_over_time({namespace="mcp-system"} |= `x-mcp-annotation-hints` 
  | regexp `x-mcp-annotation-hints\\"[^\"]*raw_value:\\\"(?P<annotation_hints>[^\\\"]+)\\\"` 
  | regexp `"readOnly=(?P<readOnly>[^,]+),destructive=(?P<destructive>[^,]+),idempotent=(?P<idempotent>[^,]+),openWorld=(?P<openWorld>[^\"]+)"` 
  | idempotent = "false" [$__auto])
)

# Query D
sum by (openWorld) (

  count_over_time({namespace="mcp-system"} |= `x-mcp-annotation-hints` 
  | regexp `x-mcp-annotation-hints\\"[^\"]*raw_value:\\\"(?P<annotation_hints>[^\\\"]+)\\\"` 
  | regexp `"readOnly=(?P<readOnly>[^,]+),destructive=(?P<destructive>[^,]+),idempotent=(?P<idempotent>[^,]+),openWorld=(?P<openWorld>[^\"]+)"` 
  | openWorld = "true\\" [$__auto])
)
```

4. Press "Run queries" to see the results.

This visualization shows you the number of true or false hints that are set coming out of the mcp-system namespace. This mostly involves logs coming from the mcp-router and relies on the INFO log that emits the headers when the response is being set in the EXT_PROC module for Envoy.

Example log line this visualization reads from:

```
time=2025-11-08T21:41:34.147Z level=INFO msg="Sending MCP body routing instructions to Envoy: request_body:{response:{header_mutation:{set_headers:{header:{key:\"x-mcp-method\"  raw_value:\"tools/call\"}}  set_headers:{header:{key:\"x-mcp-annotation-hints\"  raw_value:\"readOnly=false,destructive=true,idempotent=false,openWorld=true\"}}  set_headers:{header:{key:\"x-mcp-toolname\"  raw_value:\"headers\"}}  set_headers:{header:{key:\"x-mcp-servername\"  raw_value:\"mcp-test/mcp-server2-route\"}}  set_headers:{header:{key:\"mcp-session-id\"  raw_value:\"mcp-session-f4c2a956-b3cc-4a80-b583-ae08a760e63b\"}}  set_headers:{header:{key:\":authority\"  raw_value:\"mcp-server-2\"}}  set_headers:{header:{key:\"content-length\"  raw_value:\"119\"}}}  body_mutation:{body:\"{\\\"id\\\":11,\\\"jsonrpc\\\":\\\"2.0\\\",\\\"method\\\":\\\"tools/call\\\",\\\"params\\\":{\\\"_meta\\\":{\\\"progressToken\\\":11},\\\"arguments\\\":{},\\\"name\\\":\\\"headers\\\"}}\"}  clear_route_cache:true}}"
```
