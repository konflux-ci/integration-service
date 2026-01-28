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

	"github.com/konflux-ci/operator-toolkit/conditions"
	"github.com/konflux-ci/release-service/metadata"
	tektonutils "github.com/konflux-ci/release-service/tekton/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// ReleasePlanSpec defines the desired state of ReleasePlan.
type ReleasePlanSpec struct {
	// Application is a reference to the application to be released in the managed namespace
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	Application string `json:"application"`

	// Collectors contains all the information of the collectors to be executed as part of the release workflow
	// +optional
	Collectors *Collectors `json:"collectors,omitempty"`

	// Data is an unstructured key used for providing data for the managed Release Pipeline
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Data *runtime.RawExtension `json:"data,omitempty"`

	// TenantPipeline contains all the information about the tenant Pipeline
	// +optional
	TenantPipeline *tektonutils.ParameterizedPipeline `json:"tenantPipeline,omitempty"`

	// FinalPipeline contains all the information about the final Pipeline
	// +optional
	FinalPipeline *tektonutils.ParameterizedPipeline `json:"finalPipeline,omitempty"`

	// ReleaseGracePeriodDays is the number of days a Release should be kept
	// This value is used to define the Release ExpirationTime
	// +kubebuilder:default:=7
	// +optional
	ReleaseGracePeriodDays int `json:"releaseGracePeriodDays,omitempty"`

	// Target references where to send the release requests
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +optional
	Target string `json:"target,omitempty"`
}

// MatchedReleasePlanAdmission defines the relevant information for a matched ReleasePlanAdmission.
type MatchedReleasePlanAdmission struct {
	// Name contains the namespaced name of the releasePlanAdmission
	// +optional
	Name string `json:"name,omitempty"`

	// Active indicates whether the ReleasePlanAdmission is set to auto-release or not
	// +kubebuilder:default:false
	// +optional
	Active bool `json:"active,omitempty"`
}

// ReleasePlanStatus defines the observed state of ReleasePlan.
type ReleasePlanStatus struct {
	// Conditions represent the latest available observations for the releasePlan
	// +optional
	Conditions []metav1.Condition `json:"conditions"`

	// ReleasePlanAdmission contains the information of the releasePlanAdmission this ReleasePlan is
	// matched to
	// +optional
	ReleasePlanAdmission MatchedReleasePlanAdmission `json:"releasePlanAdmission,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=rp
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Application",type=string,JSONPath=`.spec.application`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target`

// ReleasePlan is the Schema for the ReleasePlans API.
type ReleasePlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleasePlanSpec   `json:"spec,omitempty"`
	Status ReleasePlanStatus `json:"status,omitempty"`
}

// MarkMatched marks the ReleasePlan as matched to a given ReleasePlanAdmission.
func (rp *ReleasePlan) MarkMatched(releasePlanAdmission *ReleasePlanAdmission) {
	rp.setMatchedStatus(releasePlanAdmission, metav1.ConditionTrue)
}

// MarkUnmatched marks the ReleasePlan as not matched to any ReleasePlanAdmission.
func (rp *ReleasePlan) MarkUnmatched() {
	if meta.IsStatusConditionPresentAndEqual(rp.Status.Conditions, MatchedConditionType.String(), metav1.ConditionFalse) {
		return
	}

	rp.setMatchedStatus(nil, metav1.ConditionFalse)
}

// setMatchedStatus sets the ReleasePlan Matched condition based on the passed releasePlanAdmission and status.
func (rp *ReleasePlan) setMatchedStatus(releasePlanAdmission *ReleasePlanAdmission, status metav1.ConditionStatus) {
	rp.Status.ReleasePlanAdmission = MatchedReleasePlanAdmission{}

	if releasePlanAdmission != nil {
		rp.Status.ReleasePlanAdmission.Name = fmt.Sprintf("%s%c%s", releasePlanAdmission.GetNamespace(),
			types.Separator, releasePlanAdmission.GetName())
		rp.Status.ReleasePlanAdmission.Active = (releasePlanAdmission.GetLabels()[metadata.BlockReleasesLabel] == "false")
	}

	conditions.SetCondition(&rp.Status.Conditions, MatchedConditionType, status, MatchedReason)
}

// +kubebuilder:object:root=true

// ReleasePlanList contains a list of ReleasePlan.
type ReleasePlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleasePlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ReleasePlan{}, &ReleasePlanList{})
}
