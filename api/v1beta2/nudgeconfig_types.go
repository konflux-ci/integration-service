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

import konfluxv1beta2 "github.com/konflux-ci/integration-service/api/konflux/v1beta2"

const (
	NudgeConfigSingletonName              = konfluxv1beta2.NudgeConfigSingletonName
	NudgeModeImmediate                    = konfluxv1beta2.NudgeModeImmediate
	NudgeModeValidated                    = konfluxv1beta2.NudgeModeValidated
	FailurePolicyBlock                    = konfluxv1beta2.FailurePolicyBlock
	FailurePolicyProceedWithPartial       = konfluxv1beta2.FailurePolicyProceedWithPartial
	DefaultBatchDebounceTimeout           = konfluxv1beta2.DefaultBatchDebounceTimeout
	DefaultBatchMaxWaitTime               = konfluxv1beta2.DefaultBatchMaxWaitTime
	DefaultBatchFailurePolicy             = konfluxv1beta2.DefaultBatchFailurePolicy
	NudgeConfigConditionForceFireRejected = konfluxv1beta2.NudgeConfigConditionForceFireRejected
	NudgeConfigConditionForceFireIgnored  = konfluxv1beta2.NudgeConfigConditionForceFireIgnored
	BatchPhaseAccumulating                = konfluxv1beta2.BatchPhaseAccumulating
	BatchPhaseBlocked                     = konfluxv1beta2.BatchPhaseBlocked
	BatchPhaseFiring                      = konfluxv1beta2.BatchPhaseFiring
	BatchPhaseFailed                      = konfluxv1beta2.BatchPhaseFailed
	BatchPhaseCompleted                   = konfluxv1beta2.BatchPhaseCompleted
)

type NudgeModeType = konfluxv1beta2.NudgeModeType
type NudgeRelationship = konfluxv1beta2.NudgeRelationship
type FailurePolicyType = konfluxv1beta2.FailurePolicyType
type BatchDefaults = konfluxv1beta2.BatchDefaults
type BatchPolicy = konfluxv1beta2.BatchPolicy
type TargetConfig = konfluxv1beta2.TargetConfig
type Actions = konfluxv1beta2.Actions
type ForceFireAction = konfluxv1beta2.ForceFireAction
type NudgeConfigSpec = konfluxv1beta2.NudgeConfigSpec
type BatchPhase = konfluxv1beta2.BatchPhase
type AccumulatedEntry = konfluxv1beta2.AccumulatedEntry
type FailedEntry = konfluxv1beta2.FailedEntry
type ActiveBatch = konfluxv1beta2.ActiveBatch
type NudgeConfigStatus = konfluxv1beta2.NudgeConfigStatus
type NudgeConfig = konfluxv1beta2.NudgeConfig
type NudgeConfigList = konfluxv1beta2.NudgeConfigList
