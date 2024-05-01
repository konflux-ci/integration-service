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
	"time"

	"github.com/konflux-ci/operator-toolkit/conditions"

	"github.com/konflux-ci/release-service/metrics"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReleaseSpec defines the desired state of Release.
type ReleaseSpec struct {
	// Snapshot to be released
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	Snapshot string `json:"snapshot"`

	// ReleasePlan to use for this particular Release
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	ReleasePlan string `json:"releasePlan"`

	// Data is an unstructured key used for providing data for the managed Release Pipeline
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Data *runtime.RawExtension `json:"data,omitempty"`

	// GracePeriodDays is the number of days a Release should be kept
	// This value is used to define the Release ExpirationTime
	// +optional
	GracePeriodDays int `json:"gracePeriodDays,omitempty"`
}

// ReleaseStatus defines the observed state of Release.
type ReleaseStatus struct {
	// Attribution contains information about the entity authorizing the release
	// +optional
	Attribution AttributionInfo `json:"attribution,omitempty"`

	// Conditions represent the latest available observations for the release
	// +optional
	Conditions []metav1.Condition `json:"conditions"`

	// PostActionsExecution contains information about the post-actions execution
	// +optional
	PostActionsExecution PostActionsExecutionInfo `json:"postActionsExecution,omitempty"`

	// Processing contains information about the release processing
	// +optional
	Processing ProcessingInfo `json:"processing,omitempty"`

	// Validation contains information about the release validation
	// +optional
	Validation ValidationInfo `json:"validation,omitempty"`

	// Target references where this release is intended to be released to
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +optional
	Target string `json:"target,omitempty"`

	// Automated indicates whether the Release was created as part of an automated process or manually by an end-user
	// +optional
	Automated bool `json:"automated,omitempty"`

	// CompletionTime is the time when a Release was completed
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// StartTime is the time when a Release started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// ExpirationTime is the time when a Release can be purged
	// +optional
	ExpirationTime *metav1.Time `json:"expirationTime,omitempty"`
}

// AttributionInfo defines the observed state of the release attribution.
type AttributionInfo struct {
	// Author is the username that the release is attributed to
	// +optional
	Author string `json:"author,omitempty"`

	// StandingAuthorization indicates whether the release is attributed through a ReleasePlan
	// +optional
	StandingAuthorization bool `json:"standingAuthorization,omitempty"`
}

// PostActionsExecutionInfo defines the observed state of the post-actions execution.
type PostActionsExecutionInfo struct {
	// CompletionTime is the time when the Release post-actions execution was completed
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// StartTime is the time when the Release post-actions execution started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
}

