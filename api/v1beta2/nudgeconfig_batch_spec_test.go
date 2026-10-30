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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

var _ = Describe("NudgeConfig batch spec helpers", func() {
	Context("When validating batch policy constants", func() {
		It("should use the expected failure policy string values", func() {
			Expect(string(FailurePolicyBlock)).To(Equal("Block"))
			Expect(string(FailurePolicyProceedWithPartial)).To(Equal("ProceedWithPartial"))
		})
	})

	Context("When batch default CEL literals are checked", func() {
		It("should match runtime defaults in types and CRD", func() {
			debounceLiteral, err := kubernetesDurationLiteral(DefaultBatchDebounceTimeout)
			Expect(err).NotTo(HaveOccurred())
			maxWaitLiteral, err := kubernetesDurationLiteral(DefaultBatchMaxWaitTime)
			Expect(err).NotTo(HaveOccurred())

			debounceCEL := fmt.Sprintf("duration('%s')", debounceLiteral)
			maxWaitCEL := fmt.Sprintf("duration('%s')", maxWaitLiteral)

			source, err := os.ReadFile("nudgeconfig_types.go")
			Expect(err).NotTo(HaveOccurred())
			assertContainsBatchDefaultCEL(string(source), debounceCEL, maxWaitCEL)

			crdPath := filepath.Join("..", "..", "config", "crd", "bases", "appstudio.redhat.com_nudgeconfigs.yaml")
			crd, err := os.ReadFile(crdPath)
			Expect(err).NotTo(HaveOccurred())
			assertContainsBatchDefaultCEL(strings.ReplaceAll(string(crd), "''", "'"), debounceCEL, maxWaitCEL)

			Expect(DefaultBatchFailurePolicy).To(Equal(FailurePolicyBlock))
		})
	})

	Context("When batch defaults are serialized", func() {
		It("should round-trip through JSON", func() {
			original := NudgeConfigSpec{
				BatchDefaults: &BatchDefaults{
					DebounceTimeout: durationPtr(DefaultBatchDebounceTimeout),
					MaxWaitTime:     durationPtr(DefaultBatchMaxWaitTime),
					FailurePolicy:   failurePolicyPtr(DefaultBatchFailurePolicy),
				},
				Nudges: []NudgeRelationship{{From: "component-a", To: "component-b", Mode: NudgeModeImmediate}},
			}

			encoded, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var decoded NudgeConfigSpec
			Expect(json.Unmarshal(encoded, &decoded)).To(Succeed())
			Expect(decoded.BatchDefaults).NotTo(BeNil())
			Expect(decoded.BatchDefaults.DebounceTimeout.Duration).To(Equal(DefaultBatchDebounceTimeout))
			Expect(decoded.BatchDefaults.MaxWaitTime.Duration).To(Equal(DefaultBatchMaxWaitTime))
			Expect(*decoded.BatchDefaults.FailurePolicy).To(Equal(DefaultBatchFailurePolicy))
		})

		It("should round-trip through YAML", func() {
			original := NudgeConfigSpec{
				BatchDefaults: &BatchDefaults{
					DebounceTimeout: durationPtr(15 * time.Minute),
					MaxWaitTime:     durationPtr(2 * time.Hour),
					FailurePolicy:   failurePolicyPtr(FailurePolicyProceedWithPartial),
				},
			}

			encoded, err := yaml.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var decoded NudgeConfigSpec
			Expect(yaml.Unmarshal(encoded, &decoded)).To(Succeed())
			Expect(decoded.BatchDefaults).NotTo(BeNil())
			Expect(decoded.BatchDefaults.DebounceTimeout.Duration).To(Equal(15 * time.Minute))
			Expect(decoded.BatchDefaults.MaxWaitTime.Duration).To(Equal(2 * time.Hour))
			Expect(*decoded.BatchDefaults.FailurePolicy).To(Equal(FailurePolicyProceedWithPartial))
		})

		It("should leave batchDefaults nil when omitted", func() {
			var fromJSON NudgeConfigSpec
			Expect(json.Unmarshal([]byte(`{"nudges":[]}`), &fromJSON)).To(Succeed())
			Expect(fromJSON.BatchDefaults).To(BeNil())

			var fromYAML NudgeConfigSpec
			Expect(yaml.Unmarshal([]byte("nudges: []\n"), &fromYAML)).To(Succeed())
			Expect(fromYAML.BatchDefaults).To(BeNil())

			encoded, err := json.Marshal(NudgeConfigSpec{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(encoded)).NotTo(ContainSubstring("batchDefaults"))
		})
	})

	Context("When resolving target batch configuration", func() {
		It("should reject duplicate targetConfig entries", func() {
			spec := NudgeConfigSpec{
				TargetConfig: []TargetConfig{
					{Target: "bundle", BatchPolicy: &BatchPolicy{}},
					{Target: "bundle", BatchPolicy: &BatchPolicy{DebounceTimeout: durationPtr(time.Hour)}},
				},
			}
			err := spec.ValidateUniqueTargetConfig()
			Expect(err).To(MatchError(ContainSubstring("duplicate targetConfig target")))
		})

		It("should resolve targetConfig and effective batch policy", func() {
			partial := FailurePolicyProceedWithPartial
			spec := NudgeConfigSpec{
				BatchDefaults: &BatchDefaults{
					DebounceTimeout: durationPtr(15 * time.Minute),
					MaxWaitTime:     durationPtr(2 * time.Hour),
					FailurePolicy:   &partial,
				},
				TargetConfig: []TargetConfig{
					{Target: "bundle", BatchPolicy: &BatchPolicy{}},
					{
						Target: "override",
						BatchPolicy: &BatchPolicy{
							DebounceTimeout: durationPtr(10 * time.Minute),
							FailurePolicy:   &[]FailurePolicyType{FailurePolicyProceedWithPartial}[0],
						},
					},
				},
			}

			tc := spec.ResolveTargetConfig("bundle")
			Expect(tc).NotTo(BeNil())
			Expect(tc.IsBatched()).To(BeTrue())
			Expect(spec.ResolveTargetConfig("missing")).To(BeNil())

			debounce, maxWait, failure := spec.EffectiveBatchPolicy("bundle")
			Expect(debounce).To(Equal(15 * time.Minute))
			Expect(maxWait).To(Equal(2 * time.Hour))
			Expect(failure).To(Equal(FailurePolicyProceedWithPartial))

			debounce, maxWait, failure = spec.EffectiveBatchPolicy("override")
			Expect(debounce).To(Equal(10 * time.Minute))
			Expect(maxWait).To(Equal(2 * time.Hour))
			Expect(failure).To(Equal(FailurePolicyProceedWithPartial))
		})

		It("should recognize batched targets from batchPolicy presence", func() {
			withPolicy := TargetConfig{
				Target:      "bundle",
				BatchPolicy: &BatchPolicy{DebounceTimeout: durationPtr(10 * time.Minute)},
			}
			Expect(withPolicy.IsBatched()).To(BeTrue())
			Expect(NudgeConfigSpec{TargetConfig: []TargetConfig{withPolicy}}.IsTargetBatched("bundle")).To(BeTrue())

			withoutPolicy := TargetConfig{Target: "component-c"}
			Expect(withoutPolicy.IsBatched()).To(BeFalse())
			Expect(NudgeConfigSpec{TargetConfig: []TargetConfig{withoutPolicy}}.IsTargetBatched("component-c")).To(BeFalse())

			emptyPolicy := TargetConfig{Target: "bundle", BatchPolicy: &BatchPolicy{}}
			Expect(emptyPolicy.IsBatched()).To(BeTrue())
			Expect(NudgeConfigSpec{TargetConfig: []TargetConfig{emptyPolicy}}.IsTargetBatched("bundle")).To(BeTrue())
		})
	})

	Context("When serializing forceFire actions and targetConfig", func() {
		It("should round-trip forceFire through JSON", func() {
			includePartial := false
			spec := NudgeConfigSpec{
				Actions: &Actions{
					ForceFire: &ForceFireAction{
						Target:         "operator-bundle",
						IncludePartial: &includePartial,
					},
				},
			}

			encoded, err := json.Marshal(spec)
			Expect(err).NotTo(HaveOccurred())
			jsonStr := string(encoded)
			Expect(jsonStr).To(ContainSubstring(`"forceFire"`))
			Expect(jsonStr).To(ContainSubstring(`"target":"operator-bundle"`))
			Expect(jsonStr).To(ContainSubstring(`"includePartial":false`))

			var decoded NudgeConfigSpec
			Expect(json.Unmarshal(encoded, &decoded)).To(Succeed())
			Expect(decoded.Actions.ForceFire.Target).To(Equal("operator-bundle"))
			Expect(decoded.Actions.ForceFire.IncludePartial).NotTo(BeNil())
			Expect(*decoded.Actions.ForceFire.IncludePartial).To(BeFalse())
		})

		It("should omit nil actions from JSON", func() {
			spec := NudgeConfigSpec{
				Nudges: []NudgeRelationship{{From: "component-a", To: "component-b"}},
			}
			encoded, err := json.Marshal(spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(encoded)).NotTo(ContainSubstring("actions"))
		})

		It("should round-trip targetConfig batchPolicy", func() {
			encoded, err := json.Marshal(TargetConfig{Target: "bundle", BatchPolicy: &BatchPolicy{}})
			Expect(err).NotTo(HaveOccurred())

			var decoded TargetConfig
			Expect(json.Unmarshal(encoded, &decoded)).To(Succeed())
			Expect(decoded.Target).To(Equal("bundle"))
			Expect(decoded.BatchPolicy).NotTo(BeNil())
			Expect(decoded.BatchPolicy.DebounceTimeout).To(BeNil())
			Expect(decoded.BatchPolicy.MaxWaitTime).To(BeNil())
			Expect(decoded.BatchPolicy.FailurePolicy).To(BeNil())

			var omitted TargetConfig
			Expect(json.Unmarshal([]byte(`{"target":"component-a"}`), &omitted)).To(Succeed())
			Expect(omitted.BatchPolicy).To(BeNil())
		})
	})

	Context("When resolving effective batch policy per target", func() {
		It("should apply targetConfig overrides on top of batchDefaults", func() {
			proceed := FailurePolicyProceedWithPartial
			spec := NudgeConfigSpec{
				BatchDefaults: &BatchDefaults{
					DebounceTimeout: durationPtr(45 * time.Minute),
					MaxWaitTime:     durationPtr(6 * time.Hour),
					FailurePolicy:   failurePolicyPtr(FailurePolicyBlock),
				},
				TargetConfig: []TargetConfig{{
					Target: "bundle",
					BatchPolicy: &BatchPolicy{
						DebounceTimeout: durationPtr(90 * time.Minute),
						MaxWaitTime:     durationPtr(8 * time.Hour),
						FailurePolicy:   &proceed,
					},
				}},
			}
			debounce, maxWait, failure := spec.EffectiveBatchPolicy("bundle")
			Expect(debounce).To(Equal(90 * time.Minute))
			Expect(maxWait).To(Equal(8 * time.Hour))
			Expect(failure).To(Equal(FailurePolicyProceedWithPartial))
		})
	})

	Context("When validating resolved batch timing", func() {
		It("should skip non-batched targets", func() {
			spec := NudgeConfigSpec{
				TargetConfig: []TargetConfig{
					{Target: "immediate-target"},
					{
						Target: "bundle",
						BatchPolicy: &BatchPolicy{
							DebounceTimeout: durationPtr(15 * time.Minute),
							MaxWaitTime:     durationPtr(2 * time.Hour),
						},
					},
				},
			}
			Expect(spec.ValidateEffectiveBatchPolicies()).To(Succeed())
		})

		It("should reject maxWait outside the allowed range", func() {
			spec := NudgeConfigSpec{
				TargetConfig: []TargetConfig{{
					Target: "bundle",
					BatchPolicy: &BatchPolicy{
						DebounceTimeout: durationPtr(15 * time.Minute),
						MaxWaitTime:     durationPtr(25 * time.Hour),
					},
				}},
			}
			err := spec.ValidateEffectiveBatchPolicies()
			Expect(err).To(MatchError(ContainSubstring("maxWaitTime must be between 1m and 24h")))
		})

		It("should delegate duplicate and timing checks through ValidateBatchConfig", func() {
			spec := NudgeConfigSpec{
				TargetConfig: []TargetConfig{
					{Target: "bundle", BatchPolicy: &BatchPolicy{}},
					{Target: "bundle", BatchPolicy: &BatchPolicy{}},
				},
			}
			err := spec.ValidateBatchConfig()
			Expect(err).To(MatchError(ContainSubstring("duplicate targetConfig target")))
		})

		It("should accept valid batch configuration through ValidateBatchConfig", func() {
			spec := NudgeConfigSpec{
				TargetConfig: []TargetConfig{{
					Target: "bundle",
					BatchPolicy: &BatchPolicy{
						DebounceTimeout: durationPtr(15 * time.Minute),
						MaxWaitTime:     durationPtr(2 * time.Hour),
					},
				}},
			}
			Expect(spec.ValidateBatchConfig()).To(Succeed())
		})

		It("should reject debounce shorter than one minute", func() {
			spec := NudgeConfigSpec{
				TargetConfig: []TargetConfig{{
					Target: "bundle",
					BatchPolicy: &BatchPolicy{
						DebounceTimeout: durationPtr(30 * time.Second),
						MaxWaitTime:     durationPtr(2 * time.Hour),
					},
				}},
			}
			err := spec.ValidateEffectiveBatchPolicies()
			Expect(err).To(MatchError(ContainSubstring("debounceTimeout must be between 1m and 24h")))
		})

		It("should reject maxWait that does not exceed debounce", func() {
			spec := NudgeConfigSpec{
				TargetConfig: []TargetConfig{{
					Target: "bundle",
					BatchPolicy: &BatchPolicy{
						DebounceTimeout: durationPtr(2 * time.Hour),
						MaxWaitTime:     durationPtr(2 * time.Hour),
					},
				}},
			}
			err := spec.ValidateEffectiveBatchPolicies()
			Expect(err).To(MatchError(ContainSubstring("maxWaitTime must exceed debounceTimeout")))
		})
	})
})

func assertContainsBatchDefaultCEL(contents, debounceCEL, maxWaitCEL string) {
	Expect(contents).To(ContainSubstring(debounceCEL))
	Expect(contents).To(ContainSubstring(maxWaitCEL))
}
