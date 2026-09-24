package integration

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/konflux-ci/image-controller/pkg/quay"
	"github.com/konflux-ci/operator-toolkit/metadata"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/framework"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils/build"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils/oci"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/tekton"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"

	appstudioApi "github.com/konflux-ci/application-api/api/v1alpha1"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
)

var prebuiltPullRequestNumber int64 = 13650

var _ = framework.IntegrationServiceSuiteDescribe("Integration Service E2E tests", ginkgo.Label("integration-service", "happy-path"), func() {
	defer ginkgo.GinkgoRecover()

	var f *framework.Framework
	var err error

	var integrationTestScenario *integrationv1beta2.IntegrationTestScenario
	var newIntegrationTestScenario *integrationv1beta2.IntegrationTestScenario
	var skippedIntegrationTestScenario *integrationv1beta2.IntegrationTestScenario
	var timeout, interval time.Duration
	var pipelineRun *pipeline.PipelineRun
	var pipelineRunPush *pipeline.PipelineRun
	var snapshot *appstudioApi.Snapshot
	var snapshotPush *appstudioApi.Snapshot
	var applicationName, componentName, testNamespace string

	ginkgo.AfterEach(framework.ReportFailure(&f))

	ginkgo.Describe("with happy path for general flow of Integration service", ginkgo.Ordered, func() {
		ginkgo.BeforeAll(func() {
			// Initialize the tests controllers
			f, err = framework.NewFramework(utils.GetGeneratedNamespace("integration1"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			testNamespace = f.UserNamespace

			applicationName = createApp(*f, testNamespace)
			componentName = createComponentWithoutPACMergeRequest(*f, testNamespace, applicationName)

			integrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "", []string{"application"})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			skippedIntegrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("skipped-its", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "", []string{"push"})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			pipelineRun, err = applyPrebuiltBuildPLR(f, testNamespace, applicationName, componentName, prebuiltBuildPLRSpec{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create PipelineRun from pre-built artifact")

			ginkgo.GinkgoWriter.Printf("Created pre-built PipelineRun %s in namespace %s\n", pipelineRun.Name, testNamespace)
		})

		ginkgo.AfterAll(func() {
			if !ginkgo.CurrentSpecReport().Failed() {
				cleanup(*f, testNamespace, applicationName, componentName, snapshotPush)
			}
			// Note: No PaC branch cleanup needed - configure-pac-no-mr doesn't create branches
		})

		ginkgo.When("the pre-built PipelineRun is applied", func() {
			ginkgo.It("checks if the BuildPipelineRun have the annotation of chains signed", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, chainsSignedAnnotation)).To(gomega.Succeed())
			})

			ginkgo.It("checks if the Snapshot is created", func() {
				snapshot, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", "", componentName, testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.It("checks if the Build PipelineRun got annotated with Snapshot name", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, snapshotAnnotation)).To(gomega.Succeed())
			})

			ginkgo.It("verifies that the finalizer has been removed from the build pipelinerun", func() {
				timeout := "60s"
				interval := "1s"
				gomega.Eventually(func() error {
					pipelineRun, err = f.AsKubeDeveloper.IntegrationController.GetBuildPipelineRun(componentName, applicationName, testNamespace, false, "")
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
					if controllerutil.ContainsFinalizer(pipelineRun, pipelinerunFinalizerByIntegrationService) {
						return fmt.Errorf("build pipelineRun %s/%s still contains the finalizer: %s", pipelineRun.GetNamespace(), pipelineRun.GetName(), pipelinerunFinalizerByIntegrationService)
					}
					return nil
				}, timeout, interval).Should(gomega.Succeed(), "timeout when waiting for finalizer to be removed")
			})

			ginkgo.It("checks if all of the integrationPipelineRuns passed", ginkgo.Label("slow"), func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForAllIntegrationPipelinesToBeFinished(testNamespace, applicationName, snapshot, []string{integrationTestScenario.Name})).To(gomega.Succeed())
			})

			ginkgo.It("checks if the passed status of integration test is reported in the Snapshot", func() {
				gomega.Eventually(func() error {
					snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

					statusDetail, err := f.AsKubeDeveloper.IntegrationController.GetIntegrationTestStatusDetailFromSnapshot(snapshot, integrationTestScenario.Name)
					gomega.Expect(err).ToNot(gomega.HaveOccurred())

					if statusDetail.Status != intgteststat.IntegrationTestStatusTestPassed {
						return fmt.Errorf("test status for scenario: %s, doesn't have expected value %s, within the snapshot: %s", integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestPassed, snapshot.Name)
					}
					return nil
				}, longTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed())
			})

			ginkgo.It("checks if the skipped integration test is absent from the Snapshot's status annotation", func() {
				snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				statusDetail, err := f.AsKubeDeveloper.IntegrationController.GetIntegrationTestStatusDetailFromSnapshot(snapshot, skippedIntegrationTestScenario.Name)
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(err.Error()).To(gomega.ContainSubstring(skippedIntegrationTestScenario.Name))
				gomega.Expect(statusDetail).To(gomega.BeNil())
			})

			ginkgo.It("checks if the finalizer was removed from all of the related Integration pipelineRuns", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForFinalizerToGetRemovedFromAllIntegrationPipelineRuns(testNamespace, applicationName, snapshot, []string{integrationTestScenario.Name})).To(gomega.Succeed())
			})
		})

		ginkgo.It("creates a ReleasePlan", func() {
			_, err = f.AsKubeAdmin.ReleaseController.CreateReleasePlan(autoReleasePlan, testNamespace, applicationName, targetReleaseNamespace, "", nil, nil, nil)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			testScenarios, err := f.AsKubeAdmin.IntegrationController.GetIntegrationTestScenarios(applicationName, testNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			for _, testScenario := range *testScenarios {
				ginkgo.GinkgoWriter.Printf("IntegrationTestScenario %s is found\n", testScenario.Name)
			}
		})

		ginkgo.It("applies a pre-built PipelineRun for a push event", func() {
			pipelineRunPush, err = applyPrebuiltBuildPLR(f, testNamespace, applicationName, componentName, prebuiltBuildPLRSpec{
				EventType:  tektonconsts.PipelineAsCodePushType,
				TimeOffset: 2 * time.Minute,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create push PipelineRun from pre-built artifact")

			snapshotPush, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", pipelineRunPush.Name, componentName, testNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.When("A push event pre-built PipelineRun is applied", func() {
			ginkgo.It("checks if the global candidate is updated after push event", func() {
				expectedLastPromotedImage, err := tekton.GetImagePullSpecFromPipelineRun(pipelineRunPush)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				gomega.Eventually(func() error {
					snapshotPush, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshotPush.Name, "", "", testNamespace)
					if err != nil {
						return fmt.Errorf("failed to get push Snapshot %s/%s: %w", testNamespace, snapshotPush.Name, err)
					}

					component, err := f.AsKubeAdmin.HasController.GetComponentByApplicationName(applicationName, testNamespace)
					if err != nil {
						return fmt.Errorf("failed to get Component for application %s/%s: %w", testNamespace, applicationName, err)
					}
					if component.Status.LastPromotedImage != expectedLastPromotedImage {
						return fmt.Errorf("component %s/%s global candidate lastPromotedImage = %q, want %q", component.Namespace, component.Name, component.Status.LastPromotedImage, expectedLastPromotedImage)
					}
					return nil

				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), fmt.Sprintf("time out when waiting for updating the global candidate in %s namespace", testNamespace))
			})

			ginkgo.It("checks if all of the integrationPipelineRuns created by push event passed", ginkgo.Label("slow"), func() {
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForAllIntegrationPipelinesToBeFinished(testNamespace, applicationName, snapshotPush, []string{integrationTestScenario.Name})).To(gomega.Succeed(), "Error when waiting for one of the integration pipelines to finish in %s namespace", testNamespace)
			})

			ginkgo.It("checks if a Release is created successfully", func() {
				timeout = time.Second * 60
				interval = time.Second * 5
				gomega.Eventually(func() error {
					_, err := f.AsKubeAdmin.ReleaseController.GetReleases(testNamespace)
					return err
				}, timeout, interval).Should(gomega.Succeed(), fmt.Sprintf("time out when waiting for release created for snapshot %s/%s", snapshotPush.GetNamespace(), snapshotPush.GetName()))
			})
		})
	})

	ginkgo.Describe("with an integration test fail", ginkgo.Ordered, func() {
		ginkgo.BeforeAll(func() {
			// Initialize the tests controllers
			f, err = framework.NewFramework(utils.GetGeneratedNamespace("integration2"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			testNamespace = f.UserNamespace

			applicationName = createApp(*f, testNamespace)
			componentName = createComponentWithoutPACMergeRequest(*f, testNamespace, applicationName)

			integrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoFail, "", []string{"pull_request"})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			skippedIntegrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("skipped-its-fail", applicationName, testNamespace, gitURL, revision, pathInRepoFail, "", []string{"group"})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			pipelineRun, err = applyPrebuiltBuildPLR(f, testNamespace, applicationName, componentName, prebuiltBuildPLRSpec{
				TimeOffset: time.Minute,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create PipelineRun from pre-built artifact")
		})

		ginkgo.AfterAll(func() {
			if !ginkgo.CurrentSpecReport().Failed() {
				cleanup(*f, testNamespace, applicationName, componentName, snapshotPush)
			}
			// Note: No PaC branch cleanup needed - configure-pac-no-mr doesn't create branches
		})

		ginkgo.It("checks if the BuildPipelineRun have the annotation of chains signed", func() {
			gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, chainsSignedAnnotation)).To(gomega.Succeed())
		})

		ginkgo.It("checks if the Snapshot is created", func() {
			snapshot, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", pipelineRun.Name, componentName, testNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("checks if the Build PipelineRun got annotated with Snapshot name", func() {
			gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, snapshotAnnotation)).To(gomega.Succeed())
		})

		ginkgo.It("checks if all of the integrationPipelineRuns finished", ginkgo.Label("slow"), func() {
			gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForAllIntegrationPipelinesToBeFinished(testNamespace, applicationName, snapshot, []string{integrationTestScenario.Name})).To(gomega.Succeed())
		})

		ginkgo.It("checks if the failed status of integration test is reported in the Snapshot", func() {
			gomega.Eventually(func() error {
				snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				statusDetail, err := f.AsKubeDeveloper.IntegrationController.GetIntegrationTestStatusDetailFromSnapshot(snapshot, integrationTestScenario.Name)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())

				if statusDetail.Status != intgteststat.IntegrationTestStatusTestFail {
					return fmt.Errorf("test status doesn't have expected value %s", intgteststat.IntegrationTestStatusTestFail)
				}
				return nil
			}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed())
		})

		ginkgo.It("checks if the skipped integration test is absent from the Snapshot's status annotation", func() {
			snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			statusDetail, err := f.AsKubeDeveloper.IntegrationController.GetIntegrationTestStatusDetailFromSnapshot(snapshot, skippedIntegrationTestScenario.Name)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring(skippedIntegrationTestScenario.Name))
			gomega.Expect(statusDetail).To(gomega.BeNil())
		})

		ginkgo.It("checks if snapshot is marked as failed", ginkgo.FlakeAttempts(3), func() {
			snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect(gitops.HaveAppStudioTestsSucceeded(snapshot)).To(gomega.BeFalse(), "expected tests to fail for snapshot %s/%s", snapshot.GetNamespace(), snapshot.GetName())
		})

		ginkgo.It("checks if the finalizer was removed from all of the related Integration pipelineRuns", func() {
			gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForFinalizerToGetRemovedFromAllIntegrationPipelineRuns(testNamespace, applicationName, snapshot, []string{integrationTestScenario.Name})).To(gomega.Succeed())
		})

		ginkgo.It("creates a new IntegrationTestScenario", func() {
			newIntegrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "", []string{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("updates the Snapshot with the re-run label for the new scenario", ginkgo.FlakeAttempts(3), func() {
			updatedSnapshot := snapshot.DeepCopy()
			err := metadata.AddLabels(updatedSnapshot, map[string]string{snapshotRerunLabel: newIntegrationTestScenario.Name})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect(f.AsKubeAdmin.IntegrationController.PatchSnapshot(snapshot, updatedSnapshot)).Should(gomega.Succeed())
			gomega.Expect(metadata.GetLabelsWithPrefix(updatedSnapshot, snapshotRerunLabel)).NotTo(gomega.BeEmpty())
		})

		ginkgo.When("An snapshot is updated with a re-run label for a given scenario", func() {
			ginkgo.It("checks if the new integration pipelineRun started", ginkgo.Label("slow"), func() {
				reRunPipelineRun, err := f.AsKubeDeveloper.IntegrationController.WaitForIntegrationPipelineToGetStarted(newIntegrationTestScenario.Name, snapshot.Name, testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				gomega.Expect(reRunPipelineRun).ShouldNot(gomega.BeNil())
			})

			ginkgo.It("checks if the re-run label was removed from the Snapshot", func() {
				gomega.Eventually(func() error {
					snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
					if err != nil {
						return fmt.Errorf("encountered error while getting Snapshot %s/%s: %w", snapshot.Name, snapshot.Namespace, err)
					}

					if metadata.HasLabel(snapshot, snapshotRerunLabel) {
						return fmt.Errorf("the Snapshot %s/%s shouldn't contain the %s label", snapshot.Name, snapshot.Namespace, snapshotRerunLabel)
					}
					return nil
				}, timeout, interval).Should(gomega.Succeed())
			})

			ginkgo.It("checks if all integration pipelineRuns finished successfully", ginkgo.Label("slow"), func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForAllIntegrationPipelinesToBeFinished(testNamespace, applicationName, snapshot, []string{integrationTestScenario.Name, newIntegrationTestScenario.Name})).To(gomega.Succeed())
			})

			ginkgo.It("checks if the name of the re-triggered pipelinerun is reported in the Snapshot", ginkgo.FlakeAttempts(3), func() {
				gomega.Eventually(func(g gomega.Gomega) {
					snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
					g.Expect(err).ShouldNot(gomega.HaveOccurred())

					statusDetail, err := f.AsKubeDeveloper.IntegrationController.GetIntegrationTestStatusDetailFromSnapshot(snapshot, newIntegrationTestScenario.Name)
					g.Expect(err).ToNot(gomega.HaveOccurred())
					g.Expect(statusDetail).NotTo(gomega.BeNil())

					integrationPipelineRun, err := f.AsKubeDeveloper.IntegrationController.GetIntegrationPipelineRun(newIntegrationTestScenario.Name, snapshot.Name, testNamespace)
					g.Expect(err).ToNot(gomega.HaveOccurred())
					g.Expect(integrationPipelineRun).NotTo(gomega.BeNil())

					g.Expect(statusDetail.TestPipelineRunName).To(gomega.Equal(integrationPipelineRun.Name))
				}, timeout, interval).Should(gomega.Succeed())
			})

			ginkgo.It("checks if snapshot is still marked as failed", func() {
				snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				gomega.Expect(gitops.HaveAppStudioTestsSucceeded(snapshot)).To(gomega.BeFalse(), "expected tests to fail for snapshot %s/%s", snapshot.GetNamespace(), snapshot.GetName())
			})
		})

		ginkgo.It("applies a pre-built PipelineRun for a push event", func() {
			pipelineRunPush, err = applyPrebuiltBuildPLR(f, testNamespace, applicationName, componentName, prebuiltBuildPLRSpec{
				EventType:  tektonconsts.PipelineAsCodePushType,
				TimeOffset: 3 * time.Minute,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create push PipelineRun from pre-built artifact")

			snapshotPush, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", pipelineRunPush.Name, componentName, testNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.When("A push event pre-built PipelineRun is applied", func() {
			ginkgo.It("checks no Release CRs are created", func() {
				releases, err := f.AsKubeAdmin.ReleaseController.GetReleases(testNamespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Error when fetching the Releases")
				gomega.Expect(releases.Items).To(gomega.BeEmpty(), "Expected no Release CRs to be present, but found some")
			})
		})
	})
})

type prebuiltBuildPLRSpec struct {
	EventType  string
	TimeOffset time.Duration
}

func applyPrebuiltBuildPLR(f *framework.Framework, namespace, applicationName, componentName string, spec prebuiltBuildPLRSpec) (*pipeline.PipelineRun, error) {
	preBuildPLR, err := oci.FetchPipelineRunFromOCIArtifact(prebuiltImageForGeneralIntegration, e2eImageTag)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PipelineRun from OCI artifact: %w", err)
	}

	if spec.EventType == "" {
		spec.EventType = gitops.PipelineAsCodePullRequestType
	}

	preBuildPLRStatus := oci.PreparePipelineRunForPrebuiltUsage(
		preBuildPLR,
		namespace,
		utils.GeneratePipelineRunName("prebuilt-plr"),
		applicationName,
		componentName,
	)

	if preBuildPLR.Labels == nil {
		preBuildPLR.Labels = map[string]string{}
	}
	if preBuildPLR.Annotations == nil {
		preBuildPLR.Annotations = map[string]string{}
	}

	preBuildPLR.Labels[tektonconsts.PipelineRunTypeLabel] = tektonconsts.PipelineRunBuildType
	preBuildPLR.Labels[tektonconsts.PipelineRunApplicationLabel] = applicationName
	preBuildPLR.Labels[tektonconsts.PipelineRunComponentLabel] = componentName
	preBuildPLR.Labels["app.kubernetes.io/managed-by"] = "pipelinesascode.tekton.dev"
	preBuildPLR.Labels[tektonconsts.PipelineAsCodeEventTypeLabel] = spec.EventType
	preBuildPLR.Annotations[tektonconsts.PipelineRunChainsSignedAnnotation] = "true"
	preBuildPLR.Annotations[tektonconsts.PipelineAsCodeTargetBranchAnnotation] = componentDefaultBranch
	preBuildPLR.Annotations[tektonconsts.PipelineAsCodeSourceRepoOrg] = utils.GetEnv(constants.GITHUB_E2E_ORGANIZATION_ENV, "redhat-appstudio-qe")
	controllerutil.AddFinalizer(preBuildPLR, pipelinerunFinalizerByIntegrationService)

	if spec.EventType == tektonconsts.PipelineAsCodePushType {
		delete(preBuildPLR.Labels, tektonconsts.PipelineAsCodePullRequestLabel)
		preBuildPLR.Annotations[tektonconsts.PipelineAsCodeSourceBranchAnnotation] = componentDefaultBranch
	} else {
		preBuildPLR.Labels[tektonconsts.PipelineAsCodePullRequestLabel] = strconv.FormatInt(atomic.AddInt64(&prebuiltPullRequestNumber, 1), 10)
		preBuildPLR.Annotations[tektonconsts.PipelineAsCodeSourceBranchAnnotation] = "konflux-" + componentName
		prGroup := tekton.GetPRGroupFromBuildPLR(preBuildPLR)
		preBuildPLR.Annotations[gitops.PRGroupAnnotation] = prGroup
		preBuildPLR.Labels[gitops.PRGroupHashLabel] = tekton.GenerateSHA(prGroup)
		preBuildPLR.Annotations[snapshotCreationReport] = intgteststat.BuildPLRInProgress.String()
	}

	startTime := time.Now().UTC().Add(spec.TimeOffset)
	completionTime := startTime.Add(time.Minute)
	preBuildPLRStatus.StartTime = &metav1.Time{Time: startTime}
	preBuildPLRStatus.CompletionTime = &metav1.Time{Time: completionTime}
	// Apply the PipelineRun to the cluster, then restore the completed status from the artifact.
	// PipelineRun status uses a status subresource, so status from the create payload is not persisted.
	return f.AsKubeAdmin.TektonController.CreatePipelineRunAndRestoreStatus(preBuildPLR, preBuildPLRStatus, namespace)
}

