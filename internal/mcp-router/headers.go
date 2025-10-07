package mcprouter

import (
	"fmt"

	basepb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

const (
	mcpServerNameHeader = "x-mcp-servername"
	toolHeader          = "x-mcp-toolname"
	methodHeader        = "x-mcp-method"
	//nolint:gosec // not a credential, just a header name
	mcpAPIKeyHeader     = "x-mcp-api-key" // header for backend API keys to avoid OAuth conflicts
	sessionHeader       = "mcp-session-id"
	authorityHeader     = ":authority"
	authorizationHeader = "authorization"
)

// HeadersBuilder builds headers to add to the request or response
type HeadersBuilder struct {
	headers []*basepb.HeaderValueOption
}

// NewHeaders returns a new HeadersBuilder
func NewHeaders() *HeadersBuilder {
	return &HeadersBuilder{
		headers: []*basepb.HeaderValueOption{},
	}
}

// Build will build the header ready to be added to a request or response
func (hb *HeadersBuilder) Build() []*basepb.HeaderValueOption {
	return hb.headers
}

// WithAuthority will set the :authority header
func (hb *HeadersBuilder) WithAuthority(authority string) *HeadersBuilder {
	hb.headers = append(hb.headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      authorityHeader,
			RawValue: []byte(authority),
		},
	})
	return hb
}

// WithAuth will set the authorization header
func (hb *HeadersBuilder) WithAuth(cred string) *HeadersBuilder {
	hb.headers = append(hb.headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      authorizationHeader,
			RawValue: []byte(cred),
		},
	})
	return hb
}

// WithAPIKey will set the x-mcp-api-key header
func (hb *HeadersBuilder) WithAPIKey(apiKey string) *HeadersBuilder {
	hb.headers = append(hb.headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      mcpAPIKeyHeader,
			RawValue: []byte(apiKey),
		},
	})
	return hb
}

// WithContentLength will set the content-length header
func (hb *HeadersBuilder) WithContentLength(length int) *HeadersBuilder {
	hb.headers = append(hb.headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      "content-length",
			RawValue: []byte(fmt.Sprintf("%d", length)),
		},
	})
	return hb
}

// WithMCPToolName will set the x-mcp-toolname header
func (hb *HeadersBuilder) WithMCPToolName(toolName string) *HeadersBuilder {
	hb.headers = append(hb.headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      toolHeader,
			RawValue: []byte(toolName),
		},
	})
	return hb
}

// WithMCPServerName will set the x-mcp-serername header
func (hb *HeadersBuilder) WithMCPServerName(serverName string) *HeadersBuilder {
	hb.headers = append(hb.headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      mcpServerNameHeader,
			RawValue: []byte(serverName),
		},
	})
	return hb
}

// WithMCPMethod will set the x-mcp-method header
func (hb *HeadersBuilder) WithMCPMethod(method string) *HeadersBuilder {
	hb.headers = append(hb.headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      methodHeader,
			RawValue: []byte(method),
		},
	})
	return hb
}

// WithMCPSession will set the mcp-session-id header
func (hb *HeadersBuilder) WithMCPSession(session string) *HeadersBuilder {
	hb.headers = append(hb.headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      sessionHeader,
			RawValue: []byte(session),
		},
	})
	return hb
}

// WithCustomHeader will set key with value in the headers
func (hb *HeadersBuilder) WithCustomHeader(key, value string) *HeadersBuilder {
	hb.headers = append(hb.headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      key,
			RawValue: []byte(value),
		},
	})
	return hb
}

// WithPath will set the :path header
func (hb *HeadersBuilder) WithPath(path string) *HeadersBuilder {
	hb.headers = append(hb.headers, &basepb.HeaderValueOption{
		Header: &basepb.HeaderValue{
			Key:      ":path",
			RawValue: []byte(path),
		},
	})
	return hb
}
