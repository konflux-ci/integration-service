/*
   Copyright 2025 Red Hat Inc.

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

package dag

import (
	"fmt"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
)

// GetRunAfterMapForTestGraphNodes generates a RunAfter map which contains the list of scenarios that a given scenario node
// should run after. It checks if a child is in the list of provided scenarios before appending to the runAfter map
func GetRunAfterMapForTestGraphNodes(testGraph map[string][]v1beta2.TestGraphNode, integrationTestScenarios *[]v1beta2.IntegrationTestScenario) map[string][]string {
	runAfterMap := make(map[string][]string)
	if integrationTestScenarios == nil {
		return runAfterMap
	}

	scenarioSet := make(map[string]struct{}, len(*integrationTestScenarios))
	for _, scenario := range *integrationTestScenarios {
		scenarioSet[scenario.Name] = struct{}{}
	}

	for node, parents := range testGraph {
		runAfterMap[node] = make([]string, 0)
		if _, exists := scenarioSet[node]; !exists {
			continue
		}
		for _, parentNode := range parents {
			if _, exists := scenarioSet[parentNode.Name]; exists {
				runAfterMap[node] = append(runAfterMap[node], parentNode.Name)
			}
		}
	}
	return runAfterMap
}

// hasParent reports whether parentName appears in the child's declared parent list.
func hasParent(parents []v1beta2.TestGraphNode, parentName string) bool {
	for _, parent := range parents {
		if parent.Name == parentName {
			return true
		}
	}
	return false
}

// ShouldFailFastForScenariosParent finds the requested parent's failFast option for a given scenario
func ShouldFailFastForScenariosParent(testGraph map[string][]v1beta2.TestGraphNode, scenarioName, parentScenarioName string) bool {
	if parents, ok := testGraph[scenarioName]; ok {
		for _, parentNode := range parents {
			if parentNode.Name == parentScenarioName {
				return parentNode.FailFast
			}
		}
	}
	return false
}

// CascadeFailFastDependents walks the testGraph from failedScenario and marks
// all transitively blocked dependents as TestFail when each edge has failFast: true.
func CascadeFailFastDependents(
	testGraph map[string][]v1beta2.TestGraphNode,
	statuses *intgteststat.SnapshotIntegrationTestStatuses,
	failedScenario string,
) {
	queue := []string{failedScenario}
	seen := map[string]struct{}{failedScenario: {}}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for child, parents := range testGraph {
			if !hasParent(parents, parent) {
				continue
			}
			if !ShouldFailFastForScenariosParent(testGraph, child, parent) {
				continue
			}
			detail, ok := statuses.GetScenarioStatus(child)
			if !ok || detail.Status.IsFinal() {
				continue
			}
			statuses.UpdateTestStatusIfChanged(
				child,
				intgteststat.IntegrationTestStatusTestFail,
				fmt.Sprintf("Cancelled on account of required %s failing", parent),
			)
			if _, already := seen[child]; !already {
				seen[child] = struct{}{}
				queue = append(queue, child) // cascade B → C, C → D, ...
			}
		}
	}
}
