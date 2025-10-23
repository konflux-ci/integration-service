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

package integrationpipeline

import (
	"fmt"
	"os"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"

	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Pipeline Adapter", Ordered, func() {
	var (
		adapter *Adapter
		logger  helpers.IntegrationLogger

		successfulTaskRun                     *tektonv1.TaskRun
		failedTaskRun                         *tektonv1.TaskRun
		integrationPipelineRunComponent       *tektonv1.PipelineRun
		integrationPipelineRunComponentFailed *tektonv1.PipelineRun
		intgPipelineRunWithDeletionTimestamp  *tektonv1.PipelineRun
		hasComp                               *applicationapiv1alpha1.Component
		hasComp2                              *applicationapiv1alpha1.Component
		hasApp                                *applicationapiv1alpha1.Application
		hasSnapshot                           *applicationapiv1alpha1.Snapshot
		snapshotPREvent                       *applicationapiv1alpha1.Snapshot
		overrideSnapshot                      *applicationapiv1alpha1.Snapshot
		integrationTestScenario               *v1beta2.IntegrationTestScenario
		integrationTestScenarioFailed         *v1beta2.IntegrationTestScenario
	)
	const (
		SampleRepoLink           = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		SampleCommit             = "a2ba645d50e471d5f084b"
		SampleDigest             = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		SampleImageWithoutDigest = "quay.io/redhat-appstudio/sample-image"
		SampleImage              = SampleImageWithoutDigest + "@" + SampleDigest
	)

	BeforeAll(func() {
		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "application-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		logger = helpers.IntegrationLogger{Logger: ctrl.Log}.WithApp(*hasApp)

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-sample",
				Application:    "application-sample",
				ContainerImage: "invalidImage",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: SampleCommit,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())

		hasComp2 = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "another-component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "another-component-sample",
				Application:    "application-sample",
				ContainerImage: "",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: SampleCommit,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComp2)).Should(Succeed())

		integrationTestScenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: hasApp.Name,
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "git",
					Params: []v1beta2.ResolverParameter{
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
											"timestamp": "2024-05-22T06:42:21+00:00",
											"failures": 1,
											"successes": 0,
											"warnings": 0
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, failedTaskRun)).Should(Succeed())
	})

	BeforeEach(func() {
		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      "component",
					gitops.SnapshotComponentLabel: hasComp.Name,
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation: "123",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComp.Name,
						ContainerImage: SampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		integrationPipelineRunComponent = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-component-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type":           "test",
					"pac.test.appstudio.openshift.io/url-org":         "redhat-appstudio",
					"pac.test.appstudio.openshift.io/original-prname": "build-service-on-push",
					"pac.test.appstudio.openshift.io/url-repository":  "build-service",
					"pac.test.appstudio.openshift.io/repository":      "build-service-pac",
					"appstudio.openshift.io/snapshot":                 hasSnapshot.Name,
					"test.appstudio.openshift.io/scenario":            integrationTestScenario.Name,
					"appstudio.openshift.io/application":              hasApp.Name,
					"appstudio.openshift.io/component":                hasComp.Name,
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
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/kpavic/test-bundle:component-pipeline-pass"},
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
		Expect(k8sClient.Create(ctx, integrationPipelineRunComponent)).Should(Succeed())

		integrationPipelineRunComponent.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				CompletionTime: &metav1.Time{Time: time.Now()},
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             successfulTaskRun.Name,
						PipelineTaskName: "task1",
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
		Expect(k8sClient.Status().Update(ctx, integrationPipelineRunComponent)).Should(Succeed())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationPipelineRunComponent)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, successfulTaskRun)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, failedTaskRun)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	When("NewAdapter is called", func() {
		It("creates and return a new adapter", func() {
			Expect(reflect.TypeOf(NewAdapter(ctx, integrationPipelineRunComponent, hasApp, hasSnapshot, logger, loader.NewMockLoader(), k8sClient))).To(Equal(reflect.TypeOf(&Adapter{})))
		})
	})

	When("Snapshot already exists", func() {
		BeforeEach(func() {
			adapter = NewAdapter(ctx, integrationPipelineRunComponent, hasApp, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasSnapshot,
				},
				{
					ContextKey: loader.TaskRunContextKey,
					Resource:   successfulTaskRun,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{*successfulTaskRun},
				},
			})
			existingSnapshot, err := adapter.loader.GetSnapshotFromPipelineRun(adapter.context, adapter.client, integrationPipelineRunComponent)
			Expect(err).ToNot(HaveOccurred())
			Expect(existingSnapshot).ToNot(BeNil())
		})

		It("ensures test status in snapshot is updated to passed", func() {
			controllerutil.AddFinalizer(integrationPipelineRunComponent, "test.appstudio.openshift.io/pipelinerun")
			result, err := adapter.EnsureStatusReportedInSnapshot()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())

			detail, ok := statuses.GetScenarioStatus(integrationTestScenario.Name)
			Expect(ok).To(BeTrue())
			Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusTestPassed))
		})

		When("integration pipeline failed", func() {

			BeforeEach(func() {
				//Create one failed scenario and its failed pipelineRun
				integrationTestScenarioFailed = &v1beta2.IntegrationTestScenario{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-fail",
						Namespace: "default",

						Labels: map[string]string{
							"test.appstudio.openshift.io/optional": "false",
						},
					},
					Spec: v1beta2.IntegrationTestScenarioSpec{
						Application: hasApp.Name,
						ResolverRef: v1beta2.ResolverRef{
							Resolver: "git",
							Params: []v1beta2.ResolverParameter{
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
					},
				}
				Expect(k8sClient.Create(ctx, integrationTestScenarioFailed)).Should(Succeed())

				integrationPipelineRunComponentFailed = &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pipelinerun-component-sample-failed",
						Namespace: "default",
						Labels: map[string]string{
							"pipelines.appstudio.openshift.io/type":           "test",
							"pac.test.appstudio.openshift.io/url-org":         "redhat-appstudio",
							"pac.test.appstudio.openshift.io/original-prname": "build-service-on-push",
							"pac.test.appstudio.openshift.io/url-repository":  "build-service",
							"pac.test.appstudio.openshift.io/repository":      "build-service-pac",
							"appstudio.openshift.io/snapshot":                 hasSnapshot.Name,
							"test.appstudio.openshift.io/scenario":            integrationTestScenarioFailed.Name,
							"appstudio.openshift.io/application":              hasApp.Name,
							"appstudio.openshift.io/component":                hasComp.Name,
						},
						Annotations: map[string]string{
							"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
						},
					},
					Spec: tektonv1.PipelineRunSpec{
						PipelineRef: &tektonv1.PipelineRef{
							Name: "component-pipeline-fail",
							ResolverRef: tektonv1.ResolverRef{
								Resolver: "bundle",
								Params: tektonv1.Params{
									{
										Name:  "bundle",
										Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:component-pipeline-fail"},
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

				Expect(k8sClient.Create(ctx, integrationPipelineRunComponentFailed)).Should(Succeed())

				integrationPipelineRunComponentFailed.Status = tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now().Add(5 * time.Minute)},
						ChildReferences: []tektonv1.ChildStatusReference{
							{
								Name:             failedTaskRun.Name,
								PipelineTaskName: "task1",
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
				Expect(k8sClient.Status().Update(ctx, integrationPipelineRunComponentFailed)).Should(Succeed())

				adapter = NewAdapter(ctx, integrationPipelineRunComponentFailed, hasApp, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
				adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
					{
						ContextKey: loader.ApplicationContextKey,
						Resource:   hasApp,
					},
					{
						ContextKey: loader.ComponentContextKey,
						Resource:   hasComp,
					},
					{
						ContextKey: loader.SnapshotContextKey,
						Resource:   hasSnapshot,
					},
					{
						ContextKey: loader.PipelineRunsContextKey,
						Resource:   []tektonv1.PipelineRun{*integrationPipelineRunComponent, *integrationPipelineRunComponentFailed},
					},
					{
						ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
						Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioFailed},
					},
					{
						ContextKey: loader.ApplicationComponentsContextKey,
						Resource:   []applicationapiv1alpha1.Component{*hasComp},
					},
					{
						ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
						Resource:   []tektonv1.TaskRun{*failedTaskRun},
					},
				})
			})

			AfterEach(func() {
				err := k8sClient.Delete(ctx, integrationPipelineRunComponentFailed)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
				err = k8sClient.Delete(ctx, integrationTestScenarioFailed)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})

			It("ensures test status in snapshot is updated to failed", func() {
				controllerutil.AddFinalizer(integrationPipelineRunComponentFailed, "test.appstudio.openshift.io/pipelinerun")
				result, err := adapter.EnsureStatusReportedInSnapshot()
				Expect(!result.CancelRequest && err == nil).To(BeTrue())

				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
				Expect(err).ToNot(HaveOccurred())

				detail, ok := statuses.GetScenarioStatus(integrationTestScenarioFailed.Name)
				Expect(ok).To(BeTrue())
				Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusTestFail))
				Expect(detail.TestPipelineRunName).To(Equal(integrationPipelineRunComponentFailed.Name))

			})
		})

	})

	When("EnsureStatusReportedInSnapshot is called with a PLR related to PR event", func() {
		BeforeEach(func() {
			snapshotPREvent = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snapshot-pr-event-sample",
					Namespace: "default",
					Labels: map[string]string{
						gitops.SnapshotTypeLabel:                     "component",
						gitops.SnapshotComponentLabel:                hasComp.Name,
						"pac.test.appstudio.openshift.io/event-type": "pull_request",
						gitops.PipelineAsCodePullRequestAnnotation:   "1",
					},
					Annotations: map[string]string{
						gitops.PipelineAsCodeInstallationIDAnnotation: "123",
					},
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasApp.Name,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           hasComp.Name,
							ContainerImage: SampleImage,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, snapshotPREvent)).Should(Succeed())

			//Create one failed scenario and its failed pipelineRun
			integrationTestScenarioFailed = &v1beta2.IntegrationTestScenario{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-fail",
					Namespace: "default",

					Labels: map[string]string{
						"test.appstudio.openshift.io/optional": "false",
					},
				},
				Spec: v1beta2.IntegrationTestScenarioSpec{
					Application: hasApp.Name,
					ResolverRef: v1beta2.ResolverRef{
						Resolver: "git",
						Params: []v1beta2.ResolverParameter{
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
				},
			}
			Expect(k8sClient.Create(ctx, integrationTestScenarioFailed)).Should(Succeed())

			integrationPipelineRunComponentFailed = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipelinerun-component-sample-failed",
					Namespace: "default",
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type":           "test",
						"pac.test.appstudio.openshift.io/url-org":         "redhat-appstudio",
						"pac.test.appstudio.openshift.io/original-prname": "build-service-on-push",
						"pac.test.appstudio.openshift.io/url-repository":  "build-service",
						"pac.test.appstudio.openshift.io/repository":      "build-service-pac",
						"appstudio.openshift.io/snapshot":                 snapshotPREvent.Name,
						"test.appstudio.openshift.io/scenario":            integrationTestScenarioFailed.Name,
						"appstudio.openshift.io/application":              hasApp.Name,
						"appstudio.openshift.io/component":                hasComp.Name,
					},
					Finalizers: []string{
						"test.appstudio.openshift.io/pipelinerun",
					},
					Annotations: map[string]string{
						"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
					},
				},
				Spec: tektonv1.PipelineRunSpec{
					PipelineRef: &tektonv1.PipelineRef{
						Name: "component-pipeline-fail",
						ResolverRef: tektonv1.ResolverRef{
							Resolver: "bundle",
							Params: tektonv1.Params{
								{
									Name:  "bundle",
									Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:component-pipeline-fail"},
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

			Expect(k8sClient.Create(ctx, integrationPipelineRunComponentFailed)).Should(Succeed())

			integrationPipelineRunComponentFailed.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					CompletionTime: &metav1.Time{Time: time.Now().Add(5 * time.Minute)},
					ChildReferences: []tektonv1.ChildStatusReference{
						{
							Name:             failedTaskRun.Name,
							PipelineTaskName: "task1",
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
			Expect(k8sClient.Status().Update(ctx, integrationPipelineRunComponentFailed)).Should(Succeed())

			adapter = NewAdapter(ctx, integrationPipelineRunComponentFailed, hasApp, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   snapshotPREvent,
				},
				{
					ContextKey: loader.PipelineRunsContextKey,
					Resource:   []tektonv1.PipelineRun{*integrationPipelineRunComponent, *integrationPipelineRunComponentFailed},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioFailed},
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{*failedTaskRun},
				},
			})
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, integrationPipelineRunComponentFailed)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Delete(ctx, integrationTestScenarioFailed)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures test status in snapshot is updated to failed", func() {
			result, err := adapter.EnsureStatusReportedInSnapshot()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshotPREvent)
			Expect(err).ToNot(HaveOccurred())

			detail, ok := statuses.GetScenarioStatus(integrationTestScenarioFailed.Name)
			Expect(ok).To(BeTrue())
			Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusTestFail))
			Expect(detail.TestPipelineRunName).To(Equal(integrationPipelineRunComponentFailed.Name))

			Expect(integrationPipelineRunComponentFailed.Finalizers).To(ContainElement(ContainSubstring("test.appstudio.openshift.io/pipelinerun")))
		})
	})

	When("EnsureStatusReportedInSnapshot is called with an Integration PLR with non-nil Deletion timestamp and an override Snapshot", func() {
		BeforeEach(func() {
			overrideSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snapshot-override-sample",
					Namespace: "default",
					Labels: map[string]string{
						gitops.SnapshotTypeLabel:      gitops.SnapshotOverrideType,
						gitops.SnapshotComponentLabel: hasComp.Name,
					},
					Annotations: map[string]string{
						gitops.PipelineAsCodeInstallationIDAnnotation: "123",
					},
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasApp.Name,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           hasComp.Name,
							ContainerImage: SampleImage,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, overrideSnapshot)).Should(Succeed())

			intgPipelineRunWithDeletionTimestamp = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipelinerun-component-sample-deletion-timestamp",
					Namespace: "default",
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type":           "test",
						"pac.test.appstudio.openshift.io/url-org":         "redhat-appstudio",
						"pac.test.appstudio.openshift.io/original-prname": "build-service-on-push",
						"pac.test.appstudio.openshift.io/url-repository":  "build-service",
						"pac.test.appstudio.openshift.io/repository":      "build-service-pac",
						"appstudio.openshift.io/snapshot":                 overrideSnapshot.Name,
						"test.appstudio.openshift.io/scenario":            integrationTestScenarioFailed.Name,
						"appstudio.openshift.io/application":              hasApp.Name,
						"appstudio.openshift.io/component":                hasComp.Name,
					},
					Annotations: map[string]string{
						"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
					},
					Finalizers: []string{
						"test.appstudio.openshift.io/pipelinerun",
					},
				},
				Spec: tektonv1.PipelineRunSpec{
					PipelineRef: &tektonv1.PipelineRef{
						Name: "component-pipeline-fail",
						ResolverRef: tektonv1.ResolverRef{
							Resolver: "bundle",
							Params: tektonv1.Params{
								{
									Name:  "bundle",
									Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:component-pipeline-fail"},
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

			Expect(k8sClient.Create(ctx, intgPipelineRunWithDeletionTimestamp)).Should(Succeed())

			now := metav1.NewTime(metav1.Now().Add(time.Second * 1))
			intgPipelineRunWithDeletionTimestamp.SetDeletionTimestamp(&now)

			adapter = NewAdapter(ctx, intgPipelineRunWithDeletionTimestamp, hasApp, overrideSnapshot, logger, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   overrideSnapshot,
				},
				{
					ContextKey: loader.PipelineRunsContextKey,
					Resource:   []tektonv1.PipelineRun{*intgPipelineRunWithDeletionTimestamp},
				},
			})
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, intgPipelineRunWithDeletionTimestamp)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Delete(ctx, overrideSnapshot)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures test status in snapshot is updated to deleted", func() {
			status, pipelineRunDetail, err := adapter.GetIntegrationPipelineRunStatus(adapter.context, adapter.client, intgPipelineRunWithDeletionTimestamp)

			Expect(err).ToNot(HaveOccurred())
			Expect(status).To(Equal(intgteststat.IntegrationTestStatusDeleted))
			Expect(pipelineRunDetail).To(ContainSubstring(fmt.Sprintf("Integration test which is running as pipeline run '%s', has been deleted", intgPipelineRunWithDeletionTimestamp.Name)))
			Expect(intgPipelineRunWithDeletionTimestamp.DeletionTimestamp).ToNot(BeNil())
			Expect(intgPipelineRunWithDeletionTimestamp.Finalizers).To(ContainElement(ContainSubstring("test.appstudio.openshift.io/pipelinerun")))

			result, err := adapter.EnsureStatusReportedInSnapshot()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(overrideSnapshot)
			Expect(err).ToNot(HaveOccurred())

			detail, ok := statuses.GetScenarioStatus(integrationTestScenarioFailed.Name)
			Expect(ok).To(BeTrue())
			Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusDeleted))
			Expect(detail.TestPipelineRunName).To(Equal(intgPipelineRunWithDeletionTimestamp.Name))

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      intgPipelineRunWithDeletionTimestamp.Name,
					Namespace: intgPipelineRunWithDeletionTimestamp.Namespace,
				}, intgPipelineRunWithDeletionTimestamp)
				return err == nil && !controllerutil.ContainsFinalizer(intgPipelineRunWithDeletionTimestamp, helpers.IntegrationPipelineRunFinalizer)
			}, time.Second*20).Should(BeTrue())
		})
	})

	When("GetIntegrationPipelineRunStatus is called with an Integration PLR with invalid TEST_OUTPUT result", func() {
		var (
			taskRunInvalidResult      *tektonv1.TaskRun
			intgPipelineInvalidResult *tektonv1.PipelineRun
		)

		BeforeEach(func() {

			taskRunInvalidResult = &tektonv1.TaskRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-taskrun-invalid",
					Namespace: "default",
				},
				Spec: tektonv1.TaskRunSpec{
					TaskRef: &tektonv1.TaskRef{
						Name: "test-taskrun-invalid",
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
			Expect(k8sClient.Create(ctx, taskRunInvalidResult)).Should(Succeed())

			now := time.Now()
			taskRunInvalidResult.Status = tektonv1.TaskRunStatus{
				TaskRunStatusFields: tektonv1.TaskRunStatusFields{
					StartTime:      &metav1.Time{Time: now},
					CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
					Results: []tektonv1.TaskRunResult{
						{
							Name: "TEST_OUTPUT",
							Value: *tektonv1.NewStructuredValues(`{
												"result": "INVALID",
											}`),
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, taskRunInvalidResult)).Should(Succeed())

			intgPipelineInvalidResult = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipelinerun-component-sample-invalid-result",
					Namespace: "default",
					Annotations: map[string]string{
						"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
					},
				},
				Spec: tektonv1.PipelineRunSpec{
					PipelineRef: &tektonv1.PipelineRef{
						Name: "component-pipeline-invalid",
						ResolverRef: tektonv1.ResolverRef{
							Resolver: "bundle",
							Params: tektonv1.Params{
								{
									Name:  "bundle",
									Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:component-pipeline-fail"},
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
			Expect(k8sClient.Create(ctx, intgPipelineInvalidResult)).Should(Succeed())

			intgPipelineInvalidResult.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					CompletionTime: &metav1.Time{Time: time.Now()},
					ChildReferences: []tektonv1.ChildStatusReference{
						{
							Name:             taskRunInvalidResult.Name,
							PipelineTaskName: "task1",
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
			Expect(k8sClient.Status().Update(ctx, intgPipelineInvalidResult)).Should(Succeed())

			adapter = NewAdapter(ctx, intgPipelineInvalidResult, hasApp, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{*taskRunInvalidResult},
				},
			})
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, intgPipelineInvalidResult)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Delete(ctx, taskRunInvalidResult)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures test status in snapshot is updated to failed", func() {
			status, detail, err := adapter.GetIntegrationPipelineRunStatus(adapter.context, adapter.client, intgPipelineInvalidResult)

			Expect(err).ToNot(HaveOccurred())
			Expect(status).To(Equal(intgteststat.IntegrationTestStatusTestFail))
			Expect(detail).To(ContainSubstring("Invalid result:"))
		})
	})

	When("GetIntegrationPipelineRunStatus is called with a PLR with TaskRun, mentioned in its ChildReferences field, missing from the cluster", func() {
		BeforeEach(func() {
			adapter = NewAdapter(ctx, integrationPipelineRunComponent, hasApp, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
		})

		It("ensures test status in snapshot is updated to failed", func() {
			status, detail, err := adapter.GetIntegrationPipelineRunStatus(adapter.context, adapter.client, integrationPipelineRunComponent)
			Expect(err).ToNot(HaveOccurred())
			Expect(status).To(Equal(intgteststat.IntegrationTestStatusTestInvalid))
			Expect(detail).To(ContainSubstring(fmt.Sprintf("Failed to determine status of pipelinerun '%s', due to mismatch"+
				" in TaskRuns present in cluster (0) and those referenced within childReferences (1)", integrationPipelineRunComponent.Name)))
		})
	})

	When("GetIntegrationPipelineRunStatus is called with a PLR with TaskRun, mentioned in its ChildReferences field, present within the cluster", func() {
		BeforeEach(func() {
			adapter = NewAdapter(ctx, integrationPipelineRunComponent, hasApp, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{*successfulTaskRun},
				},
			})
		})

		It("ensures test status in snapshot is updated to failed", func() {
			status, detail, err := adapter.GetIntegrationPipelineRunStatus(adapter.context, adapter.client, integrationPipelineRunComponent)
			Expect(err).ToNot(HaveOccurred())
			Expect(status).To(Equal(intgteststat.IntegrationTestStatusTestPassed))
			Expect(detail).To(ContainSubstring("Integration test passed"))
		})
	})

	When("EnsureIntegrationPipelineRunLogURL is called with a PLR with a log URL", func() {
		BeforeEach(func() {
			consoleURL := fmt.Sprintf("https://definetly.not.prod/preview/application-pipeline/ns/%s/pipelinerun/%s", integrationPipelineRunComponent.Namespace, integrationPipelineRunComponent.Name)
			os.Setenv("CONSOLE_URL", consoleURL)
			adapter = NewAdapter(ctx, integrationPipelineRunComponent, hasApp, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
		})

		It("should annotate the PipelineRun with a log URL if available", func() {
			// Call the new public method under test
			logURLKey := "pac.test.appstudio.openshift.io/log-url"
			Eventually(func() map[string]string {
				// Call the method under test
				_, err := adapter.EnsureIntegrationPipelineRunLogURL()
				Expect(err).ToNot(HaveOccurred())

				// Fetch the updated PipelineRun
				updatedPR := &tektonv1.PipelineRun{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      integrationPipelineRunComponent.Name,
					Namespace: integrationPipelineRunComponent.Namespace,
				}, updatedPR)
				Expect(err).ToNot(HaveOccurred())

				return updatedPR.Annotations
			}, time.Second*20).Should(HaveKey(logURLKey))

		})

	})

	When("EnsureStatusReportedInSnapshot is called with a PipelineRun that has no finalizer", func() {
		BeforeEach(func() {
			// Create a PipelineRun without the finalizer (simulating the state after finalizer removal)
			integrationPipelineRunNoFinalizer := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-no-finalizer",
					Namespace: "default",
					Labels: map[string]string{
						"test.appstudio.openshift.io/scenario": integrationTestScenario.Name,
					},
					Finalizers: []string{}, // No finalizer present
				},
				Spec: tektonv1.PipelineRunSpec{
					PipelineRef: &tektonv1.PipelineRef{
						Name: "test-pipeline",
					},
				},
				Status: tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						ChildReferences: []tektonv1.ChildStatusReference{
							{
								Name:             "test-taskrun",
								PipelineTaskName: "test-task",
							},
						},
					},
					Status: v1.Status{
						Conditions: []apis.Condition{
							{
								Type:   apis.ConditionSucceeded,
								Status: "True",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, integrationPipelineRunNoFinalizer)).Should(Succeed())

			adapter = NewAdapter(ctx, integrationPipelineRunNoFinalizer, hasApp, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-no-finalizer",
					Namespace: "default",
				},
			})
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("should skip processing and return ContinueProcessing when finalizer is not present", func() {
			result, err := adapter.EnsureStatusReportedInSnapshot()

			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())
			Expect(result.RequeueRequest).To(BeFalse())
			Expect(result.RequeueDelay).To(Equal(time.Duration(0)))
		})

		It("should not call GetIntegrationPipelineRunStatus when finalizer is not present", func() {
			// This test verifies that the expensive GetIntegrationPipelineRunStatus call is skipped
			// We can verify this by checking that no TaskRun mismatch error occurs
			// even though the PipelineRun has ChildReferences but no TaskRuns in the cluster

			result, err := adapter.EnsureStatusReportedInSnapshot()

			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			// Verify that the snapshot status was not updated (since we skipped processing)
			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())

			// The snapshot should not have been updated with this PipelineRun's status
			// since we skipped processing due to missing finalizer
			detail, ok := statuses.GetScenarioStatus(integrationTestScenario.Name)
			if ok {
				// If there's existing status, it should not be from this PipelineRun
				Expect(detail.TestPipelineRunName).NotTo(Equal("test-pipeline-no-finalizer"))
			}
		})
	})

	When("EnsureStatusReportedInSnapshot is called with a PipelineRun that has the finalizer", func() {
		BeforeEach(func() {
			// Create a PipelineRun with the finalizer (normal case)
			integrationPipelineRunWithFinalizer := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-with-finalizer",
					Namespace: "default",
					Labels: map[string]string{
						"test.appstudio.openshift.io/scenario": integrationTestScenario.Name,
					},
					Finalizers: []string{helpers.IntegrationPipelineRunFinalizer}, // Finalizer present
				},
				Spec: tektonv1.PipelineRunSpec{
					PipelineRef: &tektonv1.PipelineRef{
						Name: "test-pipeline",
					},
				},
				Status: tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						ChildReferences: []tektonv1.ChildStatusReference{
							{
								Name:             "test-taskrun",
								PipelineTaskName: "test-task",
							},
						},
					},
					Status: v1.Status{
						Conditions: []apis.Condition{
							{
								Type:   apis.ConditionSucceeded,
								Status: "True",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, integrationPipelineRunWithFinalizer)).Should(Succeed())

			adapter = NewAdapter(ctx, integrationPipelineRunWithFinalizer, hasApp, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasSnapshot,
				},
				{
					ContextKey: loader.TaskRunContextKey,
					Resource:   successfulTaskRun,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{*successfulTaskRun},
				},
			})
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-with-finalizer",
					Namespace: "default",
				},
			})
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("should process normally when finalizer is present", func() {
			result, err := adapter.EnsureStatusReportedInSnapshot()

			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			// Verify that the snapshot status was updated (since we processed normally)
			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())

			detail, ok := statuses.GetScenarioStatus(integrationTestScenario.Name)
			Expect(ok).To(BeTrue())
			Expect(detail.TestPipelineRunName).To(Equal("test-pipeline-with-finalizer"))
		})
	})

})
