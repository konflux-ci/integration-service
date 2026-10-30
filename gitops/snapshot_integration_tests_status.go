/*
Copyright 2023 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions andF
limitations under the License.
*/

package gitops

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/operator-toolkit/metadata"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO(KONFLUX-16551): remove statusEnumShiftIntroducedAt and
// legacyShiftedStatusNames once Snapshots written by the stale enum have
// aged out of snapshot garbage collection.

// statusEnumShiftIntroducedAt is when IntegrationTestStatusTestWarning landed
// (STONEINTG-1489, 4bcafa4f) without regenerating enumer. Status JSON last
// updated before this instant already uses the current spelling. Later JSON
// that has no SnapshotTestsStatusEnumVersionAnnotation uses the shifted names.
var statusEnumShiftIntroducedAt = time.Date(2026, 3, 16, 16, 23, 23, 0, time.UTC)

// legacyShiftedStatusNames maps a status string written by the stale enum to
// the canonical name of the value that produced it.
var legacyShiftedStatusNames = map[string]string{
	"BuildPLRInProgress":          "TestWarning",
	"SnapshotCreationFailed":      "BuildPLRInProgress",
	"BuildPLRFailed":              "SnapshotCreationFailed",
	"GroupSnapshotCreationFailed": "BuildPLRFailed",
	// Value 13 fell off the end of the stale name table.
	"IntegrationTestStatus(13)": "GroupSnapshotCreationFailed",
}

// encodeTranslatedStatusJSON and setSnapshotAnnotation are the production
// implementations. Tests replace them to exercise error returns that these
// inputs do not produce: JSON from a decoded status document remarshals, and
// SetAnnotation only fails when its object is nil.
var (
	encodeTranslatedStatusJSON = json.Marshal
	setSnapshotAnnotation      = metadata.SetAnnotation
)

// NewSnapshotIntegrationTestStatusesFromSnapshot creates new SnapshotTestStatus struct from snapshot annotation
func NewSnapshotIntegrationTestStatusesFromSnapshot(s *applicationapiv1alpha1.Snapshot) (*intgteststat.SnapshotIntegrationTestStatuses, error) {
	annotations := map[string]string{}
	if s.GetAnnotations() != nil {
		annotations = s.GetAnnotations()
	}
	statusAnnotation, ok := annotations[SnapshotTestsStatusAnnotation]
	if !ok {
		statusAnnotation = ""
	}
	if statusAnnotation != "" && annotations[SnapshotTestsStatusEnumVersionAnnotation] != SnapshotTestsStatusEnumVersion {
		translated, err := translateLegacySnapshotTestsStatus(statusAnnotation)
		if err != nil {
			return nil, fmt.Errorf("failed to translate legacy integration test statuses: %w", err)
		}
		statusAnnotation = translated
	}
	sits, err := intgteststat.NewSnapshotIntegrationTestStatuses(statusAnnotation)
	if err != nil {
		return nil, fmt.Errorf("failed to get integration tests statuses from snapshot: %w", err)
	}

	return sits, nil
}

// WriteIntegrationTestStatusesIntoSnapshot writes data to snapshot by updating CR
// Data are written only when new changes are detected
func WriteIntegrationTestStatusesIntoSnapshot(ctx context.Context, s *applicationapiv1alpha1.Snapshot, sts *intgteststat.SnapshotIntegrationTestStatuses, c client.Client) error {
	if !sts.IsDirty() {
		// No updates were done, we don't need to update snapshot
		return nil
	}
	patch := client.MergeFrom(s.DeepCopy())

	value, err := json.Marshal(sts)
	if err != nil {
		return fmt.Errorf("failed to marshal test results into JSON: %w", err)
	}

	if err := setSnapshotAnnotation(&s.ObjectMeta, SnapshotTestsStatusAnnotation, string(value)); err != nil {
		return fmt.Errorf("failed to add annotations: %w", err)
	}
	if err := setSnapshotAnnotation(&s.ObjectMeta, SnapshotTestsStatusEnumVersionAnnotation, SnapshotTestsStatusEnumVersion); err != nil {
		return fmt.Errorf("failed to add status enum version annotation: %w", err)
	}

	err = c.Patch(ctx, s, patch)
	if err != nil {
		// don't return wrapped err, so we can use RetryOnConflict
		return err
	}
	sts.ResetDirty()
	return nil
}

// translateLegacySnapshotTestsStatus rewrites status strings stored by the stale enum.
// IntegrationTestStatus(13) had no other meaning. The other shifted names are rewritten
// only when lastUpdateTime is at or after statusEnumShiftIntroducedAt, because those
// same strings were canonical before TestWarning existed.
func translateLegacySnapshotTestsStatus(raw string) (string, error) {
	var entries []map[string]any
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return raw, nil
	}

	changed := false
	for _, entry := range entries {
		status, ok := entry["status"].(string)
		if !ok {
			continue
		}
		canonical, shifted := legacyShiftedStatusNames[status]
		if !shifted {
			continue
		}
		if status != "IntegrationTestStatus(13)" && !legacyStatusWrittenAfterShift(entry) {
			continue
		}
		entry["status"] = canonical
		changed = true
	}
	if !changed {
		return raw, nil
	}

	rewritten, err := encodeTranslatedStatusJSON(entries)
	if err != nil {
		return "", fmt.Errorf("failed to encode translated integration test statuses: %w", err)
	}
	return string(rewritten), nil
}

// legacyStatusWrittenAfterShift reports whether the scenario status was last
// updated at or after statusEnumShiftIntroducedAt. A missing or unparseable
// lastUpdateTime is treated as written before the shift.
func legacyStatusWrittenAfterShift(entry map[string]any) bool {
	raw, ok := entry["lastUpdateTime"].(string)
	if !ok || raw == "" {
		return false
	}
	updated, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return false
	}
	return !updated.Before(statusEnumShiftIntroducedAt)
}
