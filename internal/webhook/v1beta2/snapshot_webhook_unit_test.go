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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSnapshotCustomValidator_ValidateCreate(t *testing.T) {
	validator := &SnapshotCustomValidator{}
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
	validator := &SnapshotCustomValidator{}
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
		if err != nil && err.Error() != "spec.components: Invalid value: []v1alpha1.SnapshotComponent{v1alpha1.SnapshotComponent{Name:\"component-1\", ContainerImage:\"registry.io/image1:v2.0.0\", Source:v1alpha1.ComponentSource{ComponentSourceUnion:v1alpha1.ComponentSourceUnion{GitSource:(*v1alpha1.GitSource)(nil)}}}}: components field is immutable and cannot be modified after creation" {
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
	validator := &SnapshotCustomValidator{}
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

	t.Run("should allow snapshot deletion", func(t *testing.T) {
		_, err := validator.ValidateDelete(ctx, snapshot)
		if err != nil {
			t.Errorf("ValidateDelete should not error, got: %v", err)
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
