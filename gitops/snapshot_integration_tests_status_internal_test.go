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

package gitops

import (
	"encoding/json"
	"testing"
)

func TestTranslateLegacySnapshotTestsStatusKeepsShiftedNameWithoutTrustedTime(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "empty lastUpdateTime",
			raw:  `[{"scenario":"scenario-1","status":"BuildPLRInProgress","lastUpdateTime":""}]`,
		},
		{
			name: "missing lastUpdateTime",
			raw:  `[{"scenario":"scenario-1","status":"BuildPLRInProgress"}]`,
		},
		{
			name: "non-string lastUpdateTime",
			raw:  `[{"scenario":"scenario-1","status":"BuildPLRInProgress","lastUpdateTime":1}]`,
		},
		{
			name: "unparseable lastUpdateTime",
			raw:  `[{"scenario":"scenario-1","status":"BuildPLRInProgress","lastUpdateTime":"not-a-time"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := translateLegacySnapshotTestsStatus(tt.raw)
			if err != nil {
				t.Fatalf("failed to translate legacy statuses: %v", err)
			}
			if got != tt.raw {
				t.Fatalf("rewrote status JSON:\n got %s\nwant %s", got, tt.raw)
			}
		})
	}
}

func TestTranslateLegacySnapshotTestsStatusRewritesOnlyTrustedEntries(t *testing.T) {
	raw := `[{"scenario":"stale","status":"BuildPLRInProgress","lastUpdateTime":"not-a-time","details":"bad"},{"scenario":"fresh","status":"BuildPLRInProgress","lastUpdateTime":"2026-04-02T12:00:00Z","details":"ok"}]`

	got, err := translateLegacySnapshotTestsStatus(raw)
	if err != nil {
		t.Fatalf("translate legacy statuses: %v", err)
	}

	statuses := scenarioStatuses(t, got)
	if statuses["stale"] != "BuildPLRInProgress" {
		t.Fatalf("stale status = %q, want BuildPLRInProgress", statuses["stale"])
	}
	if statuses["fresh"] != "TestWarning" {
		t.Fatalf("fresh status = %q, want TestWarning", statuses["fresh"])
	}
}

func TestTranslateLegacySnapshotTestsStatusRewritesUnknownValueWithoutTime(t *testing.T) {
	raw := `[{"scenario":"scenario-1","status":"IntegrationTestStatus(13)","lastUpdateTime":""}]`

	got, err := translateLegacySnapshotTestsStatus(raw)
	if err != nil {
		t.Fatalf("translate legacy statuses: %v", err)
	}

	statuses := scenarioStatuses(t, got)
	if statuses["scenario-1"] != "GroupSnapshotCreationFailed" {
		t.Fatalf("status = %q, want GroupSnapshotCreationFailed", statuses["scenario-1"])
	}
}

func scenarioStatuses(t *testing.T, raw string) map[string]string {
	t.Helper()

	var entries []map[string]any
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatalf("unmarshal translated statuses: %v", err)
	}

	statuses := make(map[string]string, len(entries))
	for _, entry := range entries {
		scenario, ok := entry["scenario"].(string)
		if !ok {
			t.Fatalf("scenario is not a string: %#v", entry["scenario"])
		}
		status, ok := entry["status"].(string)
		if !ok {
			t.Fatalf("status is not a string: %#v", entry["status"])
		}
		statuses[scenario] = status
	}
	return statuses
}
