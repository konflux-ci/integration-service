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
	"time"

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

	Context("When batch defaults are provided", func() {
		It("should accept a NudgeConfig with valid batchDefaults and round-trip the values", func() {
			policy := FailurePolicyProceedWithPartial
			nc := &NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: NudgeConfigSpec{
					BatchDefaults: &BatchDefaults{
						DebounceTimeout: &metav1.Duration{Duration: 30 * time.Minute},
						MaxWaitTime:     &metav1.Duration{Duration: 4 * time.Hour},
						FailurePolicy:   &policy,
					},
					Nudges: []NudgeRelationship{
						{From: "component-a", To: "component-b"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, nc)).To(Succeed())

			created := &NudgeConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: NudgeConfigSingletonName, Namespace: "default"}, created)).To(Succeed())
			Expect(created.Spec.BatchDefaults).NotTo(BeNil())
			Expect(created.Spec.BatchDefaults.DebounceTimeout).NotTo(BeNil())
			Expect(created.Spec.BatchDefaults.DebounceTimeout.Duration).To(Equal(30 * time.Minute))
			Expect(created.Spec.BatchDefaults.MaxWaitTime).NotTo(BeNil())
			Expect(created.Spec.BatchDefaults.MaxWaitTime.Duration).To(Equal(4 * time.Hour))
			Expect(created.Spec.BatchDefaults.FailurePolicy).NotTo(BeNil())
			Expect(*created.Spec.BatchDefaults.FailurePolicy).To(Equal(FailurePolicyProceedWithPartial))
		})

		It("should leave omitted batchDefaults as nil (no schema-level default injection)", func() {
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
			Expect(created.Spec.BatchDefaults).To(BeNil())
		})

		It("should reject debounceTimeout below 1m", func() {
			nc := &NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: NudgeConfigSpec{
					BatchDefaults: &BatchDefaults{
						DebounceTimeout: &metav1.Duration{Duration: 30 * time.Second},
					},
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("debounceTimeout"))
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject debounceTimeout above 24h", func() {
			nc := &NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: NudgeConfigSpec{
					BatchDefaults: &BatchDefaults{
						DebounceTimeout: &metav1.Duration{Duration: 25 * time.Hour},
					},
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("debounceTimeout"))
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject an invalid failurePolicy value", func() {
			bogus := FailurePolicyType("Ignore")
			nc := &NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: NudgeConfigSpec{
					BatchDefaults: &BatchDefaults{
						FailurePolicy: &bogus,
					},
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject maxWaitTime below 1m", func() {
			nc := &NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: NudgeConfigSpec{
					BatchDefaults: &BatchDefaults{
						MaxWaitTime: &metav1.Duration{Duration: 30 * time.Second},
					},
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxWaitTime"))
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject maxWaitTime above 24h", func() {
			nc := &NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: NudgeConfigSpec{
					BatchDefaults: &BatchDefaults{
						MaxWaitTime: &metav1.Duration{Duration: 25 * time.Hour},
					},
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxWaitTime"))
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject maxWaitTime that does not exceed debounceTimeout", func() {
			nc := &NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: NudgeConfigSpec{
					BatchDefaults: &BatchDefaults{
						DebounceTimeout: &metav1.Duration{Duration: 1 * time.Hour},
						MaxWaitTime:     &metav1.Duration{Duration: 30 * time.Minute},
					},
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxWaitTime must exceed debounceTimeout"))
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject maxWaitTime equal to debounceTimeout", func() {
			nc := &NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: NudgeConfigSpec{
					BatchDefaults: &BatchDefaults{
						DebounceTimeout: &metav1.Duration{Duration: 2 * time.Hour},
						MaxWaitTime:     &metav1.Duration{Duration: 2 * time.Hour},
					},
				},
			}
			err := k8sClient.Create(ctx, nc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxWaitTime must exceed debounceTimeout"))
			Expect(errors.IsInvalid(err)).To(BeTrue())
		})

		It("should accept maxWaitTime without debounceTimeout (cross-field check skipped)", func() {
			nc := &NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: NudgeConfigSpec{
					BatchDefaults: &BatchDefaults{
						MaxWaitTime: &metav1.Duration{Duration: 2 * time.Hour},
					},
				},
			}
			Expect(k8sClient.Create(ctx, nc)).To(Succeed())
		})
	})

	It("should reject spec.nudges exceeding 360 items", func() {
		nudges := make([]NudgeRelationship, 361)
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
