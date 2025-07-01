/*
Copyright 2023 Red Hat Inc.

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

package integrationteststatus

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// IntegrationTestScenario test runs status
type IntegrationTestStatus int

//go:generate enumer -type=IntegrationTestStatus -linecomment -json
const (
	// Nothing is done yet for the ITS list and snapshot
	IntegrationTestStatusPending IntegrationTestStatus = iota + 1 // Pending
	// Starting to handle an integration test scenario for a snapshot
	IntegrationTestStatusInProgress // InProgress
	// Integration PLR deleted for this ITS and snapshot
	IntegrationTestStatusDeleted // Deleted
	// (Deprecated) The environment provision experienced error for this ITS and snapshot
	IntegrationTestStatusEnvironmentProvisionError_Deprecated // EnvironmentProvisionError
	// (Deprecated) The SEB deployment experienced error for this ITS and snapshot
	IntegrationTestStatusDeploymentError_Deprecated // DeploymentError
	// Integration PLR failed for this ITS and snapshot
	IntegrationTestStatusTestFail // TestFail
	// Integration PLR passed for this ITS and snapshot
	IntegrationTestStatusTestPassed // TestPassed
	// Integration PLR is invalid
	IntegrationTestStatusTestInvalid // TestInvalid
	// Build PLR is in progress
	BuildPLRInProgress // BuildPLRInProgress
	// Snapshot is not created
	SnapshotCreationFailed // SnapshotCreationFailed
	// Build pipelinerun failed
	BuildPLRFailed // BuildPLRFailed
	// Group snapshot creation failed
	GroupSnapshotCreationFailed //GroupSnapshotCreationFailed
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
        },
        "testPipelineRunName": {
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
	// TestPipelineName name of testing pipelineRun
	TestPipelineRunName string `json:"testPipelineRunName,omitempty"`
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

func (sits *IntegrationTestStatus) IsFinal() bool {
	switch *sits {
	case IntegrationTestStatusDeleted,
		IntegrationTestStatusDeploymentError_Deprecated,
		IntegrationTestStatusEnvironmentProvisionError_Deprecated,
		IntegrationTestStatusTestFail,
		IntegrationTestStatusTestPassed,
		IntegrationTestStatusTestInvalid:
		return true
	}
	return false
}

// IsDirty returns boolean if there are any changes
func (sits *SnapshotIntegrationTestStatuses) IsDirty() bool {
	return sits.dirty
}

// ResetDirty reset repo back to clean, i.e. no changes to data
func (sits *SnapshotIntegrationTestStatuses) ResetDirty() {
	sits.dirty = false
}

// ResetStatus reset status of test back to initial Pending status and removes invalidated values
func (sits *SnapshotIntegrationTestStatuses) ResetStatus(scenarioName string) {
	sits.UpdateTestStatusIfChanged(scenarioName, IntegrationTestStatusPending, "Pending")
	detail := sits.statuses[scenarioName]
	detail.TestPipelineRunName = ""
	sits.dirty = true
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
		case IntegrationTestStatusPending, BuildPLRInProgress:
			// null all timestamps as test is not inProgress neither in final state
			detail.StartTime = nil
			detail.CompletionTime = nil
		case IntegrationTestStatusDeploymentError_Deprecated,
			IntegrationTestStatusEnvironmentProvisionError_Deprecated,
			IntegrationTestStatusDeleted,
			IntegrationTestStatusTestFail,
			IntegrationTestStatusTestPassed,
			IntegrationTestStatusTestInvalid,
			SnapshotCreationFailed,
			GroupSnapshotCreationFailed,
			BuildPLRFailed:

			detail.CompletionTime = &timestamp
		}
	}

	if detail.Details != details {
		detail.Details = details
		detail.LastUpdateTime = timestamp
		sits.dirty = true
	}

}

// UpdateTestPipelineRunName updates TestPipelineRunName if changed
// scenario must already exist in statuses
func (sits *SnapshotIntegrationTestStatuses) UpdateTestPipelineRunName(scenarioName string, pipelineRunName string) error {
	detail, ok := sits.GetScenarioStatus(scenarioName)
	if !ok {
		return fmt.Errorf("scenario name %s not found within the SnapshotIntegrationTestStatus, and cannot be updated", scenarioName)
	}

	if detail.TestPipelineRunName != pipelineRunName {
		detail.TestPipelineRunName = pipelineRunName
		sits.dirty = true
	}

	return nil
}

// InitStatuses creates initial representation all scenarios
// This function also removes scenarios which are not defined in scenarios param
func (sits *SnapshotIntegrationTestStatuses) InitStatuses(scenarioNames *[]string) {
	var expectedScenarios = make(map[string]struct{}) // map as a set

	// if given scenario doesn't exist, create it in pending state
	for _, name := range *scenarioNames {
		expectedScenarios[name] = struct{}{}
		_, ok := sits.statuses[name]
		if !ok {
			// init test statuses only if they doesn't exist
			sits.UpdateTestStatusIfChanged(name, IntegrationTestStatusPending, "Pending")
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
//	    "completionTime": "2023-07-26T16:57:49+02:00",
//	    "testPipelineRunName": "pipeline-run-feedbeef"
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
func NewSnapshotIntegrationTestStatuses(jsondata string) (*SnapshotIntegrationTestStatuses, error) {
	sits := SnapshotIntegrationTestStatuses{
		statuses: make(map[string]*IntegrationTestStatusDetail, 1),
		dirty:    false,
	}
	if jsondata != "" {
		err := json.Unmarshal([]byte(jsondata), &sits)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json: %w", err)
		}
	}
	return &sits, nil
}
