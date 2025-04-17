//go:build !ignore_autogenerated

/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1beta2

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IntegrationTestScenario) DeepCopyInto(out *IntegrationTestScenario) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IntegrationTestScenario.
func (in *IntegrationTestScenario) DeepCopy() *IntegrationTestScenario {
	if in == nil {
		return nil
	}
	out := new(IntegrationTestScenario)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *IntegrationTestScenario) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IntegrationTestScenarioCustomDefaulter) DeepCopyInto(out *IntegrationTestScenarioCustomDefaulter) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IntegrationTestScenarioCustomDefaulter.
func (in *IntegrationTestScenarioCustomDefaulter) DeepCopy() *IntegrationTestScenarioCustomDefaulter {
	if in == nil {
		return nil
	}
	out := new(IntegrationTestScenarioCustomDefaulter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IntegrationTestScenarioList) DeepCopyInto(out *IntegrationTestScenarioList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]IntegrationTestScenario, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IntegrationTestScenarioList.
func (in *IntegrationTestScenarioList) DeepCopy() *IntegrationTestScenarioList {
	if in == nil {
		return nil
	}
	out := new(IntegrationTestScenarioList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *IntegrationTestScenarioList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IntegrationTestScenarioSpec) DeepCopyInto(out *IntegrationTestScenarioSpec) {
	*out = *in
	in.ResolverRef.DeepCopyInto(&out.ResolverRef)
	if in.Params != nil {
		in, out := &in.Params, &out.Params
		*out = make([]PipelineParameter, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Contexts != nil {
		in, out := &in.Contexts, &out.Contexts
		*out = make([]TestContext, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IntegrationTestScenarioSpec.
func (in *IntegrationTestScenarioSpec) DeepCopy() *IntegrationTestScenarioSpec {
	if in == nil {
		return nil
	}
	out := new(IntegrationTestScenarioSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IntegrationTestScenarioStatus) DeepCopyInto(out *IntegrationTestScenarioStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IntegrationTestScenarioStatus.
func (in *IntegrationTestScenarioStatus) DeepCopy() *IntegrationTestScenarioStatus {
	if in == nil {
		return nil
	}
	out := new(IntegrationTestScenarioStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PipelineParameter) DeepCopyInto(out *PipelineParameter) {
	*out = *in
	if in.Values != nil {
		in, out := &in.Values, &out.Values
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PipelineParameter.
func (in *PipelineParameter) DeepCopy() *PipelineParameter {
	if in == nil {
		return nil
	}
	out := new(PipelineParameter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResolverParameter) DeepCopyInto(out *ResolverParameter) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResolverParameter.
func (in *ResolverParameter) DeepCopy() *ResolverParameter {
	if in == nil {
		return nil
	}
	out := new(ResolverParameter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResolverRef) DeepCopyInto(out *ResolverRef) {
	*out = *in
	if in.Params != nil {
		in, out := &in.Params, &out.Params
		*out = make([]ResolverParameter, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResolverRef.
func (in *ResolverRef) DeepCopy() *ResolverRef {
	if in == nil {
		return nil
	}
	out := new(ResolverRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestContext) DeepCopyInto(out *TestContext) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestContext.
func (in *TestContext) DeepCopy() *TestContext {
	if in == nil {
		return nil
	}
	out := new(TestContext)
	in.DeepCopyInto(out)
	return out
}
