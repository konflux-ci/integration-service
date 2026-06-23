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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
)

var _ = Describe("DAG utils unittests", Ordered, func() {
	Context("GetRunAfterMapForTestGraphNodes", func() {
		var (
			testGraph   map[string][]v1beta2.TestGraphNode
			scenarios   *[]v1beta2.IntegrationTestScenario
			runAfterMap map[string][]string
		)

		BeforeAll(func() {
			testGraph = map[string][]v1beta2.TestGraphNode{
				"scenarioB": createTestGraphNodesForScenarios([]string{"scenarioA"}),
				"scenarioC": createTestGraphNodesForScenarios([]string{"scenarioA", "scenarioB"}),
				"scenarioD": createTestGraphNodesForScenarios([]string{"missing-parent"}),
				"scenarioE": createTestGraphNodesForScenarios([]string{"missing-parent"}),
			}
			scenarios = createIntegrationTestScenarios("scenarioA", "scenarioB", "scenarioC", "scenarioD")
		})

		When("the child scenario is in the integration test scenario list", func() {
			It("returns the parent scenario names in the runAfter map", func() {
				runAfterMap = GetRunAfterMapForTestGraphNodes(testGraph, scenarios)
				Expect(runAfterMap).To(HaveKey("scenarioB"))
				Expect(runAfterMap["scenarioB"]).To(Equal([]string{"scenarioA"}))
				Expect(runAfterMap["scenarioC"]).To(Equal([]string{"scenarioA", "scenarioB"}))
			})
		})

		When("the parent scenario is not in the integration test scenario list", func() {
			It("returns an empty runAfter list for that node", func() {
				runAfterMap = GetRunAfterMapForTestGraphNodes(testGraph, scenarios)
				Expect(runAfterMap).To(HaveKey("scenarioD"))
				Expect(runAfterMap["scenarioD"]).To(BeEmpty())
			})
		})

		When("the child scenario is not in the integration test scenario list", func() {
			It("returns an empty runAfter list for that node", func() {
				runAfterMap = GetRunAfterMapForTestGraphNodes(testGraph, scenarios)
				Expect(runAfterMap).To(HaveKey("scenarioE"))
				Expect(runAfterMap["scenarioD"]).To(BeEmpty())
			})
		})
	})

	Context("hasParent", func() {
		var parents []v1beta2.TestGraphNode

		BeforeAll(func() {
			parents = []v1beta2.TestGraphNode{
				{Name: "scenarioA", FailFast: true},
				{Name: "scenarioB", FailFast: false},
			}
		})

		It("returns true when the parent name is in the list", func() {
			Expect(hasParent(parents, "scenarioA")).To(BeTrue())
			Expect(hasParent(parents, "scenarioB")).To(BeTrue())
		})

		It("returns false when the parent name is not in the list", func() {
			Expect(hasParent(parents, "scenarioC")).To(BeFalse())
		})

		It("returns false for an empty parent list", func() {
			Expect(hasParent(nil, "scenarioA")).To(BeFalse())
			Expect(hasParent([]v1beta2.TestGraphNode{}, "scenarioA")).To(BeFalse())
		})
	})

	Context("ShouldFailFastForScenariosParent", func() {
		var testGraph map[string][]v1beta2.TestGraphNode

		BeforeAll(func() {
			testGraph = map[string][]v1beta2.TestGraphNode{
				"scenarioB": {
					{Name: "scenarioA", FailFast: true},
				},
				"scenarioC": {
					{Name: "scenarioB", FailFast: false},
				},
			}
		})

		It("returns the failFast value for a matching parent edge", func() {
			Expect(ShouldFailFastForScenariosParent(testGraph, "scenarioB", "scenarioA")).To(BeTrue())
			Expect(ShouldFailFastForScenariosParent(testGraph, "scenarioC", "scenarioB")).To(BeFalse())
		})

		It("returns false when the parent edge is not found", func() {
			Expect(ShouldFailFastForScenariosParent(testGraph, "scenarioB", "scenarioC")).To(BeFalse())
			Expect(ShouldFailFastForScenariosParent(testGraph, "scenarioD", "scenarioA")).To(BeFalse())
		})
	})

	Context("CascadeFailFastDependents", func() {
		var (
			testGraph map[string][]v1beta2.TestGraphNode
			statuses  *intgteststat.SnapshotIntegrationTestStatuses
		)

		initPendingStatuses := func(scenarioNames ...string) {
			var err error
			statuses, err = intgteststat.NewSnapshotIntegrationTestStatuses("")
			Expect(err).NotTo(HaveOccurred())
			for _, name := range scenarioNames {
				statuses.UpdateTestStatusIfChanged(name, intgteststat.IntegrationTestStatusPending, "Pending")
			}
		}

		getScenarioStatus := func(name string) intgteststat.IntegrationTestStatus {
			detail, ok := statuses.GetScenarioStatus(name)
			Expect(ok).To(BeTrue())
			return detail.Status
		}

		BeforeAll(func() {
			testGraph = map[string][]v1beta2.TestGraphNode{
				"scenarioB": {{Name: "scenarioA", FailFast: true}},
				"scenarioC": {{Name: "scenarioB", FailFast: true}},
				"scenarioD": {{Name: "scenarioB", FailFast: false}},
			}
		})

		When("a direct dependent has failFast set to true", func() {
			BeforeEach(func() {
				initPendingStatuses("scenarioB")
			})

			It("marks the dependent as TestFail", func() {
				CascadeFailFastDependents(testGraph, statuses, "scenarioA")
				Expect(getScenarioStatus("scenarioB")).To(Equal(intgteststat.IntegrationTestStatusTestFail))
				detail, ok := statuses.GetScenarioStatus("scenarioB")
				Expect(ok).To(BeTrue())
				Expect(detail.Details).To(Equal("Cancelled on account of required scenarioA failing"))
			})
		})

		When("a transitive chain has failFast set to true on each edge", func() {
			BeforeEach(func() {
				initPendingStatuses("scenarioB", "scenarioC")
			})

			It("marks all blocked dependents as TestFail", func() {
				CascadeFailFastDependents(testGraph, statuses, "scenarioA")
				Expect(getScenarioStatus("scenarioB")).To(Equal(intgteststat.IntegrationTestStatusTestFail))
				Expect(getScenarioStatus("scenarioC")).To(Equal(intgteststat.IntegrationTestStatusTestFail))
			})
		})

		When("failFast is false on the edge to a dependent", func() {
			BeforeEach(func() {
				initPendingStatuses("scenarioB", "scenarioD")
			})

			It("does not cancel the dependent", func() {
				CascadeFailFastDependents(testGraph, statuses, "scenarioB")
				Expect(getScenarioStatus("scenarioD")).To(Equal(intgteststat.IntegrationTestStatusPending))
			})
		})

		When("the dependent already has a final status", func() {
			BeforeEach(func() {
				initPendingStatuses("scenarioB")
				statuses.UpdateTestStatusIfChanged(
					"scenarioB", intgteststat.IntegrationTestStatusTestPassed, "passed",
				)
			})

			It("does not change the dependent status", func() {
				CascadeFailFastDependents(testGraph, statuses, "scenarioA")
				Expect(getScenarioStatus("scenarioB")).To(Equal(intgteststat.IntegrationTestStatusTestPassed))
			})
		})

		When("cascade is invoked more than once", func() {
			BeforeEach(func() {
				initPendingStatuses("scenarioB")
			})

			It("is idempotent", func() {
				CascadeFailFastDependents(testGraph, statuses, "scenarioA")
				detailAfterFirst, ok := statuses.GetScenarioStatus("scenarioB")
				Expect(ok).To(BeTrue())
				CascadeFailFastDependents(testGraph, statuses, "scenarioA")
				detailAfterSecond, ok := statuses.GetScenarioStatus("scenarioB")
				Expect(ok).To(BeTrue())
				Expect(detailAfterSecond.Status).To(Equal(detailAfterFirst.Status))
				Expect(detailAfterSecond.Details).To(Equal(detailAfterFirst.Details))
				Expect(detailAfterSecond.LastUpdateTime).To(Equal(detailAfterFirst.LastUpdateTime))
			})
		})
	})
})

func createIntegrationTestScenarios(names ...string) *[]v1beta2.IntegrationTestScenario {
	scenarios := make([]v1beta2.IntegrationTestScenario, len(names))
	for i, name := range names {
		scenarios[i] = v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}
	}
	return &scenarios
}
