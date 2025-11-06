package mcprouter

import (
	"fmt"

	basepb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	eppb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typepb "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

// ResponseBuilder builds envoy external processor responses
type ResponseBuilder struct {
	response []*eppb.ProcessingResponse
}

// WithRequestHeadersReponse adds a request headers response with header mutations, clears route cache
func (rb *ResponseBuilder) WithRequestHeadersReponse(headers []*basepb.HeaderValueOption) *ResponseBuilder {
	rb.response = append(rb.response, &eppb.ProcessingResponse{
		Response: &eppb.ProcessingResponse_RequestHeaders{
			RequestHeaders: &eppb.HeadersResponse{
				Response: &eppb.CommonResponse{
					ClearRouteCache: true,
					HeaderMutation: &eppb.HeaderMutation{
						SetHeaders: headers,
					},
				},
			},
		},
	})
	return rb
}

// WithRequestBodyHeadersAndBodyReponse adds request body response with header and body mutations, clears route cache
func (rb *ResponseBuilder) WithRequestBodyHeadersAndBodyReponse(headers []*basepb.HeaderValueOption, body []byte) *ResponseBuilder {
	rb.response = append(rb.response, &eppb.ProcessingResponse{
		Response: &eppb.ProcessingResponse_RequestBody{
			RequestBody: &eppb.BodyResponse{
				Response: &eppb.CommonResponse{
					// Necessary so that the new headers are used in the routing decision.
					ClearRouteCache: true,
					HeaderMutation: &eppb.HeaderMutation{
						SetHeaders: headers,
					},
					BodyMutation: &eppb.BodyMutation{
						Mutation: &eppb.BodyMutation_Body{
							Body: body,
						},
					},
				},
			},
		},
	})
	return rb
}

// WithRequestBodyHeadersResponse adds request body response with header mutations only, clears route cache
func (rb *ResponseBuilder) WithRequestBodyHeadersResponse(headers []*basepb.HeaderValueOption) *ResponseBuilder {
	rb.response = append(rb.response, &eppb.ProcessingResponse{
		Response: &eppb.ProcessingResponse_RequestBody{
			RequestBody: &eppb.BodyResponse{
				Response: &eppb.CommonResponse{
					// Necessary so that the new headers are used in the routing decision.
					ClearRouteCache: true,
					HeaderMutation: &eppb.HeaderMutation{
						SetHeaders: headers,
					},
				},
			},
		},
	})
	return rb
}

// WithImmediateResponse adds an immediate error response that terminates request processing
func (rb *ResponseBuilder) WithImmediateResponse(statusCode int32, message string) *ResponseBuilder {
	rb.response = append(rb.response, &eppb.ProcessingResponse{
		Response: &eppb.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &eppb.ImmediateResponse{
				Status: &typepb.HttpStatus{
					Code: typepb.StatusCode(statusCode),
				},
				Body:    []byte(message),
				Details: fmt.Sprintf("ext-proc error: %s", message),
			},
		},
	})
	return rb
}

// WithStreamingResponse adds a streaming request body response with headers
func (rb *ResponseBuilder) WithStreamingResponse(headers []*basepb.HeaderValueOption, body []byte) *ResponseBuilder {
	rb.response = append(rb.response, &eppb.ProcessingResponse{
		Response: &eppb.ProcessingResponse_RequestBody{
			RequestBody: &eppb.BodyResponse{
				Response: &eppb.CommonResponse{
					HeaderMutation: &eppb.HeaderMutation{SetHeaders: headers},
					BodyMutation: &eppb.BodyMutation{
						Mutation: &eppb.BodyMutation_StreamedResponse{
							StreamedResponse: &eppb.StreamedBodyResponse{
								Body:        body,
								EndOfStream: true,
							},
						},
					},
				},
			},
		},
	})
	return rb
}

// WithDoNothingResponse adds an empty response that allows request to continue unmodified
func (rb *ResponseBuilder) WithDoNothingResponse(isStreaming bool) *ResponseBuilder {
	if isStreaming {
		rb.response = append(rb.response, &eppb.ProcessingResponse{
			Response: &eppb.ProcessingResponse_RequestHeaders{
				RequestHeaders: &eppb.HeadersResponse{},
			},
		})
	} else {
		rb.response = append(rb.response, &eppb.ProcessingResponse{
			Response: &eppb.ProcessingResponse_RequestBody{
				RequestBody: &eppb.BodyResponse{},
			},
		})
	}

	return rb
}

// Build returns the accumulated processing responses
func (rb *ResponseBuilder) Build() []*eppb.ProcessingResponse {
	return rb.response
}

// NewResponse creates a new response builder
func NewResponse() *ResponseBuilder {
	return &ResponseBuilder{
		response: []*eppb.ProcessingResponse{},
	}
}
