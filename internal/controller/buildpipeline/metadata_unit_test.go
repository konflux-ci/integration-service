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

package buildpipeline

import (
	"context"
	"strconv"
	"testing"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestApplySnapshotMetadata is a unit test for the shared applySnapshotMetadata function
func TestApplySnapshotMetadata(t *testing.T) {
	// Create test objects
	app := &applicationapiv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
	}

	comp := &applicationapiv1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-component",
			Namespace: "default",
		},
	}

	snapshot := &applicationapiv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
	}

	pipelineRun := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline-run",
			Namespace: "default",
		},
		Status: tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				StartTime:      &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}

	// Create adapter
	client := fake.NewClientBuilder().Build()
	adapter := NewAdapter(context.TODO(), pipelineRun, comp, app, helpers.IntegrationLogger{}, loader.NewMockLoader(), client)

	// Test the function
	adapter.applySnapshotMetadata(snapshot, pipelineRun)

	// Verify results
	if snapshot.Labels[gitops.BuildPipelineRunNameLabel] != pipelineRun.Name {
		t.Errorf("Expected BuildPipelineRunNameLabel to be %s, got %s", pipelineRun.Name, snapshot.Labels[gitops.BuildPipelineRunNameLabel])
	}

	if snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] == "" {
		t.Error("Expected BuildPipelineRunFinishTimeLabel to be set")
	}

	if snapshot.Annotations[gitops.BuildPipelineRunStartTime] == "" {
		t.Error("Expected BuildPipelineRunStartTime annotation to be set")
	}

	// Test with nil completion time
	pipelineRun.Status.CompletionTime = nil
	snapshot.Labels = make(map[string]string)

	beforeTime := time.Now().Unix()
	adapter.applySnapshotMetadata(snapshot, pipelineRun)
	afterTime := time.Now().Unix()

	finishTimeStr := snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel]
	finishTime, err := strconv.ParseInt(finishTimeStr, 10, 64)
	if err != nil {
		t.Errorf("Failed to parse finish time: %v", err)
	}

	if finishTime < beforeTime || finishTime > afterTime {
		t.Errorf("Finish time %d should be between %d and %d", finishTime, beforeTime, afterTime)
	}

	// Test with nil start time
	pipelineRun.Status.StartTime = nil
	snapshot.Annotations = make(map[string]string)

	adapter.applySnapshotMetadata(snapshot, pipelineRun)

	if _, exists := snapshot.Annotations[gitops.BuildPipelineRunStartTime]; exists {
		t.Error("Expected BuildPipelineRunStartTime annotation to NOT be set when start time is nil")
	}
}

// TestApplySnapshotMetadataInitialization tests that the function properly initializes nil maps
func TestApplySnapshotMetadataInitialization(t *testing.T) {
	app := &applicationapiv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
	}

	comp := &applicationapiv1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-component",
			Namespace: "default",
		},
	}

	snapshot := &applicationapiv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
			// Explicitly set to nil
			Labels:      nil,
			Annotations: nil,
		},
	}

	pipelineRun := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline-run",
			Namespace: "default",
		},
		Status: tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				StartTime:      &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
				CompletionTime: &metav1.Time{Time: time.Now()},
			},
		},
	}

	// Create adapter
	client := fake.NewClientBuilder().Build()
	adapter := NewAdapter(context.TODO(), pipelineRun, comp, app, helpers.IntegrationLogger{}, loader.NewMockLoader(), client)

	// Test the function
	adapter.applySnapshotMetadata(snapshot, pipelineRun)

	// Verify maps were initialized
	if snapshot.Labels == nil {
		t.Error("Expected Labels map to be initialized")
	}

	if snapshot.Annotations == nil {
		t.Error("Expected Annotations map to be initialized")
	}

	// Verify metadata was added
	if snapshot.Labels[gitops.BuildPipelineRunNameLabel] != pipelineRun.Name {
		t.Errorf("Expected BuildPipelineRunNameLabel to be %s, got %s", pipelineRun.Name, snapshot.Labels[gitops.BuildPipelineRunNameLabel])
	}
}
