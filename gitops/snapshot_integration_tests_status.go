/*
Copyright 2023.

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

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/operator-toolkit/metadata"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const integrationTestStatusesSchema = `{
	"$schema": "http://json-schema.org/draft/2020-12/schema#",
	"type":  "array",
	"items": {
	  "type": "object",
      "properties": {
        "scenario": {
          "type": "string"
        },
        "status": {
          "type": "string"
        },
        "lastUpdateTime": {
          "type": "string"
        },
        "details": {
          "type": "string"
        },
        "startTime": {
          "type": "string"
        },
        "completionTime": {
          "type": "string"
        }
      },
	  "required": ["scenario", "status", "lastUpdateTime"]
	}
  }`

// IntegrationTestStatusDetail contains metadata about the particular scenario testing status
type IntegrationTestStatusDetail struct {
	// ScenarioName name
	ScenarioName string `json:"scenario"`
	// The status summary for the ITS and Snapshot
	Status IntegrationTestStatus `json:"status"`
	// The time of reporting the status
	LastUpdateTime time.Time `json:"lastUpdateTime"`
	// The details of reported status
	Details string `json:"details"`
	// Startime when we moved to inProgress
	StartTime *time.Time `json:"startTime,omitempty"` // pointer to make omitempty work
	// Completion time when test failed or passed
	CompletionTime *time.Time `json:"completionTime,omitempty"` // pointer to make omitempty work
}

// SnapshotIntegrationTestStatuses type handles details about snapshot tests
// Please note that internal representation differs from marshalled representation
// Data are not written directly into snapshot, they are just cached in this structure
type SnapshotIntegrationTestStatuses struct {
	// map scenario name to test details
	statuses map[string]*IntegrationTestStatusDetail
	// flag if any updates have been done
	dirty bool
}

// IsDirty returns boolean if there are any changes
func (sits *SnapshotIntegrationTestStatuses) IsDirty() bool {
	return sits.dirty
}

// ResetDirty reset repo back to clean, i.e. no changes to data
func (sits *SnapshotIntegrationTestStatuses) ResetDirty() {
	sits.dirty = false
}

// UpdateTestStatusIfChanged updates status of scenario test when status or details changed
func (sits *SnapshotIntegrationTestStatuses) UpdateTestStatusIfChanged(scenarioName string, status IntegrationTestStatus, details string) {
	var detail *IntegrationTestStatusDetail
	detail, ok := sits.statuses[scenarioName]
	timestamp := time.Now().UTC()
	if !ok {
		newDetail := IntegrationTestStatusDetail{
			ScenarioName:   scenarioName,
			Status:         -1, // undefined, must be udpated within function
			Details:        details,
			LastUpdateTime: timestamp,
		}
		detail = &newDetail
		sits.statuses[scenarioName] = detail
		sits.dirty = true
	}

	// update only when status or details changed, otherwise it's a no-op
	// to preserve timestamps
	if detail.Status != status {
		detail.Status = status
		detail.LastUpdateTime = timestamp
		sits.dirty = true

		// update start and completion time if needed, only when status changed
		switch status {
		case IntegrationTestStatusInProgress:
			detail.StartTime = &timestamp
			// null CompletionTime because testing started again
			detail.CompletionTime = nil
		case IntegrationTestStatusPending:
			// null all timestamps as test is not inProgress neither in final state
			detail.StartTime = nil
			detail.CompletionTime = nil
		case IntegrationTestStatusDeploymentError,
			IntegrationTestStatusEnvironmentProvisionError,
			IntegrationTestStatusTestFail,
			IntegrationTestStatusTestPassed:
			detail.CompletionTime = &timestamp
		}
	}

	if detail.Details != details {
		detail.Details = details
		detail.LastUpdateTime = timestamp
		sits.dirty = true
	}

}

// InitStatuses creates initial representation all scenarios
// This function also removes scenarios which are not defined in scenarios param
func (sits *SnapshotIntegrationTestStatuses) InitStatuses(scenarios *[]v1beta1.IntegrationTestScenario) {
	var expectedScenarios map[string]struct{} = make(map[string]struct{}) // map as a set

	// if given scenario doesn't exist, create it in pending state
	for _, s := range *scenarios {
		expectedScenarios[s.Name] = struct{}{}
		_, ok := sits.statuses[s.Name]
		if !ok {
			// init test statuses only if they doesn't exist
			sits.UpdateTestStatusIfChanged(s.Name, IntegrationTestStatusPending, "Pending")
		}
	}

	// remove old scenarios which are not defined anymore
	for _, detail := range sits.statuses {
		_, ok := expectedScenarios[detail.ScenarioName]
		if !ok {
			sits.DeleteStatus(detail.ScenarioName)
		}
	}
}

// DeleteStatus deletes status of the particular scenario
func (sits *SnapshotIntegrationTestStatuses) DeleteStatus(scenarioName string) {
	_, ok := sits.statuses[scenarioName]
	if ok {
		delete(sits.statuses, scenarioName)
		sits.dirty = true
	}
}

// GetStatuses returns snapshot test statuses in external format
func (sits *SnapshotIntegrationTestStatuses) GetStatuses() []*IntegrationTestStatusDetail {
	// transform map to list of structs
	result := make([]*IntegrationTestStatusDetail, 0, len(sits.statuses))
	for _, v := range sits.statuses {
		result = append(result, v)
	}
	return result
}

// GetScenarioStatus returns detail of status for the requested scenario
// Second return value represents if result was found
func (sits *SnapshotIntegrationTestStatuses) GetScenarioStatus(scenarioName string) (*IntegrationTestStatusDetail, bool) {
	detail, ok := sits.statuses[scenarioName]
	if !ok {
		return nil, false
	}
	return detail, true
}

// MarshalJSON converts data to JSON
// Please note that internal representation of data differs from marshalled output
// Example:
//
//	[
//	  {
//	    "scenario": "scenario-1",
//	    "status": "EnvironmentProvisionError",
//	    "lastUpdateTime": "2023-07-26T16:57:49+02:00",
//	    "details": "Failed ...",
//	    "startTime": "2023-07-26T14:57:49+02:00",
//	    "completionTime": "2023-07-26T16:57:49+02:00"
//	  }
//	]
func (sits *SnapshotIntegrationTestStatuses) MarshalJSON() ([]byte, error) {
	result := sits.GetStatuses()
	return json.Marshal(result)
}

// UnmarshalJSON load data from JSON
func (sits *SnapshotIntegrationTestStatuses) UnmarshalJSON(b []byte) error {
	var inputData []*IntegrationTestStatusDetail

	sch, err := jsonschema.CompileString("schema.json", integrationTestStatusesSchema)
	if err != nil {
		return fmt.Errorf("error while compiling json data for schema validation: %w", err)
	}
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("failed to unmarshal json data raw: %w", err)
	}
	if err = sch.Validate(v); err != nil {
		return fmt.Errorf("error validating test status: %w", err)
	}
	err = json.Unmarshal(b, &inputData)
	if err != nil {
		return fmt.Errorf("failed to unmarshal json data: %w", err)
	}

	// keep data in map for easier manipulation
	for _, v := range inputData {
		sits.statuses[v.ScenarioName] = v
	}

	return nil
}

// NewSnapshotIntegrationTestStatuses creates empty SnapshotTestStatus struct
func NewSnapshotIntegrationTestStatuses() *SnapshotIntegrationTestStatuses {
	sits := SnapshotIntegrationTestStatuses{
		statuses: make(map[string]*IntegrationTestStatusDetail, 1),
		dirty:    false,
	}
	return &sits
}

// NewSnapshotIntegrationTestStatusesFromSnapshot creates new SnapshotTestStatus struct from snapshot annotation
func NewSnapshotIntegrationTestStatusesFromSnapshot(s *applicationapiv1alpha1.Snapshot) (*SnapshotIntegrationTestStatuses, error) {
	annotations := map[string]string{}
	if s.ObjectMeta.GetAnnotations() != nil {
		annotations = s.ObjectMeta.GetAnnotations()
	}
	sits := NewSnapshotIntegrationTestStatuses()

	statusAnnotation, ok := annotations[SnapshotTestsStatusAnnotation]
	if ok {
		err := json.Unmarshal([]byte(statusAnnotation), sits)
		if err != nil {
			return nil, fmt.Errorf("failed to load tests statuses from the scenario annotation: %w", err)
		}
	}

	return sits, nil
}

// WriteIntegrationTestStatusesIntoSnapshot writes data to snapshot by updating CR
// Data are written only when new changes are detected
func WriteIntegrationTestStatusesIntoSnapshot(s *applicationapiv1alpha1.Snapshot, sts *SnapshotIntegrationTestStatuses, c client.Client, ctx context.Context) error {
	if !sts.IsDirty() {
		// No updates were done, we don't need to update snapshot
		return nil
	}
	patch := client.MergeFrom(s.DeepCopy())

	value, err := json.Marshal(sts)
	if err != nil {
		return fmt.Errorf("failed to marshal test results into JSON: %w", err)
	}

	newAnnotations := map[string]string{
		SnapshotTestsStatusAnnotation: string(value),
	}
	if err := metadata.AddAnnotations(&s.ObjectMeta, newAnnotations); err != nil {
		return fmt.Errorf("failed to add annotations: %w", err)
	}

	err = c.Patch(ctx, s, patch)
	if err != nil {
		// don't return wrapped err, so we can use RetryOnConflict
		return err
	}
	sts.ResetDirty()
	return nil
}
