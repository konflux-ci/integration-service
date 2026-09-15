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
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/tonglil/buflogr"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/loader"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/status"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// wireClientMocks points the adapter at a mock client and context (call it after building the
// adapter, and pass every loader mock the spec needs — it replaces the adapter's context).
func wireClientMocks(a *Adapter, loaderMocks []toolkit.MockData, clientMocks []toolkit.ClientCallMock) {
	mockCtx, mockClient := toolkit.GetMockedContextWithClient(ctx, k8sClient, loaderMocks, clientMocks)
	a.client = mockClient
	a.context = mockCtx
}

var _ = Describe("Snapshot Adapter", Ordered, func() {
	var (
		adapter      *Adapter
		logger       helpers.IntegrationLogger
		buf          bytes.Buffer
		mockReporter *status.MockReporterInterface
		mockStatus   *status.MockStatusInterface

		hasComp                 *applicationapiv1alpha1.Component
		hasComp2                *applicationapiv1alpha1.Component
		hasApp                  *applicationapiv1alpha1.Application
		hasSnapshot             *applicationapiv1alpha1.Snapshot
		hasPRSnapshot           *applicationapiv1alpha1.Snapshot
		hasComSnapshot2         *applicationapiv1alpha1.Snapshot
		hasComSnapshot3         *applicationapiv1alpha1.Snapshot
		groupSnapshot           *applicationapiv1alpha1.Snapshot
		githubSnapshot          *applicationapiv1alpha1.Snapshot
		integrationTestScenario *v1beta2.IntegrationTestScenario
	)
	const (
		SampleRepoLink            = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		SampleImage               = "quay.io/redhat-appstudio/sample-image@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		SampleDigest              = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		SampleCommit              = "a2ba645d50e471d5f084b"
		SampleRevision            = "random-value"
		hasComSnapshot2Name       = "hascomsnapshot2-sample"
		hasComSnapshot3Name       = "hascomsnapshot3-sample"
		prGroup                   = "feature1"
		prGroupSha                = "feature1hash"
		plrstarttime        int64 = 1775992257000 // milliseconds (was 1775992257 seconds)

		inProgressStatus            = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
		inProgressStatusNoStartTime = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
		testPassedStatus            = "[{\"scenario\":\"scenario1\",\"status\":\"TestPassed\",\"testPipelineRunName\":\"plr-x\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"ok\"}]"
		olderReporterStatus         = "{\"scenarios\":{\"scenario1-snapshot-pr-sample\":{\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\"}}}"
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

		hasComSnapshot2 = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hasComSnapshot2Name,
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                         gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel:                    hasComSnapshot2Name,
					gitops.PipelineAsCodeEventTypeLabel:              gitops.PipelineAsCodePullRequestType,
					gitops.PRGroupHashLabel:                          prGroupSha,
					"pac.test.appstudio.openshift.io/url-org":        "testorg",
					"pac.test.appstudio.openshift.io/url-repository": "testrepo",
					gitops.PipelineAsCodeSHALabel:                    "sha",
					gitops.PipelineAsCodePullRequestAnnotation:       "1",
				},
				Annotations: map[string]string{
					"test.appstudio.openshift.io/pr-last-update":  "2023-08-26T17:57:50+02:00",
					gitops.BuildPipelineRunStartTime:              strconv.FormatInt(plrstarttime+100000, 10), // +100 seconds = +100000 milliseconds
					gitops.PRGroupAnnotation:                      prGroup,
					gitops.PipelineAsCodeGitProviderAnnotation:    "github",
					gitops.PipelineAsCodeInstallationIDAnnotation: "123",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComSnapshot3Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
					{
						Name:           hasComSnapshot2Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComSnapshot2)).Should(Succeed())

		hasComSnapshot3 = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hasComSnapshot3Name,
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                         gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel:                    hasComSnapshot3Name,
					gitops.PipelineAsCodeEventTypeLabel:              gitops.PipelineAsCodePullRequestType,
					gitops.PRGroupHashLabel:                          prGroupSha,
					"pac.test.appstudio.openshift.io/url-org":        "testorg",
					"pac.test.appstudio.openshift.io/url-repository": "testrepo",
					gitops.PipelineAsCodeSHALabel:                    "sha",
					gitops.PipelineAsCodePullRequestAnnotation:       "1",
				},
				Annotations: map[string]string{
					"test.appstudio.openshift.io/pr-last-update":  "2023-08-26T17:57:50+02:00",
					gitops.BuildPipelineRunStartTime:              strconv.FormatInt(plrstarttime+200000, 10), // +200 seconds = +200000 milliseconds
					gitops.PRGroupAnnotation:                      prGroup,
					gitops.PipelineAsCodeGitProviderAnnotation:    "github",
					gitops.PipelineAsCodePullRequestAnnotation:    "1",
					gitops.PipelineAsCodeInstallationIDAnnotation: "123",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComSnapshot2Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
					{
						Name:           hasComSnapshot3Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComSnapshot3)).Should(Succeed())

		groupSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "groupsnapshot",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:            gitops.SnapshotGroupType,
					gitops.PipelineAsCodeEventTypeLabel: gitops.PipelineAsCodePullRequestType,
					gitops.PRGroupHashLabel:             prGroupSha,
				},
				Annotations: map[string]string{
					gitops.PRGroupAnnotation:             prGroup,
					gitops.GroupSnapshotInfoAnnotation:   "[{\"namespace\":\"default\",\"component\":\"component1-sample\",\"buildPipelineRun\":\"\",\"snapshot\":\"hascomsnapshot2-sample\"},{\"namespace\":\"default\",\"component\":\"component3-sample\",\"buildPipelineRun\":\"\",\"snapshot\":\"hascomsnapshot3-sample\"}]",
					gitops.SnapshotTestsStatusAnnotation: "[{\"scenario\":\"scenario-1\",\"status\":\"EnvironmentProvisionError\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"Failed to find deploymentTargetClass with right provisioner for copy of existingEnvironment\"}]",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComSnapshot2Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
					{
						Name:           hasComSnapshot3Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, groupSnapshot)).Should(Succeed())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp2)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, groupSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComSnapshot2)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComSnapshot3)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	BeforeEach(func() {
		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:            "component",
					gitops.SnapshotComponentLabel:       hasComp.Name,
					gitops.PipelineAsCodeEventTypeLabel: gitops.PipelineAsCodePushType,
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

		hasPRSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-pr-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                         "component",
					gitops.SnapshotComponentLabel:                    "component-sample",
					"build.appstudio.redhat.com/pipeline":            "enterprise-contract",
					gitops.PipelineAsCodeEventTypeLabel:              "pull_request",
					gitops.PipelineAsCodePullRequestAnnotation:       "1",
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
					gitops.PipelineAsCodeGitProviderAnnotation:      gitops.PipelineAsCodeGitHubProviderType,
					gitops.PRGroupCreationAnnotation:                gitops.FailedToCreateGroupSnapshotMsg,
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
		Expect(k8sClient.Create(ctx, hasPRSnapshot)).Should(Succeed())

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
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasPRSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	When("adapter is created", func() {
		It("can create a new Adapter instance", func() {
			Expect(reflect.TypeOf(NewAdapterWithApplication(ctx, hasSnapshot, hasApp, logger, loader.NewMockLoader(), k8sClient))).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensures the statusReport is called for component snapshot", func() {

			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus := status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter)
			mockStatus.EXPECT().GetReporter(gomock.Any()).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(1)

			mockScenarios := []v1beta2.IntegrationTestScenario{}
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, logger, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasPRSnapshot,
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   mockScenarios,
				},
			})
			result, err := adapter.EnsureSnapshotTestStatusReportedToGitProvider()
			fmt.Fprintf(GinkgoWriter, "-------err: %v\n", err)
			fmt.Fprintf(GinkgoWriter, "-------result: %v\n", result)
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
		})

		It("ensures the statusReport is called for group snapshot", func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus := status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter)
			mockStatus.EXPECT().GetReporter(gomock.Any()).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(1)

			mockScenarios := []v1beta2.IntegrationTestScenario{}
			adapter = NewAdapterWithApplication(ctx, groupSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   mockScenarios,
				},
			})
			result, err := adapter.EnsureSnapshotTestStatusReportedToGitProvider()
			fmt.Fprintf(GinkgoWriter, "-------test: %v\n", buf.String())
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
		})
	})

	When("New Adapter is created for a push-type Snapshot that passed all tests [APPLICATION]", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestPassed, "testDetails")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, hasSnapshot, statuses, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			adapter = NewAdapterWithApplication(ctx, hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
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
					ContextKey: loader.EnvironmentContextKey,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
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

			expectedLogEntry := "Snapshot integration status condition is finished since all testing pipelines completed"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Snapshot integration status condition marked as passed, all of 1 required Integration PipelineRuns succeeded"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("testing function findUntriggeredIntegrationTestFromStatus ", func() {

			integrationTestScenarioTest := &v1beta2.IntegrationTestScenario{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-pass-test",
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

			// Check when all integrationTestScenarion exist in testStatuses of the snapshot
			testStatuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			integrationTestScenarios, err := adapter.loader.GetRequiredIntegrationTestScenariosForSnapshotApplication(adapter.context, adapter.client, adapter.application, adapter.snapshot)
			Expect(err).ToNot(HaveOccurred())
			result := adapter.findUntriggeredIntegrationTestFromStatus(integrationTestScenarios, testStatuses)
			Expect(result).To(BeEmpty())

			// Check when we have one integrationTestScenario not exist in testStatuses of the snapshot
			*integrationTestScenarios = append(*integrationTestScenarios, *integrationTestScenarioTest)

			result = adapter.findUntriggeredIntegrationTestFromStatus(integrationTestScenarios, testStatuses)
			Expect(result).To(BeEquivalentTo("example-pass-test"))

		})

	})

	When("New Adapter is created for a push-type Snapshot that failed one of the tests [APPLICATION]", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestFail, "Failed test")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, hasSnapshot, statuses, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			adapter = NewAdapterWithApplication(ctx, hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
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
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
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

			expectedLogEntry := "Snapshot integration status condition is finished since all testing pipelines completed"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Snapshot integration status condition marked as failed, some tests within Integration PipelineRuns failed"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})
	})

	When("New Adapter is created for a push-type Snapshot that has no tests [APPLICATION]", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestFail, "Failed test")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, hasSnapshot, statuses, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			adapter = NewAdapterWithApplication(ctx, hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
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
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{},
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})
			Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensures tests report as skipped and overall result is success", func() {
			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			Expect(meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).ToNot(BeNil())
			Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeTrue())

			Expect(meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)).ToNot(BeNil())
			Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)).To(BeTrue())

			expectedLogEntry := "Snapshot integration status condition is finished since there are no required testing pipelines defined for its application"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Snapshot integration status condition marked as passed, all of 0 required Integration PipelineRuns succeeded"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})
	})

	When("New Adapter is created for a push-type Snapshot that passed all tests, but has out-of date components", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestPassed, "testDetails")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, hasSnapshot, statuses, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			adapter = NewAdapterWithApplication(ctx, hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
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
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *hasComp2},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
				{
					ContextKey: loader.AllSnapshotsContextKey,
					Resource:   []applicationapiv1alpha1.Snapshot{},
				},
			})
		})

	})

	When("New Adapter is created for a manual override Snapshot", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockStatus.EXPECT().GetReporter(gomock.Any()).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(0)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			overrideSnapshot := hasSnapshot.DeepCopy()
			overrideSnapshot.Labels[gitops.SnapshotTypeLabel] = gitops.SnapshotOverrideType
			overrideSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation]

			adapter = NewAdapterWithApplication(ctx, overrideSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensures test status reporting is skipped", func() {
			result, err := adapter.EnsureSnapshotTestStatusReportedToGitProvider()
			Expect(result.CancelRequest).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("New Adapter is created for a manual Snapshot", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockStatus.EXPECT().GetReporter(gomock.Any()).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(0)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			manualSnapshot := hasSnapshot.DeepCopy()
			manualSnapshot.Labels[gitops.SnapshotTypeLabel] = ""
			manualSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation]

			adapter = NewAdapterWithApplication(ctx, manualSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensures test status reporting is skipped", func() {
			result, err := adapter.EnsureSnapshotTestStatusReportedToGitProvider()
			Expect(result.CancelRequest).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("New Adapter is created for a canceled PR Snapshot", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			ctrl := gomock.NewController(GinkgoT())
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockStatus.EXPECT().GetReporter(gomock.Any()).Times(0)

			canceledSnapshot := hasPRSnapshot.DeepCopy()
			condition := metav1.Condition{
				Type:    gitops.AppStudioIntegrationStatusCondition,
				Status:  metav1.ConditionTrue,
				Reason:  gitops.AppStudioIntegrationStatusCanceled,
				Message: "Snapshot canceled/superseded",
			}
			meta.SetStatusCondition(&canceledSnapshot.Status.Conditions, condition)

			adapter = NewAdapterWithApplication(ctx, canceledSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensures test status reporting is skipped", func() {
			result, err := adapter.EnsureSnapshotTestStatusReportedToGitProvider()
			Expect(result.CancelRequest).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("testing ReportSnapshotStatus", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}

			githubSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/git-provider": "github",
					},
				},
			}

			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			mockStatus = status.NewMockStatusInterface(ctrl)
		})
		It("doesn't report anything when there are not test results", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(0)   // without test results reporter shouldn't be initialized
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0) // without test results reported shouldn't report status

			adapter = NewAdapterWithApplication(ctx, githubSnapshot, hasApp, logger, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			statusCode, err := adapter.ReportSnapshotStatus(githubSnapshot)
			Expect(err).NotTo(HaveOccurred())
			Expect(statusCode).NotTo(BeNil())
		})

		It("doesn't report anything when data are older", func() {
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter)
			mockStatus.EXPECT().GetReporter(gomock.Any()).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()

			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0) // data are older, status shouldn't be reported

			hasPRSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
			hasPRSnapshot.Annotations["test.appstudio.openshift.io/git-reporter-status"] = "{\"scenarios\":{\"scenario1-snapshot-pr-sample\":{\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\"}}}"
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, logger, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			statusCode, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(err).NotTo(HaveOccurred())
			Expect(statusCode).NotTo(BeNil())
		})

		It("Report new status if it was updated", func() {
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter)
			mockStatus.EXPECT().GetReporter(gomock.Any()).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()

			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(1)

			hasPRSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
			hasPRSnapshot.Annotations["test.appstudio.openshift.io/git-reporter-status"] = "{\"scenarios\":{\"scenarios-snapshot-pr-sample\":{\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\"}}}"
			hasPRSnapshot.Annotations["test.appstudio.openshift.io/group-test-info"] = "[{\"namespace\":\"default\",\"component\":\"devfile-sample-java-springboot-basic-8969\",\"buildPipelineRun\":\"build-plr-java-qjfxz\",\"snapshot\":\"app-8969-bbn7d\",\"pullRuestNumber\":\"1\",\"repoUrl\":\"https://example.com\"},{\"namespace\":\"default\",\"component\":\"devfile-sample-go-basic-8969\",\"buildPipelineRun\":\"build-plr-go-jmsjq\",\"snapshot\":\"app-8969-kzq2l\",\"pullRuestNumber\":\"1\",\"repoUrl\":\"https://example.com\"}]"
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, logger, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			statusCode, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(err).NotTo(HaveOccurred())
			Expect(statusCode).NotTo(BeNil())
		})

		It("Report new status if it was updated (old way - migration test)", func() {
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter)
			mockStatus.EXPECT().GetReporter(gomock.Any()).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()

			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(1)

			hasPRSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
			hasPRSnapshot.Annotations["test.appstudio.openshift.io/pr-last-update"] = "2023-08-26T17:57:49+02:00"
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, logger, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			statusCode, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			fmt.Fprintf(GinkgoWriter, "-------test: %v\n", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(statusCode).NotTo(BeNil())
		})

		It("add annotation to snapshot when no git provider is found", func() {
			// Create a snapshot WITHOUT git provider info but WITH proper component labels
			snapshotWithoutGitProvider := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-snapshot-no-git",
					Namespace: "test-namespace",
					Labels: map[string]string{
						gitops.SnapshotTypeLabel:      gitops.SnapshotComponentType,
						gitops.SnapshotComponentLabel: "test-component",
					},
					Annotations: map[string]string{
						"test.appstudio.openshift.io/status": "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]",
					},
					// NO git provider labels!
				},
			}

			// Create a buffer for logging
			buf := bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			// Set up mocks
			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus := status.NewMockStatusInterface(ctrl)

			// Mock GetReporter to return nil (no git provider found)
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(nil)

			// Create adapter
			adapter = NewAdapterWithApplication(ctx, snapshotWithoutGitProvider, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus

			// Call the method
			statusCode, err := adapter.ReportSnapshotStatus(snapshotWithoutGitProvider)

			// Check results
			Expect(err).ToNot(HaveOccurred())
			Expect(statusCode).To(BeTrue())
			Expect(snapshotWithoutGitProvider.Annotations).To(HaveKey(gitops.GitReportingFailureAnnotation))

			// Check that the error was logged
			Expect(buf.String()).Should(ContainSubstring("Failed to get git reporter for snapshot - missing required labels/annotations"))
		})

		It("report expected textual data for InProgress test scenario", func() {
			os.Setenv("CONSOLE_NAME", "Konflux Staging")
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter)
			mockStatus.EXPECT().GetReporter(gomock.Any()).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()

			hasPRSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"testPipelineRunName\":\"test-pipelinerun\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
			t, err := time.Parse(time.RFC3339, "2023-07-26T16:57:49+02:00")
			Expect(err).NotTo(HaveOccurred())
			expectedTestReport := status.TestReport{
				FullName:            "Konflux Staging / scenario1 / component-sample",
				ScenarioName:        "scenario1",
				SnapshotName:        "snapshot-pr-sample",
				ComponentName:       "component-sample",
				Text:                "Test in progress",
				ShortText:           "Test in progress",
				Summary:             "Integration test for component component-sample snapshot snapshot-pr-sample and scenario scenario1 is in progress",
				Status:              intgteststat.IntegrationTestStatusInProgress,
				StartTime:           &t,
				TestPipelineRunName: "test-pipelinerun",
			}
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Eq(expectedTestReport)).Times(1)
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			statusCode, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			fmt.Fprintf(GinkgoWriter, "-------test: %v\n", buf.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(statusCode).NotTo(BeNil())
		})
	})

	When("consolidated commit status path is triggered in iterateIntegrationTestStatusDetailsInStatusReport", func() {
		var gitlabTestComp *applicationapiv1alpha1.Component
		var gitlabTestSnapshot *applicationapiv1alpha1.Snapshot

		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return(status.GitLabProvider).AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter).AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)

			// Component without a GitSource prevents IsAllCommentDisabledForPacRepositoryInComponent
			// from listing PAC RepositoryList objects (which are not in the envtest scheme).
			gitlabTestComp = &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitlab-threshold-comp",
					Namespace: "default",
				},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ComponentName:  "gitlab-threshold-comp",
					Application:    hasApp.Name,
					ContainerImage: SampleImage,
				},
			}
			Expect(k8sClient.Create(ctx, gitlabTestComp)).Should(Succeed())
			waitForCached(gitlabTestComp)

			// Build status annotation with MaxIndividualStatuses+1 InProgress scenarios to trigger the consolidated path
			var scenariosBuf bytes.Buffer
			scenariosBuf.WriteString("[")
			for i := 1; i <= status.MaxIndividualStatuses+1; i++ {
				if i > 1 {
					scenariosBuf.WriteString(",")
				}
				fmt.Fprintf(&scenariosBuf,
					`{"scenario":"scenario-%d","status":"InProgress","lastUpdateTime":"2023-08-26T17:57:50+02:00","details":"Test in progress"}`,
					i)
			}
			scenariosBuf.WriteString("]")

			// No PipelineAsCodePullRequestAnnotation so isMergeRequest = false and UpdateStatusInComment is not called.
			gitlabTestSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snapshot-gitlab-threshold",
					Namespace: "default",
					Labels: map[string]string{
						gitops.SnapshotTypeLabel:      gitops.SnapshotComponentType,
						gitops.SnapshotComponentLabel: gitlabTestComp.Name,
					},
					Annotations: map[string]string{
						gitops.SnapshotTestsStatusAnnotation: scenariosBuf.String(),
					},
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasApp.Name,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           gitlabTestComp.Name,
							ContainerImage: SampleImage,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, gitlabTestSnapshot)).Should(Succeed())

			adapter = NewAdapterWithApplication(ctx, gitlabTestSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, gitlabTestComp)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Delete(ctx, gitlabTestSnapshot)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("calls ReportConsolidatedStatus and not ReportStatus when scenario count exceeds MaxIndividualStatuses", func() {
			mockReporter.EXPECT().ReportConsolidatedStatus(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			statusCode, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(err).NotTo(HaveOccurred())
			Expect(statusCode).To(BeTrue())
			Expect(buf.String()).To(ContainSubstring("Scenario count exceeds threshold, using consolidated commit status"))
		})

		It("skips without error when consolidated report fails unrecoverably", func() {
			mockReporter.EXPECT().ReportConsolidatedStatus(gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("consolidated failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(true).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("consolidated status report failed with unrecoverable error"))
		})

		It("requeues when consolidated report fails recoverably", func() {
			mockReporter.EXPECT().ReportConsolidatedStatus(gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("consolidated failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(false).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to report consolidated status"))
		})

		It("requeues when writing report status after consolidated report fails", func() {
			mockReporter.EXPECT().ReportConsolidatedStatus(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationPatch, ObjectType: &applicationapiv1alpha1.Snapshot{}, Err: errBoom},
			})

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write snapshot report status after consolidated status"))
		})
	})

	When("Ensure group snapshot creation failure is reported to git provider [APPLICATION]", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus := status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter)
			mockStatus.EXPECT().GetReporter(gomock.Any()).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(1)

			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasPRSnapshot,
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
			})
			adapter.status = mockStatus
			Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))
		})
		It("ensure group snapshot create failure is reported to git provider", func() {
			result, err := adapter.EnsureGroupSnapshotCreationStatusReportedToGitProvider()
			Expect(result.CancelRequest).To(BeFalse())
			Expect(result.RequeueRequest).To(BeFalse())
			Expect(buf.String()).Should(ContainSubstring("Successfully report group snapshot creation failure"))
			Expect(buf.String()).Should(ContainSubstring("Successfully updated the test.appstudio.openshift.io/create-groupsnapshot-status"))
			Expect(err).Should(Succeed())
		})
	})

	When("New Adapter is created for a push-type Snapshot that passed all tests (ComponentGroup)", func() {
		var cgSnapshot *applicationapiv1alpha1.Snapshot
		var hasCompGroup *v1beta2.ComponentGroup

		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			hasCompGroup = &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-component-group",
					Namespace: "default",
				},
			}

			cgSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cg-snapshot-pass",
					Namespace: "default",
					Labels: map[string]string{
						gitops.SnapshotTypeLabel:            gitops.SnapshotComponentType,
						gitops.SnapshotComponentLabel:       hasComp.Name,
						gitops.PipelineAsCodeEventTypeLabel: gitops.PipelineAsCodePushType,
					},
					Annotations: map[string]string{
						gitops.PipelineAsCodeInstallationIDAnnotation: "123",
					},
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasApp.Name,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           "component-sample",
							ContainerImage: SampleImage,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cgSnapshot)).Should(Succeed())

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(cgSnapshot)
			Expect(err).ToNot(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestPassed, "testDetails")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, cgSnapshot, statuses, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			adapter = NewAdapter(ctx, cgSnapshot, hasCompGroup, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
			})
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, cgSnapshot)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures Snapshot passed all tests using ComponentGroup path", func() {
			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			expectedLogEntry := "Found 1 required integration test scenarios"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})
	})

	When("New Adapter is created for a push-type Snapshot that failed tests (ComponentGroup)", func() {
		var cgSnapshot *applicationapiv1alpha1.Snapshot
		var hasCompGroup *v1beta2.ComponentGroup

		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			hasCompGroup = &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-component-group-fail",
					Namespace: "default",
				},
			}

			cgSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cg-snapshot-fail",
					Namespace: "default",
					Labels: map[string]string{
						gitops.SnapshotTypeLabel:            gitops.SnapshotComponentType,
						gitops.SnapshotComponentLabel:       hasComp.Name,
						gitops.PipelineAsCodeEventTypeLabel: gitops.PipelineAsCodePushType,
					},
					Annotations: map[string]string{
						gitops.PipelineAsCodeInstallationIDAnnotation: "123",
					},
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasApp.Name,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           "component-sample",
							ContainerImage: SampleImage,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cgSnapshot)).Should(Succeed())

			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(cgSnapshot)
			Expect(err).ToNot(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestFail, "Failed test")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, cgSnapshot, statuses, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			adapter = NewAdapter(ctx, cgSnapshot, hasCompGroup, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
			})
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, cgSnapshot)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures Snapshot failed tests using ComponentGroup path", func() {
			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			expectedLogEntry := "Found 1 required integration test scenarios"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})
	})

	When("EnsureGroupSnapshotCreationStatusReportedToGitProvider is called with ComponentGroup adapter", func() {
		var hasCompGroup *v1beta2.ComponentGroup

		BeforeEach(func() {
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus := status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter)
			mockStatus.EXPECT().GetReporter(gomock.Any()).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(1)

			hasCompGroup = &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-component-group-noop",
					Namespace: "default",
				},
			}

			hasPRSnapshot.Spec.Application = ""
			hasPRSnapshot.Spec.ComponentGroup = hasCompGroup.Name
			hasPRSnapshot.Labels[gitops.ComponentGroupNameLabel] = hasCompGroup.Name
			Expect(k8sClient.Update(ctx, hasPRSnapshot)).Should(Succeed())

			adapter = NewAdapter(ctx, hasPRSnapshot, hasCompGroup, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ComponentGroupContextKey,
					Resource:   hasCompGroup,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasPRSnapshot,
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosForComponentGroupContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosForComponentGroupsContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
			})
			adapter.status = mockStatus
			Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensure group snapshot create failure is reported to git provider", func() {
			result, err := adapter.EnsureGroupSnapshotCreationStatusReportedToGitProvider()
			Expect(result.CancelRequest).To(BeFalse())
			Expect(result.RequeueRequest).To(BeFalse())
			Expect(buf.String()).Should(ContainSubstring("Successfully report group snapshot creation failure"))
			Expect(buf.String()).Should(ContainSubstring("Successfully updated the test.appstudio.openshift.io/create-groupsnapshot-status"))
			Expect(err).Should(Succeed())
		})
	})

	When("EnsureSnapshotFinishedAllTests reaches labelSnapshotToTriggerUntriggeredTest [APPLICATION]", func() {
		var loaderMocks []toolkit.MockData

		BeforeEach(func() {
			buf = bytes.Buffer{}
			loaderMocks = []toolkit.MockData{
				{ContextKey: loader.ApplicationContextKey, Resource: hasApp},
				{ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey, Resource: []v1beta2.IntegrationTestScenario{*integrationTestScenario}},
			}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapterWithApplication(ctx, hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, loaderMocks)
		})

		It("adds the run label for an untriggered scenario and patches the snapshot", func() {
			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueRequest).To(BeFalse())
			Expect(adapter.snapshot.Labels[gitops.SnapshotIntegrationTestRun]).To(Equal(integrationTestScenario.Name))
			Expect(buf.String()).To(ContainSubstring("Detected an integrationTestScenario was not triggered"))
		})

		It("requeues with an error when patching the snapshot run label fails", func() {
			wireClientMocks(adapter, loaderMocks, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationPatch, ObjectType: &applicationapiv1alpha1.Snapshot{}, Err: errBoom},
			})

			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to patch snapshot"))
		})
	})

	When("EnsureSnapshotFinishedAllTests encounters client errors", func() {
		var (
			loaderMocks []toolkit.MockData
			log         helpers.IntegrationLogger
		)

		BeforeEach(func() {
			buf = bytes.Buffer{}
			log = helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			loaderMocks = []toolkit.MockData{
				{ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey, Resource: []v1beta2.IntegrationTestScenario{*integrationTestScenario}},
			}
		})

		It("requeues when the required integration test scenarios cannot be loaded (application)", func() {
			adapter = NewAdapterWithApplication(ctx, hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{ContextKey: loader.RequiredIntegrationTestScenariosForSnapshotContextKey, Err: fmt.Errorf("boom")},
			})

			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(MatchError("boom"))
		})

		It("requeues when marking the snapshot integration status as finished fails", func() {
			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestPassed, "testDetails")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, hasSnapshot, statuses, k8sClient)
			Expect(err).ToNot(HaveOccurred())

			adapter = NewAdapterWithApplication(ctx, hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			wireClientMocks(adapter, loaderMocks, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationPatch, ObjectType: &applicationapiv1alpha1.Snapshot{}, SubResourceName: "status", Err: errBoom},
			})

			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("Failed to Update Snapshot AppStudioIntegrationStatus status"))
		})

		It("requeues when marking the snapshot as passed fails", func() {
			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestPassed, "testDetails")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, hasSnapshot, statuses, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			meta.SetStatusCondition(&hasSnapshot.Status.Conditions, metav1.Condition{
				Type:    gitops.AppStudioIntegrationStatusCondition,
				Status:  metav1.ConditionTrue,
				Reason:  gitops.AppStudioIntegrationStatusFinished,
				Message: "already finished",
			})

			adapter = NewAdapterWithApplication(ctx, hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			wireClientMocks(adapter, loaderMocks, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationPatch, ObjectType: &applicationapiv1alpha1.Snapshot{}, SubResourceName: "status", Err: errBoom},
			})

			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("Failed to Update Snapshot AppStudioTestSucceeded status"))
			Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeTrue())
			Expect(buf.String()).ToNot(ContainSubstring("marked as passed"))
		})

		It("requeues when marking the snapshot as failed fails", func() {
			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			statuses.UpdateTestStatusIfChanged(integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestFail, "Failed test")
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, hasSnapshot, statuses, k8sClient)
			Expect(err).ToNot(HaveOccurred())
			meta.SetStatusCondition(&hasSnapshot.Status.Conditions, metav1.Condition{
				Type:    gitops.AppStudioIntegrationStatusCondition,
				Status:  metav1.ConditionTrue,
				Reason:  gitops.AppStudioIntegrationStatusFinished,
				Message: "already finished",
			})

			adapter = NewAdapterWithApplication(ctx, hasSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			wireClientMocks(adapter, loaderMocks, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationPatch, ObjectType: &applicationapiv1alpha1.Snapshot{}, SubResourceName: "status", Err: errBoom},
			})

			result, err := adapter.EnsureSnapshotFinishedAllTests()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("Failed to Update Snapshot AppStudioTestSucceeded status"))
			Expect(meta.IsStatusConditionFalse(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeTrue())
			Expect(buf.String()).ToNot(ContainSubstring("marked as failed"))
		})
	})

	When("EnsureSnapshotTestStatusReportedToGitProvider handles PipelineRun finalizers and reporter errors", func() {
		olderDataStatus := func(plrName string) string {
			return fmt.Sprintf("[{\"scenario\":\"scenario1\",\"status\":\"TestPassed\",\"testPipelineRunName\":\"%s\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"ok\"}]", plrName)
		}

		BeforeEach(func() {
			buf = bytes.Buffer{}
			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter).AnyTimes()
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
		})

		It("requeues when getting a completed PipelineRun fails with a non-NotFound error", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = olderDataStatus("plr-get-error")
			hasPRSnapshot.Annotations[gitops.SnapshotStatusReportAnnotation] = olderReporterStatus

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationGet, ObjectType: &tektonv1.PipelineRun{}, Err: errBoom},
			})

			result, err := adapter.EnsureSnapshotTestStatusReportedToGitProvider()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(buf.String()).ToNot(ContainSubstring("failed to report test status to git provider"))
		})

		It("requeues when removing the finalizer from a completed PipelineRun fails", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			plr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "plr-with-finalizer",
					Namespace:  "default",
					Finalizers: []string{helpers.IntegrationPipelineRunFinalizer},
				},
				Spec: tektonv1.PipelineRunSpec{
					PipelineRef: &tektonv1.PipelineRef{
						ResolverRef: tektonv1.ResolverRef{
							Resolver: "bundle",
							Params: tektonv1.Params{
								{Name: "bundle", Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"}},
								{Name: "name", Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"}},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, plr)).Should(Succeed())
			DeferCleanup(func() {
				fetched := &tektonv1.PipelineRun{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "plr-with-finalizer"}, fetched); err == nil {
					controllerutil.RemoveFinalizer(fetched, helpers.IntegrationPipelineRunFinalizer)
					_ = k8sClient.Update(ctx, fetched)
					_ = k8sClient.Delete(ctx, fetched)
				}
			})
			waitForCached(plr)

			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = olderDataStatus("plr-with-finalizer")
			hasPRSnapshot.Annotations[gitops.SnapshotStatusReportAnnotation] = olderReporterStatus

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationPatch, ObjectType: &tektonv1.PipelineRun{}, Err: errBoom},
			})

			result, err := adapter.EnsureSnapshotTestStatusReportedToGitProvider()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("error occurred while patching the updated PipelineRun after finalizer removal"))
		})

		It("requeues when reporting fails with a recoverable error on a young snapshot", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("init failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(false).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = inProgressStatus

			result, err := adapter.EnsureSnapshotTestStatusReportedToGitProvider()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to initialize reporter"))
			Expect(buf.String()).To(ContainSubstring("failed to report test status to git provider for snapshot"))
			Expect(hasPRSnapshot.Annotations).ToNot(HaveKey(gitops.GitReportingFailureAnnotation))
		})

		It("continues without error when reporting fails with an unrecoverable error", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("init failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(true).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = inProgressStatus

			result, err := adapter.EnsureSnapshotTestStatusReportedToGitProvider()
			Expect(result.RequeueRequest).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("failed to report test status to git provider for snapshot"))
		})
	})

	When("EnsureGroupSnapshotCreationStatusReportedToGitProvider encounters errors", func() {
		var log helpers.IntegrationLogger

		BeforeEach(func() {
			buf = bytes.Buffer{}
			log = helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter).AnyTimes()
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
		})

		It("requeues when integration test scenarios cannot be loaded for the application", func() {
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{ContextKey: loader.AllIntegrationTestScenariosContextKey, Err: fmt.Errorf("boom")},
			})

			result, err := adapter.EnsureGroupSnapshotCreationStatusReportedToGitProvider()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(MatchError("boom"))
			Expect(buf.String()).To(ContainSubstring("Failed to get integration test scenarios for application application-sample in namespace default"))
		})

		It("requeues when integration test scenarios cannot be loaded for the component group", func() {
			adapter = NewAdapter(ctx, hasPRSnapshot, &v1beta2.ComponentGroup{ObjectMeta: metav1.ObjectMeta{Name: "cg-t4", Namespace: "default"}}, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{ContextKey: loader.AllIntegrationTestScenariosForComponentGroupContextKey, Err: fmt.Errorf("boom")},
			})

			result, err := adapter.EnsureGroupSnapshotCreationStatusReportedToGitProvider()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(MatchError("boom"))
			Expect(buf.String()).To(ContainSubstring("Failed to get integration test scenarios for component group cg-t4 in namespace default"))
		})

		It("requeues when writing the group snapshot creation status annotation fails", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(1)

			wireClientMocks(adapter, []toolkit.MockData{
				{ContextKey: loader.AllIntegrationTestScenariosContextKey, Resource: []v1beta2.IntegrationTestScenario{*integrationTestScenario}},
			}, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationPatch, ObjectType: &applicationapiv1alpha1.Snapshot{}, Err: errBoom},
			})

			result, err := adapter.EnsureGroupSnapshotCreationStatusReportedToGitProvider()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write group snapshot creation status to annotation"))
		})

		It("requeues when reporting group snapshot creation status fails recoverably", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("init failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(false).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{ContextKey: loader.AllIntegrationTestScenariosContextKey, Resource: []v1beta2.IntegrationTestScenario{*integrationTestScenario}},
			})

			result, err := adapter.EnsureGroupSnapshotCreationStatusReportedToGitProvider()
			Expect(result.RequeueRequest).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("failed to report group snapshot creation status to git provider from component snapshot"))
		})

		It("continues when reporting group snapshot creation status fails unrecoverably", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("init failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(true).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{ContextKey: loader.AllIntegrationTestScenariosContextKey, Resource: []v1beta2.IntegrationTestScenario{*integrationTestScenario}},
			})

			result, err := adapter.EnsureGroupSnapshotCreationStatusReportedToGitProvider()
			Expect(result.RequeueRequest).To(BeFalse())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("ReportSnapshotStatus encounters errors", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter).AnyTimes()
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
		})

		It("annotates the snapshot and does not requeue on an unrecoverable metadata error during initialize", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Return(0, helpers.NewUnrecoverableMetadataError("bad metadata")).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = inProgressStatus

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to initialize reporter"))
			Expect(hasPRSnapshot.Annotations).To(HaveKey(gitops.GitReportingFailureAnnotation))
		})

		It("requeues when writing the snapshot report status metadata fails", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)

			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = inProgressStatus

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationPatch, ObjectType: &applicationapiv1alpha1.Snapshot{}, Err: errBoom},
			})

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("issue occurred during generating or updating report status"))
			Expect(err.Error()).To(ContainSubstring("failed to write snapshot report status metadata"))
		})

		It("returns an error when generating the test report fails", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0)

			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = testPassedStatus

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationGet, ObjectType: &tektonv1.PipelineRun{}, Err: errBoom},
			})

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to generate test report"))
		})
	})

	When("reportPerScenarioStatuses handles reporter errors", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter).AnyTimes()
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
		})

		It("stops without error when a per-scenario report fails unrecoverably", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("report failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(true).Times(1)

			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = inProgressStatusNoStartTime

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())
		})

		It("requeues when a per-scenario report fails recoverably", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("report failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(false).Times(1)

			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = inProgressStatusNoStartTime

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update status"))
		})

		It("requeues when writing report status after a per-scenario report fails", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(0, nil).Times(2)

			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"p\"},{\"scenario\":\"scenario2\",\"status\":\"InProgress\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"p\"}]"

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationPatch, ObjectType: &applicationapiv1alpha1.Snapshot{}, Err: errBoom},
			})

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to report status AND write snapshot report status metadata"))
		})
	})

	When("iterateIntegrationTestStatusDetailsInStatusReport handles errors", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return(status.GitLabProvider).AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter).AnyTimes()
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			hasPRSnapshot.Annotations[gitops.SnapshotTestsStatusAnnotation] = inProgressStatus
		})

		It("returns an error when the component cannot be fetched for the comment update", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationGet, ObjectType: &applicationapiv1alpha1.Component{}, Err: errBoom},
			})

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get component for snapshot"))
		})

		It("returns an error when checking if comments are disabled fails", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationList, ObjectType: &pacv1alpha1.RepositoryList{}, Err: errBoom},
			})

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to check if comment is disabled for component"))
		})

		It("continues without error when the comment update fails unrecoverably", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)
			mockReporter.EXPECT().UpdateStatusInComment(gomock.Any(), gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("comment failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(true).Times(1)

			hasPRSnapshot.Annotations[gitops.PipelineAsCodePullRequestAnnotation] = "1"

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationList, ObjectType: &pacv1alpha1.RepositoryList{}, Err: nil},
			})

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())
		})

		It("requeues when the comment update fails recoverably", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)
			mockReporter.EXPECT().UpdateStatusInComment(gomock.Any(), gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("comment failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(false).Times(1)

			hasPRSnapshot.Annotations[gitops.PipelineAsCodePullRequestAnnotation] = "1"

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationList, ObjectType: &pacv1alpha1.RepositoryList{}, Err: nil},
			})

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create comment"))
		})

		It("skips the comment update when comments are disabled for the component", func() {
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)
			mockReporter.EXPECT().UpdateStatusInComment(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationGet, ObjectType: &applicationapiv1alpha1.Component{}, Result: &applicationapiv1alpha1.Component{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "component-sample",
						Namespace:   "default",
						Annotations: map[string]string{gitops.GitCommentPolicyAnnotation: gitops.GitCommentPolicyAllDisabled},
					},
				}},
			})

			recoverable, err := adapter.ReportSnapshotStatus(adapter.snapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("All comments are disabled for PAC repository in component or integration test disabled for component"))
		})
	})

	When("ReportGroupSnapshotCreationStatus encounters errors", func() {
		var scenarios *[]v1beta2.IntegrationTestScenario

		BeforeEach(func() {
			buf = bytes.Buffer{}
			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapterWithApplication(ctx, hasPRSnapshot, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			scenarios = &[]v1beta2.IntegrationTestScenario{*integrationTestScenario}
		})

		It("returns success when no suitable reporter is found", func() {
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(nil)

			recoverable, err := adapter.ReportGroupSnapshotCreationStatus(hasPRSnapshot, scenarios, intgteststat.GroupSnapshotCreationFailed, gitops.ComponentNameForGroupSnapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("No suitable reporter found"))
		})

		It("returns an error when the component cannot be fetched from the snapshot", func() {
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter).AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)

			wireClientMocks(adapter, nil, []toolkit.ClientCallMock{
				{Operation: toolkit.OperationGet, ObjectType: &applicationapiv1alpha1.Component{}, Err: errBoom},
			})

			recoverable, err := adapter.ReportGroupSnapshotCreationStatus(hasPRSnapshot, scenarios, intgteststat.GroupSnapshotCreationFailed, gitops.ComponentNameForGroupSnapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get component from snapshot"))
		})

		It("returns no error and skips the success log when reporting fails unrecoverably", func() {
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter).AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("report failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(true).Times(1)

			_, err := adapter.ReportGroupSnapshotCreationStatus(hasPRSnapshot, scenarios, intgteststat.GroupSnapshotCreationFailed, gitops.ComponentNameForGroupSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(buf.String()).To(ContainSubstring("failed to report group snapshot creation failure"))
			Expect(buf.String()).ToNot(ContainSubstring("Successfully report group snapshot creation failure"))
		})

		It("requeues when reporting fails recoverably", func() {
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter).AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Return(0, nil).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(500, fmt.Errorf("report failed")).Times(1)
			mockReporter.EXPECT().ReturnCodeIsUnrecoverable(500).Return(false).Times(1)

			recoverable, err := adapter.ReportGroupSnapshotCreationStatus(hasPRSnapshot, scenarios, intgteststat.GroupSnapshotCreationFailed, gitops.ComponentNameForGroupSnapshot)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to report group snapshot creation failure"))
		})
	})

	When("getDestinationSnapshots handles a malformed group snapshot", func() {
		BeforeEach(func() {
			buf = bytes.Buffer{}
		})

		It("returns an error when the group snapshot info annotation is not valid JSON", func() {
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			ctrl := gomock.NewController(GinkgoT())
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockStatus.EXPECT().GetReporter(gomock.Any()).Times(0)

			snap := groupSnapshot.DeepCopy()
			snap.Annotations[gitops.GroupSnapshotInfoAnnotation] = "not-json"
			delete(snap.Annotations, gitops.SnapshotStatusReportAnnotation)

			adapter = NewAdapterWithApplication(ctx, snap, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus

			recoverable, err := adapter.ReportSnapshotStatus(snap)
			Expect(recoverable).To(BeTrue())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get component snapshots from snapshot"))
			Expect(buf.String()).To(ContainSubstring("failed to get component snapshots included in group snapshot"))
		})
	})
})
