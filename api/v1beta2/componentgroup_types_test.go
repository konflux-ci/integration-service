/*
Copyright 2024 Red Hat Inc.

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
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponentGroupSpec(t *testing.T) {
	// Test creating a ComponentGroup with all fields
	cg := &ComponentGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "appstudio.redhat.com/v1beta2",
			Kind:       "ComponentGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cg",
			Namespace: "default",
		},
		Spec: ComponentGroupSpec{
			Components: []ComponentReference{
				{
					Name: "first-component",
					ComponentBranch: ComponentBranchReference{
						Name:               "main",
						LastPromotedImage:  "quay.io/sampleorg/first-component@sha256:1b29",
						LastPromotedCommit: "6a7c81802e785aa869f82301afe61f4e9775772b",
						LastBuildTime: &metav1.Time{
							Time: time.Date(2025, 8, 13, 12, 0, 0, 0, time.UTC),
						},
					},
				},
				{
					Name: "second-component",
					ComponentBranch: ComponentBranchReference{
						Name:               "v1",
						LastPromotedImage:  "quay.io/sampleorg/second-component@sha256:ae32",
						LastPromotedCommit: "1359836353b8e249f2fbceba47d82751d7dab902",
					},
				},
			},
			Dependents: []string{"child-cg-1", "child-cg-2"},
			TestGraph: map[string][]TestGraphNode{
				"verify": {
					{Name: "clamav-scan"},
					{Name: "dast-tests", OnFail: "skip"},
					{Name: "operator-scorecard"},
				},
				"e2e-test": {
					{Name: "clamav-scan", OnFail: "run"},
				},
			},
		},
		Status: ComponentGroupStatus{
			Conditions: []metav1.Condition{},
		},
	}

	// Verify the ComponentGroup fields
	if cg.Name != "test-cg" {
		t.Errorf("Expected name 'test-cg', got '%s'", cg.Name)
	}
	if cg.Namespace != "default" {
		t.Errorf("Expected namespace 'default', got '%s'", cg.Namespace)
	}
	if len(cg.Spec.Components) != 2 {
		t.Errorf("Expected 2 components, got %d", len(cg.Spec.Components))
	}
	if cg.Spec.Components[0].Name != "first-component" {
		t.Errorf("Expected first component name 'first-component', got '%s'", cg.Spec.Components[0].Name)
	}
	if cg.Spec.Components[0].ComponentBranch.Name != "main" {
		t.Errorf("Expected first component branch 'main', got '%s'", cg.Spec.Components[0].ComponentBranch.Name)
	}
	if cg.Spec.Components[0].ComponentBranch.LastPromotedImage != "quay.io/sampleorg/first-component@sha256:1b29" {
		t.Errorf("Expected LastPromotedImage 'quay.io/sampleorg/first-component@sha256:1b29', got '%s'", cg.Spec.Components[0].ComponentBranch.LastPromotedImage)
	}
	if cg.Spec.Components[0].ComponentBranch.LastBuildTime == nil {
		t.Error("Expected LastBuildTime to be set")
	}

	if cg.Spec.Components[1].Name != "second-component" {
		t.Errorf("Expected second component name 'second-component', got '%s'", cg.Spec.Components[1].Name)
	}
	if cg.Spec.Components[1].ComponentBranch.LastBuildTime != nil {
		t.Error("Expected LastBuildTime to be nil for second component")
	}

	if len(cg.Spec.Dependents) != 2 {
		t.Errorf("Expected 2 dependents, got %d", len(cg.Spec.Dependents))
	}

	if len(cg.Spec.TestGraph) != 2 {
		t.Errorf("Expected 2 test graph entries, got %d", len(cg.Spec.TestGraph))
	}
	if _, ok := cg.Spec.TestGraph["verify"]; !ok {
		t.Error("Expected 'verify' in TestGraph")
	}
	if _, ok := cg.Spec.TestGraph["e2e-test"]; !ok {
		t.Error("Expected 'e2e-test' in TestGraph")
	}
	if len(cg.Spec.TestGraph["verify"]) != 3 {
		t.Errorf("Expected 3 nodes in 'verify' graph, got %d", len(cg.Spec.TestGraph["verify"]))
	}
	if cg.Spec.TestGraph["verify"][1].OnFail != "skip" {
		t.Errorf("Expected OnFail 'skip', got '%s'", cg.Spec.TestGraph["verify"][1].OnFail)
	}
}

func TestComponentGroupMinimalSpec(t *testing.T) {
	// Test creating a minimal ComponentGroup
	cg := &ComponentGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal-cg",
			Namespace: "default",
		},
		Spec: ComponentGroupSpec{
			Components: []ComponentReference{
				{
					Name: "test-component",
					ComponentBranch: ComponentBranchReference{
						Name: "main",
					},
				},
			},
		},
	}

	if cg.Name != "minimal-cg" {
		t.Errorf("Expected name 'minimal-cg', got '%s'", cg.Name)
	}
	if len(cg.Spec.Components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(cg.Spec.Components))
	}
	if cg.Spec.Components[0].Name != "test-component" {
		t.Errorf("Expected component name 'test-component', got '%s'", cg.Spec.Components[0].Name)
	}
	if cg.Spec.Components[0].ComponentBranch.Name != "main" {
		t.Errorf("Expected branch name 'main', got '%s'", cg.Spec.Components[0].ComponentBranch.Name)
	}
	if cg.Spec.Components[0].ComponentBranch.LastPromotedImage != "" {
		t.Errorf("Expected empty LastPromotedImage, got '%s'", cg.Spec.Components[0].ComponentBranch.LastPromotedImage)
	}
	if len(cg.Spec.Dependents) != 0 {
		t.Errorf("Expected 0 dependents, got %d", len(cg.Spec.Dependents))
	}
	if cg.Spec.TestGraph != nil {
		t.Error("Expected nil TestGraph")
	}
}

func TestComponentGroupWithSnapshotCreator(t *testing.T) {
	cg := &ComponentGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "snapshot-creator-cg",
			Namespace: "default",
		},
		Spec: ComponentGroupSpec{
			Components: []ComponentReference{
				{
					Name: "test-component",
					ComponentBranch: ComponentBranchReference{
						Name: "main",
					},
				},
			},
			SnapshotCreator: &SnapshotCreatorSpec{
				TaskRef: &TaskRef{
					Resolver: "git",
					Params: []ResolverParameter{
						{Name: "url", Value: "https://github.com/konflux-ci/tekton-integration-catalog"},
						{Name: "revision", Value: "main"},
						{Name: "pathInRepo", Value: "tasks/snapshotCreation/singleComponent.yaml"},
					},
				},
			},
		},
	}

	if cg.Spec.SnapshotCreator == nil {
		t.Fatal("Expected SnapshotCreator to be set")
	}
	if cg.Spec.SnapshotCreator.TaskRef == nil {
		t.Fatal("Expected TaskRef to be set")
	}
	if cg.Spec.SnapshotCreator.TaskRef.Resolver != "git" {
		t.Errorf("Expected resolver 'git', got '%s'", cg.Spec.SnapshotCreator.TaskRef.Resolver)
	}
	if len(cg.Spec.SnapshotCreator.TaskRef.Params) != 3 {
		t.Errorf("Expected 3 params, got %d", len(cg.Spec.SnapshotCreator.TaskRef.Params))
	}
}

func TestComponentGroupDeepCopy(t *testing.T) {
	original := &ComponentGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "original-cg",
			Namespace: "default",
		},
		Spec: ComponentGroupSpec{
			Components: []ComponentReference{
				{
					Name: "test-component",
					ComponentBranch: ComponentBranchReference{
						Name:              "main",
						LastPromotedImage: "quay.io/test/image@sha256:abc",
					},
				},
			},
			Dependents: []string{"dependent-1"},
		},
	}

	// Test DeepCopy
	copied := original.DeepCopy()
	if copied.Name != original.Name {
		t.Errorf("DeepCopy: Expected name '%s', got '%s'", original.Name, copied.Name)
	}
	if copied.Spec.Components[0].Name != original.Spec.Components[0].Name {
		t.Errorf("DeepCopy: Expected component name '%s', got '%s'", original.Spec.Components[0].Name, copied.Spec.Components[0].Name)
	}

	// Modify the copy and verify original is unchanged
	copied.Name = "modified-cg"
	copied.Spec.Components[0].Name = "modified-component"
	if original.Name != "original-cg" {
		t.Errorf("Original name was modified: got '%s'", original.Name)
	}
	if original.Spec.Components[0].Name != "test-component" {
		t.Errorf("Original component name was modified: got '%s'", original.Spec.Components[0].Name)
	}
}

func TestComponentGroupListDeepCopy(t *testing.T) {
	list := &ComponentGroupList{
		Items: []ComponentGroup{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "cg-1"},
				Spec: ComponentGroupSpec{
					Components: []ComponentReference{
						{Name: "comp-1", ComponentBranch: ComponentBranchReference{Name: "main"}},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "cg-2"},
				Spec: ComponentGroupSpec{
					Components: []ComponentReference{
						{Name: "comp-2", ComponentBranch: ComponentBranchReference{Name: "v1"}},
					},
				},
			},
		},
	}

	copied := list.DeepCopy()
	if len(copied.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(copied.Items))
	}
	if copied.Items[0].Name != "cg-1" {
		t.Errorf("Expected first item name 'cg-1', got '%s'", copied.Items[0].Name)
	}
	if copied.Items[1].Name != "cg-2" {
		t.Errorf("Expected second item name 'cg-2', got '%s'", copied.Items[1].Name)
	}

	// Modify the copy
	copied.Items[0].Name = "modified"
	if list.Items[0].Name != "cg-1" {
		t.Errorf("Original list was modified: got '%s'", list.Items[0].Name)
	}
}

func TestTestGraphNodeOnFailDefault(t *testing.T) {
	node := TestGraphNode{
		Name: "test-scenario",
	}
	// OnFail should be empty string when not set (default in CRD is "run")
	if node.OnFail != "" {
		t.Errorf("Expected empty OnFail, got '%s'", node.OnFail)
	}

	nodeWithOnFail := TestGraphNode{
		Name:   "test-scenario",
		OnFail: "skip",
	}
	if nodeWithOnFail.OnFail != "skip" {
		t.Errorf("Expected OnFail 'skip', got '%s'", nodeWithOnFail.OnFail)
	}
}

func TestComponentGroupConstants(t *testing.T) {
	// Verify the constants are correctly defined
	if ComponentGroupLabelPrefix != "appstudio.openshift.io/component-group" {
		t.Errorf("Expected ComponentGroupLabelPrefix 'appstudio.openshift.io/component-group', got '%s'", ComponentGroupLabelPrefix)
	}
	if ParentSnapshotAnnotation != "test.appstudio.openshift.io/parent-snapshot" {
		t.Errorf("Expected ParentSnapshotAnnotation 'test.appstudio.openshift.io/parent-snapshot', got '%s'", ParentSnapshotAnnotation)
	}
	if OriginSnapshotAnnotation != "test.appstudio.openshift.io/origin-snapshot" {
		t.Errorf("Expected OriginSnapshotAnnotation 'test.appstudio.openshift.io/origin-snapshot', got '%s'", OriginSnapshotAnnotation)
	}
	if MissingComponentVersionsAnnotation != "test.appstudio.openshift.io/missing-componentversions" {
		t.Errorf("Expected MissingComponentVersionsAnnotation 'test.appstudio.openshift.io/missing-componentversions', got '%s'", MissingComponentVersionsAnnotation)
	}
}

func TestComponentGroupSpecDeepCopy(t *testing.T) {
	original := ComponentGroupSpec{
		Components: []ComponentReference{
			{Name: "comp-1", ComponentBranch: ComponentBranchReference{Name: "main"}},
		},
		Dependents: []string{"dep-1"},
		TestGraph: map[string][]TestGraphNode{
			"test-1": {{Name: "node-1", OnFail: "skip"}},
		},
	}

	copied := original.DeepCopy()

	// Verify deep copy worked
	if !reflect.DeepEqual(original, *copied) {
		t.Error("DeepCopy did not produce equal copy")
	}

	// Modify copy and verify original unchanged
	copied.Components[0].Name = "modified"
	if original.Components[0].Name != "comp-1" {
		t.Error("Original was modified when copy was changed")
	}
}

func TestComponentGroupStatusDeepCopy(t *testing.T) {
	original := ComponentGroupStatus{
		Conditions: []metav1.Condition{
			{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Reason:  "AllTestsPassed",
				Message: "All tests passed",
			},
		},
	}

	copied := original.DeepCopy()

	if len(copied.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %d", len(copied.Conditions))
	}
	if copied.Conditions[0].Type != "Ready" {
		t.Errorf("Expected condition type 'Ready', got '%s'", copied.Conditions[0].Type)
	}

	// Modify copy and verify original unchanged
	copied.Conditions[0].Type = "NotReady"
	if original.Conditions[0].Type != "Ready" {
		t.Error("Original was modified when copy was changed")
	}
}
