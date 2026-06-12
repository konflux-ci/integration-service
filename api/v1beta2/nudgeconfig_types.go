/*
Copyright 2026 Red Hat Inc.

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

// NudgeModeType defines when a nudge is triggered.
// +kubebuilder:validation:Enum=immediate;validated
type NudgeModeType string

const (
	// NudgeModeImmediate triggers the nudge as soon as the source component build succeeds.
	NudgeModeImmediate NudgeModeType = "immediate"

	// NudgeModeValidated triggers the nudge only after integration tests pass for the source component.
	NudgeModeValidated NudgeModeType = "validated"
)

// NudgeConfigSingletonName is the required name for the singleton NudgeConfig per namespace.
const NudgeConfigSingletonName = "nudge-config"

// NudgeRelationship defines a single nudge from one component to another.
type NudgeRelationship struct {
	// From is the source component name that triggers the nudge when its build succeeds.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +required
	From string `json:"from"`

	// To is the target component name that receives the nudge.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +required
	To string `json:"to"`

	// Mode defines when the nudge is triggered.
	// "immediate" triggers on build success; "validated" triggers after integration tests pass.
	// +kubebuilder:default=immediate
	// +optional
	Mode NudgeModeType `json:"mode,omitempty"`

	// GatingGroup is reserved for Phase 2 group-based gating and is not enforced in Phase 1.
	// +kubebuilder:validation:MaxLength=63
	// +optional
	GatingGroup string `json:"gatingGroup,omitempty"`
}

// NudgeConfigSpec defines the desired nudging relationships between components.
// +kubebuilder:validation:XValidation:rule="!has(self.nudges) || self.nudges.all(n, n.from != n.to)",message="self-nudge not allowed: from and to must be different"
// +kubebuilder:validation:XValidation:rule="!has(self.nudges) || self.nudges.all(i, self.nudges.exists_one(j, i.from == j.from && i.to == j.to))",message="duplicate (from, to) pair not allowed"
type NudgeConfigSpec struct {
	// Nudges is the list of component nudge relationships.
	// +kubebuilder:validation:MaxItems=256
	// +optional
	Nudges []NudgeRelationship `json:"nudges,omitempty"`
}

// NudgeConfigStatus defines the observed state of NudgeConfig.
type NudgeConfigStatus struct {
	// Conditions represent the latest available observations of the NudgeConfig's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastValidationTime is the timestamp of the last successful validation of the nudge graph.
	// +optional
	LastValidationTime *metav1.Time `json:"lastValidationTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=nc
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'nudge-config'",message="NudgeConfig must be named 'nudge-config' (singleton per namespace)"

// NudgeConfig is a namespace-scoped singleton CRD that stores component nudging relationships.
// Exactly one NudgeConfig named "nudge-config" may exist per namespace.
// Validation rules are enforced stateless by the API server via CEL expressions with no webhook overhead.
type NudgeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NudgeConfigSpec   `json:"spec,omitempty"`
	Status NudgeConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NudgeConfigList contains a list of NudgeConfigs.
type NudgeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NudgeConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NudgeConfig{}, &NudgeConfigList{})
}
