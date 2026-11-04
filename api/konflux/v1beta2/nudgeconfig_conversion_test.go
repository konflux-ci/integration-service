package v1beta2

import (
	"testing"
	"time"

	v1beta3 "github.com/konflux-ci/integration-service/api/konflux/v1beta3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	webhookconversion "sigs.k8s.io/controller-runtime/pkg/webhook/conversion"
)

func TestNudgeConfigConversionToAndFromHub(t *testing.T) {
	includePartial := false
	failurePolicy := FailurePolicyProceedWithPartial
	debounce := metav1.Duration{Duration: 5 * time.Minute}
	fireAt := metav1.NewTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	now := metav1.NewTime(time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC))

	src := &NudgeConfig{
		TypeMeta: metav1.TypeMeta{APIVersion: GroupVersion.String(), Kind: "NudgeConfig"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      NudgeConfigSingletonName,
			Namespace: "test-ns",
			Labels:    map[string]string{"test": "conversion"},
		},
		Spec: NudgeConfigSpec{
			BatchDefaults: &BatchDefaults{DebounceTimeout: &debounce, FailurePolicy: &failurePolicy},
			Actions:       &Actions{ForceFire: &ForceFireAction{Target: "target", IncludePartial: &includePartial}},
			TargetConfig:  []TargetConfig{{Target: "target", BatchPolicy: &BatchPolicy{DebounceTimeout: &debounce}}},
			Nudges: []NudgeRelationship{
				{From: "source", To: "target", Mode: NudgeModeValidated, GatingGroup: "gates"},
			},
		},
		Status: NudgeConfigStatus{
			Conditions: []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Test"}},
			ActiveBatches: []ActiveBatch{{
				Target:       "target",
				BatchID:      "batch-1",
				Phase:        BatchPhaseAccumulating,
				CreatedAt:    now,
				FireAt:       &fireAt,
				HardDeadline: metav1.NewTime(now.Add(time.Hour)),
				Accumulated:  []AccumulatedEntry{{From: "source", ImageDigest: "sha256:abc", BuildPipelineRun: "build", CapturedAt: now}},
				Failed:       []FailedEntry{{From: "other", BuildPipelineRun: "failed-build", Reason: "failed", CapturedAt: now}},
			}},
		},
	}

	hub := &v1beta3.NudgeConfig{}
	if err := src.ConvertTo(hub); err != nil {
		t.Fatalf("ConvertTo() error = %v", err)
	}
	if got := hub.Spec.Nudges[0].From.Component; got != "source" {
		t.Fatalf("hub from component = %q, want source", got)
	}
	if got := hub.Spec.Nudges[0].From.Version; got != "" {
		t.Fatalf("hub from version = %q, want empty string for v1beta2 conversion", got)
	}
	if got := hub.Spec.Nudges[0].To.Component; got != "target" {
		t.Fatalf("hub to component = %q, want target", got)
	}
	if hub.Spec.Actions == nil || hub.Spec.Actions.ForceFire == nil || *hub.Spec.Actions.ForceFire.IncludePartial {
		t.Fatal("hub forceFire includePartial was not preserved")
	}

	hub.Spec.Nudges[0].From.Version = "main"
	hub.Spec.Nudges[0].To.Version = "release"
	roundTrip := &NudgeConfig{}
	if err := roundTrip.ConvertFrom(hub); err != nil {
		t.Fatalf("ConvertFrom() error = %v", err)
	}
	if got := roundTrip.Spec.Nudges[0].From; got != "source" {
		t.Fatalf("round-trip from = %q, want source", got)
	}
	if got := roundTrip.Spec.Nudges[0].To; got != "target" {
		t.Fatalf("round-trip to = %q, want target", got)
	}
	if got := roundTrip.Status.ActiveBatches[0].Accumulated[0].ImageDigest; got != "sha256:abc" {
		t.Fatalf("round-trip accumulated image digest = %q, want sha256:abc", got)
	}
}

func TestKonfluxTypesAreConvertibleWithV1Beta3Hub(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(v1beta2) error = %v", err)
	}
	if err := v1beta3.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(v1beta3) error = %v", err)
	}

	for name, obj := range map[string]runtime.Object{
		"NudgeConfig": &NudgeConfig{},
	} {
		t.Run(name, func(t *testing.T) {
			ok, err := webhookconversion.IsConvertible(scheme, obj)
			if err != nil {
				t.Fatalf("IsConvertible() error = %v", err)
			}
			if !ok {
				t.Fatal("IsConvertible() = false, want true")
			}
		})
	}
}
