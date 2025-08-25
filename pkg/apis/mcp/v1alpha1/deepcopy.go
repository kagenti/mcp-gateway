package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto copies the receiver, writing into out. in must be non-nil.
func (in *MCPGateway) DeepCopyInto(out *MCPGateway) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy copies the receiver, creating a new MCPGateway.
func (in *MCPGateway) DeepCopy() *MCPGateway {
	if in == nil {
		return nil
	}
	out := new(MCPGateway)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject copies the receiver, creating a new runtime.Object.
func (in *MCPGateway) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver, writing into out. in must be non-nil.
func (in *MCPGatewayList) DeepCopyInto(out *MCPGatewayList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]MCPGateway, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy copies the receiver, creating a new MCPGatewayList.
func (in *MCPGatewayList) DeepCopy() *MCPGatewayList {
	if in == nil {
		return nil
	}
	out := new(MCPGatewayList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject copies the receiver, creating a new runtime.Object.
func (in *MCPGatewayList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver, writing into out. in must be non-nil.
func (in *MCPGatewaySpec) DeepCopyInto(out *MCPGatewaySpec) {
	*out = *in
	if in.TargetRefs != nil {
		in, out := &in.TargetRefs, &out.TargetRefs
		*out = make([]TargetReference, len(*in))
		copy(*out, *in)
	}
}

// DeepCopyInto copies the receiver, writing into out. in must be non-nil.
func (in *MCPGatewayStatus) DeepCopyInto(out *MCPGatewayStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}
