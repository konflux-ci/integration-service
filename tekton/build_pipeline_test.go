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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"
	"time"

	tekton "github.com/konflux-ci/integration-service/tekton"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("build pipeline", func() {

	var (
		buildPipelineRun *tektonv1.PipelineRun
	)

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
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
					"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
					"build.appstudio.openshift.io/repo":             "https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0",
					"foo":                                           "bar",
					"chains.tekton.dev/signed":                      "true",
					"pipelinesascode.tekton.dev/source-branch":      "sourceBranch",
					"pipelinesascode.tekton.dev/url-org":            "redhat",
				},
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
			buildPipelineRun.Annotations[tekton.PipelineAsCodeSourceBranchAnnotation] = "main"
			prGroup := tekton.GetPRGroupFromBuildPLR(buildPipelineRun)
			Expect(prGroup).To(Equal("main-redhat"))
			Expect(tekton.GenerateSHA(prGroup)).NotTo(BeNil())
		})

		It("can get PR group from build pipelineRun is source branch has @ charactor", func() {
			buildPipelineRun.Annotations[tekton.PipelineAsCodeSourceBranchAnnotation] = "myfeature@change1"
			prGroup := tekton.GetPRGroupFromBuildPLR(buildPipelineRun)
			Expect(prGroup).To(Equal("myfeature"))
			Expect(tekton.GenerateSHA(prGroup)).NotTo(BeNil())
		})
	})
})
