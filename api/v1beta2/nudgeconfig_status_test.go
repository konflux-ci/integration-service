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
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func TestBatchPhaseConstants(t *testing.T) {
	if BatchPhaseAccumulating != "Accumulating" {
		t.Errorf("Expected BatchPhaseAccumulating 'Accumulating', got '%s'", BatchPhaseAccumulating)
	}
	if BatchPhaseBlocked != "Blocked" {
		t.Errorf("Expected BatchPhaseBlocked 'Blocked', got '%s'", BatchPhaseBlocked)
	}
	if BatchPhaseFiring != "Firing" {
		t.Errorf("Expected BatchPhaseFiring 'Firing', got '%s'", BatchPhaseFiring)
	}
	if BatchPhaseFailed != "Failed" {
		t.Errorf("Expected BatchPhaseFailed 'Failed', got '%s'", BatchPhaseFailed)
	}
	if BatchPhaseCompleted != "Completed" {
		t.Errorf("Expected BatchPhaseCompleted 'Completed', got '%s'", BatchPhaseCompleted)
	}
}

func TestAccumulatedEntryStruct(t *testing.T) {
	now := metav1.Now()
	entry := AccumulatedEntry{
		From:             "component-a",
		ImageDigest:      "sha256:abc123def456",
		BuildPipelineRun: "build-plr-xyz",
		CapturedAt:       now,
	}

	if entry.From != "component-a" {
		t.Errorf("Expected From 'component-a', got '%s'", entry.From)
	}
	if entry.ImageDigest != "sha256:abc123def456" {
		t.Errorf("Expected ImageDigest 'sha256:abc123def456', got '%s'", entry.ImageDigest)
	}
	if entry.BuildPipelineRun != "build-plr-xyz" {
		t.Errorf("Expected BuildPipelineRun 'build-plr-xyz', got '%s'", entry.BuildPipelineRun)
	}
	if entry.CapturedAt != now {
		t.Errorf("Expected CapturedAt to match set value")
	}

	// Zero value should have empty fields
	zero := AccumulatedEntry{}
	if zero.From != "" {
		t.Errorf("Expected empty From on zero AccumulatedEntry, got '%s'", zero.From)
	}
	if zero.ImageDigest != "" {
		t.Errorf("Expected empty ImageDigest on zero AccumulatedEntry, got '%s'", zero.ImageDigest)
	}
	if zero.BuildPipelineRun != "" {
		t.Errorf("Expected empty BuildPipelineRun on zero AccumulatedEntry, got '%s'", zero.BuildPipelineRun)
	}
}

func TestFailedEntryStruct(t *testing.T) {
	now := metav1.Now()
	entry := FailedEntry{
		From:             "component-b",
		BuildPipelineRun: "build-plr-failed",
		Reason:           "image push failed: quota exceeded",
		CapturedAt:       now,
	}

	if entry.From != "component-b" {
		t.Errorf("Expected From 'component-b', got '%s'", entry.From)
	}
	if entry.BuildPipelineRun != "build-plr-failed" {
		t.Errorf("Expected BuildPipelineRun 'build-plr-failed', got '%s'", entry.BuildPipelineRun)
	}
	if entry.Reason != "image push failed: quota exceeded" {
		t.Errorf("Expected Reason 'image push failed: quota exceeded', got '%s'", entry.Reason)
	}
	if entry.CapturedAt != now {
		t.Errorf("Expected CapturedAt to match set value")
	}

	// Zero value should have empty fields
	zero := FailedEntry{}
	if zero.From != "" {
		t.Errorf("Expected empty From on zero FailedEntry, got '%s'", zero.From)
	}
	if zero.BuildPipelineRun != "" {
		t.Errorf("Expected empty BuildPipelineRun on zero FailedEntry, got '%s'", zero.BuildPipelineRun)
	}
	if zero.Reason != "" {
		t.Errorf("Expected empty Reason on zero FailedEntry, got '%s'", zero.Reason)
	}
}

