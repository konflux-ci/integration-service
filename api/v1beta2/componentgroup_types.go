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

const (
	ComponentGroupLabelPrefix          = konfluxv1beta2.ComponentGroupLabelPrefix
	ParentSnapshotAnnotation           = konfluxv1beta2.ParentSnapshotAnnotation
	OriginSnapshotAnnotation           = konfluxv1beta2.OriginSnapshotAnnotation
	MissingComponentVersionsAnnotation = konfluxv1beta2.MissingComponentVersionsAnnotation
)

type ComponentGroupSpec = konfluxv1beta2.ComponentGroupSpec
type ComponentReference = konfluxv1beta2.ComponentReference
type ComponentVersionReference = konfluxv1beta2.ComponentVersionReference
type TestGraphNode = konfluxv1beta2.TestGraphNode
type SnapshotCreatorSpec = konfluxv1beta2.SnapshotCreatorSpec
type TaskRef = konfluxv1beta2.TaskRef
type ComponentState = konfluxv1beta2.ComponentState
type ComponentGroupStatus = konfluxv1beta2.ComponentGroupStatus
type ComponentGroup = konfluxv1beta2.ComponentGroup
type ComponentGroupList = konfluxv1beta2.ComponentGroupList
