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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ComponentGroupLabelPrefix contains the prefix applied to labels and annotations related to component groups.
	ComponentGroupLabelPrefix = "appstudio.openshift.io/component-group"

	// ParentSnapshotAnnotation contains the name of the parent snapshot that triggered this snapshot creation
	ParentSnapshotAnnotation = TestLabelPrefix + "/parent-snapshot"

	// OriginSnapshotAnnotation contains the name of the original snapshot that started the snapshot chain
	OriginSnapshotAnnotation = TestLabelPrefix + "/origin-snapshot"

	// MissingComponentVersionsAnnotation contains a list of ComponentVersions that cannot be found
	MissingComponentVersionsAnnotation = TestLabelPrefix + "/missing-componentversions"
)

// ComponentGroupSpec defines the desired state of ComponentGroup
type ComponentGroupSpec struct {
	// Components is a list of Components (name and branch) that belong to the ComponentGroup.
	// This is the source of truth for logical groupings of versioned Components.
	// +required
	Components []ComponentReference `json:"components"`

	// Dependents is a list of ComponentGroup names that are dependent on this ComponentGroup.
	// When a snapshot is created for this ComponentGroup, snapshots will also be created for all dependents.
	// +optional
	Dependents []string `json:"dependents,omitempty"`

	// TestGraph describes the desired order in which tests associated with the ComponentGroup should be executed.
	// If not specified, all tests will run in parallel.
	// The map key is the test scenario name, and the value is a list of parent test scenarios it depends on.
	// +optional
	TestGraph map[string][]TestGraphNode `json:"testGraph,omitempty"`

	// SnapshotCreator is an optional field that allows custom logic for Snapshot creation.
	// This field is reserved for future implementation and should not be used yet.
	// +optional
	SnapshotCreator *SnapshotCreatorSpec `json:"snapshotCreator,omitempty"`
}

// ComponentReference references a Component and its specific branch/version
type ComponentReference struct {
	// Name is the name of the Component
	// +kubebuilder:validation:Pattern=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// +required
	Name string `json:"name"`

	// ComponentBranch references the ComponentBranch for this Component.
	// The ComponentBranch CRD will be implemented by the build team as part of STONEBLD-3604.
	// For now, this contains the branch name and GCL (Global Candidate List) information.
	// +required
	ComponentBranch ComponentBranchReference `json:"componentBranch"`
}

// ComponentBranchReference contains information about a Component's branch and its latest promoted image
type ComponentBranchReference struct {
	// Name is the name of the ComponentBranch (typically the branch name like "main", "v1", etc.)
	// This will reference the ComponentBranch CRD once it's implemented.
	// +required
	Name string `json:"name"`

	// LastPromotedImage is the latest "good" container image for this ComponentBranch.
	// This is part of the Global Candidate List (GCL) tracking.
	// Example: quay.io/sampleorg/first-component@sha256:1b29...
	// +optional
	LastPromotedImage string `json:"lastPromotedImage,omitempty"`

	// LastPromotedCommit is the git commit SHA of the last promoted image.
	// Example: 6a7c81802e785aa869f82301afe61f4e9775772b
	// +optional
	LastPromotedCommit string `json:"lastPromotedCommit,omitempty"`

	// LastBuildTime is the timestamp when the last build was completed.
	// Format: RFC3339 (e.g., "2025-08-13T12:00:00Z")
	// +optional
	LastBuildTime *metav1.Time `json:"lastBuildTime,omitempty"`
}

// TestGraphNode represents a node in the test serialization graph
type TestGraphNode struct {
	// Name is the name of the IntegrationTestScenario
	// +required
	Name string `json:"name"`

	// OnFail defines how to behave if this IntegrationTestScenario fails.
	// Options: "run" (default) - continue running dependent tests, "skip" - skip dependent tests
	// +kubebuilder:validation:Enum=run;skip
	// +kubebuilder:default=run
	// +optional
	OnFail string `json:"onFail,omitempty"`
}

// SnapshotCreatorSpec defines custom logic for creating snapshots.
// This is reserved for future implementation and should not be used yet.
type SnapshotCreatorSpec struct {
	// TaskRef references a Tekton Task that will create the Snapshot CR.
	// This field is reserved for future use.
	// +optional
	TaskRef *TaskRef `json:"taskRef,omitempty"`
}

// TaskRef references a Tekton Task using a resolver
type TaskRef struct {
	// Resolver is the name of the resolver (e.g., "git", "bundle")
	// +required
	Resolver string `json:"resolver"`

	// Params contains the parameters used to identify the referenced Task
	// +required
	Params []ResolverParameter `json:"params"`
}

// ComponentGroupStatus defines the observed state of ComponentGroup
type ComponentGroupStatus struct {
	// Conditions is an array of the ComponentGroup's status conditions
	Conditions []metav1.Condition `json:"conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=cg
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:storageversion

// ComponentGroup is the Schema for the componentgroups API.
// ComponentGroup serves as the replacement for the Application CR in the new application/component model.
// It groups Components together for testing and releasing, supports test serialization,
// ComponentGroup dependencies, and tracks the Global Candidate List (GCL) for each Component.
type ComponentGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentGroupSpec   `json:"spec,omitempty"`
	Status ComponentGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentGroupList contains a list of ComponentGroups
type ComponentGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComponentGroup{}, &ComponentGroupList{})
}
