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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

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
	if nc.Status.Conditions != nil {
		t.Errorf("Expected nil Conditions, got %v", nc.Status.Conditions)
	}
	if nc.Status.LastValidationTime != nil {
		t.Errorf("Expected nil LastValidationTime, got %v", nc.Status.LastValidationTime)
	}
	if nc.Spec.BatchDefaults != nil {
		t.Errorf("Expected nil BatchDefaults, got %v", nc.Spec.BatchDefaults)
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

func TestBatchDefaultsCELLiteralsMatchRuntimeDefaults(t *testing.T) {
	debounceLiteral, err := kubernetesDurationLiteral(DefaultBatchDebounceTimeout)
	if err != nil {
		t.Fatal(err)
	}
	maxWaitLiteral, err := kubernetesDurationLiteral(DefaultBatchMaxWaitTime)
	if err != nil {
		t.Fatal(err)
	}

	debounceCEL := fmt.Sprintf("duration('%s')", debounceLiteral)
	maxWaitCEL := fmt.Sprintf("duration('%s')", maxWaitLiteral)

	source, err := os.ReadFile("nudgeconfig_types.go")
	if err != nil {
		t.Fatalf("read nudgeconfig_types.go: %v", err)
	}
	assertContainsBatchDefaultCEL(t, "nudgeconfig_types.go", string(source), debounceCEL, maxWaitCEL)

	crdPath := filepath.Join("..", "..", "config", "crd", "bases", "appstudio.redhat.com_nudgeconfigs.yaml")
	crd, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("read %s: %v", crdPath, err)
	}
	assertContainsBatchDefaultCEL(t, crdPath, strings.ReplaceAll(string(crd), "''", "'"), debounceCEL, maxWaitCEL)

	if DefaultBatchFailurePolicy != FailurePolicyBlock {
		t.Errorf("Expected DefaultBatchFailurePolicy %q, got %q", FailurePolicyBlock, DefaultBatchFailurePolicy)
	}
}

func assertContainsBatchDefaultCEL(t *testing.T, path, contents, debounceCEL, maxWaitCEL string) {
	t.Helper()
	if !strings.Contains(contents, debounceCEL) {
		t.Errorf("%s must reference %s to stay in sync with DefaultBatchDebounceTimeout", path, debounceCEL)
	}
	if !strings.Contains(contents, maxWaitCEL) {
		t.Errorf("%s must reference %s to stay in sync with DefaultBatchMaxWaitTime", path, maxWaitCEL)
	}
}

func TestBatchDefaultsJSONRoundTrip(t *testing.T) {
	original := NudgeConfigSpec{
		BatchDefaults: &BatchDefaults{
			DebounceTimeout: durationPtr(DefaultBatchDebounceTimeout),
			MaxWaitTime:     durationPtr(DefaultBatchMaxWaitTime),
			FailurePolicy:   failurePolicyPtr(DefaultBatchFailurePolicy),
		},
		Nudges: []NudgeRelationship{
			{From: "component-a", To: "component-b", Mode: NudgeModeImmediate},
		},
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded NudgeConfigSpec
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.BatchDefaults == nil {
		t.Fatal("Expected BatchDefaults after JSON round-trip, got nil")
	}
	if decoded.BatchDefaults.DebounceTimeout == nil || decoded.BatchDefaults.DebounceTimeout.Duration != DefaultBatchDebounceTimeout {
		t.Errorf("Expected debounceTimeout %s, got %v", DefaultBatchDebounceTimeout, decoded.BatchDefaults.DebounceTimeout)
	}
	if decoded.BatchDefaults.MaxWaitTime == nil || decoded.BatchDefaults.MaxWaitTime.Duration != DefaultBatchMaxWaitTime {
		t.Errorf("Expected maxWaitTime %s, got %v", DefaultBatchMaxWaitTime, decoded.BatchDefaults.MaxWaitTime)
	}
	if decoded.BatchDefaults.FailurePolicy == nil || *decoded.BatchDefaults.FailurePolicy != DefaultBatchFailurePolicy {
		t.Errorf("Expected failurePolicy %s, got %v", DefaultBatchFailurePolicy, decoded.BatchDefaults.FailurePolicy)
	}
}

func TestBatchDefaultsYAMLRoundTrip(t *testing.T) {
	original := NudgeConfigSpec{
		BatchDefaults: &BatchDefaults{
			DebounceTimeout: durationPtr(15 * time.Minute),
			MaxWaitTime:     durationPtr(2 * time.Hour),
			FailurePolicy:   failurePolicyPtr(FailurePolicyProceedWithPartial),
		},
	}

	encoded, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("YAML Marshal: %v", err)
	}

	var decoded NudgeConfigSpec
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("YAML Unmarshal: %v", err)
	}

	if decoded.BatchDefaults == nil {
		t.Fatal("Expected BatchDefaults after YAML round-trip, got nil")
	}
	if decoded.BatchDefaults.DebounceTimeout == nil || decoded.BatchDefaults.DebounceTimeout.Duration != 15*time.Minute {
		t.Errorf("Expected debounceTimeout 15m, got %v", decoded.BatchDefaults.DebounceTimeout)
	}
	if decoded.BatchDefaults.MaxWaitTime == nil || decoded.BatchDefaults.MaxWaitTime.Duration != 2*time.Hour {
		t.Errorf("Expected maxWaitTime 2h, got %v", decoded.BatchDefaults.MaxWaitTime)
	}
	if decoded.BatchDefaults.FailurePolicy == nil || *decoded.BatchDefaults.FailurePolicy != FailurePolicyProceedWithPartial {
		t.Errorf("Expected failurePolicy ProceedWithPartial, got %v", decoded.BatchDefaults.FailurePolicy)
	}
}

