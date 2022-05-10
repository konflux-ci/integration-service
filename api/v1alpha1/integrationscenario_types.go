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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IntegrationScenarioSpec defines the desired state of IntegrationScenario
type IntegrationScenarioSpec struct {
	// Release Tekton Pipeline to execute
	Pipeline string `json:"pipeline"`
	// Tekton Bundle where to find the pipeline
	Bundle string `json:"bundle,omitempty"`
	// Params to pass to the pipeline
	Params []PipelineParameter `json:"params,omitempty"`
}

// IntegrationScenarioStatus defines the observed state of IntegrationScenario
type IntegrationScenarioStatus struct {
}

// PipelineParameter contains the name and values of a Tekton Pipeline parameter
type PipelineParameter struct {
	Name  string   `json:"name"`
	Value []string `json:"value"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// IntegrationScenario is the Schema for the integrationscenarios API
type IntegrationScenario struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IntegrationScenarioSpec   `json:"spec,omitempty"`
	Status IntegrationScenarioStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IntegrationScenarioList contains a list of IntegrationScenario
type IntegrationScenarioList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IntegrationScenario `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IntegrationScenario{}, &IntegrationScenarioList{})
}
