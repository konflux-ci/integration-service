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
	"context"
	"errors"
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
	"github.com/konflux-ci/integration-service/pkg/tracing"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			Expect(reflect.TypeOf(NewAdapter(ctx, integrationPipelineRunComponent, hasSnapshot, logger, loader.NewMockLoader(), k8sClient))).To(Equal(reflect.TypeOf(&Adapter{})))
		})
	})

	When("Snapshot already exists", func() {
		BeforeEach(func() {
			adapter = NewAdapter(ctx, integrationPipelineRunComponent, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
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

				adapter = NewAdapter(ctx, integrationPipelineRunComponentFailed, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
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
						ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
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

			adapter = NewAdapter(ctx, integrationPipelineRunComponentFailed, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
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
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
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

			adapter = NewAdapter(ctx, intgPipelineRunWithDeletionTimestamp, overrideSnapshot, logger, loader.NewMockLoader(), k8sClient)
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

			adapter = NewAdapter(ctx, intgPipelineInvalidResult, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
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
		var pipelineRun *tektonv1.PipelineRun

		BeforeEach(func() {
			pipelineRun = integrationPipelineRunComponent.DeepCopy()
			pipelineRun.Status.CompletionTime = &metav1.Time{Time: time.Now()}
			pipelineRun.Status.ChildReferences = []tektonv1.ChildStatusReference{
				{
					Name:             "absent-taskrun",
					PipelineTaskName: "task1",
				},
			}
			adapter = NewAdapter(ctx, pipelineRun, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
		})

		It("requeues while the referenced TaskRun is not visible and the PipelineRun is inside the resolution grace period", func() {
			status, detail, err := adapter.GetIntegrationPipelineRunStatus(adapter.context, adapter.client, pipelineRun)
			var retryable *retryableTaskRunsError
			Expect(errors.As(err, &retryable)).To(BeTrue())
			Expect(retryable.missing).To(ContainElement("absent-taskrun"))
			Expect(status).To(Equal(intgteststat.IntegrationTestStatus(0)))
			Expect(detail).To(BeEmpty())
		})

		It("returns an expired error when the referenced TaskRun is still missing after the resolution grace period", func() {
			pipelineRun.Status.CompletionTime = &metav1.Time{Time: time.Now().Add(-taskRunResolutionGracePeriod - time.Second)}

			status, detail, err := adapter.GetIntegrationPipelineRunStatus(adapter.context, adapter.client, pipelineRun)
			var expired *expiredTaskRunsError
			Expect(errors.As(err, &expired)).To(BeTrue())
			var retryable *retryableTaskRunsError
			Expect(errors.As(err, &retryable)).To(BeFalse())
			Expect(expired.missing).To(ContainElement("absent-taskrun"))
			Expect(err.Error()).To(ContainSubstring("resolution grace period has elapsed"))
			Expect(status).To(Equal(intgteststat.IntegrationTestStatus(0)))
			Expect(detail).To(BeEmpty())
		})
	})

	When("GetIntegrationPipelineRunStatus is called with a PLR with TaskRun, mentioned in its ChildReferences field, present within the cluster", func() {
		BeforeEach(func() {
			adapter = NewAdapter(ctx, integrationPipelineRunComponent, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
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
			adapter = NewAdapter(ctx, integrationPipelineRunComponent, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
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

			adapter = NewAdapter(ctx, integrationPipelineRunNoFinalizer, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
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
								Name:             successfulTaskRun.Name,
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

			adapter = NewAdapter(ctx, integrationPipelineRunWithFinalizer, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
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

	When("referenced TaskRuns are resolved by name", func() {
		It("ignores TaskRuns that are no longer referenced", func() {
			adapter = NewAdapter(ctx, integrationPipelineRunComponent, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{*successfulTaskRun, *failedTaskRun},
				},
			})

			status, detail, err := adapter.GetIntegrationPipelineRunStatus(adapter.context, adapter.client, integrationPipelineRunComponent)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(intgteststat.IntegrationTestStatusTestPassed))
			Expect(detail).To(Equal("Integration test passed"))
		})

		It("ignores CustomRun child references", func() {
			pipelineRun := integrationPipelineRunComponent.DeepCopy()
			customRun := tektonv1.ChildStatusReference{
				Name:             "custom-run",
				PipelineTaskName: "custom-task",
			}
			customRun.Kind = "CustomRun"
			pipelineRun.Status.ChildReferences = append(pipelineRun.Status.ChildReferences, customRun)
			adapter = NewAdapter(ctx, pipelineRun, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{*successfulTaskRun},
				},
			})

			status, _, err := adapter.GetIntegrationPipelineRunStatus(adapter.context, adapter.client, pipelineRun)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(intgteststat.IntegrationTestStatusTestPassed))
		})

		It("reads a referenced TaskRun from the API server when the cache list does not contain it", func() {
			adapter = NewAdapter(ctx, integrationPipelineRunComponent, hasSnapshot, logger, loader.NewMockLoaderWithAPIReader(k8sClient), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{},
				},
			})

			status, detail, err := adapter.GetIntegrationPipelineRunStatus(adapter.context, adapter.client, integrationPipelineRunComponent)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(intgteststat.IntegrationTestStatusTestPassed))
			Expect(detail).To(Equal("Integration test passed"))
		})

		It("records every TaskRun the API server reports as not found", func() {
			pipelineRun := integrationPipelineRunComponent.DeepCopy()
			pipelineRun.Status.ChildReferences = []tektonv1.ChildStatusReference{
				{
					Name:             "absent-taskrun",
					PipelineTaskName: "task1",
				},
				{
					Name:             "another-absent-taskrun",
					PipelineTaskName: "task2",
				},
			}
			adapter = NewAdapter(ctx, pipelineRun, hasSnapshot, logger, loader.NewMockLoaderWithAPIReader(k8sClient), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{},
				},
			})

			resolved, missing, err := adapter.loader.GetTaskRunsReferencedByPipelineRun(adapter.context, adapter.client, pipelineRun)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(BeEmpty())
			Expect(missing).To(Equal([]string{"absent-taskrun", "another-absent-taskrun"}))
		})

		It("returns a non-NotFound error from the API server", func() {
			pipelineRun := integrationPipelineRunComponent.DeepCopy()
			pipelineRun.Status.ChildReferences = []tektonv1.ChildStatusReference{
				{
					Name:             "absent-taskrun",
					PipelineTaskName: "task1",
				},
			}
			readErr := k8serrors.NewServiceUnavailable("api server overloaded")
			adapter = NewAdapter(ctx, pipelineRun, hasSnapshot, logger, loader.NewMockLoaderWithAPIReader(errReader{err: readErr}), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{},
				},
			})

			resolved, missing, err := adapter.loader.GetTaskRunsReferencedByPipelineRun(adapter.context, adapter.client, pipelineRun)
			Expect(err).To(MatchError(readErr))
			Expect(k8serrors.IsNotFound(err)).To(BeFalse())
			Expect(resolved).To(BeNil())
			Expect(missing).To(BeNil())
		})
	})

	When("a finished PipelineRun is missing referenced TaskRuns", func() {
		newPipelineRun := func(name, scenarioName string, completion time.Time) *tektonv1.PipelineRun {
			return &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "default",
					Labels: map[string]string{
						"appstudio.openshift.io/snapshot":      hasSnapshot.Name,
						"test.appstudio.openshift.io/scenario": scenarioName,
					},
					Finalizers: []string{helpers.IntegrationPipelineRunFinalizer},
				},
				Status: tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						CompletionTime: &metav1.Time{Time: completion},
						ChildReferences: []tektonv1.ChildStatusReference{
							{
								Name:             "absent-taskrun",
								PipelineTaskName: "task1",
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
		}

		// k8sClient reads from the controller cache. A write made in the same spec
		// can be missing from that cache when the API server is slow, and
		// EnsureStatusReportedInSnapshot would then overwrite a final status.
		readSnapshotFromAPI := func() *applicationapiv1alpha1.Snapshot {
			apiReader, err := client.New(cfg, client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).NotTo(HaveOccurred())
			snapshot := &applicationapiv1alpha1.Snapshot{}
			Expect(apiReader.Get(ctx, types.NamespacedName{Name: hasSnapshot.Name, Namespace: hasSnapshot.Namespace}, snapshot)).To(Succeed())
			return snapshot
		}

		prepareAdapter := func(pipelineRun *tektonv1.PipelineRun, snapshot *applicationapiv1alpha1.Snapshot) {
			mockData := []toolkit.MockData{
				{
					ContextKey: loader.AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{},
				},
			}
			if snapshot != nil {
				mockData = append(mockData, toolkit.MockData{
					ContextKey: loader.SnapshotContextKey,
					Resource:   snapshot,
				})
			}
			adapter = NewAdapter(ctx, pipelineRun, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, mockData)
		}

		It("requeues without writing status while the PipelineRun is inside the resolution grace period", func() {
			scenarioName := "requeue-missing-taskrun"
			pipelineRun := newPipelineRun("plr-requeue-missing-taskrun", scenarioName, time.Now())
			prepareAdapter(pipelineRun, nil)

			result, err := adapter.EnsureStatusReportedInSnapshot()
			var retryable *retryableTaskRunsError
			Expect(errors.As(err, &retryable)).To(BeTrue())
			Expect(result.RequeueRequest).To(BeTrue())

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(readSnapshotFromAPI())
			Expect(err).NotTo(HaveOccurred())
			_, ok := statuses.GetScenarioStatus(scenarioName)
			Expect(ok).To(BeFalse())
		})

		It("does not record a status when the PipelineRun has no scenario label after the resolution grace period", func() {
			pipelineRun := newPipelineRun("plr-missing-scenario-label", "unused-scenario", time.Now().Add(-taskRunResolutionGracePeriod-time.Second))
			delete(pipelineRun.Labels, "test.appstudio.openshift.io/scenario")
			prepareAdapter(pipelineRun, nil)

			result, err := adapter.EnsureStatusReportedInSnapshot()
			Expect(err).To(MatchError(ContainSubstring(`missing the "test.appstudio.openshift.io/scenario" label`)))
			Expect(result.RequeueRequest).To(BeTrue())

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(readSnapshotFromAPI())
			Expect(err).NotTo(HaveOccurred())
			_, ok := statuses.GetScenarioStatus("")
			Expect(ok).To(BeFalse())
		})

		It("records TestInvalid after the resolution grace period when no final status exists", func() {
			scenarioName := "invalid-missing-taskrun"
			pipelineRun := newPipelineRun("plr-invalid-missing-taskrun", scenarioName, time.Now().Add(-taskRunResolutionGracePeriod-time.Second))
			prepareAdapter(pipelineRun, nil)

			result, err := adapter.EnsureStatusReportedInSnapshot()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueRequest).To(BeFalse())

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(readSnapshotFromAPI())
			Expect(err).NotTo(HaveOccurred())
			detail, ok := statuses.GetScenarioStatus(scenarioName)
			Expect(ok).To(BeTrue())
			Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusTestInvalid))
			Expect(detail.Details).To(ContainSubstring("absent-taskrun"))
			Expect(detail.TestPipelineRunName).To(Equal(pipelineRun.Name))
		})

		It("keeps an existing final status for the PipelineRun after the resolution grace period", func() {
			scenarioName := "preserve-missing-taskrun"
			pipelineRun := newPipelineRun("plr-preserve-missing-taskrun", scenarioName, time.Now().Add(-taskRunResolutionGracePeriod-time.Second))

			fresh := readSnapshotFromAPI()
			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(fresh)
			Expect(err).NotTo(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(scenarioName, intgteststat.IntegrationTestStatusTestPassed, "Integration test passed")
			Expect(statuses.UpdateTestPipelineRunName(scenarioName, pipelineRun.Name)).To(Succeed())
			Expect(gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, fresh, statuses, k8sClient)).To(Succeed())

			// Give the adapter the snapshot that was just written. Reading it back
			// through k8sClient can still return the previous object while the
			// informer catches up, which makes this spec overwrite TestPassed.
			prepareAdapter(pipelineRun, fresh)
			result, err := adapter.EnsureStatusReportedInSnapshot()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueRequest).To(BeFalse())

			updatedStatuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(readSnapshotFromAPI())
			Expect(err).NotTo(HaveOccurred())
			detail, ok := updatedStatuses.GetScenarioStatus(scenarioName)
			Expect(ok).To(BeTrue())
			Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusTestPassed))
			Expect(detail.Details).To(Equal("Integration test passed"))
			Expect(detail.TestPipelineRunName).To(Equal(pipelineRun.Name))
		})

		It("does not replace a final status recorded for a different PipelineRun", func() {
			scenarioName := "preserve-other-pipelinerun"
			pipelineRun := newPipelineRun("plr-expired-other-status", scenarioName, time.Now().Add(-taskRunResolutionGracePeriod-time.Second))

			fresh := readSnapshotFromAPI()
			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(fresh)
			Expect(err).NotTo(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(scenarioName, intgteststat.IntegrationTestStatusTestPassed, "Integration test passed")
			Expect(statuses.UpdateTestPipelineRunName(scenarioName, "newer-pipelinerun")).To(Succeed())
			Expect(gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, fresh, statuses, k8sClient)).To(Succeed())

			prepareAdapter(pipelineRun, fresh)
			result, err := adapter.EnsureStatusReportedInSnapshot()
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueRequest).To(BeFalse())

			updatedStatuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(readSnapshotFromAPI())
			Expect(err).NotTo(HaveOccurred())
			detail, ok := updatedStatuses.GetScenarioStatus(scenarioName)
			Expect(ok).To(BeTrue())
			Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusTestPassed))
			Expect(detail.Details).To(Equal("Integration test passed"))
			Expect(detail.TestPipelineRunName).To(Equal("newer-pipelinerun"))
		})
	})

	When("emitTestTimingSpans is called", func() {
		BeforeEach(func() {
			adapter = NewAdapter(ctx, integrationPipelineRunComponent, hasSnapshot, logger, loader.NewMockLoader(), k8sClient)
		})

		It("does nothing when the PipelineRun already has the timingEmitted annotation", func() {
			adapter.pipelineRun.Annotations[tracing.TimingEmittedAnnotation] = "true"
			adapter.emitTestTimingSpans("")
			Expect(adapter.pipelineRun.Annotations[tracing.TimingEmittedAnnotation]).To(Equal("true"))
		})

		It("does not annotate the PipelineRun when the global tracer provider is the noop", func() {
			adapter.pipelineRun.Status.StartTime = &metav1.Time{Time: time.Now().Add(-time.Minute)}
			adapter.pipelineRun.Status.CompletionTime = &metav1.Time{Time: time.Now()}
			Expect(k8sClient.Status().Update(ctx, adapter.pipelineRun)).To(Succeed())

			adapter.emitTestTimingSpans("")
			Expect(adapter.pipelineRun.GetAnnotations()).NotTo(HaveKey(tracing.TimingEmittedAnnotation))
		})

		It("does not annotate the PipelineRun when start or completion time is missing", func() {
			prev := otel.GetTracerProvider()
			tp := sdktrace.NewTracerProvider()
			otel.SetTracerProvider(tp)
			DeferCleanup(func() {
				otel.SetTracerProvider(prev)
				_ = tp.Shutdown(ctx)
			})

			adapter.emitTestTimingSpans("")
			Expect(adapter.pipelineRun.GetAnnotations()).NotTo(HaveKey(tracing.TimingEmittedAnnotation))
		})

		It("emits spans and patches the timingEmitted annotation when the run has completed", func() {
			prev := otel.GetTracerProvider()
			tp := sdktrace.NewTracerProvider()
			otel.SetTracerProvider(tp)
			DeferCleanup(func() {
				otel.SetTracerProvider(prev)
				_ = tp.Shutdown(ctx)
			})

			adapter.pipelineRun.Status.StartTime = &metav1.Time{Time: time.Now().Add(-time.Minute)}
			adapter.pipelineRun.Status.CompletionTime = &metav1.Time{Time: time.Now()}
			Expect(k8sClient.Status().Update(ctx, adapter.pipelineRun)).To(Succeed())
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.GetPipelineRunContextKey,
					Resource:   adapter.pipelineRun,
				},
			})

			adapter.emitTestTimingSpans("")

			Eventually(func(g Gomega) {
				updated := &tektonv1.PipelineRun{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: adapter.pipelineRun.Name, Namespace: adapter.pipelineRun.Namespace}, updated)).To(Succeed())
				g.Expect(updated.GetAnnotations()).To(HaveKeyWithValue(tracing.TimingEmittedAnnotation, "true"))
			}).Should(Succeed())
		})

		It("logs and returns without marking the run when the refetch errors, so reconciliation continues and the next pass retries", func() {
			prev := otel.GetTracerProvider()
			tp := sdktrace.NewTracerProvider()
			otel.SetTracerProvider(tp)
			DeferCleanup(func() {
				otel.SetTracerProvider(prev)
				_ = tp.Shutdown(ctx)
			})

			adapter.pipelineRun.Status.StartTime = &metav1.Time{Time: time.Now().Add(-time.Minute)}
			adapter.pipelineRun.Status.CompletionTime = &metav1.Time{Time: time.Now()}
			Expect(k8sClient.Status().Update(ctx, adapter.pipelineRun)).To(Succeed())
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.GetPipelineRunContextKey,
					Err:        errors.New("refetch boom"),
				},
			})

			Expect(func() { adapter.emitTestTimingSpans("") }).NotTo(Panic())
			Expect(adapter.pipelineRun.GetAnnotations()).NotTo(HaveKey(tracing.TimingEmittedAnnotation))

			stored := &tektonv1.PipelineRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: adapter.pipelineRun.Name, Namespace: adapter.pipelineRun.Namespace}, stored)).To(Succeed())
			Expect(stored.GetAnnotations()).NotTo(HaveKey(tracing.TimingEmittedAnnotation))
		})
	})

})

// errReader is a client.Reader whose Get and List return a fixed error.
type errReader struct {
	err error
}

func (r errReader) Get(context.Context, types.NamespacedName, client.Object, ...client.GetOption) error {
	return r.err
}

func (r errReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return r.err
}
