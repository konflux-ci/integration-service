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

package v1beta2

import (
	"context"
	"testing"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSnapshotCustomValidator_ValidateCreate(t *testing.T) {
	validator := &SnapshotCustomValidator{Client: k8sClient}
	ctx := context.Background()

	snapshot := &applicationapiv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
		Spec: applicationapiv1alpha1.SnapshotSpec{
			Application: "test-app",
			Components: []applicationapiv1alpha1.SnapshotComponent{
				{
					Name:           "component-1",
					ContainerImage: "registry.io/image1:v1.0.0",
				},
			},
		},
	}

	t.Run("should allow snapshot creation", func(t *testing.T) {
		_, err := validator.ValidateCreate(ctx, snapshot)
		if err != nil {
			t.Errorf("ValidateCreate should not error, got: %v", err)
		}
	})

	t.Run("should reject non-snapshot object", func(t *testing.T) {
		app := &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app"},
		}
		_, err := validator.ValidateCreate(ctx, app)
		if err == nil {
			t.Error("ValidateCreate should error for non-snapshot object")
		}
		if err != nil && err.Error() != "expected a Snapshot object but got *v1alpha1.Application" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("should reject Name longer than 63 characters", func(t *testing.T) {
		// Name with 64 characters (exceeds max of 63)
		longName := "this-is-a-very-long-override-snapshot-name-that-exceeds-the-max-"
		snapshotWithLongName := &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      longName,
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "test-app",
			},
		}

		_, err := validator.ValidateCreate(ctx, snapshotWithLongName)
		if err == nil {
			t.Error("ValidateCreate should error for Name longer than 63 characters")
		}
		if err != nil && err.Error() != "metadata.name: Invalid value: \"this-is-a-very-long-override-snapshot-name-that-exceeds-the-max-\": name is too long (64 characters); must be at most 63 characters" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("should allow Name with exactly 63 characters", func(t *testing.T) {
		// Name with exactly 63 characters (at the limit)
		maxName := "this-is-a-very-long-override-snapshot-name-that-is-at-max------"
		snapshotWithMaxName := &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      maxName,
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "test-app",
			},
		}

		_, err := validator.ValidateCreate(ctx, snapshotWithMaxName)
		if err != nil {
			t.Errorf("ValidateCreate should not error for Name with 63 characters, got: %v", err)
		}
	})
}

func TestSnapshotCustomValidator_ValidateUpdate(t *testing.T) {
	validator := &SnapshotCustomValidator{Client: k8sClient}
	ctx := context.Background()

	originalSnapshot := &applicationapiv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
		Spec: applicationapiv1alpha1.SnapshotSpec{
			Application: "test-app",
			Components: []applicationapiv1alpha1.SnapshotComponent{
				{
					Name:           "component-1",
					ContainerImage: "registry.io/image1:v1.0.0",
				},
			},
		},
	}

	t.Run("should allow update with identical components", func(t *testing.T) {
		updatedSnapshot := originalSnapshot.DeepCopy()
		updatedSnapshot.Spec.DisplayName = "Updated Name"

		_, err := validator.ValidateUpdate(ctx, originalSnapshot, updatedSnapshot)
		if err != nil {
			t.Errorf("ValidateUpdate should not error for identical components, got: %v", err)
		}
	})

	t.Run("should reject update with modified components", func(t *testing.T) {
		updatedSnapshot := originalSnapshot.DeepCopy()
		updatedSnapshot.Spec.Components[0].ContainerImage = "registry.io/image1:v2.0.0"

		_, err := validator.ValidateUpdate(ctx, originalSnapshot, updatedSnapshot)
		if err == nil {
			t.Error("ValidateUpdate should error for modified components")
		}
		if err != nil && err.Error() != "spec.components: Invalid value: [{\"name\":\"component-1\",\"containerImage\":\"registry.io/image1:v2.0.0\",\"source\":{}}]: components field is immutable and cannot be modified after creation" {

			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("should reject update with added components", func(t *testing.T) {
		updatedSnapshot := originalSnapshot.DeepCopy()
		updatedSnapshot.Spec.Components = append(updatedSnapshot.Spec.Components, applicationapiv1alpha1.SnapshotComponent{
			Name:           "component-2",
			ContainerImage: "registry.io/image2:v1.0.0",
		})

		_, err := validator.ValidateUpdate(ctx, originalSnapshot, updatedSnapshot)
		if err == nil {
			t.Error("ValidateUpdate should error for added components")
		}
	})

	t.Run("should reject update with removed components", func(t *testing.T) {
		multiComponentSnapshot := originalSnapshot.DeepCopy()
		multiComponentSnapshot.Spec.Components = append(multiComponentSnapshot.Spec.Components, applicationapiv1alpha1.SnapshotComponent{
			Name:           "component-2",
			ContainerImage: "registry.io/image2:v1.0.0",
		})

		updatedSnapshot := multiComponentSnapshot.DeepCopy()
		updatedSnapshot.Spec.Components = updatedSnapshot.Spec.Components[:1] // Remove last component

		_, err := validator.ValidateUpdate(ctx, multiComponentSnapshot, updatedSnapshot)
		if err == nil {
			t.Error("ValidateUpdate should error for removed components")
		}
	})

	t.Run("should reject non-snapshot oldObj", func(t *testing.T) {
		app := &applicationapiv1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "test-app"}}
		_, err := validator.ValidateUpdate(ctx, app, originalSnapshot)
		if err == nil {
			t.Error("ValidateUpdate should error for non-snapshot oldObj")
		}
		if err != nil && err.Error() != "expected a Snapshot object for oldObj but got *v1alpha1.Application" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("should reject non-snapshot newObj", func(t *testing.T) {
		app := &applicationapiv1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "test-app"}}
		_, err := validator.ValidateUpdate(ctx, originalSnapshot, app)
		if err == nil {
			t.Error("ValidateUpdate should error for non-snapshot newObj")
		}
		if err != nil && err.Error() != "expected a Snapshot object for newObj but got *v1alpha1.Application" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})
}

