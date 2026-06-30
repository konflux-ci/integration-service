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
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	appstudiov1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newComponent(name, namespace string) *applicationapiv1alpha1.Component {
	return &applicationapiv1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: applicationapiv1alpha1.ComponentSpec{
			ComponentName: name,
			Application:   "test-app",
			Source: applicationapiv1alpha1.ComponentSource{
				ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
					GitSource: &applicationapiv1alpha1.GitSource{URL: "https://example.com/repo"},
				},
			},
		},
	}
}

func newNudgeConfig(namespace string, nudges []appstudiov1beta2.NudgeRelationship) *appstudiov1beta2.NudgeConfig {
	return &appstudiov1beta2.NudgeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appstudiov1beta2.NudgeConfigSingletonName,
			Namespace: namespace,
		},
		Spec: appstudiov1beta2.NudgeConfigSpec{
			Nudges: nudges,
		},
	}
}

var _ = Describe("NudgeConfig webhook - cycle detection", Ordered, func() {
	var (
		validator   *NudgeConfigCustomValidator
		nudgeConfig *appstudiov1beta2.NudgeConfig
		cycleNs     *corev1.Namespace
	)

	BeforeAll(func() {
		cycleNs = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "nudge-cycle-"}}
		Expect(k8sClient.Create(ctx, cycleNs)).To(Succeed())
		for _, name := range []string{"a", "b", "c", "d", "component-a", "component-b", "component-c", "component-d"} {
			Expect(k8sClient.Create(ctx, newComponent(name, cycleNs.Name))).To(Succeed())
		}
	})

	BeforeEach(func() {
		validator = &NudgeConfigCustomValidator{Client: k8sClient}
		nudgeConfig = &appstudiov1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appstudiov1beta2.NudgeConfigSingletonName,
				Namespace: cycleNs.Name,
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

var _ = Describe("NudgeConfig webhook - component existence", func() {
	var (
		validator *NudgeConfigCustomValidator
		ns        *corev1.Namespace
	)

	BeforeEach(func() {
		validator = &NudgeConfigCustomValidator{Client: k8sClient}
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "nudge-test-"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	When("creating a NudgeConfig", func() {
		It("rejects when 'from' references a non-existent Component [AC1]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())
			nc := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "missing-comp", To: "comp-b"},
			})

			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing-comp"))
			Expect(err.Error()).To(ContainSubstring("missing 'from' component(s)"))
		})

		It("rejects when 'to' references a non-existent Component [AC2]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			nc := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "missing-comp"},
			})

			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing-comp"))
			Expect(err.Error()).To(ContainSubstring("missing 'to' component(s)"))
		})

		It("succeeds when all referenced Components exist [AC5]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())
			nc := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
			})

			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects when both 'from' and 'to' are absent [E1]", func() {
			nc := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "no-from", To: "no-to"},
			})

			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no-from"))
			Expect(err.Error()).To(ContainSubstring("no-to"))
			Expect(err.Error()).To(ContainSubstring("missing 'from' component(s)"))
			Expect(err.Error()).To(ContainSubstring("missing 'to' component(s)"))
		})

		It("aggregates all missing components across multiple entries [E2]", func() {
			Expect(k8sClient.Create(ctx, newComponent("real-comp", ns.Name))).To(Succeed())
			nc := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "ghost-a", To: "real-comp"},
				{From: "real-comp", To: "ghost-b"},
				{From: "ghost-c", To: "ghost-d"},
			})

			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ghost-a"))
			Expect(err.Error()).To(ContainSubstring("ghost-b"))
			Expect(err.Error()).To(ContainSubstring("ghost-c"))
			Expect(err.Error()).To(ContainSubstring("ghost-d"))
		})

		It("succeeds with empty nudges [E3]", func() {
			nc := newNudgeConfig(ns.Name, nil)
			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("updating a NudgeConfig", func() {
		It("rejects when a newly-added entry references a non-existent Component [AC3]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())

			oldNC := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
			})
			newNC := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
				{From: "ghost-x", To: "comp-b"},
			})

			_, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ghost-x"))
		})

		It("allows update when a pre-existing entry references a now-deleted Component [AC4]", func() {
			compA := newComponent("comp-a", ns.Name)
			Expect(k8sClient.Create(ctx, compA)).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-c", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-d", ns.Name))).To(Succeed())

			oldNC := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
			})

			Expect(k8sClient.Delete(ctx, compA)).To(Succeed())

			newNC := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
				{From: "comp-c", To: "comp-d"},
			})

			_, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows update that only changes mode of a pre-existing entry [AC4b]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			compB := newComponent("comp-b", ns.Name)
			Expect(k8sClient.Create(ctx, compB)).To(Succeed())

			oldNC := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b", Mode: appstudiov1beta2.NudgeModeImmediate},
			})

			Expect(k8sClient.Delete(ctx, compB)).To(Succeed())

			newNC := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b", Mode: appstudiov1beta2.NudgeModeValidated},
			})

			_, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds when all referenced Components exist [AC5]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-c", ns.Name))).To(Succeed())

			oldNC := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
			})
			newNC := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
				{From: "comp-b", To: "comp-c"},
			})

			_, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
		})

		It("handles old nudges being nil when new adds a valid entry [E5]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())

			oldNC := newNudgeConfig(ns.Name, nil)
			newNC := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "comp-a", To: "comp-b"},
			})

			_, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("deleting a NudgeConfig", func() {
		It("always succeeds [E4]", func() {
			nc := newNudgeConfig(ns.Name, []appstudiov1beta2.NudgeRelationship{
				{From: "anything", To: "whatever"},
			})
			_, err := validator.ValidateDelete(ctx, nc)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
