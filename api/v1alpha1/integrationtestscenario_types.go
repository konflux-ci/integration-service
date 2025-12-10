/*
Copyright 2022 Red Hat Inc.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnvVarPair describes environment variables to use for the component
type EnvVarPair struct {

	// Name is the environment variable name
	Name string `json:"name"`

	// Value is the environment variable value
	Value string `json:"value"`
}

// DeploymentTargetClaimConfig specifies the DeploymentTargetClaim details for a given Environment.
type DeploymentTargetClaimConfig struct {
	ClaimName string `json:"claimName"`
}

// EnvironmentTarget provides the configuration for a deployment target.
type EnvironmentTarget struct {
	DeploymentTargetClaim DeploymentTargetClaimConfig `json:"deploymentTargetClaim"`
}

// EnvironmentConfiguration contains Environment-specific configurations details, to be used when generating
// Component/Application GitOps repository resources.
type DeprecatedEnvironmentConfiguration struct {
	// An array of standard environment variables
	Env []EnvVarPair `json:"env,omitempty"`

	// Target is used to reference a DeploymentTargetClaim for a target Environment.
	// The Environment controller uses the referenced DeploymentTargetClaim to access its bounded
	// DeploymentTarget with cluster credential secret.
	Target EnvironmentTarget `json:"target,omitempty"`
}

// DEPRECATED: EnvironmentType should no longer be used, and has no replacement.
// - It's original purpose was to indicate whether an environment is POC/Non-POC, but these data were ultimately not required.
type DeprecatedEnvironmentType string

const (
	// DEPRECATED: EnvironmentType_POC should no longer be used, and has no replacement.
	EnvironmentType_POC DeprecatedEnvironmentType = "POC"

	// DEPRECATED: EnvironmentType_NonPOC should no longer be used, and has no replacement.
	EnvironmentType_NonPOC DeprecatedEnvironmentType = "Non-POC"
)

// IntegrationTestScenarioSpec defines the desired state of IntegrationScenario
type IntegrationTestScenarioSpec struct {
	// Application that's associated with the IntegrationTestScenario
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	Application string `json:"application"`
	// Release Tekton Pipeline to execute
	// +required
	Pipeline string `json:"pipeline"`
	// Tekton Bundle where to find the pipeline
	// +required
	Bundle string `json:"bundle"`
	// Params to pass to the pipeline
	Params []PipelineParameter `json:"params,omitempty"`
	// Environment that will be utilized by the test pipeline
	Environment TestEnvironment `json:"environment,omitempty"`
	// Contexts where this IntegrationTestScenario can be applied
	Contexts []TestContext `json:"contexts,omitempty"`
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

// TestEnvironment contains the name and values of a Test environment
type TestEnvironment struct {
	Name          string                              `json:"name"`
	Type          DeprecatedEnvironmentType           `json:"type"`
	Configuration *DeprecatedEnvironmentConfiguration `json:"configuration,omitempty"`
}

// TestContext contains the name and values of a Test context
type TestContext struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Application",type=string,JSONPath=`.spec.application`
// +kubebuilder:deprecatedversion:warning="The v1alpha1 version is deprecated and will be automatically migrated to v1beta1"

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

func init() {
	SchemeBuilder.Register(&IntegrationTestScenario{}, &IntegrationTestScenarioList{})
}
