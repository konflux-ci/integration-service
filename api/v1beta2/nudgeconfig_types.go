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
	"fmt"
	"time"

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

// FailurePolicyType defines how a batch behaves when a member source build fails.
// Wire values use PascalCase to align with Kubernetes policy enums (for example admission webhook FailurePolicy).
// +kubebuilder:validation:Enum=Block;ProceedWithPartial
type FailurePolicyType string

const (
	// FailurePolicyBlock keeps the batch blocked until the failed component rebuilds
	// successfully or the user force-fires the batch.
	FailurePolicyBlock FailurePolicyType = "Block"

	// FailurePolicyProceedWithPartial fires the batch with whatever succeeded when
	// the debounce timer or hard deadline expires.
	FailurePolicyProceedWithPartial FailurePolicyType = "ProceedWithPartial"
)

const (
	// DefaultBatchDebounceTimeout is applied when a batched target's debounceTimeout is unset.
	DefaultBatchDebounceTimeout = 30 * time.Minute

	// DefaultBatchMaxWaitTime is applied when a batched target's maxWaitTime is unset.
	DefaultBatchMaxWaitTime = 4 * time.Hour

	// DefaultBatchFailurePolicy is applied when a batched target's failurePolicy is unset.
	DefaultBatchFailurePolicy FailurePolicyType = FailurePolicyBlock
)

// BatchPolicy defines per-target batch timing and failure behavior overrides.
// Field shape matches the planned BatchDefaults type; unset fields use namespace defaults at runtime.
type BatchPolicy struct {
	// DebounceTimeout is how long to wait after the last incoming build before firing the batch.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=duration
	// +optional
	DebounceTimeout *metav1.Duration `json:"debounceTimeout,omitempty"`

	// MaxWaitTime is the hard deadline from the first build event, regardless of debounce resets.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=duration
	// +optional
	MaxWaitTime *metav1.Duration `json:"maxWaitTime,omitempty"`

	// FailurePolicy controls whether a failed member build blocks the batch or allows a partial fire.
	// +optional
	FailurePolicy *FailurePolicyType `json:"failurePolicy,omitempty"`
}

// TargetConfig declares per-target batch behavior. Presence of batchPolicy opts the target into batching.
type TargetConfig struct {
	// Target is the downstream component name that may receive batched nudges.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +required
	Target string `json:"target"`

	// BatchPolicy defines per-target batch timing and failure behavior overrides.
	// When omitted, the target is not batched. An empty object opts into batching with default timing.
	// +optional
	BatchPolicy *BatchPolicy `json:"batchPolicy,omitempty"`
}

// IsBatched reports whether this target opts into batch nudging.
func (tc TargetConfig) IsBatched() bool {
	return tc.BatchPolicy != nil
}

// IsTargetBatched reports whether the given target component is configured for batch nudging.
func (spec NudgeConfigSpec) IsTargetBatched(target string) bool {
	tc := spec.ResolveTargetConfig(target)
	return tc != nil && tc.IsBatched()
}

// EffectiveBatchPolicy returns resolved timing and failure behavior for a batched target.
func (spec NudgeConfigSpec) EffectiveBatchPolicy(target string) (debounce time.Duration, maxWait time.Duration, failure FailurePolicyType) {
	debounce = DefaultBatchDebounceTimeout
	maxWait = DefaultBatchMaxWaitTime
	failure = DefaultBatchFailurePolicy

	tc := spec.ResolveTargetConfig(target)
	if tc == nil || tc.BatchPolicy == nil {
		return debounce, maxWait, failure
	}
	policy := tc.BatchPolicy
	if policy.DebounceTimeout != nil {
		debounce = policy.DebounceTimeout.Duration
	}
	if policy.MaxWaitTime != nil {
		maxWait = policy.MaxWaitTime.Duration
	}
	if policy.FailurePolicy != nil {
		failure = *policy.FailurePolicy
	}
	return debounce, maxWait, failure
}

// ValidateUniqueTargetConfig returns an error when spec.targetConfig contains duplicate target names.
func (spec NudgeConfigSpec) ValidateUniqueTargetConfig() error {
	seen := make(map[string]struct{}, len(spec.TargetConfig))
	for _, tc := range spec.TargetConfig {
		if _, dup := seen[tc.Target]; dup {
			return fmt.Errorf("duplicate targetConfig target %q not allowed", tc.Target)
		}
		seen[tc.Target] = struct{}{}
	}
	return nil
}

// ResolveTargetConfig returns the TargetConfig entry for target, or nil when not configured.
// Duplicate target names are rejected by the validating webhook; at most one entry exists per target.
func (spec NudgeConfigSpec) ResolveTargetConfig(target string) *TargetConfig {
	for i := range spec.TargetConfig {
		if spec.TargetConfig[i].Target == target {
			return &spec.TargetConfig[i]
		}
	}
	return nil
}

// BatchPhase defines the lifecycle phase of an active batch.
// +kubebuilder:validation:Enum=Accumulating;Blocked;Firing;Failed;Completed
type BatchPhase string

