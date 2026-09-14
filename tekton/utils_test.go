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

	"github.com/konflux-ci/integration-service/pkg/keys"
	"github.com/konflux-ci/integration-service/tekton"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	Context("GetShouldRelease", func() {
		It("returns true when PipelineRun is nil", func() {
			Expect(tekton.GetShouldRelease(nil)).To(BeTrue())
		})

		It("returns true when SHOULD_RELEASE result is not present", func() {
			Expect(tekton.GetShouldRelease(pipelineRun)).To(BeTrue())
		})

		It("returns true when SHOULD_RELEASE is set to 'true'", func() {
			pipelineRun.Status.Results = append(pipelineRun.Status.Results, tektonv1.PipelineRunResult{
				Name:  "SHOULD_RELEASE",
				Value: *tektonv1.NewStructuredValues("true"),
			})
			Expect(tekton.GetShouldRelease(pipelineRun)).To(BeTrue())
		})

		It("returns true when SHOULD_RELEASE is empty", func() {
			pipelineRun.Status.Results = append(pipelineRun.Status.Results, tektonv1.PipelineRunResult{
				Name:  "SHOULD_RELEASE",
				Value: *tektonv1.NewStructuredValues(""),
			})
			Expect(tekton.GetShouldRelease(pipelineRun)).To(BeTrue())
		})

		It("returns false when SHOULD_RELEASE is set to 'false'", func() {
			pipelineRun.Status.Results = append(pipelineRun.Status.Results, tektonv1.PipelineRunResult{
				Name:  "SHOULD_RELEASE",
				Value: *tektonv1.NewStructuredValues("false"),
			})
			Expect(tekton.GetShouldRelease(pipelineRun)).To(BeFalse())
		})

	})

	When("pipeline type labels are evaluated", func() {
		plr := func(labels map[string]string) *tektonv1.PipelineRun {
			return &tektonv1.PipelineRun{ObjectMeta: v1.ObjectMeta{Labels: labels}}
		}

		DescribeTable("should detect build PipelineRuns from old or new type keys",
			func(labels map[string]string, want bool) {
				Expect(tekton.IsBuildPipelineRun(plr(labels))).To(Equal(want))
			},
			Entry("old type=build", map[string]string{keys.PipelineType.Old: "build"}, true),
			Entry("new type=build", map[string]string{keys.PipelineType.New: "build"}, true),
			Entry("both prefers new build", map[string]string{
				keys.PipelineType.Old: "test",
				keys.PipelineType.New: "build",
			}, true),
			Entry("old type=test", map[string]string{keys.PipelineType.Old: "test"}, false),
			Entry("new type=test", map[string]string{keys.PipelineType.New: "test"}, false),
			Entry("missing type", map[string]string{"unrelated": "x"}, false),
		)

		It("should return false for IsBuildPipelineRun on a non-PipelineRun", func() {
			Expect(tekton.IsBuildPipelineRun(&v1.PartialObjectMetadata{})).To(BeFalse())
		})

		DescribeTable("should detect integration PipelineRuns from old or new type keys",
			func(labels map[string]string, want bool) {
				Expect(tekton.IsIntegrationPipelineRun(plr(labels))).To(Equal(want))
			},
			Entry("old type=test", map[string]string{keys.PipelineType.Old: "test"}, true),
			Entry("new type=test", map[string]string{keys.PipelineType.New: "test"}, true),
			Entry("both prefers new test", map[string]string{
				keys.PipelineType.Old: "build",
				keys.PipelineType.New: "test",
			}, true),
			Entry("old type=build", map[string]string{keys.PipelineType.Old: "build"}, false),
			Entry("new type=build", map[string]string{keys.PipelineType.New: "build"}, false),
		)

		DescribeTable("should resolve pipeline type preferring the new key",
			func(labels map[string]string, want string, wantErr bool) {
				got, err := tekton.GetTypeFromPipelineRun(plr(labels))
				if wantErr {
					Expect(err).To(HaveOccurred())
					return
				}
				Expect(err).NotTo(HaveOccurred())
				Expect(got).To(Equal(want))
			},
			Entry("old type", map[string]string{keys.PipelineType.Old: "build"}, "build", false),
			Entry("new type", map[string]string{keys.PipelineType.New: "test"}, "test", false),
			Entry("both prefers new", map[string]string{
				keys.PipelineType.Old: "build",
				keys.PipelineType.New: "test",
			}, "test", false),
			Entry("missing", map[string]string{}, "", true),
		)

		DescribeTable("should detect new-model build PipelineRuns",
			func(object client.Object, want bool) {
				Expect(tekton.IsNewModelBuildPipelineRun(object)).To(Equal(want))
			},
			Entry("new type=build", plr(map[string]string{
				keys.PipelineType.New: "build",
			}), true),
			Entry("new component label", plr(map[string]string{
				keys.PipelineType.Old:   "build",
				keys.BuildComponent.New: "component-sample",
			}), true),
			Entry("old ComponentGroup path (no application label)", plr(map[string]string{
				keys.PipelineType.Old:   "build",
				keys.BuildComponent.Old: "component-sample",
			}), false),
			Entry("new type=test is not a build", plr(map[string]string{
				keys.PipelineType.New: "test",
			}), false),
		)
	})
})
