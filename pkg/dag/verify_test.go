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

package dag

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/konflux-ci/integration-service/api/v1beta2"
)

var _ = Describe("DAG library unittests", Ordered, func() {
	var (
		//integrationTestScenario *v1beta2.IntegrationTestScenario
		testGraphWithoutCycles map[string][]v1beta2.TestGraphNode
		testGraphWithCycles    map[string][]v1beta2.TestGraphNode
	)
	Context("Checking graph for cycles", func() {
		BeforeAll(func() {
			// Create testGraph with no cycles
			//    A   E
			//   / \ /
			//  B   C
			//  |
			//  D
			testGraphWithoutCycles = make(map[string][]v1beta2.TestGraphNode)
			testGraphWithoutCycles["scenarioA"] = createTestGraphNodesForScenarios(nil)
			testGraphWithoutCycles["scenarioB"] = createTestGraphNodesForScenarios([]string{"scenarioA"})
			testGraphWithoutCycles["scenarioC"] = createTestGraphNodesForScenarios([]string{"scenarioA", "scenarioE"})
			testGraphWithoutCycles["scenarioD"] = createTestGraphNodesForScenarios([]string{"scenarioB"})
			testGraphWithoutCycles["scenarioE"] = createTestGraphNodesForScenarios(nil)

			// Create testGraph with cycles. 'E' is repeated
			//    A   E
			//   / \ /
			//  B   C
			//  |   |
			//  D   E
			testGraphWithCycles = make(map[string][]v1beta2.TestGraphNode)
			testGraphWithCycles["scenarioA"] = createTestGraphNodesForScenarios([]string{})
			testGraphWithCycles["scenarioB"] = createTestGraphNodesForScenarios([]string{"scenarioA"})
			testGraphWithCycles["scenarioC"] = createTestGraphNodesForScenarios([]string{"scenarioA", "scenarioE"})
			testGraphWithCycles["scenarioD"] = createTestGraphNodesForScenarios([]string{"scenarioB"})
			testGraphWithCycles["scenarioE"] = createTestGraphNodesForScenarios([]string{"scenarioC"})
		})

		When("A Graph without cycles is checked", func() {
			It("Returns no error", func() {
				err := checkGraphForCycles(testGraphWithoutCycles)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("A Graph with cycles is checked", func() {
			It("Returns no error", func() {
				err := checkGraphForCycles(testGraphWithCycles)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cycles found in graph"))
			})
		})
	})
})

func createTestGraphNodesForScenarios(scenarioNames []string) (nodes []v1beta2.TestGraphNode) {
	for _, name := range scenarioNames {
		nodes = append(nodes, v1beta2.TestGraphNode{Name: name, FailFast: false})
	}
	return
}
