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
	tektonutils "github.com/redhat-appstudio/release-service/tekton/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReleasePlanAdmissionSpec defines the desired state of ReleasePlanAdmission.
type ReleasePlanAdmissionSpec struct {
	// Applications is a list of references to application to be released in the managed namespace
	// +required
	Applications []string `json:"applications"`

	// Data is an unstructured key used for providing data for the release Pipeline
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Data *runtime.RawExtension `json:"data,omitempty"`

	// Environment defines which Environment will be used to release the Application
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +optional
	Environment string `json:"environment,omitempty"`

	// Origin references where the release requests should come from
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	Origin string `json:"origin"`

	// PipelineRef is a reference to the Pipeline to be executed by the release PipelineRun
	// +required
	PipelineRef *tektonutils.PipelineRef `json:"pipelineRef"`

	// Policy to validate before releasing an artifact
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	Policy string `json:"policy"`

	// ServiceAccount is the name of the service account to use in the
	// release PipelineRun to gain elevated privileges
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +optional
	ServiceAccount string `json:"serviceAccount,omitempty"`
}

// ReleasePlanAdmissionStatus defines the observed state of ReleasePlanAdmission.
type ReleasePlanAdmissionStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Origin",type=string,JSONPath=`.spec.origin`

// ReleasePlanAdmission is the Schema for the ReleasePlanAdmissions API.
type ReleasePlanAdmission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleasePlanAdmissionSpec   `json:"spec,omitempty"`
	Status ReleasePlanAdmissionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReleasePlanAdmissionList contains a list of ReleasePlanAdmission.
type ReleasePlanAdmissionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleasePlanAdmission `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ReleasePlanAdmission{}, &ReleasePlanAdmissionList{})
}
