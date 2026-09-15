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

package helpers_test

import (
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newComponent(name string) applicationapiv1alpha1.Component {
	return applicationapiv1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}

var _ = Describe("Helpers for nudges", func() {

	Context("FindMissingNudgeConfigReferences", func() {
		const namespace = "test-namespace"

		It("returns false and an empty message when all nudge references exist", func() {
			components := []applicationapiv1alpha1.Component{
				newComponent("comp-a"),
				newComponent("comp-b"),
			}
			nudges := []v1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
			}

			missing, msg := helpers.FindMissingNudgeConfigReferences(components, nudges, namespace)
			Expect(missing).To(BeFalse())
			Expect(msg).To(BeEmpty())
		})

		It("returns false and an empty message when nudges is empty", func() {
			components := []applicationapiv1alpha1.Component{
				newComponent("comp-a"),
			}

			missing, msg := helpers.FindMissingNudgeConfigReferences(components, nil, namespace)
			Expect(missing).To(BeFalse())
			Expect(msg).To(BeEmpty())
		})

		It("returns false and an empty message when both components and nudges are empty", func() {
			missing, msg := helpers.FindMissingNudgeConfigReferences(nil, nil, namespace)
			Expect(missing).To(BeFalse())
			Expect(msg).To(BeEmpty())
		})

		It("reports missing 'from' component(s)", func() {
			components := []applicationapiv1alpha1.Component{
				newComponent("comp-b"),
			}
			nudges := []v1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
			}

			missing, msg := helpers.FindMissingNudgeConfigReferences(components, nudges, namespace)
			Expect(missing).To(BeTrue())
			Expect(msg).To(ContainSubstring(`NudgeConfig references non-existent Component(s) in namespace "test-namespace"`))
			Expect(msg).To(ContainSubstring("missing 'from' component(s): comp-a"))
			Expect(msg).NotTo(ContainSubstring("missing 'to'"))
		})

		It("reports missing 'to' component(s)", func() {
			components := []applicationapiv1alpha1.Component{
				newComponent("comp-a"),
			}
			nudges := []v1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
			}

			missing, msg := helpers.FindMissingNudgeConfigReferences(components, nudges, namespace)
			Expect(missing).To(BeTrue())
			Expect(msg).To(ContainSubstring(`NudgeConfig references non-existent Component(s) in namespace "test-namespace"`))
			Expect(msg).To(ContainSubstring("missing 'to' component(s): comp-b"))
			Expect(msg).NotTo(ContainSubstring("missing 'from'"))
		})

		It("reports both missing 'from' and 'to' component(s)", func() {
			components := []applicationapiv1alpha1.Component{
				newComponent("comp-existing"),
			}
			nudges := []v1beta2.NudgeRelationship{
				{From: "missing-from", To: "missing-to"},
			}

			missing, msg := helpers.FindMissingNudgeConfigReferences(components, nudges, namespace)
			Expect(missing).To(BeTrue())
			Expect(msg).To(ContainSubstring("missing 'from' component(s): missing-from"))
			Expect(msg).To(ContainSubstring("missing 'to' component(s): missing-to"))
		})

		It("deduplicates repeated missing 'from' and 'to' references", func() {
			components := []applicationapiv1alpha1.Component{}
			nudges := []v1beta2.NudgeRelationship{
				{From: "ghost-a", To: "ghost-b"},
				{From: "ghost-a", To: "ghost-c"},
				{From: "ghost-d", To: "ghost-b"},
			}

			missing, msg := helpers.FindMissingNudgeConfigReferences(components, nudges, namespace)
			Expect(missing).To(BeTrue())
			Expect(msg).To(ContainSubstring("missing 'from' component(s): ghost-a, ghost-d"))
			Expect(msg).To(ContainSubstring("missing 'to' component(s): ghost-b, ghost-c"))
		})

		It("reports all missing references when components list is empty", func() {
			nudges := []v1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
			}

			missing, msg := helpers.FindMissingNudgeConfigReferences(nil, nudges, namespace)
			Expect(missing).To(BeTrue())
			Expect(msg).To(Equal(
				`NudgeConfig references non-existent Component(s) in namespace "test-namespace"; missing 'from' component(s): comp-a; missing 'to' component(s): comp-b`,
			))
		})
	})
})
