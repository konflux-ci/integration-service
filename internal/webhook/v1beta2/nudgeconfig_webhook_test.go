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

import (
	appstudiov1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("NudgeConfig webhook", Ordered, func() {
	var (
		validator   *NudgeConfigCustomValidator
		nudgeConfig *appstudiov1beta2.NudgeConfig
	)

	BeforeEach(func() {
		validator = &NudgeConfigCustomValidator{Client: k8sClient}
		nudgeConfig = &appstudiov1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appstudiov1beta2.NudgeConfigSingletonName,
				Namespace: "default",
			},
			Spec: appstudiov1beta2.NudgeConfigSpec{},
		}
	})

	When("a NudgeConfig is created", func() {
		It("accepts a valid linear chain", func() {
			nudgeConfig.Spec.Nudges = []appstudiov1beta2.NudgeRelationship{
				{From: "component-a", To: "component-b"},
				{From: "component-b", To: "component-c"},
				{From: "component-c", To: "component-d"},
			}
			warnings, err := validator.ValidateCreate(ctx, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("accepts a valid DAG with multiple paths", func() {
			nudgeConfig.Spec.Nudges = []appstudiov1beta2.NudgeRelationship{
				{From: "a", To: "b"},
				{From: "a", To: "c"},
				{From: "b", To: "d"},
				{From: "c", To: "d"},
			}
			warnings, err := validator.ValidateCreate(ctx, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("accepts empty nudges", func() {
			nudgeConfig.Spec.Nudges = nil
			warnings, err := validator.ValidateCreate(ctx, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("rejects a direct cycle", func() {
			nudgeConfig.Spec.Nudges = []appstudiov1beta2.NudgeRelationship{
				{From: "component-a", To: "component-b"},
				{From: "component-b", To: "component-a"},
			}
			_, err := validator.ValidateCreate(ctx, nudgeConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle detected"))
			Expect(err.Error()).To(ContainSubstring("component-a"))
			Expect(err.Error()).To(ContainSubstring("component-b"))
		})

		It("rejects a transitive 3-node cycle", func() {
			nudgeConfig.Spec.Nudges = []appstudiov1beta2.NudgeRelationship{
				{From: "component-a", To: "component-b"},
				{From: "component-b", To: "component-c"},
				{From: "component-c", To: "component-a"},
			}
			_, err := validator.ValidateCreate(ctx, nudgeConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle detected"))
			Expect(err.Error()).To(ContainSubstring("component-a"))
			Expect(err.Error()).To(ContainSubstring("component-b"))
			Expect(err.Error()).To(ContainSubstring("component-c"))
		})

		It("handles self-nudge gracefully", func() {
			nudgeConfig.Spec.Nudges = []appstudiov1beta2.NudgeRelationship{
				{From: "component-a", To: "component-a"},
			}
			_, err := validator.ValidateCreate(ctx, nudgeConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle detected: component-a -> component-a"))
		})

		It("returns an error for a wrong object type", func() {
			wrongObj := &appstudiov1beta2.ComponentGroup{}
			_, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a NudgeConfig"))
		})
	})

	When("a NudgeConfig is updated", func() {
		It("rejects an update that introduces a cycle", func() {
			nudgeConfig.Spec.Nudges = []appstudiov1beta2.NudgeRelationship{
				{From: "a", To: "b"},
			}

			newNudgeConfig := nudgeConfig.DeepCopy()
			newNudgeConfig.Spec.Nudges = []appstudiov1beta2.NudgeRelationship{
				{From: "a", To: "b"},
				{From: "b", To: "a"},
			}
			_, err := validator.ValidateUpdate(ctx, nudgeConfig, newNudgeConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle detected"))
		})

		It("skips validation when nudges are unchanged", func() {
			nudgeConfig.Spec.Nudges = []appstudiov1beta2.NudgeRelationship{
				{From: "a", To: "b"},
			}
			sameCopy := nudgeConfig.DeepCopy()
			warnings, err := validator.ValidateUpdate(ctx, nudgeConfig, sameCopy)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("accepts valid nudge changes", func() {
			nudgeConfig.Spec.Nudges = []appstudiov1beta2.NudgeRelationship{
				{From: "a", To: "b"},
			}
			newNudgeConfig := nudgeConfig.DeepCopy()
			newNudgeConfig.Spec.Nudges = append(newNudgeConfig.Spec.Nudges,
				appstudiov1beta2.NudgeRelationship{From: "b", To: "c"},
			)
			warnings, err := validator.ValidateUpdate(ctx, nudgeConfig, newNudgeConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("returns an error for wrong oldObj type", func() {
			wrongObj := &appstudiov1beta2.ComponentGroup{}
			_, err := validator.ValidateUpdate(ctx, wrongObj, nudgeConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a NudgeConfig"))
		})

		It("returns an error for wrong newObj type", func() {
			wrongObj := &appstudiov1beta2.ComponentGroup{}
			_, err := validator.ValidateUpdate(ctx, nudgeConfig, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a NudgeConfig"))
		})
	})

	When("a NudgeConfig is deleted", func() {
		It("always succeeds", func() {
			warnings, err := validator.ValidateDelete(ctx, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})
})
