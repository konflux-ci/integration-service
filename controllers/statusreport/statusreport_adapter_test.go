/*
Copyright 2023.

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

package statusreport

import (
	"bytes"
	"context"
	"fmt"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/tonglil/buflogr"
	"k8s.io/apimachinery/pkg/api/meta"
	"os"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/loader"
	intgteststat "github.com/redhat-appstudio/integration-service/pkg/integrationteststatus"
	"github.com/redhat-appstudio/integration-service/status"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"k8s.io/apimachinery/pkg/api/errors"
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
	r.ReportStatusError = nil
	return r.ReportStatusError
}

func (a *MockStatusAdapter) GetReporters(object client.Object) ([]status.Reporter, error) {
	return []status.Reporter{a.Reporter}, a.GetReportersError
}

var _ = Describe("Snapshot Adapter", Ordered, func() {
	var (
		adapter        *Adapter
		logger         helpers.IntegrationLogger
		statusAdapter  *MockStatusAdapter
		statusReporter *MockStatusReporter
		buf            bytes.Buffer

		hasComp                 *applicationapiv1alpha1.Component
		hasComp2                *applicationapiv1alpha1.Component
		hasApp                  *applicationapiv1alpha1.Application
		hasSnapshot             *applicationapiv1alpha1.Snapshot
		hasPRSnapshot           *applicationapiv1alpha1.Snapshot
		integrationTestScenario *v1beta1.IntegrationTestScenario
	)
	const (
		SampleRepoLink = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		SampleImage    = "quay.io/redhat-appstudio/sample-image@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		SampleCommit   = "a2ba645d50e471d5f084b"
		SampleRevision = "random-value"
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

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-sample",
				Application:    "application-sample",
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
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())

		hasComp2 = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "another-component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "another-component-sample",
				Application:    "application-sample",
				ContainerImage: SampleImage,
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: "lastbuiltcommit",
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
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp2)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
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

		hasPRSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-PR-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                         "component",
					gitops.SnapshotComponentLabel:                    "component-sample",
					"build.appstudio.redhat.com/pipeline":            "enterprise-contract",
					gitops.PipelineAsCodeEventTypeLabel:              "pull_request",
					"pac.test.appstudio.openshift.io/url-org":        "testorg",
					"pac.test.appstudio.openshift.io/url-repository": "testrepo",
					"pac.test.appstudio.openshift.io/sha":            "testsha",
					gitops.PipelineAsCodeGitProviderLabel:            gitops.PipelineAsCodeGitHubProviderType,
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation:   "123",
					"build.appstudio.redhat.com/commit_sha":         "6c65b2fcaea3e1a0a92476c8b5dc89e92a85f025",
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
					gitops.SnapshotTestsStatusAnnotation:            "[{\"scenario\":\"scenario-1\",\"status\":\"EnvironmentProvisionError\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"Failed to find deploymentTargetClass with right provisioner for copy of existingEnvironment\"}]",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComp.Name,
						ContainerImage: SampleImage,
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{
									Revision: SampleRevision,
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		// enable feature flag for testing
		err := os.Setenv(FeatureFlagStatusReprotingEnabled, "yes")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasPRSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	When("adapter is created", func() {
		It("can create a new Adapter instance", func() {
			Expect(reflect.TypeOf(NewAdapter(hasSnapshot, hasApp, logger, loader.NewMockLoader(), k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensures the statusReport is called", func() {
			adapter = NewAdapter(hasPRSnapshot, hasApp, logger, loader.NewMockLoader(), k8sClient, ctx)
			statusReporter = &MockStatusReporter{}
			statusAdapter = &MockStatusAdapter{Reporter: statusReporter}
			adapter.status = statusAdapter
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasPRSnapshot,
				},
			})
			result, err := adapter.EnsureSnapshotTestStatusReported()
			fmt.Fprintf(GinkgoWriter, "-------err: %v\n", err)
			fmt.Fprintf(GinkgoWriter, "-------result: %v\n", result)
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
		})
	})

	When("New Adapter is created for a push-type Snapshot that passed all tests", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).To(BeNil())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestPassed, "testDetails")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(hasSnapshot, statuses, k8sClient, ctx)
			Expect(err).To(BeNil())

			adapter = NewAdapter(hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient, ctx)
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
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
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
		})

		It("ensures Snapshot passed all tests", func() {
			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			Expect(meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).ToNot(BeNil())
			Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeTrue())

			Expect(meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)).ToNot(BeNil())
			Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)).To(BeTrue())

			expectedLogEntry := "Snapshot integration status condition marked as finished, all testing pipelines completed"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Snapshot integration status condition marked as passed, all Integration PipelineRuns succeeded"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("ensures the global component list unchanged and compositeSnapshot shouldn't be created ", func() {
			// Check if the global component list changed in the meantime and create a composite snapshot if it did.
			compositeSnapshot, err := adapter.createCompositeSnapshotsIfConflictExists(hasApp, hasComp, hasSnapshot)
			Expect(err).To(BeNil())
			Expect(compositeSnapshot).To(BeNil())
		})

	})

	When("New Adapter is created for a push-type Snapshot that failed one of the tests", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).To(BeNil())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestFail, "Failed test")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(hasSnapshot, statuses, k8sClient, ctx)
			Expect(err).To(BeNil())

			adapter = NewAdapter(hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient, ctx)
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
					ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario},
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})
			Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensures Snapshot failed", func() {
			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			Expect(meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).ToNot(BeNil())
			Expect(meta.IsStatusConditionFalse(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeTrue())

			Expect(meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)).ToNot(BeNil())
			Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)).To(BeTrue())

			expectedLogEntry := "Snapshot integration status condition marked as finished, all testing pipelines completed"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Snapshot integration status condition marked as failed, some tests within Integration PipelineRuns failed"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})
	})

	When("New Adapter is created for a push-type Snapshot that passed all tests, but has out-of date components", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).To(BeNil())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestPassed, "testDetails")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(hasSnapshot, statuses, k8sClient, ctx)
			Expect(err).To(BeNil())

			adapter = NewAdapter(hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient, ctx)
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
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *hasComp2},
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
		})

		It("ensures the component Snapshot is marked as invalid when the global component list is changed", func() {
			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(!result.RequeueRequest && !result.CancelRequest && err == nil).To(BeTrue())

			condition := meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)
			Expect(condition.Status).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("Invalid"))

			expectedLogEntry := "The global component list has changed in the meantime, marking snapshot as Invalid"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "CompositeSnapshot created"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			compositeSnapshots := &applicationapiv1alpha1.SnapshotList{}
			opts := []client.ListOption{
				client.InNamespace(hasApp.Namespace),
				client.MatchingLabels{
					gitops.SnapshotTypeLabel: gitops.SnapshotCompositeType,
				},
			}
			Eventually(func() error {
				if err := k8sClient.List(adapter.context, compositeSnapshots, opts...); err != nil {
					return err
				}

				if expected, got := 1, len(compositeSnapshots.Items); expected != got {
					return fmt.Errorf("found %d composite Snapshots, expected: %d", got, expected)
				}
				return nil
			}, time.Second*10).Should(BeNil())

			compositeSnapshot := &compositeSnapshots.Items[0]
			Expect(compositeSnapshot).NotTo(BeNil())

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
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *hasComp2},
				},
				{
					ContextKey: loader.AllSnapshotsContextKey,
					Resource:   []applicationapiv1alpha1.Snapshot{*compositeSnapshot},
				},
			})
			existingCompositeSnapshot, err := adapter.createCompositeSnapshotsIfConflictExists(hasApp, hasComp, hasSnapshot)
			Expect(err).To(BeNil())
			Expect(existingCompositeSnapshot).NotTo(BeNil())
			Expect(existingCompositeSnapshot.Name).To(Equal(compositeSnapshot.Name))

			expectedLogEntry = "Found existing composite Snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			componentSource, _ := adapter.getComponentSourceFromSnapshotComponent(existingCompositeSnapshot, hasComp2)
			Expect(componentSource.GitSource.Revision).To(Equal("lastbuiltcommit"))

			componentImagePullSpec, _ := adapter.getImagePullSpecFromSnapshotComponent(existingCompositeSnapshot, hasComp2)
			Expect(componentImagePullSpec).To(Equal(SampleImage))
		})
	})
})