func TestOmittedBatchDefaultsIsNil(t *testing.T) {
	var fromJSON NudgeConfigSpec
	if err := json.Unmarshal([]byte(`{"nudges":[]}`), &fromJSON); err != nil {
		t.Fatalf("JSON Unmarshal: %v", err)
	}
	if fromJSON.BatchDefaults != nil {
		t.Errorf("Expected nil BatchDefaults from JSON without the field, got %+v", fromJSON.BatchDefaults)
	}

	var fromYAML NudgeConfigSpec
	if err := yaml.Unmarshal([]byte("nudges: []\n"), &fromYAML); err != nil {
		t.Fatalf("YAML Unmarshal: %v", err)
	}
	if fromYAML.BatchDefaults != nil {
		t.Errorf("Expected nil BatchDefaults from YAML without the field, got %+v", fromYAML.BatchDefaults)
	}

	encoded, err := json.Marshal(NudgeConfigSpec{})
	if err != nil {
		t.Fatalf("Marshal empty spec: %v", err)
	}
	if strings.Contains(string(encoded), "batchDefaults") {
		t.Errorf("Expected omitted batchDefaults in serialized empty spec, got %s", encoded)
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
		BatchDefaults: &BatchDefaults{
			DebounceTimeout: durationPtr(30 * time.Minute),
			FailurePolicy:   failurePolicyPtr(FailurePolicyBlock),
		},
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
	if copied.BatchDefaults == nil || copied.BatchDefaults.DebounceTimeout == nil {
		t.Fatal("Expected copied BatchDefaults.DebounceTimeout")
	}
	if copied.BatchDefaults.DebounceTimeout.Duration != 30*time.Minute {
		t.Errorf("Expected copied debounceTimeout 30m, got %v", copied.BatchDefaults.DebounceTimeout)
	}

	copied.Nudges[0].From = "modified"
	copied.BatchDefaults.DebounceTimeout.Duration = time.Hour
	*copied.BatchDefaults.FailurePolicy = FailurePolicyProceedWithPartial
	if original.Nudges[0].From != "comp-a" {
		t.Errorf("Original Spec was modified: got '%s'", original.Nudges[0].From)
	}
	if original.BatchDefaults.DebounceTimeout.Duration != 30*time.Minute {
		t.Errorf("Original BatchDefaults.DebounceTimeout was modified: got %v", original.BatchDefaults.DebounceTimeout)
	}
	if *original.BatchDefaults.FailurePolicy != FailurePolicyBlock {
		t.Errorf("Original BatchDefaults.FailurePolicy was modified: got %v", *original.BatchDefaults.FailurePolicy)
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
