package integration

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/framework"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils/build"
	"github.com/konflux-ci/integration-service/gitops"

	appstudioApi "github.com/konflux-ci/application-api/api/v1alpha1"
	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	"github.com/konflux-ci/operator-toolkit/metadata"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = framework.IntegrationServiceSuiteDescribe("Creation of group snapshots for monorepo and multiple repos", ginkgo.Label("integration-service", "group-snapshot-creation"), func() {
	defer ginkgo.GinkgoRecover()

	var f *framework.Framework
	var err error

	var componentA *appstudioApi.Component
	var componentB *appstudioApi.Component
	var componentC *appstudioApi.Component
	var groupSnapshots *appstudioApi.SnapshotList
	var groupSnapshot *appstudioApi.Snapshot
	var componentSnapshots *[]appstudioApi.Snapshot
	var testPipelinerun *pipeline.PipelineRun
	var nudgePLR *pipeline.PipelineRun
	var firstGoPLR, firstPythonPLR, firstClonePLR, secondGoPLR, secondPythonPLR, clonePushPLR *pipeline.PipelineRun
	var integrationTestScenarioPass, invalidIntegrationTestScenario *integrationv1beta2.IntegrationTestScenario
	var nudgeConfig *integrationv1beta2.NudgeConfig

	var applicationName, testNamespace string
	var multiComponentBaseBranchName, multiComponentPRBranchName string
	var prShaMonorepo, prShaMultirepo, secondFileSha, mergeMultiResultSha string
	var multiRepoPRNumber, cloneRepoPRNumber int
	var branchesToDelete []groupSnapshotBranchRef

	ginkgo.AfterEach(framework.ReportFailure(&f))

	ginkgo.Describe("with status reporting of Integration tests in CheckRuns", ginkgo.Ordered, func() {
		ginkgo.BeforeAll(func() {
			if os.Getenv(constants.SKIP_PAC_TESTS_ENV) == "true" {
				ginkgo.Skip("Skipping this test due to configuration issue with Spray proxy")
			}

			f, err = framework.NewFramework(utils.GetGeneratedNamespace("group"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			testNamespace = f.UserNamespace

			if utils.IsPrivateHostname(f.OpenshiftConsoleHost) {
				ginkgo.Skip("Using private cluster (not reachable from Github), skipping...")
			}

			applicationName = createApp(*f, testNamespace)
			multiComponentBaseBranchName = fmt.Sprintf("prebuilt-group-base-%s", utils.GenerateRandomString(6))
			multiComponentPRBranchName = fmt.Sprintf("prebuilt-group-pr-%s", utils.GenerateRandomString(6))

			integrationTestScenarioPass, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoPassPipelinerun, "pipelinerun", []string{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.AfterAll(func() {
			if f == nil {
				return
			}

			if nudgeConfig != nil {
				if err := f.AsKubeAdmin.IntegrationController.DeleteNudgeConfig(nudgeConfig); err != nil {
					ginkgo.GinkgoWriter.Printf("Failed to delete NudgeConfig: %v\n", err)
				}
			}

			for _, pr := range []struct {
				repo   string
				number int
			}{
				{multiComponentRepoNameForGroupSnapshot, multiRepoPRNumber},
				{componentRepoNameForGroupIntegration, cloneRepoPRNumber},
			} {
				if pr.number > 0 {
					if err := f.AsKubeAdmin.CommonController.Github.ClosePullRequest(pr.repo, pr.number); err != nil {
						ginkgo.GinkgoWriter.Printf("Failed to close PR #%d in %s: %v\n", pr.number, pr.repo, err)
					}
				}
			}

			for _, branch := range branchesToDelete {
				if err := f.AsKubeAdmin.CommonController.Github.DeleteRef(branch.repo, branch.name); err != nil {
					if !strings.Contains(err.Error(), referenceDoesntExist) {
						ginkgo.GinkgoWriter.Printf("Failed to delete branch %s in %s: %v\n", branch.name, branch.repo, err)
					}
				}
			}

			if !ginkgo.CurrentSpecReport().Failed() {
				deleteGroupSnapshotSuiteResources(*f, testNamespace, applicationName, integrationTestScenarioPass, invalidIntegrationTestScenario, componentA, componentB, componentC)
			}
		})

		ginkgo.When("the components are created without PaC merge requests", func() {
			ginkgo.It("creates the go, python, and clone components", func() {
				componentA = createGroupSnapshotComponentWithoutPACMergeRequest(*f, testNamespace, applicationName, multiComponentContextDirs[0]+"-"+utils.GenerateRandomString(6), multiComponentGitSourceURLForGroupSnapshotA, multiComponentDefaultBranch, multiComponentContextDirs[0])
				componentB = createGroupSnapshotComponentWithoutPACMergeRequest(*f, testNamespace, applicationName, multiComponentContextDirs[1]+"-"+utils.GenerateRandomString(6), multiComponentGitSourceURLForGroupSnapshotB, multiComponentDefaultBranch, multiComponentContextDirs[1])
				componentC = createGroupSnapshotComponentWithoutPACMergeRequest(*f, testNamespace, applicationName, componentRepoNameForGroupIntegration+"-"+utils.GenerateRandomString(6), componentGitSourceURLForGroupIntegration, multiComponentDefaultBranch, "")
			})

			ginkgo.It("creates NudgeConfig with C→A relationship", func() {
				nudgeConfig, err = f.AsKubeAdmin.IntegrationController.CreateNudgeConfig(
					testNamespace,
					[]integrationv1beta2.NudgeRelationship{{
						From: componentC.Name,
						To:   componentA.Name,
						Mode: integrationv1beta2.NudgeModeImmediate,
					}},
				)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})
		})

		ginkgo.When("real PRs are opened and prebuilt build PipelineRuns are applied", func() {
			ginkgo.It("opens matching PRs in the monorepo and clone repositories", func() {
				multiRepoPRNumber, prShaMonorepo = createGroupSnapshotPullRequest(f, multiComponentRepoNameForGroupSnapshot, multiComponentGitRevision, multiComponentBaseBranchName, multiComponentPRBranchName, map[string]string{
					fmt.Sprintf("%s/prebuilt-go-%s.txt", multiComponentContextDirs[0], utils.GenerateRandomString(5)):     "prebuilt group snapshot go change",
					fmt.Sprintf("%s/prebuilt-python-%s.txt", multiComponentContextDirs[1], utils.GenerateRandomString(5)): "prebuilt group snapshot python change",
				})
				cloneRepoPRNumber, prShaMultirepo = createGroupSnapshotPullRequest(f, componentRepoNameForGroupIntegration, multiRepoComponentGitRevision, multiComponentBaseBranchName, multiComponentPRBranchName, map[string]string{
					fmt.Sprintf("prebuilt-clone-%s.txt", utils.GenerateRandomString(5)): "prebuilt group snapshot clone change",
				})
				branchesToDelete = append(branchesToDelete,
					groupSnapshotBranchRef{repo: multiComponentRepoNameForGroupSnapshot, name: multiComponentPRBranchName},
					groupSnapshotBranchRef{repo: componentRepoNameForGroupIntegration, name: multiComponentPRBranchName},
					groupSnapshotBranchRef{repo: multiComponentRepoNameForGroupSnapshot, name: multiComponentBaseBranchName},
					groupSnapshotBranchRef{repo: componentRepoNameForGroupIntegration, name: multiComponentBaseBranchName},
				)
			})

			ginkgo.It("applies the first prebuilt PR build PipelineRuns", func() {
				firstGoPLR = applyGroupSnapshotPrebuiltBuildPLR(f, testNamespace, applicationName, componentA.Name, prebuiltImageForMultiComponentGo, multiComponentRepoNameForGroupSnapshot, multiComponentGitSourceURLForGroupSnapshotA, multiRepoPRNumber, prShaMonorepo, multiComponentPRBranchName, multiComponentBaseBranchName, 0)
				firstPythonPLR = applyGroupSnapshotPrebuiltBuildPLR(f, testNamespace, applicationName, componentB.Name, prebuiltImageForMultiComponentPython, multiComponentRepoNameForGroupSnapshot, multiComponentGitSourceURLForGroupSnapshotB, multiRepoPRNumber, prShaMonorepo, multiComponentPRBranchName, multiComponentBaseBranchName, time.Minute)
				firstClonePLR = applyGroupSnapshotPrebuiltBuildPLR(f, testNamespace, applicationName, componentC.Name, prebuiltImageForGroupIntegration, componentRepoNameForGroupIntegration, componentGitSourceURLForGroupIntegration, cloneRepoPRNumber, prShaMultirepo, multiComponentPRBranchName, multiComponentBaseBranchName, 2*time.Minute)
			})

			ginkgo.It("creates component snapshots for the prebuilt PipelineRuns", func() {
				for _, tc := range []struct {
					component string
					plr       *pipeline.PipelineRun
				}{
					{componentA.Name, firstGoPLR},
					{componentB.Name, firstPythonPLR},
					{componentC.Name, firstClonePLR},
				} {
					_, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", tc.plr.Name, tc.component, testNamespace)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				}
			})

			ginkgo.It("annotates the prebuilt PipelineRuns with Snapshot names", func() {
				for _, component := range []string{componentA.Name, componentB.Name, componentC.Name} {
					gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, component, snapshotAnnotation)).To(gomega.Succeed())
				}
			})

			ginkgo.It("waits for component snapshots to have PR group annotations", func() {
				gomega.Eventually(func() error {
					componentSnapshotsList, err := f.AsKubeAdmin.HasController.GetAllComponentSnapshotsForApplication(applicationName, testNamespace)
					if err != nil {
						return err
					}
					wantedPLRs := map[string]bool{firstGoPLR.Name: false, firstPythonPLR.Name: false, firstClonePLR.Name: false}
					for _, snap := range componentSnapshotsList.Items {
						plrName := snap.Labels[gitops.BuildPipelineRunNameLabel]
						if _, ok := wantedPLRs[plrName]; !ok {
							continue
						}
						if snap.Annotations[groupSnapshotAnnotation] == "" {
							return fmt.Errorf("component snapshot %s for PLR %s is missing PR group annotation", snap.Name, plrName)
						}
						wantedPLRs[plrName] = true
					}
					for plrName, found := range wantedPLRs {
						if !found {
							return fmt.Errorf("component snapshot for PLR %s was not found with PR group annotation", plrName)
						}
					}
					return nil
				}, 10*time.Minute, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), "Timeout while waiting for component snapshots with PR group annotations")
			})

			ginkgo.It("creates a group snapshot containing all components and prebuilt PipelineRuns", func() {
				groupSnapshot = waitForGroupSnapshotWithInfo(f, applicationName, testNamespace, []string{componentA.Name, componentB.Name, componentC.Name, firstGoPLR.Name, firstPythonPLR.Name, firstClonePLR.Name})
				groupSnapshots, err = f.AsKubeAdmin.HasController.GetAllGroupSnapshotsForApplication(applicationName, testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				groupTestInfo := groupSnapshot.Annotations[testGroupSnapshotAnnotation]
				gomega.Expect(groupTestInfo).To(gomega.ContainSubstring(componentRepoNameForGroupIntegration), "Group test info should contain multirepo component name")
				gomega.Expect(groupTestInfo).To(gomega.ContainSubstring(multiComponentContextDirs[0]), "Group test info should contain first monorepo component context dir")
				gomega.Expect(groupTestInfo).To(gomega.ContainSubstring(multiComponentContextDirs[1]), "Group test info should contain second monorepo component context dir")
			})

			ginkgo.It("starts the group snapshot integration PipelineRun", func() {
				testPipelinerun, err = f.AsKubeDeveloper.IntegrationController.WaitForIntegrationPipelineToGetStarted(integrationTestScenarioPass.Name, groupSnapshot.Name, testNamespace)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(testPipelinerun.Labels[snapshotAnnotation]).To(gomega.ContainSubstring(groupSnapshot.Name))
				gomega.Expect(testPipelinerun.Labels[scenarioAnnotation]).To(gomega.ContainSubstring(integrationTestScenarioPass.Name))
			})
		})

		ginkgo.When("a newer prebuilt cycle is applied for the monorepo components", func() {
			ginkgo.It("updates the existing monorepo PR and applies second-cycle prebuilt PLRs", func() {
				newFile, err := f.AsKubeAdmin.CommonController.Github.CreateFile(multiComponentRepoNameForGroupSnapshot, fmt.Sprintf("%s/prebuilt-second-cycle-%s.txt", multiComponentContextDirs[0], utils.GenerateRandomString(5)), "second prebuilt group snapshot cycle", multiComponentPRBranchName)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				secondFileSha = newFile.GetSHA()

				secondGoPLR = applyGroupSnapshotPrebuiltBuildPLR(f, testNamespace, applicationName, componentA.Name, prebuiltImageForMultiComponentGo, multiComponentRepoNameForGroupSnapshot, multiComponentGitSourceURLForGroupSnapshotA, multiRepoPRNumber, secondFileSha, multiComponentPRBranchName, multiComponentBaseBranchName, 3*time.Minute)
				secondPythonPLR = applyGroupSnapshotPrebuiltBuildPLR(f, testNamespace, applicationName, componentB.Name, prebuiltImageForMultiComponentPython, multiComponentRepoNameForGroupSnapshot, multiComponentGitSourceURLForGroupSnapshotB, multiRepoPRNumber, secondFileSha, multiComponentPRBranchName, multiComponentBaseBranchName, 4*time.Minute)
			})

			ginkgo.It("creates second-cycle component and group snapshots", func() {
				for _, tc := range []struct {
					component string
					plr       *pipeline.PipelineRun
				}{
					{componentA.Name, secondGoPLR},
					{componentB.Name, secondPythonPLR},
				} {
					_, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", tc.plr.Name, tc.component, testNamespace)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				}
				groupSnapshot = waitForGroupSnapshotWithInfo(f, applicationName, testNamespace, []string{componentA.Name, componentB.Name, secondGoPLR.Name, secondPythonPLR.Name})
				groupSnapshots, err = f.AsKubeAdmin.HasController.GetAllGroupSnapshotsForApplication(applicationName, testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.It("cancels the older component snapshot and integration PipelineRun for component A", func() {
				gomega.Eventually(func() error {
					componentSnapshots, err = f.AsKubeAdmin.HasController.GetAllComponentSnapshotsForApplicationAndComponent(applicationName, testNamespace, componentA.Name)
					if err != nil {
						return err
					}
					if len(*componentSnapshots) < 2 {
						return fmt.Errorf("the length of component snapshot is %d, less than expected 2", len(*componentSnapshots))
					}
					isCancelled, err := f.AsKubeAdmin.IntegrationController.IsOlderSnapshotAndIntegrationPlrCancelled(*componentSnapshots, integrationTestScenarioPass.Name)
					if err != nil {
						return err
					}
					if !isCancelled {
						return fmt.Errorf("older component snapshot/integration test has not been cancelled")
					}
					return nil
				}, superLongTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), "timeout while waiting for component snapshot and integration pipelinerun to be cancelled")
			})

			ginkgo.It("cancels the older group snapshot and integration PipelineRun", func() {
				gomega.Eventually(func() error {
					groupSnapshots, err = f.AsKubeAdmin.HasController.GetAllGroupSnapshotsForApplication(applicationName, testNamespace)
					if err != nil {
						return err
					}
					if len(groupSnapshots.Items) < 2 {
						return fmt.Errorf("the length of group snapshot is %d, less than expected 2", len(groupSnapshots.Items))
					}
					isCancelled, err := f.AsKubeAdmin.IntegrationController.IsOlderSnapshotAndIntegrationPlrCancelled(groupSnapshots.Items, integrationTestScenarioPass.Name)
					if err != nil {
						return err
					}
					if !isCancelled {
						return fmt.Errorf("older group snapshot/integration test has not been cancelled")
					}
					return nil
				}, superLongTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), "timeout while waiting for group snapshot and integration pipelinerun to be cancelled")
			})
		})

		ginkgo.When("component C push build triggers an immediate nudge for component A", ginkgo.Ordered, func() {
			ginkgo.It("applies a prebuilt push PipelineRun for component C", func() {
				mergeMultiResultSha = fmt.Sprintf("%040d", time.Now().UnixNano())
				clonePushPLR, err = applyPrebuiltBuildPLR(f, testNamespace, applicationName, componentC.Name, prebuiltBuildPLRSpec{
					Image:        prebuiltImageForGroupIntegration,
					EventType:    tektonconsts.PipelineAsCodePushType,
					SourceBranch: multiComponentBaseBranchName,
					TargetBranch: multiComponentBaseBranchName,
					SHA:          mergeMultiResultSha,
					RepoName:     componentRepoNameForGroupIntegration,
					RepoURL:      componentGitSourceURLForGroupIntegration,
					TimeOffset:   5 * time.Minute,
				})
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Component C push PipelineRun from pre-built artifact")
				_, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", clonePushPLR.Name, componentC.Name, testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.It("should create a nudge PipelineRun for component A", func() {
				nudgePLR, err = f.AsKubeDeveloper.IntegrationController.WaitForNudgePipelineRunToExist(componentA.Name, testNamespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})

			ginkgo.It("should complete the nudge PipelineRun successfully", func() {
				gomega.Eventually(func() error {
					plr, err := f.AsKubeDeveloper.IntegrationController.GetNudgePipelineRun(componentA.Name, testNamespace)
					if err != nil {
						return err
					}
					if !plr.IsDone() {
						return fmt.Errorf("nudge PLR %s still running", plr.Name)
					}
					if !plr.GetStatusCondition().GetCondition(apis.ConditionSucceeded).IsTrue() {
						return fmt.Errorf("nudge PLR %s failed", plr.Name)
					}
					nudgePLR = plr
					return nil
				}, superLongTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), "nudge PipelineRun for component A should complete successfully")
			})

			ginkgo.It("should annotate source build PLR (component C push) as nudge-processed", func() {
				gomega.Eventually(func() bool {
					sourcePLR, err := f.AsKubeAdmin.HasController.GetComponentPipelineRun(componentC.Name, applicationName, testNamespace, mergeMultiResultSha)
					if err != nil || sourcePLR == nil {
						return false
					}
					return metadata.HasAnnotation(sourcePLR, tektonconsts.NudgeProcessedAnnotation)
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.BeTrue(), "source build PLR should be annotated with nudge-processed")
			})

			ginkgo.It("should remove nudge finalizer from source build PLR", func() {
				gomega.Eventually(func() bool {
					sourcePLR, err := f.AsKubeAdmin.HasController.GetComponentPipelineRun(componentC.Name, applicationName, testNamespace, mergeMultiResultSha)
					if err != nil || sourcePLR == nil {
						return false
					}
					return !controllerutil.ContainsFinalizer(sourcePLR, helpers.NudgePipelineRunFinalizer)
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.BeTrue(), "nudge finalizer should be removed from source build PLR")
			})

			ginkgo.It("should set correct labels and annotations on nudge PLR", func() {
				gomega.Expect(nudgePLR.Labels[tektonconsts.NudgeTypeLabel]).To(gomega.Equal(tektonconsts.NudgePipelineRunTypeValue))
				gomega.Expect(nudgePLR.Annotations[tektonconsts.NudgedComponentsAnnotation]).To(gomega.ContainSubstring(componentA.Name))
				gomega.Expect(nudgePLR.Annotations[tektonconsts.NudgingComponentAnnotation]).To(gomega.Equal(componentC.Name))
			})
		})

		ginkgo.When("ResolutionRequest is deleted after pipeline completes", func() {
			ginkgo.It("verifies that ResolutionRequest is deleted after pipeline resolution", func() {
				gomega.Eventually(func() error {
					relatedResolutionRequests, err := f.AsKubeDeveloper.IntegrationController.GetRelatedResolutionRequests(testNamespace, integrationTestScenarioPass)
					if err != nil {
						if strings.Contains(err.Error(), "ResolutionRequest CRD not available") {
							return nil
						}
						return fmt.Errorf("failed to get related ResolutionRequests: %v", err)
					}

					if len(relatedResolutionRequests) > 0 {
						names := f.AsKubeDeveloper.IntegrationController.GetResolutionRequestNames(relatedResolutionRequests)
						return fmt.Errorf("found %d ResolutionRequest(s) still present in namespace %s for scenario %s: %v", len(relatedResolutionRequests), testNamespace, integrationTestScenarioPass.Name, names)
					}

					return nil
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), "ResolutionRequest objects should be cleaned up after pipeline resolution is complete")
			})

			ginkgo.It("verifies that no orphaned ResolutionRequests remain in namespace after test completion", func() {
				relatedResolutionRequests, err := f.AsKubeDeveloper.IntegrationController.GetRelatedResolutionRequests(testNamespace, integrationTestScenarioPass)
				if err != nil {
					if strings.Contains(err.Error(), "ResolutionRequest CRD not available") {
						ginkgo.Skip("ResolutionRequest CRD not available in cluster, skipping orphan check")
						return
					}
					gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to check for orphaned ResolutionRequests")
				}

				if len(relatedResolutionRequests) > 0 {
					names := f.AsKubeDeveloper.IntegrationController.GetResolutionRequestNames(relatedResolutionRequests)
					fmt.Printf("Found %d ResolutionRequest(s) in namespace %s: %v\n", len(relatedResolutionRequests), testNamespace, names)

					currentTime := time.Now()
					oldResolutionRequests := []string{}

					for _, rr := range relatedResolutionRequests {
						creationTime := rr.GetCreationTimestamp()
						if currentTime.Sub(creationTime.Time) > 5*time.Minute {
							oldResolutionRequests = append(oldResolutionRequests, rr.GetName())
						}
					}

					gomega.Expect(oldResolutionRequests).To(gomega.BeEmpty(), "Found old ResolutionRequest objects that should have been cleaned up")
				}
			})
		})

		ginkgo.When("IntegrationTestScenario reference to task as pipelinerun resolution", func() {
			ginkgo.BeforeAll(func() {
				invalidIntegrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "pipelinerun", []string{"application"})
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.It("trigger pipelinerun for invalid integrationTestScenario by annotating snapshot and verify failing to create integration pipelinerun", func() {
				if groupSnapshot == nil {
					groupSnapshot = waitForGroupSnapshotWithInfo(f, applicationName, testNamespace, []string{componentA.Name, componentB.Name})
				}
				gomega.Eventually(func() error {
					err = f.AsKubeAdmin.IntegrationController.AddIntegrationTestRerunLabel(groupSnapshot, invalidIntegrationTestScenario.Name)
					if err != nil {
						return fmt.Errorf("failed to set annotation %s to group snapshot %s/%s", gitops.SnapshotIntegrationTestRun, groupSnapshot.Namespace, groupSnapshot.Name)
					}

					groupSnapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(groupSnapshot.Name, "", "", testNamespace)
					if err != nil {
						return fmt.Errorf("failing to get snapshot %s/%s", groupSnapshot.Namespace, groupSnapshot.Name)
					}

					statusDetail, err := f.AsKubeDeveloper.IntegrationController.GetIntegrationTestStatusDetailFromSnapshot(groupSnapshot, invalidIntegrationTestScenario.Name)
					if err != nil {
						return fmt.Errorf("failing to get snapshot integration test status detail %s/%s", groupSnapshot.Namespace, groupSnapshot.Name)
					}

					detailsNorm := strings.Join(strings.Fields(statusDetail.Details), " ")
					invalidResolutionMsg := "expected exactly one, got neither"
					validRefs := strings.Contains(detailsNorm, "spec.pipelineRef, spec.pipelineSpec") ||
						strings.Contains(detailsNorm, "pipelineRef, pipelineSpec") ||
						(strings.Contains(detailsNorm, "pipelineRef") && strings.Contains(detailsNorm, "pipelineSpec"))
					if !strings.Contains(detailsNorm, "denied the request") || !strings.Contains(detailsNorm, invalidResolutionMsg) || !validRefs {
						return fmt.Errorf("failing to find the integration test status detail %s/%s for invalid resolution, but found status details %s", groupSnapshot.Namespace, groupSnapshot.Name, statusDetail.Details)
					}

					return nil
				}, longTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), "timeout while waiting for group snapshot and failing integration pipelinerun with invalid resolution")
			})
		})
	})
})

type groupSnapshotBranchRef struct {
	repo string
	name string
}

func createGroupSnapshotComponentWithoutPACMergeRequest(f framework.Framework, testNamespace, applicationName, componentName, componentRepoURL, revision, contextDir string) *appstudioApi.Component {
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
					Revision:      revision,
					Context:       contextDir,
					DockerfileURL: dockerFileURL,
				},
			},
		},
	}
	component, err := f.AsKubeAdmin.HasController.CreateComponentWithoutPACMergeRequest(componentObj, testNamespace, applicationName, buildPipelineAnnotation)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return component
}

func createGroupSnapshotPullRequest(f *framework.Framework, repoName, baseRevision, baseBranchName, prBranchName string, files map[string]string) (int, string) {
	err := f.AsKubeAdmin.CommonController.Github.EnsureBranchExists(repoName, multiComponentDefaultBranch, fallbackBranchName)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	err = f.AsKubeAdmin.CommonController.Github.CreateRef(repoName, multiComponentDefaultBranch, baseRevision, baseBranchName)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	err = f.AsKubeAdmin.CommonController.Github.CreateRef(repoName, multiComponentDefaultBranch, baseRevision, prBranchName)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	for path, content := range files {
		_, err = f.AsKubeAdmin.CommonController.Github.CreateFile(repoName, path, content, prBranchName)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), fmt.Sprintf("error while creating file %s in repo %s", path, repoName))
	}
	pr, err := f.AsKubeAdmin.CommonController.Github.CreatePullRequest(repoName, "Prebuilt group snapshot PR", "sample pr body", prBranchName, baseBranchName)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	ginkgo.GinkgoWriter.Printf("PR #%d got created in %s with sha %s\n", pr.GetNumber(), repoName, pr.Head.GetSHA())
	return pr.GetNumber(), pr.Head.GetSHA()
}

func applyGroupSnapshotPrebuiltBuildPLR(f *framework.Framework, namespace, applicationName, componentName, image, repoName, repoURL string, prNumber int, sha, sourceBranch, targetBranch string, offset time.Duration) *pipeline.PipelineRun {
	plr, err := applyPrebuiltBuildPLR(f, namespace, applicationName, componentName, prebuiltBuildPLRSpec{
		Image:             image,
		PullRequestNumber: prNumber,
		SourceBranch:      sourceBranch,
		TargetBranch:      targetBranch,
		SHA:               sha,
		RepoName:          repoName,
		RepoURL:           repoURL,
		TimeOffset:        offset,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create group snapshot PipelineRun from pre-built artifact")
	return plr
}

func waitForGroupSnapshotWithInfo(f *framework.Framework, applicationName, namespace string, expectedSubstrings []string) *appstudioApi.Snapshot {
	var selected *appstudioApi.Snapshot
	gomega.Eventually(func() error {
		groupSnapshots, err := f.AsKubeAdmin.HasController.GetAllGroupSnapshotsForApplication(applicationName, namespace)
		if err != nil {
			return err
		}
		for i := range groupSnapshots.Items {
			snap := &groupSnapshots.Items[i]
			groupTestInfo := snap.Annotations[testGroupSnapshotAnnotation]
			if groupTestInfo == "" {
				continue
			}
			missing := []string{}
			for _, expected := range expectedSubstrings {
				if !strings.Contains(groupTestInfo, expected) {
					missing = append(missing, expected)
				}
			}
			if len(missing) == 0 {
				selected = snap.DeepCopy()
				return nil
			}
			ginkgo.GinkgoWriter.Printf("Group snapshot %s is missing expected substrings %v in group-test-info %s\n", snap.Name, missing, groupTestInfo)
		}
		return fmt.Errorf("no group snapshot for application %s contains expected substrings %v", applicationName, expectedSubstrings)
	}, 30*time.Minute, 30*time.Second).Should(gomega.Succeed(), "Timeout while waiting for group snapshot creation")
	return selected
}

func deleteGroupSnapshotSuiteResources(f framework.Framework, testNamespace, applicationName string, integrationTestScenarioPass, invalidIntegrationTestScenario *integrationv1beta2.IntegrationTestScenario, components ...*appstudioApi.Component) {
	for _, scenario := range []*integrationv1beta2.IntegrationTestScenario{integrationTestScenarioPass, invalidIntegrationTestScenario} {
		if scenario != nil {
			if err := f.AsKubeAdmin.IntegrationController.DeleteIntegrationTestScenario(scenario, testNamespace); err != nil {
				ginkgo.GinkgoWriter.Printf("Failed to delete IntegrationTestScenario %s: %v\n", scenario.Name, err)
			}
		}
	}
	for _, component := range components {
		if component != nil {
			if err := f.AsKubeAdmin.HasController.DeleteComponent(component.Name, testNamespace, false); err != nil {
				ginkgo.GinkgoWriter.Printf("Failed to delete Component %s: %v\n", component.Name, err)
			}
		}
	}
	if applicationName != "" {
		if err := f.AsKubeAdmin.HasController.DeleteApplication(applicationName, testNamespace, false); err != nil {
			ginkgo.GinkgoWriter.Printf("Failed to delete Application %s: %v\n", applicationName, err)
		}
	}
}
