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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"

	"github.com/konflux-ci/integration-service/gitops"
	tekton "github.com/konflux-ci/integration-service/tekton"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("build pipeline", Ordered, func() {

	var (
		buildPipelineRun, buildPipelineRun2 *tektonv1.PipelineRun
		successfulTaskRun                   *tektonv1.TaskRun
	)
	const (
		SampleRepoLink           = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		SampleCommit             = "a2ba645d50e471d5f084b"
		SampleDigest             = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		SampleImageWithoutDigest = "quay.io/redhat-appstudio/sample-image"
	)

	BeforeAll(func() {
		successfulTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-pass",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-pass",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{Name: "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{Name: "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, successfulTaskRun)).Should(Succeed())

		now := time.Now()
		successfulTaskRun.Status = tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				Results: []tektonv1.TaskRunResult{
					{
						Name: "TEST_OUTPUT",
						Value: *tektonv1.NewStructuredValues(`{
											"result": "SUCCESS",
											"timestamp": "2024-05-22T06:42:21+00:00",
											"failures": 0,
											"successes": 10,
											"warnings": 0
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, successfulTaskRun)).Should(Succeed())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, successfulTaskRun)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	BeforeEach(func() {
		buildPipelineRun = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-build-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type":    "build",
					"pipelines.openshift.io/used-by":           "build-cloud",
					"pipelines.openshift.io/runtime":           "nodejs",
					"pipelines.openshift.io/strategy":          "s2i",
					"appstudio.openshift.io/component":         "component-sample",
					"build.appstudio.redhat.com/target_branch": "main",
					"pipelinesascode.tekton.dev/pull-request":  "1",
					"pipelinesascode.tekton.dev/event-type":    "pull_request",
				},
				Annotations: map[string]string{
					"appstudio.redhat.com/updateComponentOnSuccess":    "false",
					"pipelinesascode.tekton.dev/on-target-branch":      "[main,master]",
					"build.appstudio.openshift.io/repo":                "https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0",
					"foo":                                              "bar",
					"chains.tekton.dev/signed":                         "true",
					"pipelinesascode.tekton.dev/source-branch":         "sourceBranch",
					"pipelinesascode.tekton.dev/url-org":               "redhat",
					tektonconsts.PipelineRunComponentVersionAnnotation: "v1",
				},
				CreationTimestamp: metav1.NewTime(time.Now().Add(time.Hour * 1)),
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineRef: &tektonv1.PipelineRef{
					Name: "build-pipeline-pass",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{Name: "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{Name: "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
				Params: []tektonv1.Param{
					{
						Name: "output-image",
						Value: tektonv1.ParamValue{
							Type:      tektonv1.ParamTypeString,
							StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, buildPipelineRun)).Should(Succeed())

		buildPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				Results: []tektonv1.PipelineRunResult{
					{
						Name:  "IMAGE_DIGEST",
						Value: *tektonv1.NewStructuredValues(SampleDigest),
					},
					{
						Name:  "IMAGE_URL",
						Value: *tektonv1.NewStructuredValues(SampleImageWithoutDigest),
					},
					{
						Name:  "CHAINS-GIT_URL",
						Value: *tektonv1.NewStructuredValues(SampleRepoLink),
					},
					{
						Name:  "CHAINS-GIT_COMMIT",
						Value: *tektonv1.NewStructuredValues(SampleCommit),
					},
				},
				StartTime: &metav1.Time{Time: time.Now()},
			},
			Status: v1.Status{
				Conditions: v1.Conditions{
					apis.Condition{
						Reason: "Completed",
						Status: "True",
						Type:   apis.ConditionSucceeded,
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

		buildPipelineRun2 = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-build-sample",
				Namespace: "default",
				Labels: map[string]string{
					"appstudio.openshift.io/component": "component-sample",
				},
				CreationTimestamp: metav1.NewTime(time.Now()),
			},
		}
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, buildPipelineRun)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	Context("When a build pipelineRun exists", func() {
		It("can get PR group from build pipelineRun", func() {
			Expect(tekton.IsPLRCreatedByPACPushEvent(buildPipelineRun)).To(BeFalse())
			prGroup := tekton.GetPRGroupFromBuildPLR(buildPipelineRun)
			Expect(prGroup).To(Equal("sourceBranch"))
			Expect(tekton.GenerateSHA(prGroup)).NotTo(BeNil())
		})

		It("can get PR group from build pipelineRun is source branch is main", func() {
			buildPipelineRun.Annotations[tektonconsts.PipelineAsCodeSourceBranchAnnotation] = "main"
			prGroup := tekton.GetPRGroupFromBuildPLR(buildPipelineRun)
			Expect(prGroup).To(Equal("main-redhat"))
			Expect(tekton.GenerateSHA(prGroup)).NotTo(BeNil())
		})

		It("can get PR group from build pipelineRun is source branch has @ charactor", func() {
			buildPipelineRun.Annotations[tektonconsts.PipelineAsCodeSourceBranchAnnotation] = "myfeature@change1"
			prGroup := tekton.GetPRGroupFromBuildPLR(buildPipelineRun)
			Expect(prGroup).To(Equal("myfeature"))
			Expect(tekton.GenerateSHA(prGroup)).NotTo(BeNil())
		})

		It("can detect that the build pipelineRun originated from a merge queue and not consider it a push pipelineRun", func() {
			buildPipelineRun.Labels[tektonconsts.PipelineAsCodeEventTypeLabel] = "push"
			buildPipelineRun.Annotations[tektonconsts.PipelineAsCodeSourceBranchAnnotation] = "gh-readonly-queue/main/pr-2987-bda9b312bf224a6b5fb1e7ed6ae76dd9e6b1b75b"
			Expect(tekton.IsPLRCreatedByPACPushEvent(buildPipelineRun)).To(BeFalse())
			buildPipelineRun.Annotations[tektonconsts.PipelineAsCodeSourceBranchAnnotation] = "refs/heads/gh-readonly-queue/main/pr-7-54e7d2bfec0e0570915f5770c890407c714e6139"
			Expect(tekton.IsPLRCreatedByPACPushEvent(buildPipelineRun)).To(BeFalse())
		})

		It("can get the latest build pipelinerun for given component", func() {
			plrs := []tektonv1.PipelineRun{*buildPipelineRun, *buildPipelineRun2}
			Expect(tekton.IsLatestBuildPipelineRunInComponent(buildPipelineRun, &plrs)).To(BeTrue())
		})

		It("can mark build PLR as AddedToGlobalCandidateList", func() {
			addedToGlobalCandidateListStatus := gitops.AddedToGlobalCandidateListStatus{
				Result:          true,
				Reason:          gitops.Success,
				LastUpdatedTime: time.Now().Format(time.RFC3339),
			}

			annotationJson, err := json.Marshal(addedToGlobalCandidateListStatus)
			Expect(err).NotTo(HaveOccurred())

			err = tekton.MarkBuildPLRAsAddedToGlobalCandidateList(ctx, k8sClient, buildPipelineRun, string(annotationJson))
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Name:      buildPipelineRun.Name,
					Namespace: buildPipelineRun.Namespace,
				}, buildPipelineRun)
				return tekton.IsBuildPLRMarkedAsAddedToGlobalCandidateList(buildPipelineRun)
			}, time.Second*15).Should(BeTrue())
		})

		It("ensure err is returned when pipelinerun doesn't have Result for ", func() {
			// We don't need to update the underlying resource on the control plane,
			// so we create a copy and modify its status. This prevents update conflicts in other tests.
			buildPipelineRunNoSource := buildPipelineRun.DeepCopy()
			buildPipelineRunNoSource.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					ChildReferences: []tektonv1.ChildStatusReference{
						{
							Name:             successfulTaskRun.Name,
							PipelineTaskName: "task1",
						},
					},
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1.NewStructuredValues(SampleRepoLink),
						},
					},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Completed",
							Status: "True",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}

			componentSource, err := tekton.GetComponentSourceFromPipelineRun(buildPipelineRunNoSource)
			Expect(componentSource).To(BeNil())
			Expect(err).To(HaveOccurred())
		})

		It("ensures the Imagepullspec and ComponentSource from pipelinerun can be accessed", func() {
			imagePullSpec, err := tekton.GetImagePullSpecFromPipelineRun(buildPipelineRun)
			Expect(err).ToNot(HaveOccurred())
			Expect(imagePullSpec).NotTo(BeEmpty())

			componentSource, err := tekton.GetComponentSourceFromPipelineRun(buildPipelineRun)
			Expect(err).ToNot(HaveOccurred())
			Expect(componentSource).NotTo(BeNil())
		})

		It("ensures the component version can be accessed", func() {
			version, err := tekton.GetComponentVersionFromPipelineRun(buildPipelineRun)
			Expect(err).NotTo(HaveOccurred())
			Expect(version).NotTo(BeEmpty())
			Expect(version).To(Equal("v1"))
		})

		It("ensures error is returned when pipelinerun does not have component version annotation", func() {
			version, err := tekton.GetComponentVersionFromPipelineRun(buildPipelineRun2)
			Expect(err).To(HaveOccurred())
			Expect(version).To(BeEmpty())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("does not have '%s' annotation", tektonconsts.PipelineRunComponentVersionAnnotation)))
		})
	})

})
