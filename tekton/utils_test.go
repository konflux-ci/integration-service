/*
Copyright 2022 Red Hat Inc.

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
	"fmt"

	"github.com/konflux-ci/integration-service/tekton"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"
)

var _ = Describe("Utils", func() {

	var pipelineRun *tektonv1.PipelineRun

	BeforeEach(func() {

		pipelineRun = &tektonv1.PipelineRun{
			ObjectMeta: v1.ObjectMeta{},
			Spec: tektonv1.PipelineRunSpec{
				Params: []tektonv1.Param{
					{
						Name: "output-image",
						Value: tektonv1.ParamValue{
							StringVal: "test-image",
						},
					},
				},
			},
			Status: tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "IMAGE_DIGEST",
							Value: *tektonv1.NewStructuredValues("image_digest_value"),
						},
						{
							Name:  "IMAGE_URL",
							Value: *tektonv1.NewStructuredValues("test-image"),
						},
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1.NewStructuredValues("https://github.com/devfile-samples/devfile-sample-java-springboot-basic"),
						},
						{
							Name:  "CHAINS-GIT_COMMIT",
							Value: *tektonv1.NewStructuredValues("a2ba645d50e471d5f084b"),
						},
					},
				},
			},
		}
	})

	It("can get output-image", func() {
		image, _ := tekton.GetOutputImage(pipelineRun)
		if image != "test-image" {
			Fail(fmt.Sprintf("Expected image is test-image, but got %s", image))
		}
		klog.Infoln("Got expected image")
	})

	It("can get output-image-digest", func() {
		image_digest, _ := tekton.GetOutputImageDigest(pipelineRun)
		if image_digest != "image_digest_value" {
			Fail(fmt.Sprintf("Expected image_digest is image_digest_value, but got %s", image_digest))
		}
		klog.Infoln("Got expected git_url")
	})

	It("can get git-url", func() {
		git_url, _ := tekton.GetComponentSourceGitUrl(pipelineRun)
		if git_url != "https://github.com/devfile-samples/devfile-sample-java-springboot-basic" {
			Fail(fmt.Sprintf("Expected git_url is https://github.com/devfile-samples/devfile-sample-java-springboot-basic, but got %s", git_url))
		}
		klog.Infoln("Got expected git_url")
	})

	It("can return err when can't find result for CHAINS-GIT_URL", func() {
		pipelineRun.Status.Results = []tektonv1.PipelineRunResult{}
		_, err := tekton.GetComponentSourceGitUrl(pipelineRun)
		Expect(err).To(HaveOccurred())
	})

	It("can get git-commit", func() {
		commit, _ := tekton.GetComponentSourceGitCommit(pipelineRun)
		if commit != "a2ba645d50e471d5f084b" {
			Fail(fmt.Sprintf("Expected commit is a2ba645d50e471d5f084b, but got %s", commit))
		}
		klog.Infoln("Got expected commit")
	})

	It("can return err when can't find result CHAINS-GIT_COMMIT", func() {
		pipelineRun.Status.Results = []tektonv1.PipelineRunResult{}
		_, err := tekton.GetComponentSourceGitCommit(pipelineRun)
		Expect(err).To(HaveOccurred())
	})

	It("can get snapshot name from SNAPSHOT result", func() {
		// Add SNAPSHOT result to the existing pipelineRun
		pipelineRun.Status.Results = append(pipelineRun.Status.Results, tektonv1.PipelineRunResult{
			Name:  "SNAPSHOT",
			Value: *tektonv1.NewStructuredValues("test-snapshot-name"),
		})

		snapshotName := tekton.GetSnapshotName(pipelineRun)
		Expect(snapshotName).To(Equal("test-snapshot-name"))
	})

	It("returns empty string when SNAPSHOT result is not found", func() {
		// pipelineRun should not have SNAPSHOT result by default
		snapshotName := tekton.GetSnapshotName(pipelineRun)
		Expect(snapshotName).To(Equal(""))
	})

	It("returns empty string when object is not a PipelineRun", func() {
		snapshotName := tekton.GetSnapshotName(nil)
		Expect(snapshotName).To(Equal(""))
	})
})
