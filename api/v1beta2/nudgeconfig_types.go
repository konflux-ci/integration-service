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

import konfluxv1beta2 "github.com/konflux-ci/integration-service/api/konflux/v1beta2"

const NudgeConfigSingletonName = konfluxv1beta2.NudgeConfigSingletonName

type NudgeModeType = konfluxv1beta2.NudgeModeType
type NudgeRelationship = konfluxv1beta2.NudgeRelationship
type NudgeConfigSpec = konfluxv1beta2.NudgeConfigSpec
type NudgeConfigStatus = konfluxv1beta2.NudgeConfigStatus
type NudgeConfig = konfluxv1beta2.NudgeConfig
type NudgeConfigList = konfluxv1beta2.NudgeConfigList

const (
	NudgeModeImmediate = konfluxv1beta2.NudgeModeImmediate
	NudgeModeValidated = konfluxv1beta2.NudgeModeValidated
)