func TestActiveBatchFullStruct(t *testing.T) {
	now := metav1.Now()
	fireAt := metav1.NewTime(now.Add(5 * time.Minute))
	deadline := metav1.NewTime(now.Add(30 * time.Minute))

	batch := ActiveBatch{
		Target:       "component-b",
		BatchID:      "batch-20260908-001",
		Phase:        BatchPhaseAccumulating,
		CreatedAt:    now,
		FireAt:       &fireAt,
		HardDeadline: deadline,
		Accumulated: []AccumulatedEntry{
			{
				From:             "component-a",
				ImageDigest:      "sha256:deadbeef",
				BuildPipelineRun: "build-plr-1",
				CapturedAt:       now,
			},
		},
		Failed: []FailedEntry{
			{
				From:             "component-c",
				BuildPipelineRun: "build-plr-2",
				Reason:           "timeout",
				CapturedAt:       now,
			},
		},
		NudgePipelineRun: "",
		Message:          "waiting for batch window to close",
	}

	if batch.Target != "component-b" {
		t.Errorf("Expected Target 'component-b', got '%s'", batch.Target)
	}
	if batch.BatchID != "batch-20260908-001" {
		t.Errorf("Expected BatchID 'batch-20260908-001', got '%s'", batch.BatchID)
	}
	if batch.Phase != BatchPhaseAccumulating {
		t.Errorf("Expected Phase '%s', got '%s'", BatchPhaseAccumulating, batch.Phase)
	}
	if len(batch.Accumulated) != 1 {
		t.Fatalf("Expected 1 AccumulatedEntry, got %d", len(batch.Accumulated))
	}
	if batch.Accumulated[0].From != "component-a" {
		t.Errorf("Expected Accumulated[0].From 'component-a', got '%s'", batch.Accumulated[0].From)
	}
	if batch.Accumulated[0].ImageDigest != "sha256:deadbeef" {
		t.Errorf("Expected Accumulated[0].ImageDigest 'sha256:deadbeef', got '%s'", batch.Accumulated[0].ImageDigest)
	}
	if batch.Accumulated[0].BuildPipelineRun != "build-plr-1" {
		t.Errorf("Expected Accumulated[0].BuildPipelineRun 'build-plr-1', got '%s'", batch.Accumulated[0].BuildPipelineRun)
	}
	if len(batch.Failed) != 1 {
		t.Fatalf("Expected 1 FailedEntry, got %d", len(batch.Failed))
	}
	if batch.Failed[0].From != "component-c" {
		t.Errorf("Expected Failed[0].From 'component-c', got '%s'", batch.Failed[0].From)
	}
	if batch.Failed[0].Reason != "timeout" {
		t.Errorf("Expected Failed[0].Reason 'timeout', got '%s'", batch.Failed[0].Reason)
	}
	if batch.NudgePipelineRun != "" {
		t.Errorf("Expected empty NudgePipelineRun, got '%s'", batch.NudgePipelineRun)
	}
	if batch.Message != "waiting for batch window to close" {
		t.Errorf("Expected Message 'waiting for batch window to close', got '%s'", batch.Message)
	}
}

func TestNudgeConfigStatusActiveBatches(t *testing.T) {
	// Empty status — ActiveBatches must be nil (omitempty in JSON)
	status := NudgeConfigStatus{}
	if status.ActiveBatches != nil {
		t.Errorf("Expected nil ActiveBatches on zero NudgeConfigStatus, got %v", status.ActiveBatches)
	}

	// Populated status
	now := metav1.Now()
	status.ActiveBatches = []ActiveBatch{
		{
			Target:       "component-b",
			BatchID:      "batch-001",
			Phase:        BatchPhaseFiring,
			CreatedAt:    now,
			FireAt:       &now,
			HardDeadline: metav1.NewTime(now.Add(30 * time.Minute)),
		},
	}
	if len(status.ActiveBatches) != 1 {
		t.Fatalf("Expected 1 ActiveBatch, got %d", len(status.ActiveBatches))
	}
	if status.ActiveBatches[0].Target != "component-b" {
		t.Errorf("Expected ActiveBatches[0].Target 'component-b', got '%s'", status.ActiveBatches[0].Target)
	}
	if status.ActiveBatches[0].Phase != BatchPhaseFiring {
		t.Errorf("Expected ActiveBatches[0].Phase '%s', got '%s'", BatchPhaseFiring, status.ActiveBatches[0].Phase)
	}
}

func TestActiveBatchDeepCopy(t *testing.T) {
	now := metav1.Now()
	fireAt := metav1.NewTime(now.Add(5 * time.Minute))
	original := ActiveBatch{
		Target:       "component-b",
		BatchID:      "batch-001",
		Phase:        BatchPhaseAccumulating,
		CreatedAt:    now,
		FireAt:       &fireAt,
		HardDeadline: metav1.NewTime(now.Add(30 * time.Minute)),
		Accumulated: []AccumulatedEntry{
			{From: "component-a", ImageDigest: "sha256:aabb", BuildPipelineRun: "plr-1", CapturedAt: now},
		},
	}

	copied := original.DeepCopy()
	if copied == nil {
		t.Fatal("DeepCopy returned nil")
	}

	// Verify copy matches original
	if copied.Target != original.Target {
		t.Errorf("DeepCopy: Expected Target '%s', got '%s'", original.Target, copied.Target)
	}
	if copied.BatchID != original.BatchID {
		t.Errorf("DeepCopy: Expected BatchID '%s', got '%s'", original.BatchID, copied.BatchID)
	}
	if len(copied.Accumulated) != 1 {
		t.Fatalf("DeepCopy: Expected 1 AccumulatedEntry, got %d", len(copied.Accumulated))
	}
	if copied.Accumulated[0].From != original.Accumulated[0].From {
		t.Errorf("DeepCopy: Expected Accumulated[0].From '%s', got '%s'", original.Accumulated[0].From, copied.Accumulated[0].From)
	}

	// Mutate the copy and verify original is unchanged
	copied.Target = "modified-target"
	copied.BatchID = "modified-batch"
	copied.Accumulated[0].From = "modified-component"

	if original.Target != "component-b" {
		t.Errorf("Original Target was modified: got '%s'", original.Target)
	}
	if original.BatchID != "batch-001" {
		t.Errorf("Original BatchID was modified: got '%s'", original.BatchID)
	}
	if original.Accumulated[0].From != "component-a" {
		t.Errorf("Original Accumulated[0].From was modified: got '%s'", original.Accumulated[0].From)
	}
}

