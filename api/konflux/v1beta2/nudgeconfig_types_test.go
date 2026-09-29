package v1beta2

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNudgeConfigTypeHelpers(t *testing.T) {
	partial := FailurePolicyProceedWithPartial
	debounce := metav1.Duration{Duration: 15 * time.Minute}
	maxWait := metav1.Duration{Duration: 2 * time.Hour}
	overrideDebounce := metav1.Duration{Duration: 10 * time.Minute}
	overrideMaxWait := metav1.Duration{Duration: 3 * time.Hour}

	spec := NudgeConfigSpec{
		BatchDefaults: &BatchDefaults{
			DebounceTimeout: &debounce,
			MaxWaitTime:     &maxWait,
			FailurePolicy:   &partial,
		},
		TargetConfig: []TargetConfig{
			{Target: "bundle", BatchPolicy: &BatchPolicy{}},
			{
				Target: "override",
				BatchPolicy: &BatchPolicy{
					DebounceTimeout: &overrideDebounce,
					MaxWaitTime:     &overrideMaxWait,
					FailurePolicy:   &partial,
				},
			},
			{Target: "unbatched"},
		},
	}

	t.Run("target batching", func(t *testing.T) {
		if !spec.TargetConfig[0].IsBatched() {
			t.Fatal("expected a target with a batch policy to be batched")
		}
		if spec.TargetConfig[2].IsBatched() {
			t.Fatal("expected a target without a batch policy not to be batched")
		}
		if !spec.IsTargetBatched("bundle") {
			t.Fatal("expected bundle to be batched")
		}
		if spec.IsTargetBatched("unbatched") {
			t.Fatal("expected unbatched not to be batched")
		}
		if spec.IsTargetBatched("missing") {
			t.Fatal("expected a missing target not to be batched")
		}
	})

	t.Run("effective batch policy", func(t *testing.T) {
		debounceTimeout, maxWaitTime, failure := spec.EffectiveBatchPolicy("bundle")
		if debounceTimeout != 15*time.Minute || maxWaitTime != 2*time.Hour || failure != FailurePolicyProceedWithPartial {
			t.Fatalf("unexpected inherited policy: %s, %s, %s", debounceTimeout, maxWaitTime, failure)
		}

		debounceTimeout, maxWaitTime, failure = spec.EffectiveBatchPolicy("override")
		if debounceTimeout != 10*time.Minute || maxWaitTime != 3*time.Hour || failure != FailurePolicyProceedWithPartial {
			t.Fatalf("unexpected overridden policy: %s, %s, %s", debounceTimeout, maxWaitTime, failure)
		}

		defaults, defaultMaxWait, defaultFailure := (NudgeConfigSpec{}).EffectiveBatchPolicy("missing")
		if defaults != DefaultBatchDebounceTimeout || defaultMaxWait != DefaultBatchMaxWaitTime || defaultFailure != DefaultBatchFailurePolicy {
			t.Fatalf("unexpected runtime defaults: %s, %s, %s", defaults, defaultMaxWait, defaultFailure)
		}
	})

	t.Run("target config lookup", func(t *testing.T) {
		if got := spec.ResolveTargetConfig("bundle"); got == nil || got.Target != "bundle" {
			t.Fatal("expected bundle target config")
		}
		if got := spec.ResolveTargetConfig("missing"); got != nil {
			t.Fatal("expected no target config for missing target")
		}
	})

	t.Run("duplicate target validation", func(t *testing.T) {
		if err := (NudgeConfigSpec{TargetConfig: []TargetConfig{{Target: "bundle"}, {Target: "other"}}}).ValidateUniqueTargetConfig(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if err := (NudgeConfigSpec{TargetConfig: []TargetConfig{{Target: "bundle"}, {Target: "bundle"}}}).ValidateUniqueTargetConfig(); err == nil {
			t.Fatal("expected duplicate target validation error")
		}
	})
}
