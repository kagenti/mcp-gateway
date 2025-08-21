package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=mcpgw

// MCPGateway defines the configuration for an MCP Gateway
type MCPGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPGatewaySpec   `json:"spec,omitempty"`
	Status MCPGatewayStatus `json:"status,omitempty"`
}

// MCPGatewaySpec defines the desired state of MCPGateway
type MCPGatewaySpec struct {
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

// MCPGatewayStatus defines the observed state of MCPGateway
type MCPGatewayStatus struct {
	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// MCPGatewayList contains a list of MCPGateway
type MCPGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPGateway `json:"items"`
}
