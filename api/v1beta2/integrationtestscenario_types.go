/*
Copyright 2024 Red Hat Inc.

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

package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// TestLabelPrefix contains the prefix applied to labels and annotations related to testing.
	TestLabelPrefix = "test.appstudio.openshift.io"

	// PipelineTimeoutAnnotation contains the name of the application
	PipelineTimeoutAnnotation = TestLabelPrefix + "/pipeline_timeout"

	// TasksTimeoutAnnotation contains the name of the application
	TasksTimeoutAnnotation = TestLabelPrefix + "/tasks_timeout"

	// FinallyTimeoutAnnotation contains the name of the application
	FinallyTimeoutAnnotation = TestLabelPrefix + "/finally_timeout"
)

// IntegrationTestScenarioSpec defines the desired state of IntegrationScenario
type IntegrationTestScenarioSpec struct {
	// Application that's associated with the IntegrationTestScenario
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	Application string `json:"application"`
	// Tekton Resolver where to store the Tekton resolverRef trigger Tekton pipeline used to refer to a Pipeline or Task in a remote location like a git repo.
	// +required
	ResolverRef ResolverRef `json:"resolverRef"`
	// Params to pass to the pipeline
	Params []PipelineParameter `json:"params,omitempty"`
	// Contexts where this IntegrationTestScenario can be applied
	Contexts []TestContext `json:"contexts,omitempty"`
	// List of IntegrationTestScenario which are blocked by the successful completion of this IntegrationTestScenario
	Dependents []string `json:"dependents,omitempty"`
}

// IntegrationTestScenarioStatus defines the observed state of IntegrationTestScenario
type IntegrationTestScenarioStatus struct {
	Conditions []metav1.Condition `json:"conditions"`
}

// PipelineParameter contains the name and values of a Tekton Pipeline parameter
type PipelineParameter struct {
	Name   string   `json:"name"`
	Value  string   `json:"value,omitempty"`
	Values []string `json:"values,omitempty"`
}

// TestContext contains the name and values of a Test context
type TestContext struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=its
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Application",type=string,JSONPath=`.spec.application`
// +kubebuilder:storageversion

// IntegrationTestScenario is the Schema for the integrationtestscenarios API
type IntegrationTestScenario struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IntegrationTestScenarioSpec   `json:"spec,omitempty"`
	Status IntegrationTestScenarioStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IntegrationTestScenarioList contains a list of IntegrationTestScenario
type IntegrationTestScenarioList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IntegrationTestScenario `json:"items"`
}

// Tekton Resolver where to store the Tekton resolverRef trigger Tekton pipeline used to refer to a Pipeline or Task in a remote location like a git repo.
// +required
type ResolverRef struct {
	// Resolver is the name of the resolver that should perform resolution of the referenced Tekton resource, such as "git" or "bundle"..
	// +required
	Resolver string `json:"resolver"`
	// Params contains the parameters used to identify the
	// referenced Tekton resource. Example entries might include
	// "repo" or "path" but the set of params ultimately depends on
	// the chosen resolver.
	// +required
	Params []ResolverParameter `json:"params"`
	// ResourceKind defines the kind of resource being resolved. It can either
	// be "pipeline" or "pipelinerun" but defaults to "pipeline" if no value is
	// set
	// +optional
	ResourceKind string `json:"resourceKind"`
}

// ResolverParameter contains the name and values used to identify the referenced Tekton resource
type ResolverParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func init() {
	SchemeBuilder.Register(&IntegrationTestScenario{}, &IntegrationTestScenarioList{})
}
