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
	"fmt"
	"sort"

	"github.com/konflux-ci/operator-toolkit/conditions"
	"github.com/konflux-ci/release-service/metadata"
	tektonutils "github.com/konflux-ci/release-service/tekton/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// ReleasePlanAdmissionSpec defines the desired state of ReleasePlanAdmission.
type ReleasePlanAdmissionSpec struct {
	// Applications is a list of references to applications to be released in the managed namespace
	// +required
	Applications []string `json:"applications"`

	// Collectors contains all the information of the collectors to be executed as part of the release workflow
	// +optional
	Collectors *Collectors `json:"collectors,omitempty"`

	// Data is an unstructured key used for providing data for the managed Release Pipeline
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

	// Pipeline contains all the information about the managed Pipeline
	// +optional
	Pipeline *tektonutils.Pipeline `json:"pipeline,omitempty"`

	// Policy to validate before releasing an artifact
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	Policy string `json:"policy"`
}

// MatchedReleasePlan defines the relevant information for a matched ReleasePlan.
type MatchedReleasePlan struct {
	// Name contains the namespaced name of the ReleasePlan
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?\/[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +optional
	Name string `json:"name,omitempty"`

	// Active indicates whether the ReleasePlan is set to auto-release or not
	// +kubebuilder:default:false
	// +optional
	Active bool `json:"active,omitempty"`
}

// ReleasePlanAdmissionStatus defines the observed state of ReleasePlanAdmission.
type ReleasePlanAdmissionStatus struct {
	// Conditions represent the latest available observations for the releasePlanAdmission
	// +optional
	Conditions []metav1.Condition `json:"conditions"`

	// ReleasePlan is a list of releasePlans matched to the ReleasePlanAdmission
	// +optional
	ReleasePlans []MatchedReleasePlan `json:"releasePlans"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=rpa
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

// ClearMatchingInfo marks the ReleasePlanAdmission as no longer matched to any ReleasePlan.
func (rpa *ReleasePlanAdmission) ClearMatchingInfo() {
	rpa.Status.ReleasePlans = []MatchedReleasePlan{}
	conditions.SetCondition(&rpa.Status.Conditions, MatchedConditionType, metav1.ConditionFalse, MatchedReason)
}

// MarkMatched marks the ReleasePlanAdmission as matched to a given ReleasePlan.
func (rpa *ReleasePlanAdmission) MarkMatched(releasePlan *ReleasePlan) {
	pairedReleasePlan := MatchedReleasePlan{
		Name:   fmt.Sprintf("%s%c%s", releasePlan.GetNamespace(), types.Separator, releasePlan.GetName()),
		Active: (releasePlan.GetLabels()[metadata.AutoReleaseLabel] == "true"),
	}

	rpa.Status.ReleasePlans = append(rpa.Status.ReleasePlans, pairedReleasePlan)
	sort.Slice(rpa.Status.ReleasePlans, func(i, j int) bool {
		return rpa.Status.ReleasePlans[i].Name < rpa.Status.ReleasePlans[j].Name
	})

	// Update the condition every time one is added so lastTransitionTime updates
	conditions.SetCondition(&rpa.Status.Conditions, MatchedConditionType, metav1.ConditionTrue, MatchedReason)
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
