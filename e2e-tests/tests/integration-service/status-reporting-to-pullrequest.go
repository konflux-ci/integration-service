package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/framework"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	"github.com/konflux-ci/integration-service/gitops"

	appstudioApi "github.com/konflux-ci/application-api/api/v1alpha1"
	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/operator-toolkit/metadata"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

var _ = framework.IntegrationServiceSuiteDescribe("Status Reporting of Integration tests", ginkgo.Label("integration-service", "github-status-reporting"), func() {
	defer ginkgo.GinkgoRecover()

	var f *framework.Framework
	var err error

	var prNumber int
	var prHeadSha, prBranchName string
	var snapshot, snapshotPush *appstudioApi.Snapshot
	var component *appstudioApi.Component
	var pipelineRun, pipelineRunPush, testPipelinerun, failedPipelineRun *tektonv1.PipelineRun
	var integrationTestScenarioPass, integrationTestScenarioFail, integrationTestScenarioOptional *integrationv1beta2.IntegrationTestScenario
	var applicationName, componentName, testNamespace string

	ginkgo.AfterEach(framework.ReportFailure(&f))

	ginkgo.Describe("with status reporting of Integration tests in CheckRuns", ginkgo.Ordered, func() {
		ginkgo.BeforeAll(func() {
			if os.Getenv(constants.SKIP_PAC_TESTS_ENV) == "true" {
				ginkgo.Skip("Skipping this test due to configuration issue with Spray proxy")
			}

			f, err = framework.NewFramework(utils.GetGeneratedNamespace("stat-rep"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			testNamespace = f.UserNamespace

			if utils.IsPrivateHostname(f.OpenshiftConsoleHost) {
				ginkgo.Skip("Using private cluster (not reachable from Github), skipping...")
			}

			componentName = fmt.Sprintf("%s-%s", "test-component-prebuilt", utils.GenerateRandomString(6))
			prNumber, prHeadSha, prBranchName, err = createEphemeralStatusReportingPullRequest(f, testNamespace, componentName)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			applicationName = createApp(*f, testNamespace)

			// Wait for application to be available before creating integration test scenarios
			gomega.Eventually(func() error {
				_, err := f.AsKubeAdmin.HasController.GetApplication(applicationName, testNamespace)
				return err
			}, time.Minute*2, time.Second*5).Should(gomega.Succeed(),
				fmt.Sprintf("Application %s should be available in namespace %s", applicationName, testNamespace))

			component = createComponentWithoutPACMergeRequestForRepo(*f, testNamespace, applicationName, componentName, componentGitSourceURLForStatusReporting, componentDefaultBranch)
			gomega.Eventually(func() error {
				_, err := f.AsKubeAdmin.TektonController.GetRepositoryParams(componentName, testNamespace)
				return err
			}, time.Minute*2, time.Second*5).Should(gomega.Succeed(),
				fmt.Sprintf("PaC Repository for Component %s should be available in namespace %s", componentName, testNamespace))

			integrationTestScenarioPass, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "", []string{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			integrationTestScenarioFail, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoFail, "", []string{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			integrationTestScenarioOptional, err = f.AsKubeAdmin.IntegrationController.CreateOptionalIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoFail, "", []string{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.AfterAll(func() {
			if !ginkgo.CurrentSpecReport().Failed() {
				cleanup(*f, testNamespace, applicationName, componentName, snapshotPush)
			}

			if prNumber != 0 {
				err = f.AsKubeAdmin.CommonController.Github.ClosePullRequest(componentRepoNameForStatusReporting, prNumber)
				if err != nil {
					ginkgo.GinkgoWriter.Printf("failed to close pull request #%d in repository %s: %v\n", prNumber, componentRepoNameForStatusReporting, err)
				}
			}
			if prBranchName != "" {
				err = f.AsKubeAdmin.CommonController.Github.DeleteRef(componentRepoNameForStatusReporting, prBranchName)
				if err != nil && !strings.Contains(err.Error(), referenceDoesntExist) {
					ginkgo.GinkgoWriter.Printf("failed to delete branch %s in repository %s: %v\n", prBranchName, componentRepoNameForStatusReporting, err)
				}
			}
		})

		ginkgo.When("a new Component with specified custom branch is created", ginkgo.Label("custom-branch"), func() {
			ginkgo.It("should have a related ephemeral PR created", func() {
				gomega.Expect(prNumber).To(gomega.BeNumerically(">", 0))
				gomega.Expect(prHeadSha).ToNot(gomega.BeEmpty())
				gomega.Expect(prBranchName).ToNot(gomega.BeEmpty())
			})

			ginkgo.It("applies a pre-built pull request PipelineRun", func() {
				pipelineRun, err = applyPrebuiltStatusReportingBuildPLR(f, testNamespace, applicationName, componentName, prNumber, prHeadSha, prBranchName, prebuiltBuildPLRSpec{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create status-reporting pre-built PipelineRun")
				gomega.Expect(pipelineRun.Annotations[snapshotAnnotation]).To(gomega.Equal(""))
			})

			ginkgo.It("initialized integration test status is reported to github", func() {
				gomega.Eventually(func() error {
					status, err := f.AsKubeAdmin.CommonController.Github.GetCheckRunStatus(integrationTestScenarioPass.Name, componentRepoNameForStatusReporting, prHeadSha, prNumber)
					if err != nil {
						return fmt.Errorf("error occurred when checking integration test checkRun: %w", err)
					}
					if status != "queued" && status != "in_progress" && status != checkrunStatusCompleted {
						return fmt.Errorf("unexpected integration test checkRun status %q", status)
					}
					return nil
				}, longTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), fmt.Sprintf("timed out when waiting for the checkrun for the component  %s/%s and integrationTestScenarioPass %s", testNamespace, componentName, integrationTestScenarioPass.Name))
			})

			ginkgo.It("should lead to build PipelineRun finishing successfully", func() {
				logs, err := f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineToBeFinished(testNamespace, applicationName, component.Name, "")
				gomega.Expect(err).Should(gomega.Succeed(), fmt.Sprintf("build pipelinerun fails for NameSpace/Application/Component %s/%s/%s with logs: %s", testNamespace, applicationName, componentName, logs))
			})
		})

		ginkgo.When("the PaC build pipelineRun run succeeded", func() {
			ginkgo.It("checks if the BuildPipelineRun have the annotation of chains signed", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, chainsSignedAnnotation)).To(gomega.Succeed())
			})

			ginkgo.It("checks if the Snapshot is created", func() {
				snapshot, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", pipelineRun.Name, componentName, testNamespace)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.It("checks if the Build PipelineRun got annotated with Snapshot name", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, snapshotAnnotation)).To(gomega.Succeed())
			})
		})

		ginkgo.When("the Snapshot was created", func() {
			ginkgo.It("should find both the related Integration PipelineRuns", func() {
				testPipelinerun, err = f.AsKubeDeveloper.IntegrationController.WaitForIntegrationPipelineToGetStarted(integrationTestScenarioPass.Name, snapshot.Name, testNamespace)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(testPipelinerun.Labels[snapshotAnnotation]).To(gomega.ContainSubstring(snapshot.Name))
				gomega.Expect(testPipelinerun.Labels[scenarioAnnotation]).To(gomega.ContainSubstring(integrationTestScenarioPass.Name))

				testPipelinerun, err = f.AsKubeDeveloper.IntegrationController.WaitForIntegrationPipelineToGetStarted(integrationTestScenarioFail.Name, snapshot.Name, testNamespace)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(testPipelinerun.Labels[snapshotAnnotation]).To(gomega.ContainSubstring(snapshot.Name))
				gomega.Expect(testPipelinerun.Labels[scenarioAnnotation]).To(gomega.ContainSubstring(integrationTestScenarioFail.Name))

				testPipelinerun, err = f.AsKubeDeveloper.IntegrationController.WaitForIntegrationPipelineToGetStarted(integrationTestScenarioOptional.Name, snapshot.Name, testNamespace)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(testPipelinerun.Labels[snapshotAnnotation]).To(gomega.ContainSubstring(snapshot.Name))
				gomega.Expect(testPipelinerun.Labels[scenarioAnnotation]).To(gomega.ContainSubstring(integrationTestScenarioOptional.Name))
			})
		})

		ginkgo.When("Integration PipelineRuns are created", func() {
			ginkgo.It("should eventually complete successfully", func() {
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioPass, snapshot, testNamespace)).To(gomega.Succeed(), fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshot.GetName()))
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioFail, snapshot, testNamespace)).To(gomega.Succeed(), fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshot.GetName()))
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioOptional, snapshot, testNamespace)).To(gomega.Succeed(), fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshot.GetName()))
			})
		})

		ginkgo.When("Integration PipelineRuns completes successfully", func() {
			ginkgo.It("should lead to Snapshot CR being marked as failed", ginkgo.FlakeAttempts(3), func() {
				// Snapshot marked as Failed because one of its Integration test failed (as expected)
				pipelineRun, err = f.AsKubeAdmin.HasController.GetComponentPipelineRunWithType(componentName, applicationName, testNamespace, "build", prHeadSha, "")
				gomega.Expect(err).Should(gomega.Succeed())
				gomega.Eventually(func() bool {
					pipelineRun, err = f.AsKubeAdmin.HasController.GetComponentPipelineRunWithType(componentName, applicationName, testNamespace, "build", prHeadSha, "")
					if err != nil {
						return false
					}
					snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot("", pipelineRun.Name, "", testNamespace)
					return err == nil && !gitops.HaveAppStudioTestsSucceeded(snapshot)
				}, time.Minute*3, time.Second*5).Should(gomega.BeTrue(), fmt.Sprintf("Timed out waiting for Snapshot to be marked as failed %s/%s", snapshot.GetNamespace(), snapshot.GetName()))
			})

			ginkgo.It("eventually leads to the status reported at Checks tab for the successful Integration PipelineRun", func() {
				gomega.Expect(f.AsKubeAdmin.CommonController.Github.GetCheckRunConclusion(integrationTestScenarioPass.Name, componentRepoNameForStatusReporting, prHeadSha, prNumber)).To(gomega.Equal(constants.CheckrunConclusionSuccess))
			})

			ginkgo.It("eventually leads to the status reported at Checks tab for the failed Integration PipelineRun", func() {
				gomega.Expect(f.AsKubeAdmin.CommonController.Github.GetCheckRunConclusion(integrationTestScenarioFail.Name, componentRepoNameForStatusReporting, prHeadSha, prNumber)).To(gomega.Equal(constants.CheckrunConclusionFailure))
			})

			ginkgo.It("eventually leads to the status reported at Checks tab for the optional Integration PipelineRun", func() {
				gomega.Expect(f.AsKubeAdmin.CommonController.Github.GetCheckRunConclusion(integrationTestScenarioOptional.Name, componentRepoNameForStatusReporting, prHeadSha, prNumber)).To(gomega.Equal(constants.CheckrunConclusionNeutral))
			})

			ginkgo.It("checks if the optional Integration Test Scenario status is reported in the Snapshot", func() {
				gomega.Eventually(func() error {
					snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

					statusDetail, err := f.AsKubeDeveloper.IntegrationController.GetIntegrationTestStatusDetailFromSnapshot(snapshot, integrationTestScenarioOptional.Name)
					gomega.Expect(err).ToNot(gomega.HaveOccurred())

					if statusDetail.Status != intgteststat.IntegrationTestStatusTestFail {
						return fmt.Errorf("test status doesn't have expected value %s", intgteststat.IntegrationTestStatusTestFail)
					}
					return nil
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed())
			})

			ginkgo.It("checks if the finalizer was removed from the optional Integration PipelineRun", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForFinalizerToGetRemovedFromIntegrationPipeline(integrationTestScenarioOptional, snapshot, testNamespace)).To(gomega.Succeed())
			})

			ginkgo.It("applies a pre-built push PipelineRun", func() {
				pipelineRunPush, err = applyPrebuiltStatusReportingBuildPLR(f, testNamespace, applicationName, componentName, prNumber, prHeadSha, componentDefaultBranch, prebuiltBuildPLRSpec{
					EventType:  gitops.PipelineAsCodePushType,
					TimeOffset: 2 * time.Minute,
				})
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create status-reporting pre-built push PipelineRun")
			})

			ginkgo.It("checks if the push Snapshot is created", func() {
				snapshotPush, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", pipelineRunPush.Name, componentName, testNamespace)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.It("verifies that Push PipelineRuns completed", func() {
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioPass, snapshotPush, testNamespace)).To(gomega.Succeed(), fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshotPush.GetName()))
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioFail, snapshotPush, testNamespace)).To(gomega.Succeed(), fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshotPush.GetName()))
			})

			ginkgo.It("validates the Integration test scenario PipelineRun is reported to merge request CheckRuns, and it pass", func() {
				gomega.Expect(f.AsKubeAdmin.CommonController.Github.GetCheckRunConclusion(integrationTestScenarioPass.Name, componentRepoNameForStatusReporting, prHeadSha, prNumber)).To(gomega.Equal(constants.CheckrunConclusionSuccess))

			})

			ginkgo.It("eventually leads to the status reported at Checks tab for the failed Integration PipelineRun", func() {
				gomega.Expect(f.AsKubeAdmin.CommonController.Github.GetCheckRunConclusion(integrationTestScenarioFail.Name, componentRepoNameForStatusReporting, prHeadSha, prNumber)).To(gomega.Equal(constants.CheckrunConclusionFailure))
			})
		})

		ginkgo.When("The git-provider annotation is missing", func() {
			ginkgo.It("should set the git-reporting-failure annotation correctly", func() {
				// Get snapshot from the cluster
				snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				updatedSnapshot := snapshot.DeepCopy()

				// update the provider annotation to a non-existent provider and reset the git-reporting-failure annotation
				err = metadata.SetAnnotation(updatedSnapshot, pipelinesAsCodeGitProviderAnnotation, "not-supported-provider")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				err = metadata.DeleteAnnotation(updatedSnapshot, gitReportingFailureAnnotation)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				// Change lastUpdateTime for one scenario in order to trigger re-reconciliation from statusreport controller
				currentStatus := updatedSnapshot.Annotations[snapshotStatusAnnotation]
				var arr []map[string]any
				err = json.Unmarshal([]byte(currentStatus), &arr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(arr).ToNot(gomega.BeEmpty())
				arr[0]["lastUpdateTime"] = time.Now().UTC().Format(time.RFC3339)
				newStatusBytes, err := json.Marshal(arr)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				err = metadata.SetAnnotation(updatedSnapshot, snapshotStatusAnnotation, string(newStatusBytes))
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				gomega.Expect(f.AsKubeAdmin.IntegrationController.PatchSnapshot(snapshot, updatedSnapshot)).Should(gomega.Succeed())
				gomega.Eventually(func() error {
					reconciledSnapshot, err := f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", snapshot.Namespace)
					if err != nil {
						ginkgo.GinkgoWriter.Printf("failed to get snapshot: %v", err)
						return fmt.Errorf("error occurred when fetching snapshot to check gitReportingFailureAnnotation: %v", err)
					}
					val, ok := reconciledSnapshot.Annotations[gitReportingFailureAnnotation]
					if !ok {
						ginkgo.GinkgoWriter.Printf("gitReportingFailureAnnotation does not exist. Annotations: %+v", reconciledSnapshot.Annotations)
						return fmt.Errorf("gitReportingFailureAnnotation does not exist. Annotations: %+v", reconciledSnapshot.Annotations)
					}
					if val == "" {
						ginkgo.GinkgoWriter.Printf("gitReportingFailureAnnotation is empty. Annotations: %+v", reconciledSnapshot.Annotations)
						return fmt.Errorf("gitReportingFailureAnnotation is empty. Annotations: %+v", reconciledSnapshot.Annotations)
					}
					return nil
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), "git-reporting-failure-annotation was never set")
			})
		})

		ginkgo.When("build pipelinerun fails", func() {
			ginkgo.It("build pipelinerun is created but fails", func() {
				failedPipelineRun, err = applyPrebuiltStatusReportingBuildPLR(f, testNamespace, applicationName, componentName, prNumber, prHeadSha, prBranchName, prebuiltBuildPLRSpec{
					TimeOffset:            4 * time.Minute,
					BreakSnapshotCreation: true,
				})
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create status-reporting snapshot-creation-failure pre-built PipelineRun")
				gomega.Expect(failedPipelineRun).NotTo(gomega.BeNil())
			})

			ginkgo.It("build pipelinerun failure is reported to integration test checkRun", func() {
				gomega.Eventually(func() error {
					text, err := f.AsKubeAdmin.CommonController.Github.GetCheckRunText(integrationTestScenarioPass.Name, componentRepoNameForStatusReporting, prHeadSha, prNumber)
					if err != nil || !strings.Contains(text, "Failed to create snapshot") {
						ginkgo.GinkgoWriter.Printf("failed to check expected checkRun text, actual text is %s: \n", err, text)
						return fmt.Errorf("error occurred when checking failing integration test checkRun text")
					}
					return nil
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), fmt.Sprintf("timed out when waiting for the failing checkrun for the component  %s/%s and integrationTestScenarioPass %s", testNamespace, componentName, integrationTestScenarioPass.Name))
			})
		})

	})
})

