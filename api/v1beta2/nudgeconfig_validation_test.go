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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("NudgeConfig CEL validation", Ordered, func() {

	AfterEach(func() {
		nc := &NudgeConfig{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: NudgeConfigSingletonName, Namespace: "default"}, nc)
		if err == nil {
			Expect(k8sClient.Delete(ctx, nc)).To(Succeed())
		}
	})

	It("should accept a valid NudgeConfig with correct name and valid nudges", func() {
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NudgeConfigSingletonName,
				Namespace: "default",
			},
			Spec: NudgeConfigSpec{
				Nudges: []NudgeRelationship{
					{From: "component-a", To: "component-b", Mode: NudgeModeValidated},
					{From: "component-a", To: "component-c"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, nc)).To(Succeed())
	})

	It("should accept a NudgeConfig with an empty nudges list", func() {
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NudgeConfigSingletonName,
				Namespace: "default",
			},
		}
		Expect(k8sClient.Create(ctx, nc)).To(Succeed())
	})

	It("should reject a NudgeConfig whose name is not 'nudge-config'", func() {
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wrong-name",
				Namespace: "default",
			},
			Spec: NudgeConfigSpec{
				Nudges: []NudgeRelationship{
					{From: "component-a", To: "component-b"},
				},
			},
		}
		err := k8sClient.Create(ctx, nc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(NudgeConfigSingletonName))

		// Ensure it was not created
		Expect(errors.IsInvalid(err)).To(BeTrue())
	})

	It("should reject a NudgeConfig containing a self-nudge (from == to)", func() {
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NudgeConfigSingletonName,
				Namespace: "default",
			},
			Spec: NudgeConfigSpec{
				Nudges: []NudgeRelationship{
					{From: "component-a", To: "component-b"},
					{From: "component-x", To: "component-x"},
				},
			},
		}
		err := k8sClient.Create(ctx, nc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("self-nudge"))
		Expect(errors.IsInvalid(err)).To(BeTrue())
	})

	It("should reject a NudgeConfig with duplicate (from, to) pairs with different modes", func() {
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NudgeConfigSingletonName,
				Namespace: "default",
			},
			Spec: NudgeConfigSpec{
				Nudges: []NudgeRelationship{
					{From: "component-a", To: "component-b"},
					{From: "component-a", To: "component-b", Mode: NudgeModeValidated},
				},
			},
		}
		err := k8sClient.Create(ctx, nc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("duplicate"))
		Expect(errors.IsInvalid(err)).To(BeTrue())
	})

	It("should reject a NudgeConfig with exact duplicate entries", func() {
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NudgeConfigSingletonName,
				Namespace: "default",
			},
			Spec: NudgeConfigSpec{
				Nudges: []NudgeRelationship{
					{From: "component-a", To: "component-b"},
					{From: "component-a", To: "component-b"},
				},
			},
		}
		err := k8sClient.Create(ctx, nc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("duplicate"))
		Expect(errors.IsInvalid(err)).To(BeTrue())
	})

	It("should default mode to 'immediate' when not specified", func() {
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NudgeConfigSingletonName,
				Namespace: "default",
			},
			Spec: NudgeConfigSpec{
				Nudges: []NudgeRelationship{
					{From: "component-a", To: "component-b"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, nc)).To(Succeed())

		created := &NudgeConfig{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: NudgeConfigSingletonName, Namespace: "default"}, created)).To(Succeed())
		Expect(created.Spec.Nudges[0].Mode).To(Equal(NudgeModeImmediate))
	})

	It("should accept mode 'validated'", func() {
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NudgeConfigSingletonName,
				Namespace: "default",
			},
			Spec: NudgeConfigSpec{
				Nudges: []NudgeRelationship{
					{From: "component-a", To: "component-b", Mode: NudgeModeValidated},
				},
			},
		}
		Expect(k8sClient.Create(ctx, nc)).To(Succeed())

		created := &NudgeConfig{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: NudgeConfigSingletonName, Namespace: "default"}, created)).To(Succeed())
		Expect(created.Spec.Nudges[0].Mode).To(Equal(NudgeModeValidated))
	})

	It("should reject invalid component names in from/to fields", func() {
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NudgeConfigSingletonName,
				Namespace: "default",
			},
			Spec: NudgeConfigSpec{
				Nudges: []NudgeRelationship{
					{From: "Component_A", To: "component-b"},
				},
			},
		}
		err := k8sClient.Create(ctx, nc)
		Expect(err).To(HaveOccurred())
		Expect(errors.IsInvalid(err)).To(BeTrue())
	})

	It("should reject an invalid mode value", func() {
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NudgeConfigSingletonName,
				Namespace: "default",
			},
			Spec: NudgeConfigSpec{
				Nudges: []NudgeRelationship{
					{From: "component-a", To: "component-b", Mode: NudgeModeType("bogus")},
				},
			},
		}
		err := k8sClient.Create(ctx, nc)
		Expect(err).To(HaveOccurred())
		Expect(errors.IsInvalid(err)).To(BeTrue())
	})

	It("should reject spec.nudges exceeding 256 items", func() {
		nudges := make([]NudgeRelationship, 257)
		for i := range nudges {
			nudges[i] = NudgeRelationship{
				From: fmt.Sprintf("src-%d", i),
				To:   fmt.Sprintf("tgt-%d", i),
			}
		}
		nc := &NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      NudgeConfigSingletonName,
				Namespace: "default",
			},
			Spec: NudgeConfigSpec{Nudges: nudges},
		}
		err := k8sClient.Create(ctx, nc)
		Expect(err).To(HaveOccurred())
		Expect(errors.IsInvalid(err)).To(BeTrue())
	})
})
