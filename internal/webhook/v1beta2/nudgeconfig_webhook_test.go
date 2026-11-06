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
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	konfluxv1beta2 "github.com/konflux-ci/integration-service/api/konflux/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
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

func newNudgeConfig(namespace string, nudges []konfluxv1beta2.NudgeRelationship) *konfluxv1beta2.NudgeConfig {
	return &konfluxv1beta2.NudgeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      konfluxv1beta2.NudgeConfigSingletonName,
			Namespace: namespace,
		},
		Spec: konfluxv1beta2.NudgeConfigSpec{
			Nudges: nudges,
		},
	}
}

var _ = Describe("NudgeConfig webhook - cycle detection", Ordered, func() {
	var (
		validator   *NudgeConfigCustomValidator
		nudgeConfig *konfluxv1beta2.NudgeConfig
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
		nudgeConfig = &konfluxv1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      konfluxv1beta2.NudgeConfigSingletonName,
				Namespace: cycleNs.Name,
			},
			Spec: konfluxv1beta2.NudgeConfigSpec{},
		}
	})

	When("a NudgeConfig is created", func() {
		It("accepts a valid linear chain", func() {
			nudgeConfig.Spec.Nudges = []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "component-a"}, To: konfluxv1beta2.ComponentVersion{Component: "component-b"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "component-b"}, To: konfluxv1beta2.ComponentVersion{Component: "component-c"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "component-c"}, To: konfluxv1beta2.ComponentVersion{Component: "component-d"}},
			}
			warnings, err := validator.ValidateCreate(ctx, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("accepts a valid DAG with multiple paths", func() {
			nudgeConfig.Spec.Nudges = []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "a"}, To: konfluxv1beta2.ComponentVersion{Component: "b"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "a"}, To: konfluxv1beta2.ComponentVersion{Component: "c"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "b"}, To: konfluxv1beta2.ComponentVersion{Component: "d"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "c"}, To: konfluxv1beta2.ComponentVersion{Component: "d"}},
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
			nudgeConfig.Spec.Nudges = []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "component-a"}, To: konfluxv1beta2.ComponentVersion{Component: "component-b"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "component-b"}, To: konfluxv1beta2.ComponentVersion{Component: "component-a"}},
			}
			_, err := validator.ValidateCreate(ctx, nudgeConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle detected"))
			Expect(err.Error()).To(ContainSubstring("component-a"))
			Expect(err.Error()).To(ContainSubstring("component-b"))
		})

		It("rejects a transitive 3-node cycle", func() {
			nudgeConfig.Spec.Nudges = []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "component-a"}, To: konfluxv1beta2.ComponentVersion{Component: "component-b"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "component-b"}, To: konfluxv1beta2.ComponentVersion{Component: "component-c"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "component-c"}, To: konfluxv1beta2.ComponentVersion{Component: "component-a"}},
			}
			_, err := validator.ValidateCreate(ctx, nudgeConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle detected"))
			Expect(err.Error()).To(ContainSubstring("component-a"))
			Expect(err.Error()).To(ContainSubstring("component-b"))
			Expect(err.Error()).To(ContainSubstring("component-c"))
		})

		It("handles self-nudge gracefully", func() {
			nudgeConfig.Spec.Nudges = []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "component-a"}, To: konfluxv1beta2.ComponentVersion{Component: "component-a"}},
			}
			_, err := validator.ValidateCreate(ctx, nudgeConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle detected: component-a -> component-a"))
		})

		It("returns an error for a wrong object type", func() {
			wrongObj := &konfluxv1beta2.ComponentGroup{}
			_, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a NudgeConfig"))
		})
	})

	When("a NudgeConfig is updated", func() {
		It("rejects an update that introduces a cycle", func() {
			nudgeConfig.Spec.Nudges = []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "a"}, To: konfluxv1beta2.ComponentVersion{Component: "b"}},
			}

			newNudgeConfig := nudgeConfig.DeepCopy()
			newNudgeConfig.Spec.Nudges = []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "a"}, To: konfluxv1beta2.ComponentVersion{Component: "b"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "b"}, To: konfluxv1beta2.ComponentVersion{Component: "a"}},
			}
			_, err := validator.ValidateUpdate(ctx, nudgeConfig, newNudgeConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle detected"))
		})

		It("skips validation when nudges are unchanged", func() {
			nudgeConfig.Spec.Nudges = []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "a"}, To: konfluxv1beta2.ComponentVersion{Component: "b"}},
			}
			sameCopy := nudgeConfig.DeepCopy()
			warnings, err := validator.ValidateUpdate(ctx, nudgeConfig, sameCopy)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("accepts valid nudge changes", func() {
			nudgeConfig.Spec.Nudges = []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "a"}, To: konfluxv1beta2.ComponentVersion{Component: "b"}},
			}
			newNudgeConfig := nudgeConfig.DeepCopy()
			newNudgeConfig.Spec.Nudges = append(newNudgeConfig.Spec.Nudges,
				konfluxv1beta2.NudgeRelationship{From: konfluxv1beta2.ComponentVersion{Component: "b"}, To: konfluxv1beta2.ComponentVersion{Component: "c"}},
			)
			warnings, err := validator.ValidateUpdate(ctx, nudgeConfig, newNudgeConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("returns an error for wrong oldObj type", func() {
			wrongObj := &konfluxv1beta2.ComponentGroup{}
			_, err := validator.ValidateUpdate(ctx, wrongObj, nudgeConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a NudgeConfig"))
		})

		It("returns an error for wrong newObj type", func() {
			wrongObj := &konfluxv1beta2.ComponentGroup{}
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
			nc := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "missing-comp"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
			})

			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing-comp"))
			Expect(err.Error()).To(ContainSubstring("missing 'from' component(s)"))
		})

		It("rejects when 'to' references a non-existent Component [AC2]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			nc := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "missing-comp"}},
			})

			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing-comp"))
			Expect(err.Error()).To(ContainSubstring("missing 'to' component(s)"))
		})

		It("succeeds when all referenced Components exist [AC5]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())
			nc := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
			})

			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects when both 'from' and 'to' are absent [E1]", func() {
			nc := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "no-from"}, To: konfluxv1beta2.ComponentVersion{Component: "no-to"}},
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
			nc := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "ghost-a"}, To: konfluxv1beta2.ComponentVersion{Component: "real-comp"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "real-comp"}, To: konfluxv1beta2.ComponentVersion{Component: "ghost-b"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "ghost-c"}, To: konfluxv1beta2.ComponentVersion{Component: "ghost-d"}},
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

		It("rejects duplicate targetConfig targets", func() {
			nc := newNudgeConfig(ns.Name, nil)
			nc.Spec.TargetConfig = []konfluxv1beta2.TargetConfig{
				{Target: "bundle", BatchPolicy: &konfluxv1beta2.BatchPolicy{}},
				{Target: "bundle", BatchPolicy: &konfluxv1beta2.BatchPolicy{}},
			}
			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate targetConfig target"))
		})
	})

	When("updating a NudgeConfig", func() {
		It("rejects when a newly-added entry references a non-existent Component [AC3]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())

			oldNC := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
			})
			newNC := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "ghost-x"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
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

			oldNC := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
			})

			Expect(k8sClient.Delete(ctx, compA)).To(Succeed())

			newNC := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-c"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-d"}},
			})

			_, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows update that only changes mode of a pre-existing entry [AC4b]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			compB := newComponent("comp-b", ns.Name)
			Expect(k8sClient.Create(ctx, compB)).To(Succeed())

			oldNC := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}, Mode: konfluxv1beta2.NudgeModeImmediate},
			})

			Expect(k8sClient.Delete(ctx, compB)).To(Succeed())

			newNC := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}, Mode: konfluxv1beta2.NudgeModeValidated},
			})

			_, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds when all referenced Components exist [AC5]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-c", ns.Name))).To(Succeed())

			oldNC := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
			})
			newNC := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-b"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-c"}},
			})

			_, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
		})

		It("handles old nudges being nil when new adds a valid entry [E5]", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())

			oldNC := newNudgeConfig(ns.Name, nil)
			newNC := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
			})

			_, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("targetConfig batch validation", func() {
		It("rejects duplicate target names with the duplicate target in the error", func() {
			nc := newNudgeConfig(ns.Name, nil)
			nc.Spec.TargetConfig = []konfluxv1beta2.TargetConfig{
				{Target: "bundle", BatchPolicy: &konfluxv1beta2.BatchPolicy{}},
				{Target: "bundle", BatchPolicy: &konfluxv1beta2.BatchPolicy{}},
			}

			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`duplicate targetConfig target "bundle"`))
		})

		It("rejects resolved maxWaitTime not exceeding debounceTimeout after batchDefaults inheritance", func() {
			nc := newNudgeConfig(ns.Name, nil)
			nc.Spec.BatchDefaults = &konfluxv1beta2.BatchDefaults{
				DebounceTimeout: &metav1.Duration{Duration: 2 * time.Hour},
				MaxWaitTime:     &metav1.Duration{Duration: 4 * time.Hour},
			}
			nc.Spec.TargetConfig = []konfluxv1beta2.TargetConfig{
				{
					Target: "bundle",
					BatchPolicy: &konfluxv1beta2.BatchPolicy{
						MaxWaitTime: &metav1.Duration{Duration: time.Hour},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`targetConfig target "bundle"`))
			Expect(err.Error()).To(ContainSubstring("maxWaitTime must exceed debounceTimeout"))
		})

		It("accepts valid targetConfig batchPolicy overrides", func() {
			nc := newNudgeConfig(ns.Name, nil)
			nc.Spec.TargetConfig = []konfluxv1beta2.TargetConfig{
				{
					Target: "bundle",
					BatchPolicy: &konfluxv1beta2.BatchPolicy{
						DebounceTimeout: &metav1.Duration{Duration: 15 * time.Minute},
						MaxWaitTime:     &metav1.Duration{Duration: 2 * time.Hour},
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, nc)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("rejects update that adds invalid targetConfig batch timing", func() {
			oldNC := newNudgeConfig(ns.Name, nil)
			newNC := newNudgeConfig(ns.Name, nil)
			newNC.Spec.TargetConfig = []konfluxv1beta2.TargetConfig{
				{
					Target: "bundle",
					BatchPolicy: &konfluxv1beta2.BatchPolicy{
						DebounceTimeout: &metav1.Duration{Duration: 2 * time.Hour},
						MaxWaitTime:     &metav1.Duration{Duration: 2 * time.Hour},
					},
				},
			}

			_, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxWaitTime must exceed debounceTimeout"))
		})
	})

	Describe("batchDefaults admission warnings", func() {
		It("warns on create when spec.batchDefaults is set", func() {
			nc := newNudgeConfig(ns.Name, nil)
			nc.Spec.BatchDefaults = &konfluxv1beta2.BatchDefaults{}

			warnings, err := validator.ValidateCreate(ctx, nc)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(ConsistOf(helpers.BatchDefaultsNotImplementedMessage))
		})

		It("does not warn on create when spec.batchDefaults is unset", func() {
			nc := newNudgeConfig(ns.Name, nil)

			warnings, err := validator.ValidateCreate(ctx, nc)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("warns on update when spec.batchDefaults is set", func() {
			oldNC := newNudgeConfig(ns.Name, nil)
			newNC := newNudgeConfig(ns.Name, nil)
			newNC.Spec.BatchDefaults = &konfluxv1beta2.BatchDefaults{}

			warnings, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(ConsistOf(helpers.BatchDefaultsNotImplementedMessage))
		})

		It("warns when an update removes spec.targetConfig", func() {
			oldNC := newNudgeConfig(ns.Name, nil)
			oldNC.Spec.TargetConfig = []konfluxv1beta2.TargetConfig{
				{Target: "bundle", BatchPolicy: &konfluxv1beta2.BatchPolicy{}},
			}
			newNC := newNudgeConfig(ns.Name, nil)

			warnings, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(ContainElement(ContainSubstring("removed spec.targetConfig")))
		})

		It("warns when an update removes spec.actions without other spec changes", func() {
			oldNC := newNudgeConfig(ns.Name, nil)
			oldNC.Spec.Actions = &konfluxv1beta2.Actions{
				ForceFire: &konfluxv1beta2.ForceFireAction{Target: "bundle"},
			}
			newNC := oldNC.DeepCopy()
			newNC.Spec.Actions = nil

			warnings, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(ContainElement(ContainSubstring("removed spec.actions")))
		})

		It("does not warn about removed spec.actions when nudges also change", func() {
			Expect(k8sClient.Create(ctx, newComponent("comp-a", ns.Name))).To(Succeed())
			Expect(k8sClient.Create(ctx, newComponent("comp-b", ns.Name))).To(Succeed())
			oldNC := newNudgeConfig(ns.Name, nil)
			oldNC.Spec.Actions = &konfluxv1beta2.Actions{
				ForceFire: &konfluxv1beta2.ForceFireAction{Target: "bundle"},
			}
			newNC := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "comp-a"}, To: konfluxv1beta2.ComponentVersion{Component: "comp-b"}},
			})

			warnings, err := validator.ValidateUpdate(ctx, oldNC, newNC)
			Expect(err).NotTo(HaveOccurred())
			for _, w := range warnings {
				Expect(w).NotTo(ContainSubstring("removed spec.actions"))
			}
		})

		It("accepts create with spec.actions.forceFire", func() {
			nc := newNudgeConfig(ns.Name, nil)
			nc.Spec.TargetConfig = []konfluxv1beta2.TargetConfig{
				{Target: "bundle", BatchPolicy: &konfluxv1beta2.BatchPolicy{}},
			}
			includePartial := false
			nc.Spec.Actions = &konfluxv1beta2.Actions{
				ForceFire: &konfluxv1beta2.ForceFireAction{
					Target:         "bundle",
					IncludePartial: &includePartial,
				},
			}

			warnings, err := validator.ValidateCreate(ctx, nc)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})

	When("deleting a NudgeConfig", func() {
		It("always succeeds [E4]", func() {
			nc := newNudgeConfig(ns.Name, []konfluxv1beta2.NudgeRelationship{
				{From: konfluxv1beta2.ComponentVersion{Component: "anything"}, To: konfluxv1beta2.ComponentVersion{Component: "whatever"}},
			})
			_, err := validator.ValidateDelete(ctx, nc)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