func createEphemeralStatusReportingPullRequest(f *framework.Framework, testNamespace, componentName string) (int, string, string, error) {
	branchName := fmt.Sprintf("mock-plr-pr-%s-%s", testNamespace, utils.GenerateRandomString(6))
	if err := f.AsKubeAdmin.CommonController.Github.EnsureBranchExists(componentRepoNameForStatusReporting, componentDefaultBranch, fallbackBranchName); err != nil {
		return 0, "", "", fmt.Errorf("failed to ensure base branch %q exists: %w", componentDefaultBranch, err)
	}
	if err := f.AsKubeAdmin.CommonController.Github.CreateRef(componentRepoNameForStatusReporting, componentDefaultBranch, "", branchName); err != nil {
		return 0, "", branchName, fmt.Errorf("failed to create status-reporting PR branch %q: %w", branchName, err)
	}

	content := fmt.Sprintf("Temporary file for integration-service status-reporting e2e tests.\n\nNamespace: %s\nComponent: %s\nBranch: %s\nCreatedAt: %s\nPurpose: provide a harmless diff for the prebuilt PipelineRun PR reporting test.\n",
		testNamespace, componentName, branchName, time.Now().UTC().Format(time.RFC3339))
	if _, err := f.AsKubeAdmin.CommonController.Github.CreateFile(componentRepoNameForStatusReporting, statusReportingDummyPRFilePath, content, branchName); err != nil {
		return 0, "", branchName, fmt.Errorf("failed to create status-reporting PR dummy file %q on branch %q: %w", statusReportingDummyPRFilePath, branchName, err)
	}

	pr, err := f.AsKubeAdmin.CommonController.Github.CreatePullRequest(
		componentRepoNameForStatusReporting,
		fmt.Sprintf("E2E prebuilt PLR status reporting %s", testNamespace),
		"Temporary PR for integration-service status-reporting e2e tests. This PR is closed automatically.",
		branchName,
		componentDefaultBranch,
	)
	if err != nil {
		return 0, "", branchName, fmt.Errorf("failed to create status-reporting pull request from branch %q: %w", branchName, err)
	}
	if pr.GetNumber() == 0 || pr.Head.GetSHA() == "" {
		return pr.GetNumber(), pr.Head.GetSHA(), branchName, fmt.Errorf("created status-reporting pull request from branch %q but PR number or head SHA was empty", branchName)
	}
	return pr.GetNumber(), pr.Head.GetSHA(), branchName, nil
}