func TestActiveBatchOptionalFieldsAbsent(t *testing.T) {
	now := metav1.Now()
	batch := ActiveBatch{
		Target:       "component-x",
		BatchID:      "batch-min",
		Phase:        BatchPhaseCompleted,
		CreatedAt:    now,
		FireAt:       &now,
		HardDeadline: now,
	}

	if batch.Accumulated != nil {
		t.Errorf("Expected nil Accumulated on minimal ActiveBatch, got %v", batch.Accumulated)
	}
	if batch.Failed != nil {
		t.Errorf("Expected nil Failed on minimal ActiveBatch, got %v", batch.Failed)
	}
	if batch.NudgePipelineRun != "" {
		t.Errorf("Expected empty NudgePipelineRun, got '%s'", batch.NudgePipelineRun)
	}
	if batch.Message != "" {
		t.Errorf("Expected empty Message, got '%s'", batch.Message)
	}
}

func TestActiveBatchBlockedState(t *testing.T) {
	now := metav1.Now()
	// Per ADR 72 (pending merge: https://github.com/konflux-ci/architecture/pull/372):
	// during a blocked state, FireAt should be blank
	batch := ActiveBatch{
		Target:    "component-x",
		BatchID:   "batch-blocked",
		Phase:     BatchPhaseBlocked,
		CreatedAt: now,
		// FireAt intentionally omitted (zero value) for blocked state
		HardDeadline: metav1.NewTime(now.Add(30 * time.Minute)),
		Message:      "waiting for dependency component-y",
	}

	if batch.Phase != BatchPhaseBlocked {
		t.Errorf("Expected Phase '%s', got '%s'", BatchPhaseBlocked, batch.Phase)
	}
	if batch.FireAt != nil {
		t.Errorf("Expected nil FireAt for blocked batch, got %v", batch.FireAt)
	}
	if batch.Target != "component-x" {
		t.Errorf("Expected Target 'component-x', got '%s'", batch.Target)
	}
	if batch.BatchID != "batch-blocked" {
		t.Errorf("Expected BatchID 'batch-blocked', got '%s'", batch.BatchID)
	}
	if batch.Message != "waiting for dependency component-y" {
		t.Errorf("Expected Message 'waiting for dependency component-y', got '%s'", batch.Message)
	}
}

func TestActiveBatchYAMLShape(t *testing.T) {
	now := metav1.Now()
	fireAt := metav1.NewTime(now.Add(5 * time.Minute))
	deadline := metav1.NewTime(now.Add(30 * time.Minute))

	// Test full batch with FireAt set (Accumulating phase)
	batch := ActiveBatch{
		Target:       "component-b",
		BatchID:      "batch-001",
		Phase:        BatchPhaseAccumulating,
		CreatedAt:    now,
		FireAt:       &fireAt,
		HardDeadline: deadline,
		Accumulated: []AccumulatedEntry{
			{From: "component-a", ImageDigest: "sha256:abc", BuildPipelineRun: "plr-1", CapturedAt: now},
		},
		RetryCount: 0,
	}

	yamlBytes, err := yaml.Marshal(batch)
	if err != nil {
		t.Fatalf("Failed to marshal ActiveBatch: %v", err)
	}
	yamlStr := string(yamlBytes)

	// Verify required fields present
	if !strings.Contains(yamlStr, "target: component-b") {
		t.Errorf("YAML missing target field")
	}
	if !strings.Contains(yamlStr, "batchId: batch-001") {
		t.Errorf("YAML missing batchId field")
	}
	if !strings.Contains(yamlStr, "phase: Accumulating") {
		t.Errorf("YAML missing phase field")
	}
	if !strings.Contains(yamlStr, "fireAt:") {
		t.Errorf("YAML missing fireAt field when set")
	}

	// Test blocked batch with FireAt nil
	blockedBatch := ActiveBatch{
		Target:       "component-x",
		BatchID:      "batch-blocked",
		Phase:        BatchPhaseBlocked,
		CreatedAt:    now,
		FireAt:       nil, // Should be omitted in YAML
		HardDeadline: deadline,
		Message:      "waiting for dependency",
	}

	blockedYaml, err := yaml.Marshal(blockedBatch)
	if err != nil {
		t.Fatalf("Failed to marshal blocked ActiveBatch: %v", err)
	}
	blockedStr := string(blockedYaml)

	// Verify fireAt is NOT present when nil
	if strings.Contains(blockedStr, "fireAt:") {
		t.Errorf("YAML should NOT contain fireAt when nil, got: %s", blockedStr)
	}
}
