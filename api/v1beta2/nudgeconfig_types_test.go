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
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func durationPtr(d time.Duration) *metav1.Duration {
	return &metav1.Duration{Duration: d}
}

func TestNudgeConfigSpec(t *testing.T) {
	nc := &NudgeConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1beta2",
			Kind:       "NudgeConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      NudgeConfigSingletonName,
			Namespace: "default",
		},
		Spec: NudgeConfigSpec{
			Nudges: []NudgeRelationship{
				{
					From:        "component-a",
					To:          "component-b",
					Mode:        NudgeModeValidated,
					GatingGroup: "frontend-group",
				},
				{
					From: "component-a",
					To:   "component-c",
					Mode: NudgeModeImmediate,
				},
				{
					From:        "component-d",
					To:          "component-e",
					Mode:        NudgeModeValidated,
					GatingGroup: "backend-group",
				},
			},
		},
	}

	if nc.Name != NudgeConfigSingletonName {
		t.Errorf("Expected name 'nudge-config', got '%s'", nc.Name)
	}
	if nc.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", nc.Namespace)
	}
	if len(nc.Spec.Nudges) != 3 {
		t.Errorf("Expected 3 nudges, got %d", len(nc.Spec.Nudges))
	}

	first := nc.Spec.Nudges[0]
	if first.From != "component-a" {
		t.Errorf("Expected From 'component-a', got '%s'", first.From)
	}
	if first.To != "component-b" {
		t.Errorf("Expected To 'component-b', got '%s'", first.To)
	}
	if first.Mode != NudgeModeValidated {
		t.Errorf("Expected Mode 'validated', got '%s'", first.Mode)
	}
	if first.GatingGroup != "frontend-group" {
		t.Errorf("Expected GatingGroup 'frontend-group', got '%s'", first.GatingGroup)
	}

	second := nc.Spec.Nudges[1]
	if second.Mode != NudgeModeImmediate {
		t.Errorf("Expected Mode 'immediate', got '%s'", second.Mode)
	}
	if second.GatingGroup != "" {
		t.Errorf("Expected empty GatingGroup, got '%s'", second.GatingGroup)
	}
}

func TestNudgeConfigMinimalSpec(t *testing.T) {
	nc := &NudgeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NudgeConfigSingletonName,
			Namespace: "default",
		},
	}

	if nc.Name != NudgeConfigSingletonName {
		t.Errorf("Expected name 'nudge-config', got '%s'", nc.Name)
	}
	if nc.Spec.Nudges != nil {
		t.Errorf("Expected nil Nudges, got %v", nc.Spec.Nudges)
	}
	if nc.Spec.TargetConfig != nil {
		t.Errorf("Expected nil TargetConfig, got %v", nc.Spec.TargetConfig)
	}
	if nc.Spec.Actions != nil {
		t.Errorf("Expected nil Actions, got %v", nc.Spec.Actions)
	}
	if nc.Status.Conditions != nil {
		t.Errorf("Expected nil Conditions, got %v", nc.Status.Conditions)
	}
	if nc.Status.LastValidationTime != nil {
		t.Errorf("Expected nil LastValidationTime, got %v", nc.Status.LastValidationTime)
	}
}

func TestNudgeModeConstants(t *testing.T) {
	if NudgeModeImmediate != "immediate" {
		t.Errorf("Expected NudgeModeImmediate 'immediate', got '%s'", NudgeModeImmediate)
	}
	if NudgeModeValidated != "validated" {
		t.Errorf("Expected NudgeModeValidated 'validated', got '%s'", NudgeModeValidated)
	}
}

func TestFailurePolicyConstants(t *testing.T) {
	if FailurePolicyBlock != "Block" {
		t.Errorf("Expected FailurePolicyBlock 'Block', got '%s'", FailurePolicyBlock)
	}
	if FailurePolicyProceedWithPartial != "ProceedWithPartial" {
		t.Errorf("Expected FailurePolicyProceedWithPartial 'ProceedWithPartial', got '%s'", FailurePolicyProceedWithPartial)
	}
}

