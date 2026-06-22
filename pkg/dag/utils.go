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
	"maps"

	"github.com/konflux-ci/integration-service/api/v1beta2"
)

// FilterRootIntegrationTestScenarios uses Kahn's algorithm on the reversed dependency graph to
// get root IntegrationTestScenarios.
func FilterRootIntegrationTestScenarios(testGraph map[string][]v1beta2.TestGraphNode, scenarios *[]v1beta2.IntegrationTestScenario) *[]v1beta2.IntegrationTestScenario {
	// indegrees[x] = number of nodes that list x as a parent (i.e. x's child count).
	var filteredScenarios []v1beta2.IntegrationTestScenario
	graphCopy := maps.Clone(testGraph)

	for _, scenario := range *scenarios {
		// Ensure every scenario appears in testGraph even if it has no declared
		// parents.
		if _, ok := graphCopy[scenario.Name]; !ok {
			graphCopy[scenario.Name] = []v1beta2.TestGraphNode{}
		}
	}

	// Find root nodes (ones which don't depend on any other nodes) and match them with a scenario.
	for node := range graphCopy {
		if len(graphCopy[node]) == 0 {
			for _, scenario := range *scenarios {
				if scenario.Name == node {
					filteredScenarios = append(filteredScenarios, scenario)
					break
				}
			}
		}
	}
	return &filteredScenarios
}

// GetRunAfterMapForTestGraphNodes generates a RunAfter map which contains the list of scenarios that a given scenario node
// should run after. It checks if a parentNode is in the list of provided scenarios before appending to the runAfter map
func GetRunAfterMapForTestGraphNodes(testGraph map[string][]v1beta2.TestGraphNode, integrationTestScenarios *[]v1beta2.IntegrationTestScenario) map[string][]string {
	runAfterMap := make(map[string][]string)

	for node := range testGraph {
		runAfterMap[node] = make([]string, 0)
		for _, parentNode := range testGraph[node] {
			for _, integrationTestScenario := range *integrationTestScenarios {
				if integrationTestScenario.Name == node {
					runAfterMap[node] = append(runAfterMap[node], parentNode.Name)
					break
				}
			}
		}
	}
	return runAfterMap
}

// ShouldFailFastForScenariosParent find the requested parent's failFast option for a given scenario
func ShouldFailFastForScenariosParent(testGraph map[string][]v1beta2.TestGraphNode, scenarioName, parentScenarioName string) bool {
	failFast := false

	for node := range testGraph {
		if node == scenarioName {
			for _, parentNode := range testGraph[node] {
				if parentNode.Name == parentScenarioName {
					failFast = parentNode.FailFast
				}
			}
		}
	}
	return failFast
}
