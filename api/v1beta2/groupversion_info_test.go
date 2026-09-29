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

package v1beta2

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGroupVersions(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("adding API types to scheme: %v", err)
	}

	tests := []struct {
		name string
		obj  runtime.Object
		want schema.GroupVersionKind
	}{
		{
			name: "ComponentGroup",
			obj:  &ComponentGroup{},
			want: schema.GroupVersionKind{Group: "konflux-ci.dev", Version: "v1beta2", Kind: "ComponentGroup"},
		},
		{
			name: "NudgeConfig",
			obj:  &NudgeConfig{},
			want: schema.GroupVersionKind{Group: "konflux-ci.dev", Version: "v1beta2", Kind: "NudgeConfig"},
		},
		{
			name: "IntegrationTestScenario",
			obj:  &IntegrationTestScenario{},
			want: schema.GroupVersionKind{Group: "appstudio.redhat.com", Version: "v1beta2", Kind: "IntegrationTestScenario"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvks, _, err := scheme.ObjectKinds(tt.obj)
			if err != nil {
				t.Fatalf("getting object kinds: %v", err)
			}
			if len(gvks) != 1 || gvks[0] != tt.want {
				t.Fatalf("got GVKs %v, want [%s]", gvks, tt.want)
			}
		})
	}
}
