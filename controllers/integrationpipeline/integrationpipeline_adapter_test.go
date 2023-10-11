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
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/loader"
	intgteststat "github.com/redhat-appstudio/integration-service/pkg/integrationteststatus"

	"k8s.io/apimachinery/pkg/api/meta"
	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/status"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tonglil/buflogr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockStatusAdapter struct {
	Reporter          *MockStatusReporter
	GetReportersError error
}

type MockStatusReporter struct {
	Called            bool
	ReportStatusError error
}

func (r *MockStatusReporter) ReportStatus(client.Client, context.Context, *tektonv1beta1.PipelineRun) error {
	r.Called = true
	return r.ReportStatusError
}

func (r *MockStatusReporter) ReportStatusForSnapshot(client.Client, context.Context, *helpers.IntegrationLogger, *applicationapiv1alpha1.Snapshot) error {
	r.Called = true
	return r.ReportStatusError
}

func (a *MockStatusAdapter) GetReporters(object client.Object) ([]status.Reporter, error) {
	return []status.Reporter{a.Reporter}, a.GetReportersError
}

var _ = Describe("Pipeline Adapter", Ordered, func() {
	var (
		adapter        *Adapter
		createAdapter  func() *Adapter
		logger         helpers.IntegrationLogger
		statusAdapter  *MockStatusAdapter
		statusReporter *MockStatusReporter

		successfulTaskRun                     *tektonv1beta1.TaskRun
		failedTaskRun                         *tektonv1beta1.TaskRun
		integrationPipelineRunComponent       *tektonv1beta1.PipelineRun
		integrationPipelineRunComponentFailed *tektonv1beta1.PipelineRun
		hasComp                               *applicationapiv1alpha1.Component
		hasComp2                              *applicationapiv1alpha1.Component
		hasCompNew                            *applicationapiv1alpha1.Component
		hasApp                                *applicationapiv1alpha1.Application
		hasSnapshot                           *applicationapiv1alpha1.Snapshot
		hasEnv                                *applicationapiv1alpha1.Environment
		deploymentTarget                      *applicationapiv1alpha1.DeploymentTarget
		deploymentTargetClaim                 *applicationapiv1alpha1.DeploymentTargetClaim
		deploymentTargetClass                 *applicationapiv1alpha1.DeploymentTargetClass
		snapshotEnvironmentBinding            *applicationapiv1alpha1.SnapshotEnvironmentBinding
		integrationTestScenario               *v1beta1.IntegrationTestScenario
		integrationTestScenarioFailed         *v1beta1.IntegrationTestScenario
	)
	const (
		SampleRepoLink           = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		SampleCommit             = "a2ba645d50e471d5f084b"
		SampleDigest             = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		SampleImageWithoutDigest = "quay.io/redhat-appstudio/sample-image"
		SampleImage              = SampleImageWithoutDigest + "@" + SampleDigest
	)

	BeforeAll(func() {
		hasEnv = &applicationapiv1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-env",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.EnvironmentSpec{
				Type:               "POC",
				DisplayName:        "my-environment",
				DeploymentStrategy: applicationapiv1alpha1.DeploymentStrategy_Manual,
				ParentEnvironment:  "",
				Tags:               []string{"ephemeral"},
				Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
					Env: []applicationapiv1alpha1.EnvVarPair{
						{
							Name:  "var_name",
							Value: "test",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasEnv)).Should(Succeed())

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

		integrationTestScenario = &v1beta1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta1.IntegrationTestScenarioSpec{
				Application: hasApp.Name,
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

		successfulTaskRun = &tektonv1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-pass",
				Namespace: "default",
			},
			Spec: tektonv1beta1.TaskRunSpec{
				TaskRef: &tektonv1beta1.TaskRef{
					Name:   "test-taskrun-pass",
					Bundle: "quay.io/redhat-appstudio/example-tekton-bundle:test",
				},
			},
		}
		Expect(k8sClient.Create(ctx, successfulTaskRun)).Should(Succeed())

		now := time.Now()
		successfulTaskRun.Status = tektonv1beta1.TaskRunStatus{
			TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				TaskRunResults: []tektonv1beta1.TaskRunResult{
					{
						Name: "TEST_OUTPUT",
						Value: *tektonv1beta1.NewStructuredValues(`{
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

		failedTaskRun = &tektonv1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-fail",
				Namespace: "default",
			},
			Spec: tektonv1beta1.TaskRunSpec{
				TaskRef: &tektonv1beta1.TaskRef{
					Name:   "test-taskrun-fail",
					Bundle: "quay.io/redhat-appstudio/example-tekton-bundle:test",
				},
			},
		}

		Expect(k8sClient.Create(ctx, failedTaskRun)).Should(Succeed())

		failedTaskRun.Status = tektonv1beta1.TaskRunStatus{
			TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				TaskRunResults: []tektonv1beta1.TaskRunResult{
					{
						Name: "TEST_OUTPUT",
						Value: *tektonv1beta1.NewStructuredValues(`{
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

		integrationPipelineRunComponent = &tektonv1beta1.PipelineRun{
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
					"appstudio.openshift.io/environment":              hasEnv.Name,
					"appstudio.openshift.io/application":              hasApp.Name,
					"appstudio.openshift.io/component":                hasComp.Name,
				},
				Annotations: map[string]string{
					"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
				},
			},
			Spec: tektonv1beta1.PipelineRunSpec{
				PipelineRef: &tektonv1beta1.PipelineRef{
					Name:   "component-pipeline-pass",
					Bundle: "quay.io/kpavic/test-bundle:component-pipeline-pass",
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationPipelineRunComponent)).Should(Succeed())

		integrationPipelineRunComponent.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				CompletionTime: &metav1.Time{Time: time.Now()},
				ChildReferences: []tektonv1beta1.ChildStatusReference{
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
		err = k8sClient.Delete(ctx, hasEnv)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, failedTaskRun)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	When("NewAdapter is called", func() {
		It("creates and return a new adapter", func() {
			Expect(reflect.TypeOf(NewAdapter(integrationPipelineRunComponent, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
		})
	})

	When("NewAdapter is created", func() {
		BeforeEach(func() {
			adapter = createAdapter()
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
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
					ContextKey: loader.EnvironmentContextKey,
					Resource:   hasEnv,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *hasComp2},
				},
			})
		})

		It("ensures the global component list unchanged and compositeSnapshot shouldn't be created ", func() {
			// Check if the global component list changed in the meantime and create a composite snapshot if it did.
			compositeSnapshot, err := adapter.createCompositeSnapshotsIfConflictExists(hasApp, hasComp, hasSnapshot)
			Expect(err).To(BeNil())
			Expect(compositeSnapshot).To(BeNil())
		})

		It("ensures the component Snapshot is marked as invalid when the global component list is changed", func() {
			createdSnapshot, err := adapter.loader.GetSnapshotFromPipelineRun(adapter.client, adapter.context, integrationPipelineRunComponent)
			Expect(err).To(BeNil())
			Expect(createdSnapshot).ToNot(BeNil())

			// A new component is added to the application in the meantime, to change the global component list.
			hasCompNew = &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "component-sample-2",
					Namespace: "default",
				},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ComponentName:  "component-sample-2",
					Application:    hasApp.Name,
					ContainerImage: SampleImage,
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
			Expect(k8sClient.Create(ctx, hasCompNew)).Should(Succeed())
			hasCompNew.Status = applicationapiv1alpha1.ComponentStatus{
				LastBuiltCommit: "lastbuildcommit",
			}
			Expect(k8sClient.Status().Update(ctx, hasCompNew)).Should(Succeed())

			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
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
					ContextKey: loader.EnvironmentContextKey,
					Resource:   hasEnv,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *hasCompNew},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario},
				},
				{
					ContextKey: loader.AllSnapshotsContextKey,
					Resource:   []applicationapiv1alpha1.Snapshot{},
				},
			})

			result, err := adapter.EnsureSnapshotPassedAllTests()
			Expect(!result.RequeueRequest && !result.CancelRequest && err == nil).To(BeTrue())
			condition := meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("Invalid"))
			err = k8sClient.Delete(ctx, hasCompNew)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures the global component list is changed and compositeSnapshot should be created", func() {
			createdSnapshot, err := adapter.loader.GetSnapshotFromPipelineRun(adapter.client, adapter.context, integrationPipelineRunComponent)
			Expect(err).To(BeNil())
			Expect(createdSnapshot).ToNot(BeNil())

			// A new component is added to the application in the meantime, to change the global component list.
			hasCompNew = &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "component-sample-2",
					Namespace: "default",
				},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ComponentName:  "component-sample-2",
					Application:    hasApp.Name,
					ContainerImage: SampleImage,
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
			Expect(k8sClient.Create(ctx, hasCompNew)).Should(Succeed())
			hasCompNew.Status = applicationapiv1alpha1.ComponentStatus{
				LastBuiltCommit: "lastbuildcommit",
			}
			Expect(k8sClient.Status().Update(ctx, hasCompNew)).Should(Succeed())

			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
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
					ContextKey: loader.EnvironmentContextKey,
					Resource:   hasEnv,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *hasCompNew},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario},
				},
				{
					ContextKey: loader.AllSnapshotsContextKey,
					Resource:   []applicationapiv1alpha1.Snapshot{},
				},
			})
			applicationComponents, err := adapter.loader.GetAllApplicationComponents(k8sClient, adapter.context, hasApp)
			Expect(err == nil && len(*applicationComponents) > 1).To(BeTrue())

			// Check if the global component list changed in the meantime and create a composite snapshot if it did.
			compositeSnapshot, err := adapter.createCompositeSnapshotsIfConflictExists(hasApp, hasComp, createdSnapshot)
			fmt.Fprintf(GinkgoWriter, "compositeSnapshot.Name: %v\n", compositeSnapshot.Name)
			Expect(err).To(BeNil())
			Expect(compositeSnapshot).NotTo(BeNil())
			Eventually(func() error {
				err := k8sClient.Get(adapter.context, types.NamespacedName{
					Name:      compositeSnapshot.Name,
					Namespace: compositeSnapshot.Namespace,
				}, compositeSnapshot)
				return err
			}, time.Second*10).ShouldNot(HaveOccurred())

			// Check if the composite snapshot that was already created above was correctly detected and returned.
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
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
					ContextKey: loader.EnvironmentContextKey,
					Resource:   hasEnv,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *hasCompNew},
				},
				{
					ContextKey: loader.AllSnapshotsContextKey,
					Resource:   []applicationapiv1alpha1.Snapshot{*compositeSnapshot},
				},
			})
			existingCompositeSnapshot, err := adapter.createCompositeSnapshotsIfConflictExists(hasApp, hasComp, createdSnapshot)
			Expect(err).To(BeNil())
			Expect(existingCompositeSnapshot).NotTo(BeNil())
			Expect(existingCompositeSnapshot.Name).To(Equal(compositeSnapshot.Name))

			componentSource, _ := adapter.getComponentSourceFromSnapshotComponent(existingCompositeSnapshot, hasCompNew)
			Expect(componentSource.GitSource.Revision).To(Equal("lastbuildcommit"))
			err = k8sClient.Delete(ctx, hasCompNew)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})
	})

	When("Snapshot already exists", func() {
		BeforeEach(func() {
			adapter = NewAdapter(integrationPipelineRunComponent, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient, ctx)
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
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
					ContextKey: loader.EnvironmentContextKey,
					Resource:   hasEnv,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})
			existingSnapshot, err := adapter.loader.GetSnapshotFromPipelineRun(adapter.client, adapter.context, integrationPipelineRunComponent)
			Expect(err).To(BeNil())
			Expect(existingSnapshot).ToNot(BeNil())
		})

		It("ensures Snapshot passed all tests", func() {
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
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
					Resource:   []tektonv1beta1.PipelineRun{*integrationPipelineRunComponent},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario},
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})
			Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

			result, err := adapter.EnsureSnapshotPassedAllTests()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			integrationTestScenarios, err := adapter.loader.GetRequiredIntegrationTestScenariosForApplication(k8sClient, adapter.context, hasApp)
			Expect(err).To(BeNil())
			Expect(len(*integrationTestScenarios) > 0).To(BeTrue())

			integrationPipelineRuns, err := adapter.getAllPipelineRunsForSnapshot(hasSnapshot, integrationTestScenarios)
			Expect(err).To(BeNil())
			Expect(len(*integrationPipelineRuns) > 0).To(BeTrue())

			allIntegrationPipelineRunsPassed, err := adapter.determineIfAllIntegrationPipelinesPassed(integrationPipelineRuns)
			Expect(err).To(BeNil())
			Expect(allIntegrationPipelineRunsPassed).To(BeTrue())

			Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeTrue())
		})

		It("ensures test status in snapshot is updated to passed", func() {
			result, err := adapter.EnsureStatusReportedInSnapshot()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).To(BeNil())

			detail, ok := statuses.GetScenarioStatus(integrationTestScenario.Name)
			Expect(ok).To(BeTrue())
			Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusTestPassed))
		})

		When("integration pipeline failed", func() {

			BeforeEach(func() {
				//Create one failed scenario and its failed pipelineRun
				integrationTestScenarioFailed = &v1beta1.IntegrationTestScenario{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-fail",
						Namespace: "default",

						Labels: map[string]string{
							"test.appstudio.openshift.io/optional": "false",
						},
					},
					Spec: v1beta1.IntegrationTestScenarioSpec{
						Application: hasApp.Name,
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
				Expect(k8sClient.Create(ctx, integrationTestScenarioFailed)).Should(Succeed())

				integrationPipelineRunComponentFailed = &tektonv1beta1.PipelineRun{
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
							"appstudio.openshift.io/environment":              hasEnv.Name,
							"appstudio.openshift.io/application":              hasApp.Name,
							"appstudio.openshift.io/component":                hasComp.Name,
						},
						Annotations: map[string]string{
							"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
						},
					},
					Spec: tektonv1beta1.PipelineRunSpec{
						PipelineRef: &tektonv1beta1.PipelineRef{
							Name:   "component-pipeline-fail",
							Bundle: "quay.io/kpavic/test-bundle:component-pipeline-fail",
						},
					},
				}

				Expect(k8sClient.Create(ctx, integrationPipelineRunComponentFailed)).Should(Succeed())

				integrationPipelineRunComponentFailed.Status = tektonv1beta1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
						CompletionTime: &metav1.Time{Time: time.Now().Add(5 * time.Minute)},
						ChildReferences: []tektonv1beta1.ChildStatusReference{
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

				adapter = NewAdapter(integrationPipelineRunComponentFailed, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient, ctx)
				adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
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
						Resource:   []tektonv1beta1.PipelineRun{*integrationPipelineRunComponent, *integrationPipelineRunComponentFailed},
					},
					{
						ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
						Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioFailed},
					},
					{
						ContextKey: loader.ApplicationComponentsContextKey,
						Resource:   []applicationapiv1alpha1.Component{*hasComp},
					},
				})
			})

			AfterEach(func() {
				err := k8sClient.Delete(ctx, integrationPipelineRunComponentFailed)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
				err = k8sClient.Delete(ctx, integrationTestScenarioFailed)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})

			It("ensures Snapshot failed", func() {
				result, err := adapter.EnsureSnapshotPassedAllTests()
				Expect(!result.CancelRequest && err == nil).To(BeTrue())

				integrationTestScenarios, err := adapter.loader.GetRequiredIntegrationTestScenariosForApplication(k8sClient, adapter.context, hasApp)
				Expect(err).To(BeNil())
				Expect(len(*integrationTestScenarios) > 0).To(BeTrue())

				integrationPipelineRuns, err := adapter.getAllPipelineRunsForSnapshot(hasSnapshot, integrationTestScenarios)
				Expect(err).To(BeNil())
				Expect(len(*integrationPipelineRuns) > 0).To(BeTrue())

				allIntegrationPipelineRunsPassed, err := adapter.determineIfAllIntegrationPipelinesPassed(integrationPipelineRuns)
				Expect(err).To(BeNil())
				Expect(allIntegrationPipelineRunsPassed).To(BeFalse())

				Expect(meta.IsStatusConditionFalse(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeTrue())

			})

			It("ensures test status in snapshot is updated to failed", func() {
				result, err := adapter.EnsureStatusReportedInSnapshot()
				Expect(!result.CancelRequest && err == nil).To(BeTrue())

				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
				Expect(err).To(BeNil())

				detail, ok := statuses.GetScenarioStatus(integrationTestScenarioFailed.Name)
				Expect(ok).To(BeTrue())
				Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusTestFail))
				Expect(detail.TestPipelineRunName).To(Equal(integrationPipelineRunComponentFailed.Name))

			})
		})

	})

	When("EnsureStatusReported is called", func() {
		It("ensures status is reported for integration PipelineRuns", func() {
			adapter = createAdapter()
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
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
					ContextKey: loader.EnvironmentContextKey,
					Resource:   hasEnv,
				},
				{
					ContextKey: loader.PipelineRunsContextKey,
					Resource:   []tektonv1beta1.PipelineRun{*integrationPipelineRunComponent},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario},
				},
			})

			adapter.pipelineRun = &tektonv1beta1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipelinerun-status-sample",
					Namespace: "default",
					Labels: map[string]string{
						"appstudio.openshift.io/application":              "test-application",
						"appstudio.openshift.io/component":                "devfile-sample-go-basic",
						"appstudio.openshift.io/snapshot":                 "test-application-s8tnj",
						"test.appstudio.openshift.io/scenario":            "example-pass",
						"pac.test.appstudio.openshift.io/state":           "started",
						"pac.test.appstudio.openshift.io/sender":          "foo",
						"pac.test.appstudio.openshift.io/check-run-id":    "9058825284",
						"pac.test.appstudio.openshift.io/branch":          "main",
						"pac.test.appstudio.openshift.io/url-org":         "devfile-sample",
						"pac.test.appstudio.openshift.io/original-prname": "devfile-sample-go-basic-on-pull-request",
						"pac.test.appstudio.openshift.io/url-repository":  "devfile-sample-go-basic",
						"pac.test.appstudio.openshift.io/repository":      "devfile-sample-go-basic",
						"pac.test.appstudio.openshift.io/sha":             "12a4a35ccd08194595179815e4646c3a6c08bb77",
						"pac.test.appstudio.openshift.io/git-provider":    "github",
						"pac.test.appstudio.openshift.io/event-type":      "pull_request",
						"pipelines.appstudio.openshift.io/type":           "test",
					},
					Annotations: map[string]string{
						"pac.test.appstudio.openshift.io/on-target-branch": "[main,master]",
						"pac.test.appstudio.openshift.io/repo-url":         "https://github.com/devfile-samples/devfile-sample-go-basic",
						"pac.test.appstudio.openshift.io/sha-title":        "Appstudio update devfile-sample-go-basic",
						"pac.test.appstudio.openshift.io/git-auth-secret":  "pac-gitauth-zjib",
						"pac.test.appstudio.openshift.io/pull-request":     "16",
						"pac.test.appstudio.openshift.io/on-event":         "[pull_request]",
						"pac.test.appstudio.openshift.io/installation-id":  "30353543",
					},
				},
				Spec: tektonv1beta1.PipelineRunSpec{
					PipelineRef: &tektonv1beta1.PipelineRef{
						Name:   "component-pipeline-pass",
						Bundle: "quay.io/kpavic/test-bundle:component-pipeline-pass",
					},
				},
			}

			result, err := adapter.EnsureStatusReported()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			Expect(statusReporter.Called).To(BeTrue())

			statusAdapter.GetReportersError = errors.New("GetReportersError")

			result, err = adapter.EnsureStatusReported()
			Expect(result.RequeueRequest && err != nil && err.Error() == "GetReportersError").To(BeTrue())

			statusAdapter.GetReportersError = nil
			statusReporter.ReportStatusError = errors.New("ReportStatusError")

			result, err = adapter.EnsureStatusReported()
			Expect(result.RequeueRequest && err != nil && err.Error() == "ReportStatusError").To(BeTrue())
		})
	})

	When("EnsureEphemeralEnvironmentsCleanedUp is called", func() {
		BeforeEach(func() {
			deploymentTargetClass = &applicationapiv1alpha1.DeploymentTargetClass{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "dtcls" + "-",
				},
				Spec: applicationapiv1alpha1.DeploymentTargetClassSpec{
					Provisioner: applicationapiv1alpha1.Provisioner_Devsandbox,
				},
			}
			Expect(k8sClient.Create(ctx, deploymentTargetClass)).Should(Succeed())

			deploymentTarget = &applicationapiv1alpha1.DeploymentTarget{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "dt" + "-",
					Namespace:    "default",
				},
				Spec: applicationapiv1alpha1.DeploymentTargetSpec{
					DeploymentTargetClassName: "dtcls-name",
					KubernetesClusterCredentials: applicationapiv1alpha1.DeploymentTargetKubernetesClusterCredentials{
						DefaultNamespace:           "default",
						APIURL:                     "https://url",
						ClusterCredentialsSecret:   "secret-sample",
						AllowInsecureSkipTLSVerify: false,
					},
				},
			}
			Expect(k8sClient.Create(ctx, deploymentTarget)).Should(Succeed())

			deploymentTargetClaim = &applicationapiv1alpha1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "dtc" + "-",
					Namespace:    "default",
				},
				Spec: applicationapiv1alpha1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: applicationapiv1alpha1.DeploymentTargetClassName("dtcls-name"),
					TargetName:                deploymentTarget.Name,
				},
			}
			Expect(k8sClient.Create(ctx, deploymentTargetClaim)).Should(Succeed())

			snapshotEnvironmentBinding = &applicationapiv1alpha1.SnapshotEnvironmentBinding{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "binding" + "-",
					Namespace:    "default",
				},
				Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
					Application: hasApp.Name,
					Environment: hasEnv.Name,
					Snapshot:    hasSnapshot.Name,
					Components:  []applicationapiv1alpha1.BindingComponent{},
				},
			}
			Expect(k8sClient.Create(ctx, snapshotEnvironmentBinding)).Should(Succeed())

			hasEnv.Spec.Configuration.Target = applicationapiv1alpha1.EnvironmentTarget{
				DeploymentTargetClaim: applicationapiv1alpha1.DeploymentTargetClaimConfig{
					ClaimName: deploymentTargetClaim.Name,
				},
			}
			Expect(k8sClient.Update(ctx, hasEnv)).Should(Succeed())
		})
		It("ensures ephemeral environment is deleted for the given pipelineRun ", func() {
			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(integrationPipelineRunComponent, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient, ctx)
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.EnvironmentContextKey,
					Resource:   hasEnv,
				},
				{
					ContextKey: loader.PipelineRunsContextKey,
					Resource:   []tektonv1beta1.PipelineRun{*integrationPipelineRunComponent},
				},
				{
					ContextKey: loader.SnapshotEnvironmentBindingContextKey,
					Resource:   snapshotEnvironmentBinding,
				},
				{
					ContextKey: loader.DeploymentTargetContextKey,
					Resource:   deploymentTarget,
				},
				{
					ContextKey: loader.DeploymentTargetClaimContextKey,
					Resource:   deploymentTargetClaim,
				},
				{
					ContextKey: loader.DeploymentTargetClassContextKey,
					Resource:   deploymentTargetClass,
				},
			})

			dtc, _ := adapter.loader.GetDeploymentTargetClaimForEnvironment(k8sClient, adapter.context, hasEnv)
			Expect(dtc).NotTo(BeNil())

			dt, _ := adapter.loader.GetDeploymentTargetForDeploymentTargetClaim(k8sClient, adapter.context, dtc)
			Expect(dt).NotTo(BeNil())

			binding, _ := adapter.loader.FindExistingSnapshotEnvironmentBinding(k8sClient, adapter.context, hasApp, hasEnv)
			Expect(binding).NotTo(BeNil())

			result, err := adapter.EnsureEphemeralEnvironmentsCleanedUp()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			expectedLogEntry := "DeploymentTargetClaim deleted"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			expectedLogEntry = "Ephemeral environment is deleted and its owning SnapshotEnvironmentBinding is in the process of being deleted"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})
	})

	createAdapter = func() *Adapter {
		adapter = NewAdapter(integrationPipelineRunComponent, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient, ctx)
		statusReporter = &MockStatusReporter{}
		statusAdapter = &MockStatusAdapter{Reporter: statusReporter}
		adapter.status = statusAdapter
		return adapter
	}
})