func TestNudgeConfigDeepCopy(t *testing.T) {
	original := &NudgeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NudgeConfigSingletonName,
			Namespace: "default",
		},
		Spec: NudgeConfigSpec{
			Nudges: []NudgeRelationship{
				{From: "component-a", To: "component-b", Mode: NudgeModeImmediate},
			},
		},
	}

	copied := original.DeepCopy()

	if copied.Name != original.Name {
		t.Errorf("DeepCopy: Expected name '%s', got '%s'", original.Name, copied.Name)
	}
	if copied.Spec.Nudges[0].From != original.Spec.Nudges[0].From {
		t.Errorf("DeepCopy: Expected From '%s', got '%s'", original.Spec.Nudges[0].From, copied.Spec.Nudges[0].From)
	}

	copied.Name = "modified"
	copied.Spec.Nudges[0].From = "modified-component"
	if original.Name != NudgeConfigSingletonName {
		t.Errorf("Original name was modified: got '%s'", original.Name)
	}
	if original.Spec.Nudges[0].From != "component-a" {
		t.Errorf("Original nudge From was modified: got '%s'", original.Spec.Nudges[0].From)
	}
}

func TestNudgeConfigListDeepCopy(t *testing.T) {
	list := &NudgeConfigList{
		Items: []NudgeConfig{
			{
				ObjectMeta: metav1.ObjectMeta{Name: NudgeConfigSingletonName, Namespace: "ns-1"},
				Spec: NudgeConfigSpec{
					Nudges: []NudgeRelationship{
						{From: "comp-a", To: "comp-b"},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: NudgeConfigSingletonName, Namespace: "ns-2"},
			},
		},
	}

	copied := list.DeepCopy()
	if len(copied.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(copied.Items))
	}
	if copied.Items[0].Namespace != "ns-1" {
		t.Errorf("Expected namespace 'ns-1', got '%s'", copied.Items[0].Namespace)
	}

	copied.Items[0].Namespace = "modified"
	if list.Items[0].Namespace != "ns-1" {
		t.Errorf("Original list was modified: got '%s'", list.Items[0].Namespace)
	}
}

func TestNudgeConfigSpecDeepCopy(t *testing.T) {
	original := NudgeConfigSpec{
		TargetConfig: []TargetConfig{
			{
				Target:      "bundle",
				BatchPolicy: &BatchPolicy{DebounceTimeout: durationPtr(45 * time.Minute)},
			},
		},
		Nudges: []NudgeRelationship{
			{From: "comp-a", To: "comp-b", Mode: NudgeModeValidated, GatingGroup: "grp-1"},
		},
	}

	copied := original.DeepCopy()
	if copied.Nudges[0].From != "comp-a" {
		t.Errorf("Expected From 'comp-a', got '%s'", copied.Nudges[0].From)
	}
	if len(copied.TargetConfig) != 1 || copied.TargetConfig[0].Target != "bundle" {
		t.Fatalf("Expected copied targetConfig, got %+v", copied.TargetConfig)
	}
	if copied.TargetConfig[0].BatchPolicy == nil || copied.TargetConfig[0].BatchPolicy.DebounceTimeout == nil {
		t.Fatal("Expected copied TargetConfig.BatchPolicy.DebounceTimeout")
	}

	copied.Nudges[0].From = "modified"
	copied.TargetConfig[0].BatchPolicy.DebounceTimeout.Duration = time.Hour
	if original.Nudges[0].From != "comp-a" {
		t.Errorf("Original Spec was modified: got '%s'", original.Nudges[0].From)
	}
	if original.TargetConfig[0].BatchPolicy.DebounceTimeout.Duration != 45*time.Minute {
		t.Errorf("Original TargetConfig.BatchPolicy.DebounceTimeout was modified: got %v", original.TargetConfig[0].BatchPolicy.DebounceTimeout.Duration)
	}
}

