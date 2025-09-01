package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=mcpsrv

// MCPServer defines the configuration for an MCP Server
type MCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerSpec   `json:"spec,omitempty"`
	Status MCPServerStatus `json:"status,omitempty"`
}

// MCPServerSpec defines the desired state of MCPServer
type MCPServerSpec struct {
	// TargetRefs references the HTTPRoutes for MCP servers
	TargetRefs []TargetReference `json:"targetRefs"`

	// ToolPrefix is the default prefix to add to all tools (can be overridden per targetRef)
	// +optional
	ToolPrefix string `json:"toolPrefix,omitempty"`
}

// TargetReference identifies an API object to reference, following Gateway API patterns
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

	// ToolPrefix to use for this specific server (overrides spec-level toolPrefix)
	// +optional
	ToolPrefix string `json:"toolPrefix,omitempty"`
}

// MCPServerStatus defines the observed state of MCPServer
type MCPServerStatus struct {
	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// MCPServerList contains a list of MCPServer
type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPServer `json:"items"`
}
