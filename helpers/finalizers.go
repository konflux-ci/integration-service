/*
Copyright 2023 Red Hat Inc.

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

package helpers

import "github.com/konflux-ci/integration-service/pkg/keys"

// IntegrationPipelineRunFinalizer is the finalizer name to be added to the Integration PipelineRuns
// Migration note: retained during ComponentGroup migration; remove after migration is complete.
const IntegrationPipelineRunFinalizer string = keys.PrefixTestOld + "/pipelinerun"

// Migration note: retained during ComponentGroup migration; remove after migration is complete.
const IntegrationTestScenarioFinalizer string = keys.PrefixTestOld + "/scenario"

// Migration note: retained during ComponentGroup migration; remove after migration is complete.
const ComponentFinalizer string = keys.PrefixTestOld + "/component"

// NudgePipelineRunFinalizer is the finalizer name added to build PipelineRuns while IS is
// actively creating a nudge PipelineRun, preventing premature GC before nudging completes.
// Migration note: retained during ComponentGroup migration; remove after migration is complete.
const NudgePipelineRunFinalizer string = keys.PrefixTestOld + "/nudge-pipelinerun"