func TestSnapshotCustomValidator_ValidateDelete(t *testing.T) {
	ctx := context.Background()
	if k8sClient == nil {
		t.Log("k8sClient is nil, falling back to fake client")
		scheme := runtime.NewScheme()
		_ = applicationapiv1alpha1.AddToScheme(scheme)
		_ = tektonv1.AddToScheme(scheme)
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	}

	integrationPipelineRun := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pipelinerun-component-sample",
			Namespace: "default",
			Labels: map[string]string{
				"pipelines.appstudio.openshift.io/type": "test",
				"appstudio.openshift.io/application":    "test-app",
				"appstudio.openshift.io/snapshot":       "test-snapshot",
			},
			Annotations: map[string]string{
				"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
			},
			Finalizers: []string{
				"test.appstudio.openshift.io/pipelinerun",
			},
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineRef: &tektonv1.PipelineRef{
				Name: "test-pipeline",
			},
		},
		Status: tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             "test-taskrun",
						PipelineTaskName: "test-task",
					},
				},
			},
			Status: duckv1.Status{
				Conditions: []apis.Condition{
					{
						Type:   apis.ConditionSucceeded,
						Status: "True",
					},
				},
			},
		},
	}
	err := k8sClient.Create(ctx, integrationPipelineRun)
	if err != nil {
		t.Fatalf("Failed to create PipelineRun: %v", err)
	}

	validator := &SnapshotCustomValidator{Client: k8sClient}
	if validator.Client == nil {
		scheme := runtime.NewScheme()
		_ = applicationapiv1alpha1.AddToScheme(scheme)
		_ = tektonv1.AddToScheme(scheme)
		validator.Client = fake.NewClientBuilder().WithScheme(scheme).Build()
	}

	snapshot := &applicationapiv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
		Spec: applicationapiv1alpha1.SnapshotSpec{
			Application: "test-app",
			Components: []applicationapiv1alpha1.SnapshotComponent{
				{
					Name:           "component-1",
					ContainerImage: "registry.io/image1:v1.0.0",
				},
			},
		},
	}

	t.Run("should allow snapshot deletion", func(t *testing.T) {
		if validator == nil || validator.Client == nil {
			t.Fatal("Validator or Client is nil before calling ValidateDelete")
		}
		_, err := validator.ValidateDelete(ctx, snapshot)
		if err != nil {
			t.Errorf("ValidateDelete should not error, got: %v", err)
		}

		updatedPlr := &tektonv1.PipelineRun{}
		err = validator.Client.Get(ctx, types.NamespacedName{Namespace: integrationPipelineRun.Namespace, Name: integrationPipelineRun.Name}, updatedPlr)
		if err != nil {
			t.Errorf("Failed to get PipelineRun after Snapshot deletion: %v", err)
		}
		if len(updatedPlr.Finalizers) != 0 {
			t.Error("Finalizers should be removed from PipelineRun after Snapshot deletion", "finalizers", updatedPlr.Finalizers)
		}
	})

	t.Run("should reject non-snapshot object", func(t *testing.T) {
		app := &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app"},
		}
		_, err := validator.ValidateDelete(ctx, app)
		if err == nil {
			t.Error("ValidateDelete should error for non-snapshot object")
		}
		if err != nil && err.Error() != "expected a Snapshot object but got *v1alpha1.Application" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})
}

