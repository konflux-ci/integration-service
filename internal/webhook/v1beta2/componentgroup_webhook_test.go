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

package v1beta2

import (
	appstudiov1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ComponentGroup webhook", Ordered, func() {
	var (
		validator        *ComponentGroupCustomValidator
		componentGroup   *appstudiov1beta2.ComponentGroup
		validTestGraph   map[string][]appstudiov1beta2.TestGraphNode
		invalidTestGraph map[string][]appstudiov1beta2.TestGraphNode
	)

	BeforeAll(func() {
		// A   E
		//  \ / \
		//   C   B
		//       |
		//       D
		validTestGraph = map[string][]appstudiov1beta2.TestGraphNode{
			"scenarioA": {},
			"scenarioE": {},
			"scenarioB": {{Name: "scenarioE"}},
			"scenarioC": {{Name: "scenarioA"}, {Name: "scenarioE"}},
			"scenarioD": {{Name: "scenarioB"}},
		}

		// Cycle: scenarioC -> scenarioE -> scenarioC
		invalidTestGraph = map[string][]appstudiov1beta2.TestGraphNode{
			"scenarioA": {},
			"scenarioB": {{Name: "scenarioA"}},
			"scenarioC": {{Name: "scenarioA"}, {Name: "scenarioE"}},
			"scenarioD": {{Name: "scenarioB"}},
			"scenarioE": {{Name: "scenarioC"}},
		}
	})

	BeforeEach(func() {
		validator = &ComponentGroupCustomValidator{Client: k8sClient}
		componentGroup = &appstudiov1beta2.ComponentGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-component-group",
				Namespace: "default",
			},
			Spec: appstudiov1beta2.ComponentGroupSpec{
				Components: []appstudiov1beta2.ComponentReference{
					{
						Name: "component-a",
						ComponentVersion: appstudiov1beta2.ComponentVersionReference{
							Name:     "main",
							Revision: "abc123",
						},
					},
					{
						Name: "component-b",
						ComponentVersion: appstudiov1beta2.ComponentVersionReference{
							Name:     "v1",
							Revision: "def456",
						},
					},
				},
			},
		}
	})

	When("a new component is created", func() {
		It("Does not return an error for a valid test graph", func() {
			componentGroup.Spec.TestGraph = validTestGraph
			warnings, err := validator.ValidateCreate(ctx, componentGroup)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("Returns an error for an invalid test graph", func() {
			componentGroup.Spec.TestGraph = invalidTestGraph
			_, err := validator.ValidateCreate(ctx, componentGroup)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error validating test graph: invalid TestGraph - cycle detected"))
		})
	})

	When("a component is updated", func() {
		It("Does not return an error for a valid test graph", func() {

			componentGroup.Spec.TestGraph = validTestGraph
			warnings, err := validator.ValidateCreate(ctx, componentGroup)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())

			// make valid testGraph update
			newComponentGroup := componentGroup.DeepCopy()
			newComponentGroup.Spec.TestGraph["scenarioD"] = []appstudiov1beta2.TestGraphNode{{Name: "scenarioC"}}
			warnings, err = validator.ValidateUpdate(ctx, componentGroup, newComponentGroup)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())

		})

		It("Returns an error for an invalid test graph", func() {
			warnings, err := validator.ValidateCreate(ctx, componentGroup)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())

			newComponentGroup := componentGroup.DeepCopy()
			newComponentGroup.Spec.TestGraph = invalidTestGraph
			_, err = validator.ValidateUpdate(ctx, componentGroup, newComponentGroup)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error validating test graph: invalid TestGraph - cycle detected"))
		})
	})
})