func TestTargetConfigBatchRecognition(t *testing.T) {
	withPolicy := TargetConfig{
		Target: "bundle",
		BatchPolicy: &BatchPolicy{
			DebounceTimeout: durationPtr(10 * time.Minute),
		},
	}
	if !withPolicy.IsBatched() {
		t.Fatal("expected target with batchPolicy to be batched")
	}
	specWithPolicy := NudgeConfigSpec{TargetConfig: []TargetConfig{withPolicy}}
	if !specWithPolicy.IsTargetBatched("bundle") {
		t.Fatal("expected IsTargetBatched true for target with batchPolicy")
	}

	withoutPolicy := TargetConfig{Target: "component-c"}
	if withoutPolicy.IsBatched() {
		t.Fatal("expected target without batchPolicy not to be batched")
	}
	specWithoutPolicy := NudgeConfigSpec{TargetConfig: []TargetConfig{withoutPolicy}}
	if specWithoutPolicy.IsTargetBatched("component-c") {
		t.Fatal("expected IsTargetBatched false when batchPolicy is nil")
	}

	emptyPolicy := TargetConfig{Target: "bundle", BatchPolicy: &BatchPolicy{}}
	if !emptyPolicy.IsBatched() {
		t.Fatal("expected empty batchPolicy object to opt target into batching")
	}
	if emptyPolicy.BatchPolicy.DebounceTimeout != nil || emptyPolicy.BatchPolicy.MaxWaitTime != nil || emptyPolicy.BatchPolicy.FailurePolicy != nil {
		t.Fatal("expected empty batchPolicy to have all-nil overrides")
	}
	specEmptyPolicy := NudgeConfigSpec{TargetConfig: []TargetConfig{emptyPolicy}}
	if !specEmptyPolicy.IsTargetBatched("bundle") {
		t.Fatal("expected IsTargetBatched true for empty batchPolicy")
	}
}

func TestNudgeConfigActionsForceFireJSON(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	jsonStr := string(encoded)
	if !strings.Contains(jsonStr, `"forceFire"`) {
		t.Fatalf("expected forceFire in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"target":"operator-bundle"`) {
		t.Fatalf("expected target in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"includePartial":false`) {
		t.Fatalf("expected includePartial false in JSON, got %s", jsonStr)
	}

	var decoded NudgeConfigSpec
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Actions == nil || decoded.Actions.ForceFire == nil {
		t.Fatal("expected decoded actions.forceFire")
	}
	if decoded.Actions.ForceFire.Target != "operator-bundle" {
		t.Errorf("expected target operator-bundle, got %q", decoded.Actions.ForceFire.Target)
	}
	if decoded.Actions.ForceFire.IncludePartial == nil || *decoded.Actions.ForceFire.IncludePartial != false {
		t.Errorf("expected includePartial false, got %v", decoded.Actions.ForceFire.IncludePartial)
	}
}

func TestNudgeConfigNilActionsOmittedFromJSON(t *testing.T) {
	spec := NudgeConfigSpec{
		Nudges: []NudgeRelationship{{From: "component-a", To: "component-b"}},
	}

	encoded, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), "actions") {
		t.Fatalf("expected actions omitted from JSON, got %s", string(encoded))
	}
}

func TestTargetConfigJSONRoundTrip(t *testing.T) {
	encoded, err := json.Marshal(TargetConfig{Target: "bundle", BatchPolicy: &BatchPolicy{}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded TargetConfig
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Target != "bundle" {
		t.Errorf("Expected target bundle, got %q", decoded.Target)
	}
	if decoded.BatchPolicy == nil {
		t.Fatal("Expected non-nil batchPolicy after unmarshaling {}")
	}
	if decoded.BatchPolicy.DebounceTimeout != nil || decoded.BatchPolicy.MaxWaitTime != nil || decoded.BatchPolicy.FailurePolicy != nil {
		t.Fatal("Expected empty batchPolicy overrides after round-trip")
	}

	var omitted TargetConfig
	if err := json.Unmarshal([]byte(`{"target":"component-a"}`), &omitted); err != nil {
		t.Fatalf("Unmarshal without batchPolicy: %v", err)
	}
	if omitted.BatchPolicy != nil {
		t.Fatalf("Expected nil batchPolicy when omitted, got %+v", omitted.BatchPolicy)
	}
}

func TestNudgeConfigStatusDeepCopy(t *testing.T) {
	original := NudgeConfigStatus{
		Conditions: []metav1.Condition{
			{
				Type:    "Valid",
				Status:  metav1.ConditionTrue,
				Reason:  "AllComponentsExist",
				Message: "All referenced components exist in namespace",
			},
		},
	}

	copied := original.DeepCopy()
	if len(copied.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %d", len(copied.Conditions))
	}
	if copied.Conditions[0].Type != "Valid" {
		t.Errorf("Expected condition type 'Valid', got '%s'", copied.Conditions[0].Type)
	}

	copied.Conditions[0].Type = "Invalid"
	if original.Conditions[0].Type != "Valid" {
		t.Errorf("Original Status was modified: got '%s'", original.Conditions[0].Type)
	}
}