const (
	BatchPhaseAccumulating BatchPhase = "Accumulating"
	BatchPhaseBlocked      BatchPhase = "Blocked"
	BatchPhaseFiring       BatchPhase = "Firing"
	BatchPhaseFailed       BatchPhase = "Failed"
	BatchPhaseCompleted    BatchPhase = "Completed"
)

// AccumulatedEntry defines a build event that has been collected into a batch.
type AccumulatedEntry struct {
	// From is the source component name whose build was captured.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +required
	From string `json:"from"`

	// ImageDigest is the OCI image digest produced by the build pipeline run.
	// +kubebuilder:validation:MinLength=1
	// +required
	ImageDigest string `json:"imageDigest"`

	// BuildPipelineRun is the name of the build PipelineRun that produced this entry.
	// +kubebuilder:validation:MinLength=1
	// +required
	BuildPipelineRun string `json:"buildPipelineRun"`

	// CapturedAt is the timestamp when this entry was added to the batch.
	// +required
	CapturedAt metav1.Time `json:"capturedAt"`
}

// ActiveBatch defines the runtime state of a single nudge batch.
type ActiveBatch struct {
	// Target is the component name that will receive the nudge when the batch fires.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +required
	Target string `json:"target"`

	// BatchID is a unique identifier for this batch instance.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	BatchID string `json:"batchId"`

	// Phase is the current lifecycle phase of this batch.
	// +required
	Phase BatchPhase `json:"phase"`

	// CreatedAt is the timestamp when this batch was first created.
	// +required
	CreatedAt metav1.Time `json:"createdAt"`

	// FireAt is the scheduled time when this batch will fire and create the nudge pipeline run.
	// +optional
	FireAt *metav1.Time `json:"fireAt,omitempty"`

	// HardDeadline is the absolute deadline by which an accumulating batch must fire.
	// +required
	HardDeadline metav1.Time `json:"hardDeadline"`

	// Accumulated is the list of build events collected into this batch.
	// +optional
	Accumulated []AccumulatedEntry `json:"accumulated,omitempty"`

	// NudgePipelineRun is the name of the nudge PipelineRun created when this batch fired.
	// +optional
	NudgePipelineRun string `json:"nudgePipelineRun,omitempty"`

	// Message provides additional human-readable context about the current batch state.
	// +optional
	Message string `json:"message,omitempty"`
}

// Actions holds one-shot operations processed by the NudgeConfig controller and cleared after reconcile.
// The pattern matches spec.actions on Component (ADR 0056).
type Actions struct {
	// ForceFire immediately fires a batched nudge for the given target, bypassing debounce timing.
	// +optional
	ForceFire *ForceFireAction `json:"forceFire,omitempty"`
}

// ForceFireAction requests an immediate batch fire for a target component.
type ForceFireAction struct {
	// Target is the downstream component whose active batch should be force-fired.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +required
	Target string `json:"target"`

	// IncludePartial includes successful members when some batch sources failed and FailurePolicy is Block.
	// Defaults to true when unset.
	// +kubebuilder:default=true
	// +optional
	IncludePartial *bool `json:"includePartial,omitempty"`
}

// NudgeConfigSpec defines the desired nudging relationships between components.
// +kubebuilder:validation:XValidation:rule="!has(self.nudges) || self.nudges.all(n, n.from != n.to)",message="self-nudge not allowed: from and to must be different"
// +kubebuilder:validation:XValidation:rule="!has(self.nudges) || self.nudges.all(i, self.nudges.exists_one(j, i.from == j.from && i.to == j.to))",message="duplicate (from, to) pair not allowed"
type NudgeConfigSpec struct {
	// Actions holds one-shot operations processed by the controller and then cleared from spec.
	// +optional
	Actions *Actions `json:"actions,omitempty"`

	// TargetConfig lists per-target batch policies. Targets without batchPolicy are not batched.
	// Controllers and operators must patch NudgeConfig spec with merge semantics so targetConfig
	// and actions are not dropped when updating other spec fields.
	// Duplicate target names are rejected by the validating webhook (CEL cost limits preclude a spec rule).
	// +kubebuilder:validation:MaxItems=360
	// +optional
	TargetConfig []TargetConfig `json:"targetConfig,omitempty"`

	// Nudges is the list of component nudge relationships.
	// +kubebuilder:validation:MaxItems=360
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

	// ActiveBatches tracks in-progress nudge batches keyed by batchId.
	// +listType=map
	// +listMapKey=batchId
	// +kubebuilder:validation:MaxItems=360
	// +optional
	ActiveBatches []ActiveBatch `json:"activeBatches,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=nc
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'nudge-config'",message="NudgeConfig must be named 'nudge-config' (singleton per namespace)"

// NudgeConfig is a namespace-scoped singleton CRD that stores component nudging relationships.
// Exactly one NudgeConfig named "nudge-config" may exist per namespace.
// Structural and singleton rules are enforced by the API server via CEL expressions;
// graph-cycle and cross-resource Component-existence checks are enforced by a validating webhook.
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