func TestSnapshotCustomDefaulter_Default(t *testing.T) {
	defaulter := &SnapshotCustomDefaulter{}
	ctx := context.Background()

	t.Run("should handle empty application name", func(t *testing.T) {
		snapshot := &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "",
			},
		}

		err := defaulter.Default(ctx, snapshot)

		if err != nil {
			t.Errorf("Defaulter should not error: %v", err)
		}

		if snapshot.Labels == nil {
			t.Error("Labels map should be initialised")
		}

		if snapshot.Labels["appstudio.openshift.io/application"] != snapshot.Spec.Application {
			t.Errorf("Expected label to match spec (%s), got: %v", snapshot.Spec.Application, snapshot.Labels["appstudio.openshift.io/application"])
		}

	})

	t.Run("should overwrite existing application name", func(t *testing.T) {
		snapshot := &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "default",
				Labels: map[string]string{
					"appstudio.openshift.io/application": "wrong-name",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "correct-name",
			},
		}

		err := defaulter.Default(ctx, snapshot)

		if err != nil {
			t.Errorf("Defaulter should not error: %v", err)
		}

		if snapshot.Labels == nil {
			t.Error("Labels map should be initialised")
		}

		if snapshot.Labels["appstudio.openshift.io/application"] != "correct-name" {
			t.Errorf("Expected label to be 'correct-name', got: %v",
				snapshot.Labels["appstudio.openshift.io/application"])
		}

	})

	t.Run("should add application label when application label map is nil", func(t *testing.T) {
		snapshot := &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "my-app",
			},
		}
		err := defaulter.Default(ctx, snapshot)

		if err != nil {
			t.Error("Defaulter should not error")
		}

		if snapshot.Labels == nil {
			t.Error("Labels map should be initialised")
		}

		if snapshot.Labels["appstudio.openshift.io/application"] != "my-app" {
			t.Errorf("Expected label to be 'my-app', got: %v",
				snapshot.Labels["appstudio.openshift.io/application"])
		}

	})

	t.Run("should add application label when labels exist but application label is missing", func(t *testing.T) {
		snapshot := &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "default",
				Labels: map[string]string{
					"test.label":    "test-value",
					"testing.label": "testing-value",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "my-app",
			},
		}

		err := defaulter.Default(ctx, snapshot)

		if err != nil {
			t.Errorf("Defaulter should not error: %v", err)
		}

		if snapshot.Labels == nil {
			t.Error("Labels map should be initialised")
		}

		if snapshot.Labels["appstudio.openshift.io/application"] != "my-app" {
			t.Errorf("Expected label to be 'my-app', got: %v",
				snapshot.Labels["appstudio.openshift.io/application"])
		}

		if snapshot.Labels["test.label"] != "test-value" {
			t.Error("Existing labels should be preserved")
		}

		if snapshot.Labels["testing.label"] != "testing-value" {
			t.Error("Existing labels should be preserved")
		}

	})

	t.Run("should reject non-snapshot object", func(t *testing.T) {
		app := &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app"},
		}

		err := defaulter.Default(ctx, app)
		if err == nil {
			t.Error("Defaulter should error for non-snapshot object")
		}
		if err != nil && err.Error() != "expected a Snapshot object but got *v1alpha1.Application" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})
}
