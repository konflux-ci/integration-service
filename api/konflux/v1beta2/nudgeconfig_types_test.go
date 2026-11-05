package v1beta2

import (
	"strings"
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

	t.Run("resolved batch validation", func(t *testing.T) {
		valid := NudgeConfigSpec{
			TargetConfig: []TargetConfig{
				{Target: "immediate-only"},
				{
					Target: "bundle",
					BatchPolicy: &BatchPolicy{
						DebounceTimeout: &metav1.Duration{Duration: 15 * time.Minute},
						MaxWaitTime:     &metav1.Duration{Duration: 2 * time.Hour},
					},
				},
			},
		}
		if err := valid.ValidateEffectiveBatchPolicies(); err != nil {
			t.Fatalf("expected valid resolved policy: %v", err)
		}
		if err := valid.ValidateBatchConfig(); err != nil {
			t.Fatalf("expected valid batch config: %v", err)
		}

		duplicate := NudgeConfigSpec{
			TargetConfig: []TargetConfig{
				{Target: "bundle", BatchPolicy: &BatchPolicy{}},
				{Target: "bundle", BatchPolicy: &BatchPolicy{}},
			},
		}
		if err := duplicate.ValidateBatchConfig(); err == nil || !strings.Contains(err.Error(), "duplicate targetConfig target") {
			t.Fatalf("expected duplicate target error, got %v", err)
		}

		shortDebounce := NudgeConfigSpec{
			TargetConfig: []TargetConfig{{
				Target: "bundle",
				BatchPolicy: &BatchPolicy{
					DebounceTimeout: &metav1.Duration{Duration: 30 * time.Second},
					MaxWaitTime:     &metav1.Duration{Duration: 2 * time.Hour},
				},
			}},
		}
		if err := shortDebounce.ValidateEffectiveBatchPolicies(); err == nil || !strings.Contains(err.Error(), "debounceTimeout must be between 1m and 24h") {
			t.Fatalf("expected debounce range error, got %v", err)
		}

		longMaxWait := NudgeConfigSpec{
			TargetConfig: []TargetConfig{{
				Target: "bundle",
				BatchPolicy: &BatchPolicy{
					DebounceTimeout: &metav1.Duration{Duration: 15 * time.Minute},
					MaxWaitTime:     &metav1.Duration{Duration: 25 * time.Hour},
				},
			}},
		}
		if err := longMaxWait.ValidateEffectiveBatchPolicies(); err == nil || !strings.Contains(err.Error(), "maxWaitTime must be between 1m and 24h") {
			t.Fatalf("expected maxWait range error, got %v", err)
		}

		maxWaitNotGreater := NudgeConfigSpec{
			TargetConfig: []TargetConfig{{
				Target: "bundle",
				BatchPolicy: &BatchPolicy{
					DebounceTimeout: &metav1.Duration{Duration: 2 * time.Hour},
					MaxWaitTime:     &metav1.Duration{Duration: 2 * time.Hour},
				},
			}},
		}
		if err := maxWaitNotGreater.ValidateEffectiveBatchPolicies(); err == nil || !strings.Contains(err.Error(), "maxWaitTime must exceed debounceTimeout") {
			t.Fatalf("expected maxWait/debounce ordering error, got %v", err)
		}
	})

	t.Run("duplicate nudge validation", func(t *testing.T) {
		valid := NudgeConfigSpec{Nudges: []NudgeRelationship{
			{From: ComponentVersion{Component: "component-a"}, To: ComponentVersion{Component: "component-b"}},
			{From: ComponentVersion{Component: "component-a"}, To: ComponentVersion{Component: "component-c"}},
		}}
		if err := valid.ValidateUniqueNudges(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}

		duplicate := NudgeConfigSpec{Nudges: []NudgeRelationship{
			{From: ComponentVersion{Component: "component-a"}, To: ComponentVersion{Component: "component-b"}},
			{From: ComponentVersion{Component: "component-a"}, To: ComponentVersion{Component: "component-b"}, Mode: NudgeModeValidated},
		}}
		err := duplicate.ValidateUniqueNudges()
		if err == nil {
			t.Fatal("expected duplicate nudge validation error")
		}
		if !strings.Contains(err.Error(), "duplicate (from, to) pair not allowed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