func createApp(f framework.Framework, testNamespace string) string {
	applicationName := fmt.Sprintf("integ-app-%s", utils.GenerateRandomString(4))

	_, err := f.AsKubeAdmin.HasController.CreateApplication(applicationName, testNamespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return applicationName
}

func createComponentWithoutPACMergeRequest(f framework.Framework, testNamespace, applicationName string) string {
	componentName := fmt.Sprintf("%s-%s", "test-component-prebuilt", utils.GenerateRandomString(6))
	componentObj := appstudioApi.ComponentSpec{
		ComponentName: componentName,
		Application:   applicationName,
		Source: appstudioApi.ComponentSource{
			ComponentSourceUnion: appstudioApi.ComponentSourceUnion{
				GitSource: &appstudioApi.GitSource{
					URL:      componentGitSourceURLForGeneralIntegration,
					Revision: componentDefaultBranch,
				},
			},
		},
	}
	buildPipelineAnnotation := build.GetBuildPipelineBundleAnnotation(constants.DockerBuild)
	_, err := f.AsKubeAdmin.HasController.CreateComponentWithoutPACMergeRequest(componentObj, testNamespace, applicationName, buildPipelineAnnotation)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return componentName
}

func createComponent(f framework.Framework, testNamespace, applicationName, componentRepoName, componentRepoURL string) (*appstudioApi.Component, string, string, string) {
	componentName := fmt.Sprintf("%s-%s", "test-component-pac", utils.GenerateRandomString(6))
	pacBranchName := constants.PaCPullRequestBranchPrefix + componentName
	componentBaseBranchName := fmt.Sprintf("base-%s", utils.GenerateRandomString(6))

	err := f.AsKubeAdmin.CommonController.Github.EnsureBranchExists(componentRepoName, componentDefaultBranch, fallbackBranchName)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	err = f.AsKubeAdmin.CommonController.Github.CreateRef(componentRepoName, componentDefaultBranch, componentRevision, componentBaseBranchName)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	// get the build pipeline bundle annotation
	buildPipelineAnnotation := build.GetBuildPipelineBundleAnnotation(constants.DockerBuild)

	componentObj := appstudioApi.ComponentSpec{
		ComponentName: componentName,
		Application:   applicationName,
		Source: appstudioApi.ComponentSource{
			ComponentSourceUnion: appstudioApi.ComponentSourceUnion{
				GitSource: &appstudioApi.GitSource{
					URL:      componentRepoURL,
					Revision: componentBaseBranchName,
				},
			},
		},
	}

	originalComponent, err := f.AsKubeAdmin.HasController.CreateComponentCheckImageRepository(componentObj, testNamespace, "", "", applicationName, false, utils.MergeMaps(utils.MergeMaps(constants.ComponentPaCRequestAnnotation, constants.ImageControllerAnnotationRequestPublicRepo), buildPipelineAnnotation))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return originalComponent, componentName, pacBranchName, componentBaseBranchName
}

func createComponentWithCustomBranch(f framework.Framework, testNamespace, applicationName, componentName, componentRepoURL string, toBranchName string, contextDir string) *appstudioApi.Component {
	// get the build pipeline bundle annotation
	buildPipelineAnnotation := build.GetBuildPipelineBundleAnnotation(constants.DockerBuild)
	dockerFileURL := constants.DockerFilePath
	if contextDir == "" {
		dockerFileURL = "Dockerfile"
	}
	componentObj := appstudioApi.ComponentSpec{
		ComponentName: componentName,
		Application:   applicationName,
		Source: appstudioApi.ComponentSource{
			ComponentSourceUnion: appstudioApi.ComponentSourceUnion{
				GitSource: &appstudioApi.GitSource{
					URL:           componentRepoURL,
					Revision:      toBranchName,
					Context:       contextDir,
					DockerfileURL: dockerFileURL,
				},
			},
		},
	}

	originalComponent, err := f.AsKubeAdmin.HasController.CreateComponentCheckImageRepository(componentObj, testNamespace, "", "", applicationName, true, utils.MergeMaps(utils.MergeMaps(constants.ComponentPaCRequestAnnotation, constants.ImageControllerAnnotationRequestPublicRepo), buildPipelineAnnotation))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return originalComponent
}

func cleanup(f framework.Framework, testNamespace, applicationName, componentName string, snapshot *appstudioApi.Snapshot) {
	if !ginkgo.CurrentSpecReport().Failed() {
		gomega.Expect(f.AsKubeAdmin.IntegrationController.DeleteSnapshot(snapshot, testNamespace)).To(gomega.Succeed())
		integrationTestScenarios, err := f.AsKubeAdmin.IntegrationController.GetIntegrationTestScenarios(applicationName, testNamespace)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		for _, testScenario := range *integrationTestScenarios {
			gomega.Expect(f.AsKubeAdmin.IntegrationController.DeleteIntegrationTestScenario(&testScenario, testNamespace)).To(gomega.Succeed())
		}
		gomega.Expect(f.AsKubeAdmin.HasController.DeleteComponent(componentName, testNamespace, false)).To(gomega.Succeed())
		gomega.Expect(f.AsKubeAdmin.HasController.DeleteApplication(applicationName, testNamespace, false)).To(gomega.Succeed())
		err = deleteQuayRepo(componentName, testNamespace)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}
}

func deleteQuayRepo(componentName string, testNamespace string) error {
	quayOrgToken := os.Getenv("DEFAULT_QUAY_ORG_TOKEN")
	if quayOrgToken == "" {
		return fmt.Errorf("%s", "DEFAULT_QUAY_ORG_TOKEN env var was not found")
	}
	quayOrg := utils.GetEnv("DEFAULT_QUAY_ORG", "redhat-appstudio-qe")

	quayClient := quay.NewQuayClient(&http.Client{Transport: utils.NewRetryTransport(&http.Transport{})}, quayOrgToken, "https://quay.io/api/v1")

	r, err := regexp.Compile(fmt.Sprintf(`^(%s)`, testNamespace))
	if err != nil {
		return err
	}

	repos, err := quayClient.GetAllRepositories(quayOrg)
	if err != nil {
		return err
	}
	// Key is the repo name without slashes which is the same as robot name
	// Value is the repo name with slashes
	reposMap := make(map[string]string)

	for _, repo := range repos {
		if r.MatchString(repo.Name) {
			sanitizedRepoName := strings.ReplaceAll(repo.Name, "/", "") // repo name without slashes
			reposMap[sanitizedRepoName] = repo.Name
		}
	}

	sanitizedName := testNamespace + componentName
	if repo, exists := reposMap[sanitizedName]; exists {
		deleted, err := quayClient.DeleteRepository(quayOrg, repo)
		if err != nil {
			return fmt.Errorf("failed to delete repository %s, error: %s", repo, err)
		}
		if !deleted {
			fmt.Printf("repository %s has already been deleted, skipping\n", repo)
		}
	}
	return nil
}
