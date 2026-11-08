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

	v1beta3 "github.com/konflux-ci/integration-service/api/konflux/v1beta3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this NudgeConfig to the Hub version (v1beta3).
func (src *NudgeConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*v1beta3.NudgeConfig)
	if !ok {
		return fmt.Errorf("expected *v1beta3.NudgeConfig hub, got %T", dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertNudgeConfigSpecToV1Beta3(src.Spec)
	dst.Status = convertNudgeConfigStatusToV1Beta3(src.Status)
	return nil
}

// ConvertFrom converts this NudgeConfig from the Hub version (v1beta3).
func (dst *NudgeConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*v1beta3.NudgeConfig)
	if !ok {
		return fmt.Errorf("expected *v1beta3.NudgeConfig hub, got %T", srcRaw)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertNudgeConfigSpecFromV1Beta3(src.Spec)
	dst.Status = convertNudgeConfigStatusFromV1Beta3(src.Status)
	return nil
}

func convertNudgeConfigSpecToV1Beta3(src NudgeConfigSpec) v1beta3.NudgeConfigSpec {
	dst := v1beta3.NudgeConfigSpec{
		BatchDefaults: convertBatchDefaultsToV1Beta3(src.BatchDefaults),
		Actions:       convertActionsToV1Beta3(src.Actions),
		TargetConfig:  convertTargetConfigToV1Beta3(src.TargetConfig),
	}
	if src.Nudges != nil {
		dst.Nudges = make([]v1beta3.NudgeRelationship, len(src.Nudges))
		for i, nudge := range src.Nudges {
			dst.Nudges[i] = v1beta3.NudgeRelationship{
				From:        v1beta3.ComponentVersion{Component: nudge.From},
				To:          v1beta3.ComponentVersion{Component: nudge.To},
				Mode:        v1beta3.NudgeModeType(nudge.Mode),
				GatingGroup: nudge.GatingGroup,
			}
		}
	}
	return dst
}

func convertNudgeConfigSpecFromV1Beta3(src v1beta3.NudgeConfigSpec) NudgeConfigSpec {
	dst := NudgeConfigSpec{
		BatchDefaults: convertBatchDefaultsFromV1Beta3(src.BatchDefaults),
		Actions:       convertActionsFromV1Beta3(src.Actions),
		TargetConfig:  convertTargetConfigFromV1Beta3(src.TargetConfig),
	}
	if src.Nudges != nil {
		dst.Nudges = make([]NudgeRelationship, len(src.Nudges))
		for i, nudge := range src.Nudges {
			dst.Nudges[i] = NudgeRelationship{
				From:        nudge.From.Component,
				To:          nudge.To.Component,
				Mode:        NudgeModeType(nudge.Mode),
				GatingGroup: nudge.GatingGroup,
			}
		}
	}
	return dst
}

func convertBatchDefaultsToV1Beta3(src *BatchDefaults) *v1beta3.BatchDefaults {
	if src == nil {
		return nil
	}
	dst := &v1beta3.BatchDefaults{
		DebounceTimeout: copyDuration(src.DebounceTimeout),
		MaxWaitTime:     copyDuration(src.MaxWaitTime),
	}
	if src.FailurePolicy != nil {
		failurePolicy := v1beta3.FailurePolicyType(*src.FailurePolicy)
		dst.FailurePolicy = &failurePolicy
	}
	return dst
}

func convertBatchDefaultsFromV1Beta3(src *v1beta3.BatchDefaults) *BatchDefaults {
	if src == nil {
		return nil
	}
	dst := &BatchDefaults{
		DebounceTimeout: copyDuration(src.DebounceTimeout),
		MaxWaitTime:     copyDuration(src.MaxWaitTime),
	}
	if src.FailurePolicy != nil {
		failurePolicy := FailurePolicyType(*src.FailurePolicy)
		dst.FailurePolicy = &failurePolicy
	}
	return dst
}

func convertBatchPolicyToV1Beta3(src *BatchPolicy) *v1beta3.BatchPolicy {
	if src == nil {
		return nil
	}
	dst := &v1beta3.BatchPolicy{
		DebounceTimeout: copyDuration(src.DebounceTimeout),
		MaxWaitTime:     copyDuration(src.MaxWaitTime),
	}
	if src.FailurePolicy != nil {
		failurePolicy := v1beta3.FailurePolicyType(*src.FailurePolicy)
		dst.FailurePolicy = &failurePolicy
	}
	return dst
}

func convertBatchPolicyFromV1Beta3(src *v1beta3.BatchPolicy) *BatchPolicy {
	if src == nil {
		return nil
	}
	dst := &BatchPolicy{
		DebounceTimeout: copyDuration(src.DebounceTimeout),
		MaxWaitTime:     copyDuration(src.MaxWaitTime),
	}
	if src.FailurePolicy != nil {
		failurePolicy := FailurePolicyType(*src.FailurePolicy)
		dst.FailurePolicy = &failurePolicy
	}
	return dst
}

func convertTargetConfigToV1Beta3(src []TargetConfig) []v1beta3.TargetConfig {
	if src == nil {
		return nil
	}
	dst := make([]v1beta3.TargetConfig, len(src))
	for i, targetConfig := range src {
		dst[i] = v1beta3.TargetConfig{
			Target:      targetConfig.Target,
			BatchPolicy: convertBatchPolicyToV1Beta3(targetConfig.BatchPolicy),
		}
	}
	return dst
}

func convertTargetConfigFromV1Beta3(src []v1beta3.TargetConfig) []TargetConfig {
	if src == nil {
		return nil
	}
	dst := make([]TargetConfig, len(src))
	for i, targetConfig := range src {
		dst[i] = TargetConfig{
			Target:      targetConfig.Target,
			BatchPolicy: convertBatchPolicyFromV1Beta3(targetConfig.BatchPolicy),
		}
	}
	return dst
}

func convertActionsToV1Beta3(src *Actions) *v1beta3.Actions {
	if src == nil {
		return nil
	}
	dst := &v1beta3.Actions{}
	if src.ForceFire != nil {
		dst.ForceFire = &v1beta3.ForceFireAction{
			Target:         src.ForceFire.Target,
			IncludePartial: copyBool(src.ForceFire.IncludePartial),
		}
	}
	return dst
}

func convertActionsFromV1Beta3(src *v1beta3.Actions) *Actions {
	if src == nil {
		return nil
	}
	dst := &Actions{}
	if src.ForceFire != nil {
		dst.ForceFire = &ForceFireAction{
			Target:         src.ForceFire.Target,
			IncludePartial: copyBool(src.ForceFire.IncludePartial),
		}
	}
	return dst
}

func convertNudgeConfigStatusToV1Beta3(src NudgeConfigStatus) v1beta3.NudgeConfigStatus {
	dst := v1beta3.NudgeConfigStatus{
		Conditions:         copyConditions(src.Conditions),
		LastValidationTime: copyTime(src.LastValidationTime),
	}
	if src.ActiveBatches != nil {
		dst.ActiveBatches = make([]v1beta3.ActiveBatch, len(src.ActiveBatches))
		for i, batch := range src.ActiveBatches {
			dst.ActiveBatches[i] = v1beta3.ActiveBatch{
				Target:           batch.Target,
				BatchID:          batch.BatchID,
				Phase:            v1beta3.BatchPhase(batch.Phase),
				CreatedAt:        batch.CreatedAt,
				FireAt:           copyTime(batch.FireAt),
				HardDeadline:     batch.HardDeadline,
				Accumulated:      convertAccumulatedEntriesToV1Beta3(batch.Accumulated),
				Failed:           convertFailedEntriesToV1Beta3(batch.Failed),
				NudgePipelineRun: batch.NudgePipelineRun,
				Message:          batch.Message,
				RetryCount:       batch.RetryCount,
			}
		}
	}
	return dst
}

func convertNudgeConfigStatusFromV1Beta3(src v1beta3.NudgeConfigStatus) NudgeConfigStatus {
	dst := NudgeConfigStatus{
		Conditions:         copyConditions(src.Conditions),
		LastValidationTime: copyTime(src.LastValidationTime),
	}
	if src.ActiveBatches != nil {
		dst.ActiveBatches = make([]ActiveBatch, len(src.ActiveBatches))
		for i, batch := range src.ActiveBatches {
			dst.ActiveBatches[i] = ActiveBatch{
				Target:           batch.Target,
				BatchID:          batch.BatchID,
				Phase:            BatchPhase(batch.Phase),
				CreatedAt:        batch.CreatedAt,
				FireAt:           copyTime(batch.FireAt),
				HardDeadline:     batch.HardDeadline,
				Accumulated:      convertAccumulatedEntriesFromV1Beta3(batch.Accumulated),
				Failed:           convertFailedEntriesFromV1Beta3(batch.Failed),
				NudgePipelineRun: batch.NudgePipelineRun,
				Message:          batch.Message,
				RetryCount:       batch.RetryCount,
			}
		}
	}
	return dst
}

func convertAccumulatedEntriesToV1Beta3(src []AccumulatedEntry) []v1beta3.AccumulatedEntry {
	if src == nil {
		return nil
	}
	dst := make([]v1beta3.AccumulatedEntry, len(src))
	for i, entry := range src {
		dst[i] = v1beta3.AccumulatedEntry{
			From:               entry.From,
			ImageDigest:        entry.ImageDigest,
			BuildPipelineRun:   entry.BuildPipelineRun,
			CapturedAt:         entry.CapturedAt,
			ServiceAccountName: entry.ServiceAccountName,
		}
	}
	return dst
}

func convertAccumulatedEntriesFromV1Beta3(src []v1beta3.AccumulatedEntry) []AccumulatedEntry {
	if src == nil {
		return nil
	}
	dst := make([]AccumulatedEntry, len(src))
	for i, entry := range src {
		dst[i] = AccumulatedEntry{
			From:               entry.From,
			ImageDigest:        entry.ImageDigest,
			BuildPipelineRun:   entry.BuildPipelineRun,
			CapturedAt:         entry.CapturedAt,
			ServiceAccountName: entry.ServiceAccountName,
		}
	}
	return dst
}

func convertFailedEntriesToV1Beta3(src []FailedEntry) []v1beta3.FailedEntry {
	if src == nil {
		return nil
	}
	dst := make([]v1beta3.FailedEntry, len(src))
	for i, entry := range src {
		dst[i] = v1beta3.FailedEntry(entry)
	}
	return dst
}

func convertFailedEntriesFromV1Beta3(src []v1beta3.FailedEntry) []FailedEntry {
	if src == nil {
		return nil
	}
	dst := make([]FailedEntry, len(src))
	for i, entry := range src {
		dst[i] = FailedEntry(entry)
	}
	return dst
}

func copyDuration(src *metav1.Duration) *metav1.Duration {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}

func copyTime(src *metav1.Time) *metav1.Time {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}

func copyBool(src *bool) *bool {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}

func copyConditions(src []metav1.Condition) []metav1.Condition {
	if src == nil {
		return nil
	}
	dst := make([]metav1.Condition, len(src))
	copy(dst, src)
	return dst
}
