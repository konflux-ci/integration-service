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

package helpers_test

import (
	"bytes"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/tonglil/buflogr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"
)

var _ = Describe("Pipeline Adapter", Ordered, func() {
	const (
		applicationName = "application-sample"
		SampleRepoLink  = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
	)

	var (
		//two integration pipeline for integrationTestScenario
		integrationPipelineRun       *tektonv1.PipelineRun
		integrationPipelineRunFailed *tektonv1.PipelineRun
		buildPipelineRun             *tektonv1.PipelineRun
		successfulTaskRun            *tektonv1.TaskRun
		failedTaskRun                *tektonv1.TaskRun
		warningTaskRun               *tektonv1.TaskRun
		skippedTaskRun               *tektonv1.TaskRun
		emptyTaskRun                 *tektonv1.TaskRun
		malformedTaskRun             *tektonv1.TaskRun
		brokenJSONTaskRun            *tektonv1.TaskRun
		now                          time.Time
		hasComp                      *applicationapiv1alpha1.Component
		hasApp                       *applicationapiv1alpha1.Application
		hasSnapshot                  *applicationapiv1alpha1.Snapshot
		integrationTestScenario      *v1beta1.IntegrationTestScenario
		sample_image                 string
	)

	BeforeAll(func() {
		now = time.Now().Truncate(time.Second) // saved resources doesn't have subsecond values in timestamps

		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: applicationName,
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName: "component-sample",
				Application:   applicationName,
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: SampleRepoLink,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())

		integrationTestScenario = &v1beta1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta1.IntegrationTestScenarioSpec{
				Application: "application-sample",
				ResolverRef: v1beta1.ResolverRef{
					Resolver: "git",
					Params: []v1beta1.ResolverParameter{
						{
							Name:  "url",
							Value: "https://github.com/redhat-appstudio/integration-examples.git",
						},
						{
							Name:  "revision",
							Value: "main",
						},
						{
							Name:  "pathInRepo",
							Value: "pipelineruns/integration_pipelinerun_pass.yaml",
						},
					},
				},
				Environment: v1beta1.TestEnvironment{
					Name: "envname",
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())

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

		successfulTaskRun.Status = tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				Results: []tektonv1.TaskRunResult{
					{
						Name: "TEST_OUTPUT",
						Value: *tektonv1.NewStructuredValues(`{
											"result": "SUCCESS",
											"timestamp": "1665405318",
											"failures": 0,
											"successes": 10,
											"warnings": 0
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, successfulTaskRun)).Should(Succeed())

		failedTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-fail",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-fail",
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

		Expect(k8sClient.Create(ctx, failedTaskRun)).Should(Succeed())

		failedTaskRun.Status = tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				Results: []tektonv1.TaskRunResult{
					{
						Name: "TEST_OUTPUT",
						Value: *tektonv1.NewStructuredValues(`{
											"result": "FAILURE",
											"timestamp": "1665405317",
											"failures": 1,
											"successes": 0,
											"warnings": 0
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, failedTaskRun)).Should(Succeed())

		skippedTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-skip",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-skip",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, skippedTaskRun)).Should(Succeed())

		skippedTaskRun.Status = tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now.Add(5 * time.Minute)},
				CompletionTime: &metav1.Time{Time: now.Add(10 * time.Minute)},
				Results: []tektonv1.TaskRunResult{
					{
						Name: "TEST_OUTPUT",
						Value: *tektonv1.NewStructuredValues(`{
											"result": "SKIPPED",
											"timestamp": "1665405318",
											"failures": 0,
											"successes": 0,
											"warnings": 0
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, skippedTaskRun)).Should(Succeed())

		warningTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-warning",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-warning",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, warningTaskRun)).Should(Succeed())

		warningTaskRun.Status = tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now.Add(5 * time.Minute)},
				CompletionTime: &metav1.Time{Time: now.Add(10 * time.Minute)},
				Results: []tektonv1.TaskRunResult{
					{
						Name: "TEST_OUTPUT",
						Value: *tektonv1.NewStructuredValues(`{
							"result": "WARNING",
							"timestamp": "1665405320",
							"failures": 0,
							"successes": 0,
							"warnings": 1
						}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, warningTaskRun)).Should(Succeed())

		emptyTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-empty",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-empty",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, emptyTaskRun)).Should(Succeed())

		emptyTaskRun.Status = tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{},
		}
		Expect(k8sClient.Status().Update(ctx, emptyTaskRun)).Should(Succeed())

		malformedTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-malformed",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-malformed",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, malformedTaskRun)).Should(Succeed())

		malformedTaskRun.Status = tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				Results: []tektonv1.TaskRunResult{
					{
						Name:  "TEST_OUTPUT",
						Value: *tektonv1.NewStructuredValues("invalid json"),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, malformedTaskRun)).Should(Succeed())

		brokenJSONTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-broken",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-broken",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, brokenJSONTaskRun)).Should(Succeed())

		brokenJSONTaskRun.Status = tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				Results: []tektonv1.TaskRunResult{
					{
						Name: "TEST_OUTPUT",
						Value: *tektonv1.NewStructuredValues(`{
											"success":false,
											"errors":[{"code":6007,"message":"Malformed JSON in request body"}],
											"messages":[],
											"result":null,}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, brokenJSONTaskRun)).Should(Succeed())

	})

	BeforeEach(func() {
		sample_image = "quay.io/redhat-appstudio/sample-image"

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      "component",
					gitops.SnapshotComponentLabel: "component-sample",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           "component-sample",
						ContainerImage: sample_image,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		integrationPipelineRun = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-component-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pac.test.appstudio.openshift.io/url-org":         "redhat-appstudio",
					"pac.test.appstudio.openshift.io/original-prname": "build-service-on-push",
					"pac.test.appstudio.openshift.io/url-repository":  "build-service",
					"pac.test.appstudio.openshift.io/repository":      "build-service-pac",
					"pipelines.appstudio.openshift.io/type":           "test",
					"appstudio.openshift.io/snapshot":                 hasSnapshot.Name,
					"test.appstudio.openshift.io/scenario":            integrationTestScenario.Name,
				},
				Annotations: map[string]string{
					"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
				},
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineRef: &tektonv1.PipelineRef{
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/kpavic/test-bundle:component-pipeline-pass"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "component-pipeline-pass"},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationPipelineRun)).Should(Succeed())

		integrationPipelineRunFailed = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-component-sample-failed",
				Namespace: "default",
				Labels: map[string]string{
					"pac.test.appstudio.openshift.io/url-org":         "redhat-appstudio",
					"pac.test.appstudio.openshift.io/original-prname": "build-service-on-push",
					"pac.test.appstudio.openshift.io/url-repository":  "build-service",
					"pac.test.appstudio.openshift.io/repository":      "build-service-pac",
					"pipelines.appstudio.openshift.io/type":           "test",
					"appstudio.openshift.io/snapshot":                 hasSnapshot.Name,
					"test.appstudio.openshift.io/scenario":            integrationTestScenario.Name,
				},
				Annotations: map[string]string{
					"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
				},
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineRef: &tektonv1.PipelineRef{
					Name: "component-pipeline-pass",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationPipelineRunFailed)).Should(Succeed())

		buildPipelineRun = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "build-pipelinerun-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type": "build",
					"pipelines.openshift.io/used-by":        "build-cloud",
					"pipelines.openshift.io/runtime":        "nodejs",
					"pipelines.openshift.io/strategy":       "s2i",
					"appstudio.openshift.io/component":      "component-sample",
					"appstudio.openshift.io/application":    applicationName,
				},
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineRef: &tektonv1.PipelineRef{
					Name: "build-pipeline-pass",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{
								Name:  "name",
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
							StringVal: "quay.io/redhat-appstudio/sample-image",
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
						Value: *tektonv1.NewStructuredValues("image_digest_value"),
					},
					{
						Name:  "IMAGE_URL",
						Value: *tektonv1.NewStructuredValues(sample_image),
					},
					{
						Name:  "CHAINS-GIT_URL",
						Value: *tektonv1.NewStructuredValues("git_url_value"),
					},
					{
						Name:  "CHAINS-GIT_COMMIT",
						Value: *tektonv1.NewStructuredValues("git_commit_value"),
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
		Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      buildPipelineRun.Name,
				Namespace: "default",
			}, buildPipelineRun)
			if err != nil {
				return err
			}
			if !helpers.HasPipelineRunSucceeded(buildPipelineRun) {
				return fmt.Errorf("Pipeline is not marked as succeeded yet")
			}
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasSnapshot.Name,
				Namespace: "default",
			}, hasSnapshot)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())
	})
	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationPipelineRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, buildPipelineRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationPipelineRunFailed)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, successfulTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, failedTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, skippedTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, emptyTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, malformedTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, brokenJSONTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

	})

	It("can create an accurate Integration TaskRun from the given TaskRun status", func() {
		integrationTaskRun := helpers.NewTaskRunFromTektonTaskRun("task-success", &successfulTaskRun.Status)
		Expect(integrationTaskRun).NotTo(BeNil())
		Expect(integrationTaskRun.GetPipelineTaskName()).To(Equal("task-success"))
		Expect(integrationTaskRun.GetStartTime()).To(Equal(now))
		Expect(integrationTaskRun.GetDuration().Minutes()).To(Equal(5.0))

		integrationTaskRun = helpers.NewTaskRunFromTektonTaskRun("task-instant", &emptyTaskRun.Status)
		Expect(integrationTaskRun).NotTo(BeNil())
		Expect(integrationTaskRun.GetPipelineTaskName()).To(Equal("task-instant"))
		Expect(integrationTaskRun.GetDuration().Minutes()).To(Equal(0.0))
		Expect(integrationTaskRun.GetTestResult()).To(BeNil())
	})

	It("ensures multiple task pipelinerun outcome when AppStudio Tests succeeded", func() {
		integrationPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             successfulTaskRun.Name,
						PipelineTaskName: "pipeline1-task1",
					},
					{
						Name:             skippedTaskRun.Name,
						PipelineTaskName: "pipeline1-task2",
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
		Expect(k8sClient.Status().Update(ctx, integrationPipelineRun)).Should(Succeed())

		pipelineRunOutcome, err := helpers.GetIntegrationPipelineRunOutcome(k8sClient, ctx, integrationPipelineRun)
		Expect(err).To(BeNil())
		Expect(pipelineRunOutcome.HasPipelineRunPassedTesting()).To(BeTrue())
		Expect(pipelineRunOutcome.HasPipelineRunValidTestOutputs()).To(BeTrue())
		Expect(pipelineRunOutcome.GetValidationErrorsList()).Should(BeEmpty())

		err = gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(err).To(Succeed())
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
	})

	It("ensures multiple task pipelinerun outcome when AppStudio Tests warned", func() {
		integrationPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             successfulTaskRun.Name,
						PipelineTaskName: "pipeline1-task1",
					},
					{
						Name:             warningTaskRun.Name,
						PipelineTaskName: "pipeline1-task2",
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
		Expect(k8sClient.Status().Update(ctx, integrationPipelineRun)).Should(Succeed())

		pipelineRunOutcome, err := helpers.GetIntegrationPipelineRunOutcome(k8sClient, ctx, integrationPipelineRun)
		Expect(err).To(BeNil())
		Expect(pipelineRunOutcome.HasPipelineRunPassedTesting()).To(BeTrue())
		Expect(pipelineRunOutcome.HasPipelineRunValidTestOutputs()).To(BeTrue())
		Expect(pipelineRunOutcome.GetValidationErrorsList()).Should(BeEmpty())

		err = gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(err).To(Succeed())
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
	})

	It("ensures multiple task pipelinerun outcome when AppStudio Tests failed", func() {
		integrationPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{},
		}
		integrationPipelineRun.Status.SetCondition(&apis.Condition{
			Type:   apis.ConditionSucceeded,
			Status: "False",
			Reason: "NotFindPipeline",
		})
		Expect(k8sClient.Status().Update(ctx, integrationPipelineRun)).Should(Succeed())

		pipelineRunOutcome, err := helpers.GetIntegrationPipelineRunOutcome(k8sClient, ctx, integrationPipelineRun)
		Expect(err).To(BeNil())
		Expect(pipelineRunOutcome.HasPipelineRunPassedTesting()).To(BeFalse())
		Expect(pipelineRunOutcome.HasPipelineRunValidTestOutputs()).To(BeTrue())
		Expect(pipelineRunOutcome.GetValidationErrorsList()).Should(BeEmpty())

		err = gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
		Expect(err).To(Succeed())
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeFalse())
	})

	It("no error from pipelinrun when AppStudio Tests failed", func() {
		integrationPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             failedTaskRun.Name,
						PipelineTaskName: "pipeline1-task1",
					},
					{
						Name:             skippedTaskRun.Name,
						PipelineTaskName: "pipeline1-task2",
					},
				},
			},
			Status: v1.Status{
				Conditions: v1.Conditions{
					apis.Condition{
						Reason: "Failed",
						Status: "False",
						Type:   apis.ConditionSucceeded,
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, integrationPipelineRun)).Should(Succeed())

		pipelineRunOutcome, err := helpers.GetIntegrationPipelineRunOutcome(k8sClient, ctx, integrationPipelineRun)
		Expect(err).To(BeNil())
		Expect(pipelineRunOutcome.HasPipelineRunPassedTesting()).To(BeFalse())
		Expect(pipelineRunOutcome.HasPipelineRunValidTestOutputs()).To(BeTrue())
		Expect(pipelineRunOutcome.GetValidationErrorsList()).Should(BeEmpty())

		err = gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
		Expect(err).To(Succeed())
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeFalse())
	})

	It("no error from pipelinrun when AppStudio Tests failed but pipeline passed", func() {
		var (
			buf bytes.Buffer
		)

		integrationPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             failedTaskRun.Name,
						PipelineTaskName: "pipeline1-task1",
					},
					{
						Name:             skippedTaskRun.Name,
						PipelineTaskName: "pipeline1-task2",
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
		Expect(k8sClient.Status().Update(ctx, integrationPipelineRun)).Should(Succeed())

		pipelineRunOutcome, err := helpers.GetIntegrationPipelineRunOutcome(k8sClient, ctx, integrationPipelineRun)
		Expect(err).To(BeNil())
		Expect(pipelineRunOutcome.HasPipelineRunPassedTesting()).To(BeFalse())
		Expect(pipelineRunOutcome.HasPipelineRunValidTestOutputs()).To(BeTrue())
		Expect(pipelineRunOutcome.GetValidationErrorsList()).Should(BeEmpty())

		pipelineRunOutcome.LogResults(buflogr.NewWithBuffer(&buf))
		expectedLogEntry := "Found task results for pipeline run"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

	})

	It("ensure No Task pipelinerun passed when AppStudio Tests passed", func() {

		integrationPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             emptyTaskRun.Name,
						PipelineTaskName: "pipeline1-task1",
					},
					{
						Name:             emptyTaskRun.Name,
						PipelineTaskName: "pipeline1-task2",
					},
				},
			},
			Status: v1.Status{
				Conditions: v1.Conditions{
					apis.Condition{
						Reason: "Failed",
						Status: "False",
						Type:   apis.ConditionSucceeded,
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, integrationPipelineRun)).Should(Succeed())

		pipelineRunOutcome, err := helpers.GetIntegrationPipelineRunOutcome(k8sClient, ctx, integrationPipelineRun)
		Expect(err).To(BeNil())
		Expect(pipelineRunOutcome.HasPipelineRunPassedTesting()).To(BeFalse())
		Expect(pipelineRunOutcome.HasPipelineRunValidTestOutputs()).To(BeTrue())
		Expect(pipelineRunOutcome.GetValidationErrorsList()).Should(BeEmpty())

		err = gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(err).To(Succeed())
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
	})

	It("can handle malformed TEST_OUTPUT result", func() {
		var (
			buf bytes.Buffer
		)

		integrationPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             malformedTaskRun.Name,
						PipelineTaskName: "pipeline1-task1",
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

		Expect(k8sClient.Status().Update(ctx, integrationPipelineRun)).Should(Succeed())
		pipelineRunOutcome, err := helpers.GetIntegrationPipelineRunOutcome(k8sClient, ctx, integrationPipelineRun)
		Expect(err).To(BeNil())
		Expect(pipelineRunOutcome).NotTo(BeNil())
		Expect(pipelineRunOutcome.HasPipelineRunPassedTesting()).To(BeFalse())
		Expect(pipelineRunOutcome.HasPipelineRunValidTestOutputs()).To(BeFalse())
		Expect(pipelineRunOutcome.GetValidationErrorsList()).ShouldNot(BeEmpty())

		pipelineRunOutcome.LogResults(buflogr.NewWithBuffer(&buf))
		expectedLogEntry := "Invalid task results for pipeline run"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

	})

	It("can handle broken json as TEST_OUTPUT result", func() {
		var (
			buf bytes.Buffer
		)

		integrationPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             brokenJSONTaskRun.Name,
						PipelineTaskName: "pipeline1-task1",
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

		Expect(k8sClient.Status().Update(ctx, integrationPipelineRun)).Should(Succeed())
		pipelineRunOutcome, err := helpers.GetIntegrationPipelineRunOutcome(k8sClient, ctx, integrationPipelineRun)
		Expect(err).To(BeNil())
		Expect(pipelineRunOutcome).NotTo(BeNil())

		Expect(pipelineRunOutcome.HasPipelineRunValidTestOutputs()).To(BeFalse())
		Expect(pipelineRunOutcome.GetValidationErrorsList()).ShouldNot(BeEmpty())

		pipelineRunOutcome.LogResults(buflogr.NewWithBuffer(&buf))
		expectedLogEntry := "Invalid task results for pipeline run"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
	})

	It("can get all the TaskRuns for a PipelineRun with childReferences", func() {
		integrationPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             successfulTaskRun.Name,
						PipelineTaskName: "pipeline1-task1",
					},
					{
						Name:             skippedTaskRun.Name,
						PipelineTaskName: "pipeline1-task2",
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

		taskRuns, err := helpers.GetAllChildTaskRunsForPipelineRun(k8sClient, ctx, integrationPipelineRun)
		Expect(err).To(BeNil())
		Expect(taskRuns).To(HaveLen(2))

		// We expect the tasks to be sorted by start time
		tr1 := taskRuns[0]
		Expect(tr1.GetPipelineTaskName()).To(Equal("pipeline1-task1"))
		Expect(tr1.GetStartTime()).To(Equal(now))
		Expect(tr1.GetDuration().Minutes()).To(Equal(5.0))

		result1, err := tr1.GetTestResult()
		Expect(err).To(BeNil())
		Expect(result1).ToNot(BeNil())
		Expect(result1.TestOutput.Result).To(Equal("SUCCESS"))
		Expect(result1.TestOutput.Successes).To(Equal(10))

		result2, err := tr1.GetTestResult()
		Expect(err).To(BeNil())
		Expect(result1).To(Equal(result2))

		tr2 := taskRuns[1]
		Expect(tr2.GetStartTime()).To(Equal(now.Add(5 * time.Minute)))
		Expect(tr2.GetDuration().Minutes()).To(Equal(5.0))

		result3, err := tr2.GetTestResult()
		Expect(err).To(BeNil())
		Expect(result3).ToNot(BeNil())
		Expect(result3.TestOutput.Result).To(Equal("SKIPPED"))
		Expect(result3.TestOutput.Successes).To(Equal(0))
	})

	It("can return nil for a PipelineRun with no childReferences", func() {
		integrationPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{},
		}

		taskRuns, err := helpers.GetAllChildTaskRunsForPipelineRun(k8sClient, ctx, integrationPipelineRun)
		Expect(err).To(BeNil())
		Expect(taskRuns).To(BeNil())
	})

	It("can add and remove finalizer from an IntegrationTestScenario", func() {
		var buf bytes.Buffer

		// calling AddFinalizerToScenario() when the IntegrationTestScenario doesn't contain the finalizer
		log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
		Expect(helpers.AddFinalizerToScenario(k8sClient, log, ctx, integrationTestScenario, helpers.IntegrationTestScenarioFinalizer)).To(Succeed())
		Expect(integrationTestScenario.Finalizers).To(ContainElement(ContainSubstring(helpers.IntegrationTestScenarioFinalizer)))
		logEntry := "Added Finalizer to the IntegrationTestScenario"
		Expect(buf.String()).Should(ContainSubstring(logEntry))

		// calling RemoveFinalizerFromScenario() when the IntegrationTestScenario contains the finalizer
		Expect(helpers.RemoveFinalizerFromScenario(k8sClient, log, ctx, integrationTestScenario, helpers.IntegrationTestScenarioFinalizer)).To(Succeed())
		Expect(integrationTestScenario.Finalizers).NotTo(ContainElement(ContainSubstring(helpers.IntegrationTestScenarioFinalizer)))
		logEntry = "Removed Finalizer from the IntegrationTestScenario"
		Expect(buf.String()).Should(ContainSubstring(logEntry))
	})

	It("can add and remove finalizer from a component", func() {
		var buf bytes.Buffer

		// calling AddFinalizerToComponent() when the Component doesn't contain the finalizer
		log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
		Expect(helpers.AddFinalizerToComponent(k8sClient, log, ctx, hasComp, helpers.ComponentFinalizer)).To(Succeed())
		Expect(hasComp.Finalizers).To(ContainElement(ContainSubstring(helpers.ComponentFinalizer)))
		logEntry := "Added Finalizer to the Component"
		Expect(buf.String()).Should(ContainSubstring(logEntry))

		// calling RemoveFinalizerFromComponent() when the Component contains the finalizer
		Expect(helpers.RemoveFinalizerFromComponent(k8sClient, log, ctx, hasComp, helpers.ComponentFinalizer)).To(Succeed())
		Expect(hasComp.Finalizers).NotTo(ContainElement(ContainSubstring(helpers.ComponentFinalizer)))
		logEntry = "Removed Finalizer from the Component"
		Expect(buf.String()).Should(ContainSubstring(logEntry))
	})
})
