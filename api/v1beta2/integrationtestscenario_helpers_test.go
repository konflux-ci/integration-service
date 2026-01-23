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
	"testing"
)

func TestHasApplication(t *testing.T) {
	tests := []struct {
		name     string
		its      *IntegrationTestScenario
		expected bool
	}{
		{
			name: "returns true when application is set",
			its: &IntegrationTestScenario{
				Spec: IntegrationTestScenarioSpec{
					Application: "my-app",
				},
			},
			expected: true,
		},
		{
			name: "returns false when application is empty",
			its: &IntegrationTestScenario{
				Spec: IntegrationTestScenarioSpec{},
			},
			expected: false,
		},
		{
			name: "returns false when only componentGroup is set",
			its: &IntegrationTestScenario{
				Spec: IntegrationTestScenarioSpec{
					ComponentGroup: "my-cg",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.its.HasApplication(); got != tt.expected {
				t.Errorf("HasApplication() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHasComponentGroup(t *testing.T) {
	tests := []struct {
		name     string
		its      *IntegrationTestScenario
		expected bool
	}{
		{
			name: "returns true when componentGroup is set",
			its: &IntegrationTestScenario{
				Spec: IntegrationTestScenarioSpec{
					ComponentGroup: "my-cg",
				},
			},
			expected: true,
		},
		{
			name: "returns false when componentGroup is empty",
			its: &IntegrationTestScenario{
				Spec: IntegrationTestScenarioSpec{},
			},
			expected: false,
		},
		{
			name: "returns false when only application is set",
			its: &IntegrationTestScenario{
				Spec: IntegrationTestScenarioSpec{
					Application: "my-app",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.its.HasComponentGroup(); got != tt.expected {
				t.Errorf("HasComponentGroup() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestOwnerName(t *testing.T) {
	tests := []struct {
		name     string
		its      *IntegrationTestScenario
		expected string
	}{
		{
			name: "returns application name when application is set",
			its: &IntegrationTestScenario{
				Spec: IntegrationTestScenarioSpec{
					Application: "my-app",
				},
			},
			expected: "my-app",
		},
		{
			name: "returns componentGroup name when componentGroup is set",
			its: &IntegrationTestScenario{
				Spec: IntegrationTestScenarioSpec{
					ComponentGroup: "my-cg",
				},
			},
			expected: "my-cg",
		},
		{
			name: "returns empty string when neither is set",
			its: &IntegrationTestScenario{
				Spec: IntegrationTestScenarioSpec{},
			},
			expected: "",
		},
		{
			name: "returns application name when both are set (application takes precedence)",
			its: &IntegrationTestScenario{
				Spec: IntegrationTestScenarioSpec{
					Application:    "my-app",
					ComponentGroup: "my-cg",
				},
			},
			expected: "my-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.its.OwnerName(); got != tt.expected {
				t.Errorf("OwnerName() = %v, want %v", got, tt.expected)
			}
		})
	}
}
