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

package tekton_test

import (
	"testing"

	"github.com/konflux-ci/integration-service/tekton"
	"github.com/konflux-ci/integration-service/tekton/consts"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

func TestGetImagePullSpecFromPipelineRun(t *testing.T) {
	const digest = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
	const oldDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, tc := range []struct {
		image      string
		repository string
	}{
		{"quay.io/team/image", "quay.io/team/image"},
		{"quay.io/team/image:latest", "quay.io/team/image"},
		{"registry.example:5000/team/image", "registry.example:5000/team/image"},
		{"registry.example:5000/team/image:latest", "registry.example:5000/team/image"},
		{"172.18.0.5:5000/prototype/source", "172.18.0.5:5000/prototype/source"},
		{"registry.example:443/team/nested/image:build", "registry.example:443/team/nested/image"},
		{"[::1]:5000/team/image:latest", "[::1]:5000/team/image"},
		{"quay.io/team/image@" + oldDigest, "quay.io/team/image"},
		{"registry.example:5000/team/image:latest@" + oldDigest, "registry.example:5000/team/image"},
		{"ubuntu:latest", "index.docker.io/library/ubuntu"},
	} {
		t.Run(tc.image, func(t *testing.T) {
			pr := &tektonv1.PipelineRun{}
			pr.Status.Results = []tektonv1.PipelineRunResult{
				{Name: consts.PipelineRunImageUrlParamName, Value: *tektonv1.NewStructuredValues(tc.image)},
				{Name: consts.PipelineRunImageDigestParamName, Value: *tektonv1.NewStructuredValues(digest)},
			}
			got, err := tekton.GetImagePullSpecFromPipelineRun(pr)
			if err != nil {
				t.Fatal(err)
			}
			if want := tc.repository + "@" + digest; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestGetImagePullSpecFromPipelineRunErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		results []tektonv1.PipelineRunResult
	}{
		{"missing image", nil},
		{"missing digest", []tektonv1.PipelineRunResult{
			{Name: consts.PipelineRunImageUrlParamName, Value: *tektonv1.NewStructuredValues("quay.io/team/image")},
		}},
		{"invalid image", []tektonv1.PipelineRunResult{
			{Name: consts.PipelineRunImageUrlParamName, Value: *tektonv1.NewStructuredValues("invalid image")},
			{Name: consts.PipelineRunImageDigestParamName, Value: *tektonv1.NewStructuredValues("sha256:digest")},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr := &tektonv1.PipelineRun{}
			pr.Status.Results = tc.results
			got, err := tekton.GetImagePullSpecFromPipelineRun(pr)
			if err == nil || got != "" {
				t.Fatalf("expected an error and empty reference, got %q, %v", got, err)
			}
		})
	}
}
