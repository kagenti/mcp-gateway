package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=mcpsrv

// MCPServer defines a collection of MCP (Model Context Protocol) servers to be aggregated by the gateway.
// It enables discovery and federation of tools from multiple backend MCP servers through HTTPRoute references,
// providing a declarative way to configure which MCP servers should be accessible through the gateway.
type MCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerSpec   `json:"spec,omitempty"`
	Status MCPServerStatus `json:"status,omitempty"`
}

// MCPServerSpec defines the desired state of MCPServer.
// It specifies which HTTPRoutes point to MCP servers and how their tools should be federated.
type MCPServerSpec struct {
	// TargetRef specifies an HTTPRoute that points to a backend MCP server.
	// The referenced HTTPRoute should have backend services that implement the MCP protocol.
	// The controller will discover the backend service from this HTTPRoute and configure
	// the broker to federate tools from that MCP server.
	TargetRef TargetReference `json:"targetRef"`

	// ToolPrefix is the prefix to add to all federated tools from referenced servers.
	// This helps avoid naming conflicts when aggregating tools from multiple sources.
	// For example, if two servers both provide a 'search' tool, prefixes like 'server1_' and 'server2_'
	// ensure they can coexist as 'server1_search' and 'server2_search'.
	// +optional
	ToolPrefix string `json:"toolPrefix,omitempty"`
}

// TargetReference identifies an HTTPRoute that points to MCP servers.
// It follows Gateway API patterns for cross-resource references.
type TargetReference struct {
	// Group is the group of the target resource.
	// +kubebuilder:default=gateway.networking.k8s.io
	// +kubebuilder:validation:Enum=gateway.networking.k8s.io
	Group string `json:"group"`

	// Kind is the kind of the target resource.
	// +kubebuilder:default=HTTPRoute
	// +kubebuilder:validation:Enum=HTTPRoute
	Kind string `json:"kind"`

	// Name is the name of the target resource.
	Name string `json:"name"`

	// Namespace of the target resource (optional, defaults to same namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// MCPServerStatus represents the observed state of the MCPServer resource.
// It contains conditions that indicate whether the referenced servers have been
// successfully discovered and are ready for use.
type MCPServerStatus struct {
	// Conditions represent the latest available observations of the MCPServer's state.
	// Common conditions include 'Ready' to indicate if all referenced servers are accessible.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// MCPServerList contains a list of MCPServer
type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPServer `json:"items"`
}