func applyPrebuiltStatusReportingBuildPLR(f *framework.Framework, namespace, applicationName, componentName string, prNumber int, prHeadSha, sourceBranch string, spec prebuiltBuildPLRSpec) (*tektonv1.PipelineRun, error) {
	if spec.Image == "" {
		spec.Image = prebuiltImageForStatusReporting
	}
	if spec.PullRequestNumber == 0 {
		spec.PullRequestNumber = prNumber
	}
	if spec.SHA == "" {
		spec.SHA = prHeadSha
	}
	if spec.SourceBranch == "" {
		spec.SourceBranch = sourceBranch
	}
	if spec.TargetBranch == "" {
		spec.TargetBranch = componentDefaultBranch
	}
	if spec.RepoName == "" {
		spec.RepoName = componentRepoNameForStatusReporting
	}
	if spec.RepoURL == "" {
		spec.RepoURL = componentGitSourceURLForStatusReporting
	}
	if spec.SourceRepoOrg == "" {
		spec.SourceRepoOrg = utils.GetEnv(constants.GITHUB_E2E_ORGANIZATION_ENV, "redhat-appstudio-qe")
	}
	if spec.GitProvider == "" {
		spec.GitProvider = gitops.PipelineAsCodeGitHubProviderType
	}
	return applyPrebuiltBuildPLR(f, namespace, applicationName, componentName, spec)
}
