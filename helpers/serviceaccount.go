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

package helpers

import (
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
)

// ComponentIntegrationPipelineServiceAccountName returns the name of the per-component ServiceAccount
// that integration PipelineRuns on a component Snapshot run as (STONEINTG-1799). The name derives from
// the Component's metadata name, which is also the value of the Snapshot's component label that
// STONEINTG-1800 uses to select the account.
func ComponentIntegrationPipelineServiceAccountName(componentName string) string {
	return tektonconsts.ComponentIntegrationPipelineServiceAccountPrefix + componentName
}
