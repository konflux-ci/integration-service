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
		testGraphWithoutCycles map[string][]v1beta2.TestGraphNode
		testGraphWithCycles    map[string][]v1beta2.TestGraphNode
	)
	Context("Checking graph for cycles via ValidateTestGraph", func() {
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
				err := ValidateTestGraph(testGraphWithoutCycles)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("A Graph with cycles is checked", func() {
			It("Returns an error with the cycle path", func() {
				err := ValidateTestGraph(testGraphWithCycles)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid TestGraph - cycle detected"))
				Expect(err.Error()).To(ContainSubstring("scenarioC"))
				Expect(err.Error()).To(ContainSubstring("scenarioE"))
			})
		})
	})

	Context("Shared checkGraphForCycles", func() {
		When("the graph is a valid linear chain", func() {
			It("returns no error", func() {
				adj := map[string][]string{
					"a": {"b"},
					"b": {"c"},
					"c": {"d"},
					"d": nil,
				}
				Expect(checkGraphForCycles(adj)).NotTo(HaveOccurred())
			})
		})

		When("the graph is a valid DAG with diamond shape", func() {
			It("returns no error", func() {
				adj := map[string][]string{
					"a": {"b", "c"},
					"b": {"d"},
					"c": {"d"},
					"d": nil,
				}
				Expect(checkGraphForCycles(adj)).NotTo(HaveOccurred())
			})
		})

		When("the graph has a direct cycle", func() {
			It("returns the cycle path", func() {
				adj := map[string][]string{
					"a": {"b"},
					"b": {"a"},
				}
				err := checkGraphForCycles(adj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("a"))
				Expect(err.Error()).To(ContainSubstring("b"))
			})
		})

		When("the graph has a transitive 3-node cycle", func() {
			It("returns the full cycle path", func() {
				adj := map[string][]string{
					"a": {"b"},
					"b": {"c"},
					"c": {"a"},
				}
				err := checkGraphForCycles(adj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("a"))
				Expect(err.Error()).To(ContainSubstring("b"))
				Expect(err.Error()).To(ContainSubstring("c"))
			})
		})

		When("the graph has a self-loop", func() {
			It("returns the self-loop as a cycle", func() {
				adj := map[string][]string{
					"a": {"a"},
				}
				err := checkGraphForCycles(adj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("a -> a"))
			})
		})

		When("the graph is empty", func() {
			It("returns no error", func() {
				Expect(checkGraphForCycles(map[string][]string{})).NotTo(HaveOccurred())
			})
		})

		When("the graph is disconnected with a cycle in one component", func() {
			It("detects the cycle and reports the cycle path", func() {
				adj := map[string][]string{
					"e": {"f"},
					"f": nil,
					"a": {"b"},
					"b": {"c"},
					"c": {"a"},
				}
				err := checkGraphForCycles(adj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("a"))
				Expect(err.Error()).To(ContainSubstring("b"))
				Expect(err.Error()).To(ContainSubstring("c"))
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

var _ = Describe("Nudge graph cycle detection", func() {
	Context("ValidateNudgeGraph", func() {
		When("nudges form a valid chain", func() {
			It("returns no error", func() {
				nudges := []v1beta2.NudgeRelationship{
					{From: "a", To: "b"},
					{From: "b", To: "c"},
				}
				Expect(ValidateNudgeGraph(nudges)).NotTo(HaveOccurred())
			})
		})

		When("nudges form a cycle", func() {
			It("returns an error with the cycle path", func() {
				nudges := []v1beta2.NudgeRelationship{
					{From: "a", To: "b"},
					{From: "b", To: "c"},
					{From: "c", To: "a"},
				}
				err := ValidateNudgeGraph(nudges)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid NudgeGraph - cycle detected"))
				Expect(err.Error()).To(ContainSubstring("a"))
				Expect(err.Error()).To(ContainSubstring("b"))
				Expect(err.Error()).To(ContainSubstring("c"))
			})
		})

		When("nudges list is empty", func() {
			It("returns no error", func() {
				Expect(ValidateNudgeGraph(nil)).NotTo(HaveOccurred())
				Expect(ValidateNudgeGraph([]v1beta2.NudgeRelationship{})).NotTo(HaveOccurred())
			})
		})

		When("a self-nudge is present", func() {
			It("detects it as a cycle without panicking", func() {
				nudges := []v1beta2.NudgeRelationship{
					{From: "a", To: "a"},
				}
				err := ValidateNudgeGraph(nudges)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid NudgeGraph - cycle detected: a -> a"))
			})
		})

		When("nudges form a valid DAG with multiple paths", func() {
			It("returns no error", func() {
				nudges := []v1beta2.NudgeRelationship{
					{From: "a", To: "b"},
					{From: "a", To: "c"},
					{From: "b", To: "d"},
					{From: "c", To: "d"},
				}
				Expect(ValidateNudgeGraph(nudges)).NotTo(HaveOccurred())
			})
		})
	})
})