// ProcessingInfo defines the observed state of the release processing.
type ProcessingInfo struct {
	// CompletionTime is the time when the Release processing was completed
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// PipelineRun contains the namespaced name of the managed Release PipelineRun executed as part of this release
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?\/[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +optional
	PipelineRun string `json:"pipelineRun,omitempty"`

	// RoleBinding contains the namespaced name of the roleBinding created for the managed Release PipelineRun
	// executed as part of this release
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?\/[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +optional
	RoleBinding string `json:"roleBinding,omitempty"`

	// StartTime is the time when the Release processing started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
}

// ValidationInfo defines the observed state of the release validation.
type ValidationInfo struct {
	// FailedPostValidation indicates whether the Release was marked as invalid after being initially marked as valid
	FailedPostValidation bool `json:"failedPostValidation,omitempty"`

	// Time is the time when the Release was validated or when the validation state changed
	// +optional
	Time *metav1.Time `json:"time,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=rel
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Snapshot",type=string,JSONPath=`.spec.snapshot`
// +kubebuilder:printcolumn:name="ReleasePlan",type=string,JSONPath=`.spec.releasePlan`
// +kubebuilder:printcolumn:name="Release status",type=string,JSONPath=`.status.conditions[?(@.type=="Released")].reason`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Release is the Schema for the releases API
type Release struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleaseSpec   `json:"spec,omitempty"`
	Status ReleaseStatus `json:"status,omitempty"`
}

// HasEveryPostActionExecutionFinished checks whether the Release post-actions execution has finished,
// regardless of the result.
func (r *Release) HasEveryPostActionExecutionFinished() bool {
	return r.hasPhaseFinished(postActionsExecutedConditionType)
}

// HasProcessingFinished checks whether the Release processing has finished, regardless of the result.
func (r *Release) HasProcessingFinished() bool {
	return r.hasPhaseFinished(processedConditionType)
}

// HasReleaseFinished checks whether the Release has finished, regardless of the result.
func (r *Release) HasReleaseFinished() bool {
	return r.hasPhaseFinished(releasedConditionType)
}

// IsAttributed checks whether the Release was marked as attributed.
func (r *Release) IsAttributed() bool {
	return r.Status.Attribution.Author != ""
}

// IsAutomated checks whether the Release was marked as automated.
func (r *Release) IsAutomated() bool {
	return r.Status.Automated
}

// IsEveryPostActionExecuted checks whether the Release post-actions were successfully executed.
func (r *Release) IsEveryPostActionExecuted() bool {
	return meta.IsStatusConditionTrue(r.Status.Conditions, postActionsExecutedConditionType.String())
}

// IsEachPostActionExecuting checks whether the Release post-actions are in progress.
func (r *Release) IsEachPostActionExecuting() bool {
	return r.isPhaseProgressing(postActionsExecutedConditionType)
}

// IsProcessed checks whether the Release was successfully processed.
func (r *Release) IsProcessed() bool {
	return meta.IsStatusConditionTrue(r.Status.Conditions, processedConditionType.String())
}

// IsProcessing checks whether the Release processing is in progress.
func (r *Release) IsProcessing() bool {
	return r.isPhaseProgressing(processedConditionType)
}

// IsReleased checks whether the Release has finished successfully.
func (r *Release) IsReleased() bool {
	return meta.IsStatusConditionTrue(r.Status.Conditions, releasedConditionType.String())
}

// IsReleasing checks whether the Release is in progress.
func (r *Release) IsReleasing() bool {
	return r.isPhaseProgressing(releasedConditionType)
}

// IsValid checks whether the Release validation has finished successfully.
func (r *Release) IsValid() bool {
	return meta.IsStatusConditionTrue(r.Status.Conditions, validatedConditionType.String())
}

// MarkProcessed marks the Release as processed.
func (r *Release) MarkProcessed() {
	if !r.IsProcessing() || r.HasProcessingFinished() {
		return
	}

	r.Status.Processing.CompletionTime = &metav1.Time{Time: time.Now()}
	conditions.SetCondition(&r.Status.Conditions, processedConditionType, metav1.ConditionTrue, SucceededReason)

	go metrics.RegisterCompletedReleaseProcessing(
		r.Status.Processing.StartTime,
		r.Status.Processing.CompletionTime,
		SucceededReason.String(),
		r.Status.Target,
	)
}

// MarkProcessing marks the Release as processing.
func (r *Release) MarkProcessing(message string) {
	if r.HasProcessingFinished() {
		return
	}

	if !r.IsProcessing() {
		r.Status.Processing.StartTime = &metav1.Time{Time: time.Now()}
	}

	conditions.SetConditionWithMessage(&r.Status.Conditions, processedConditionType, metav1.ConditionFalse, ProgressingReason, message)

	go metrics.RegisterNewReleaseProcessing(
		r.Status.Processing.StartTime,
		r.Status.StartTime,
		ProgressingReason.String(),
		r.Status.Target,
	)
}

// MarkProcessingFailed marks the Release processing as failed.
func (r *Release) MarkProcessingFailed(message string) {
	if !r.IsProcessing() || r.HasProcessingFinished() {
		return
	}

	r.Status.Processing.CompletionTime = &metav1.Time{Time: time.Now()}
	conditions.SetConditionWithMessage(&r.Status.Conditions, processedConditionType, metav1.ConditionFalse, FailedReason, message)

	go metrics.RegisterCompletedReleaseProcessing(
		r.Status.Processing.StartTime,
		r.Status.Processing.CompletionTime,
		FailedReason.String(),
		r.Status.Target,
	)
}

// MarkPostActionsExecuted marks the Release post-actions as executed.
func (r *Release) MarkPostActionsExecuted() {
	if !r.IsEachPostActionExecuting() || r.HasEveryPostActionExecutionFinished() {
		return
	}

	r.Status.PostActionsExecution.CompletionTime = &metav1.Time{Time: time.Now()}
	conditions.SetCondition(&r.Status.Conditions, postActionsExecutedConditionType, metav1.ConditionTrue, SucceededReason)

	go metrics.RegisterCompletedReleasePostActionsExecuted(
		r.Status.PostActionsExecution.StartTime,
		r.Status.PostActionsExecution.CompletionTime,
		SucceededReason.String(),
	)
}

// MarkPostActionsExecuting marks the Release post-actions as executing.
func (r *Release) MarkPostActionsExecuting(message string) {
	if r.HasEveryPostActionExecutionFinished() {
		return
	}

	if !r.IsEachPostActionExecuting() {
		r.Status.PostActionsExecution.StartTime = &metav1.Time{Time: time.Now()}
	}

	conditions.SetConditionWithMessage(&r.Status.Conditions, postActionsExecutedConditionType, metav1.ConditionFalse, ProgressingReason, message)

	go metrics.RegisterNewReleasePostActionsExecution()
}

// MarkPostActionsExecutionFailed marks the Release post-actions execution as failed.
func (r *Release) MarkPostActionsExecutionFailed(message string) {
	if !r.IsEachPostActionExecuting() || r.HasEveryPostActionExecutionFinished() {
		return
	}

	r.Status.PostActionsExecution.CompletionTime = &metav1.Time{Time: time.Now()}
	conditions.SetConditionWithMessage(&r.Status.Conditions, postActionsExecutedConditionType, metav1.ConditionFalse, FailedReason, message)

	go metrics.RegisterCompletedReleasePostActionsExecuted(
		r.Status.Processing.StartTime,
		r.Status.Processing.CompletionTime,
		FailedReason.String(),
	)
}

// MarkReleased marks the Release as released.
func (r *Release) MarkReleased() {
	if !r.IsReleasing() || r.HasReleaseFinished() {
		return
	}

	r.Status.CompletionTime = &metav1.Time{Time: time.Now()}
	conditions.SetCondition(&r.Status.Conditions, releasedConditionType, metav1.ConditionTrue, SucceededReason)

	go metrics.RegisterCompletedRelease(
		r.Status.StartTime,
		r.Status.CompletionTime,
		r.getPhaseReason(postActionsExecutedConditionType),
		r.getPhaseReason(processedConditionType),
		SucceededReason.String(),
		r.Status.Target,
		r.getPhaseReason(validatedConditionType),
	)
}

// MarkReleasing marks the Release as releasing.
func (r *Release) MarkReleasing(message string) {
	if r.HasReleaseFinished() {
		return
	}

	if !r.IsReleasing() {
		r.Status.StartTime = &metav1.Time{Time: time.Now()}
	}

	conditions.SetConditionWithMessage(&r.Status.Conditions, releasedConditionType, metav1.ConditionFalse, ProgressingReason, message)

	go metrics.RegisterNewRelease()
}

// MarkReleaseFailed marks the Release as failed.
func (r *Release) MarkReleaseFailed(message string) {
	if !r.IsReleasing() || r.HasReleaseFinished() {
		return
	}

	r.Status.CompletionTime = &metav1.Time{Time: time.Now()}
	conditions.SetConditionWithMessage(&r.Status.Conditions, releasedConditionType, metav1.ConditionFalse, FailedReason, message)

	go metrics.RegisterCompletedRelease(
		r.Status.StartTime,
		r.Status.CompletionTime,
		r.getPhaseReason(postActionsExecutedConditionType),
		r.getPhaseReason(processedConditionType),
		FailedReason.String(),
		r.Status.Target,
		r.getPhaseReason(validatedConditionType),
	)
}

// MarkValidated marks the Release as validated.
func (r *Release) MarkValidated() {
	if r.IsValid() {
		return
	}

	r.Status.Validation.Time = &metav1.Time{Time: time.Now()}
	conditions.SetCondition(&r.Status.Conditions, validatedConditionType, metav1.ConditionTrue, SucceededReason)

	go metrics.RegisterValidatedRelease(
		r.Status.StartTime,
		r.Status.Validation.Time,
		SucceededReason.String(),
		r.Status.Target,
	)
}

// MarkValidationFailed marks the Release validation as failed.
func (r *Release) MarkValidationFailed(message string) {
	if r.IsValid() {
		r.Status.Validation.FailedPostValidation = true
	}

	r.Status.Validation.Time = &metav1.Time{Time: time.Now()}
	conditions.SetConditionWithMessage(&r.Status.Conditions, validatedConditionType, metav1.ConditionFalse, FailedReason, message)

	go metrics.RegisterValidatedRelease(
		r.Status.StartTime,
		r.Status.Validation.Time,
		FailedReason.String(),
		r.Status.Target,
	)
}

// SetAutomated marks the Release as automated.
func (r *Release) SetAutomated() {
	if r.IsAutomated() {
		return
	}

	r.Status.Automated = true
}

// SetExpirationTime set the time when this release can be purged
func (r *Release) SetExpirationTime(expireDays time.Duration) {
	creationTime := r.CreationTimestamp
	r.Status.ExpirationTime = &metav1.Time{Time: creationTime.Add(time.Hour * 24 * expireDays)}
}

// getPhaseReason returns the current reason for the given ConditionType or empty string if no condition is found.
func (r *Release) getPhaseReason(conditionType conditions.ConditionType) string {
	var reason string

	condition := meta.FindStatusCondition(r.Status.Conditions, conditionType.String())
	if condition != nil {
		reason = condition.Reason
	}

	return reason
}

// hasPhaseFinished checks whether a Release phase (e.g. deployment or processing) has finished.
func (r *Release) hasPhaseFinished(conditionType conditions.ConditionType) bool {
	condition := meta.FindStatusCondition(r.Status.Conditions, conditionType.String())

	switch {
	case condition == nil:
		return false
	case condition.Status == metav1.ConditionTrue:
		return true
	default:
		return condition.Status == metav1.ConditionFalse && condition.Reason != ProgressingReason.String()
	}
}

// isPhaseProgressing checks whether a Release phase (e.g. deployment or processing) is progressing.
func (r *Release) isPhaseProgressing(conditionType conditions.ConditionType) bool {
	condition := meta.FindStatusCondition(r.Status.Conditions, conditionType.String())

	switch {
	case condition == nil:
		return false
	case condition.Status == metav1.ConditionTrue:
		return false
	default:
		return condition.Status == metav1.ConditionFalse && condition.Reason == ProgressingReason.String()
	}
}

// +kubebuilder:object:root=true

// ReleaseList contains a list of Release
type ReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Release `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Release{}, &ReleaseList{})
}
