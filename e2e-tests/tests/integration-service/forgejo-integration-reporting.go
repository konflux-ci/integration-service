package integration

import (
	"fmt"
	"os"
	"strings"
	"time"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"

	appstudioApi "github.com/konflux-ci/application-api/api/v1alpha1"
	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/git"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/framework"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils/build"
	"github.com/konflux-ci/integration-service/gitops"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

var _ = framework.IntegrationServiceSuiteDescribe("Forgejo Status Reporting of Integration tests", ginkgo.Label("integration-service", "forgejo-status-reporting"), func() {
	defer ginkgo.GinkgoRecover()

	var f *framework.Framework
	var err error

	var mrID int
	var mrSha, forgejoToken, prBranchName string
	var snapshot, snapshotPush *appstudioApi.Snapshot
	var component *appstudioApi.Component
	var buildPipelineRun, buildPipelineRunPush, testPipelinerun *pipeline.PipelineRun
	var integrationTestScenarioPass *integrationv1beta2.IntegrationTestScenario
	var applicationName, componentName, testNamespace string

	var gitClient git.Client
	var forgejoGitClient *git.ForgejoClient
	var reportingRepoURL, reportingRepository string

	ginkgo.AfterEach(framework.ReportFailure(&f))

	ginkgo.Describe("Forgejo with status reporting of Integration tests in the associated merge request", ginkgo.Ordered, func() {
		ginkgo.BeforeAll(func() {
			if os.Getenv(constants.SKIP_PAC_TESTS_ENV) == "true" {
				ginkgo.Skip("Skipping this test due to configuration issue with Spray proxy")
			}

			f, err = framework.NewFramework(utils.GetGeneratedNamespace("forgejo-rep"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			testNamespace = f.UserNamespace

			if utils.IsPrivateHostname(f.OpenshiftConsoleHost) {
				ginkgo.Skip("Using private cluster (not reachable from Forgejo/Codeberg), skipping...")
			}

			if f.AsKubeAdmin.CommonController.Forgejo == nil {
				ginkgo.Fail("Codeberg/Forgejo client unavailable; check Codeberg access from CI")
			}

			forgejoToken = utils.GetEnv(constants.CODEBERG_BOT_TOKEN_ENV, "")
			gomega.Expect(forgejoToken).ShouldNot(gomega.BeEmpty(), fmt.Sprintf("'%s' env var is not set", constants.CODEBERG_BOT_TOKEN_ENV))
			forgejoGitClient = git.NewForgejoClient(f.AsKubeAdmin.CommonController.Forgejo)
			gitClient = forgejoGitClient

			reportingRepository = fmt.Sprintf("%s/%s", forgejoOrg, forgejoComponentRepoName+"-"+utils.GenerateRandomString(6))
			err = gitClient.ForkRepository(forgejoProjectIDForStatusReporting, reportingRepository)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			reportingRepoURL = fmt.Sprintf("https://codeberg.org/%s", reportingRepository)

			componentName = fmt.Sprintf("%s-%s", "test-comp-prebuilt-forgejo", utils.GenerateRandomString(6))
			mrID, mrSha, prBranchName, err = createEphemeralForgejoStatusReportingPullRequest(gitClient, reportingRepository, testNamespace, componentName)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			applicationName = createApp(*f, testNamespace)

			gomega.Eventually(func() error {
				_, err := f.AsKubeAdmin.HasController.GetApplication(applicationName, testNamespace)
				return err
			}, time.Minute*2, time.Second*5).Should(gomega.Succeed(),
				fmt.Sprintf("Application %s should be available in namespace %s", applicationName, testNamespace))

			integrationTestScenarioPass, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "", []string{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			err = build.CreateCodebergBuildSecret(f, "forgejo-build-secret", map[string]string{}, forgejoToken)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			component = createForgejoComponentWithoutPACMergeRequest(*f, testNamespace, applicationName, componentName, reportingRepoURL)
			gomega.Eventually(func() error {
				_, getErr := f.AsKubeAdmin.TektonController.GetRepositoryParams(componentName, testNamespace)
				return getErr
			}, time.Minute*5, time.Second*5).Should(gomega.Succeed(),
				fmt.Sprintf("timed out waiting for PaC Repository CR for component %s in namespace %s", componentName, testNamespace))
		})

		ginkgo.AfterAll(func() {
			if mrID != 0 {
				if err := gitClient.DeleteBranchAndClosePullRequest(reportingRepository, mrID); err != nil {
					ginkgo.GinkgoWriter.Printf("failed to close pull request #%d and delete branch in repository %s: %v\n", mrID, reportingRepository, err)
				}
			} else if prBranchName != "" {
				if err := gitClient.DeleteBranch(reportingRepository, prBranchName); err != nil {
					ginkgo.GinkgoWriter.Printf("failed to delete branch %s in repository %s: %v\n", prBranchName, reportingRepository, err)
				}
			}

			if reportingRepository != "" {
				if err := gitClient.DeleteRepositoryIfExists(reportingRepository); err != nil {
					ginkgo.GinkgoWriter.Printf("failed to delete repository %s: %v\n", reportingRepository, err)
				}
			}

			if !ginkgo.CurrentSpecReport().Failed() {
				gomega.Expect(f.AsKubeAdmin.HasController.DeleteAllComponentsInASpecificNamespace(testNamespace, time.Minute*2)).To(gomega.Succeed())
				gomega.Expect(f.AsKubeAdmin.HasController.DeleteAllApplicationsInASpecificNamespace(testNamespace, time.Minute*2)).To(gomega.Succeed())
			}
		})

		ginkgo.When("a new Component with specified custom branch is created", ginkgo.Label("custom-branch"), func() {
			ginkgo.It("should have a related ephemeral PR created", func() {
				gomega.Expect(mrID).NotTo(gomega.BeZero())
				gomega.Expect(mrSha).ToNot(gomega.BeEmpty())
				gomega.Expect(prBranchName).ToNot(gomega.BeEmpty())
			})

			ginkgo.It("applies a pre-built pull request PipelineRun", func() {
				buildPipelineRun, err = applyPrebuiltForgejoStatusReportingBuildPLR(f, testNamespace, applicationName, componentName, mrID, mrSha, prBranchName, reportingRepoURL, reportingRepository, prebuiltBuildPLRSpec{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Forgejo status-reporting pre-built PipelineRun")
			})

			ginkgo.It("does not contain an annotation with a Snapshot Name", func() {
				gomega.Expect(buildPipelineRun.Annotations[snapshotAnnotation]).To(gomega.Equal(""))
			})

			ginkgo.It(fmt.Sprintf("the PipelineRun should eventually finish successfully for component %s", componentName), func() {
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForBuildPipelineToBeFinished(testNamespace, applicationName, component.Name, mrSha)).To(gomega.Succeed())
			})
		})

		ginkgo.When("the PaC build pipelineRun run succeeded", func() {
			ginkgo.It("checks if the BuildPipelineRun has the annotation of chains signed", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, chainsSignedAnnotation)).To(gomega.Succeed())
			})

			ginkgo.It("checks if the Snapshot is created", func() {
				snapshot, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", buildPipelineRun.Name, componentName, testNamespace)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.It("checks if the Build PipelineRun got annotated with Snapshot name", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, snapshotAnnotation)).To(gomega.Succeed())
			})
		})

		ginkgo.When("the Snapshot was created", func() {
			ginkgo.It("should find the Integration Test Scenario PipelineRun", func() {
				testPipelinerun, err = f.AsKubeDeveloper.IntegrationController.WaitForIntegrationPipelineToGetStarted(integrationTestScenarioPass.Name, snapshot.Name, testNamespace)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				gomega.Expect(testPipelinerun.Labels[snapshotAnnotation]).To(gomega.ContainSubstring(snapshot.Name))
				gomega.Expect(testPipelinerun.Labels[scenarioAnnotation]).To(gomega.ContainSubstring(integrationTestScenarioPass.Name))
			})
		})

		ginkgo.When("Integration PipelineRun is created", func() {
			ginkgo.It("should eventually complete successfully", func() {
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioPass, snapshot, testNamespace)).To(gomega.Succeed(),
					fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshot.GetName()))
			})

			ginkgo.It("eventually leads to the integration test PipelineRun's Pass status reported at MR commit status", func() {
				gomega.Expect(
					forgejoGitClient.GetCommitStatusConclusion(integrationTestScenarioPass.Name, reportingRepository, mrSha, mrID),
				).To(gomega.Equal(integrationPipelineRunCommitStatusSuccess))
			})

			ginkgo.It("validates at least one MR comment contains the final integration test result", func() {
				gomega.Eventually(func() bool {
					owner, repo, ok := strings.Cut(reportingRepository, "/")
					if !ok {
						return false
					}
					comments, _, err := f.AsKubeAdmin.CommonController.Forgejo.GetClient().ListIssueComments(owner, repo, int64(mrID), forgejo.ListIssueCommentOptions{})
					if err != nil {
						ginkgo.GinkgoWriter.Printf("failed to list issue/PR comments: %v\n", err)
						return false
					}
					for _, c := range comments {
						if c == nil {
							continue
						}
						body := c.Body
						if strings.Contains(body, integrationTestScenarioPass.Name) {
							return true
						}
						if strings.Contains(body, "integration") &&
							(strings.Contains(body, "pass") || strings.Contains(body, "success")) {
							return true
						}
					}
					return false
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.BeTrue(),
					fmt.Sprintf("no MR comment found containing integration test result for PR #%d in project %s", mrID, reportingRepository))
			})

			ginkgo.It("applies a pre-built push PipelineRun", func() {
				buildPipelineRunPush, err = applyPrebuiltForgejoStatusReportingBuildPLR(f, testNamespace, applicationName, componentName, mrID, mrSha, componentDefaultBranch, reportingRepoURL, reportingRepository, prebuiltBuildPLRSpec{
					EventType:  gitops.PipelineAsCodePushType,
					TimeOffset: 2 * time.Minute,
				})
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to create Forgejo status-reporting pre-built push PipelineRun")
			})

			ginkgo.It("checks if the push Snapshot is created", func() {
				snapshotPush, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", buildPipelineRunPush.Name, componentName, testNamespace)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})
		})

		ginkgo.When("Run integration tests after Merged MR", func() {
			ginkgo.It("should eventually complete successfully", func() {
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioPass, snapshotPush, testNamespace)).To(gomega.Succeed(),
					fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshotPush.GetName()))
			})

			ginkgo.It("eventually leads to the integration test PipelineRun's Pass status reported at MR commit status", func() {
				gomega.Expect(
					forgejoGitClient.GetCommitStatusConclusion(integrationTestScenarioPass.Name, reportingRepository, mrSha, mrID),
				).To(gomega.Equal(integrationPipelineRunCommitStatusSuccess))
			})
		})
	})
})

func createEphemeralForgejoStatusReportingPullRequest(gitClient git.Client, reportingRepository, testNamespace, componentName string) (int, string, string, error) {
	branchName := fmt.Sprintf("mock-plr-pr-%s-%s", testNamespace, utils.GenerateRandomString(6))
	if err := gitClient.CreateBranch(reportingRepository, componentDefaultBranch, "", branchName); err != nil {
		return 0, "", branchName, fmt.Errorf("failed to create Forgejo status-reporting PR branch %q: %w", branchName, err)
	}

	content := fmt.Sprintf("Temporary file for integration-service Forgejo status-reporting e2e tests.\n\nNamespace: %s\nComponent: %s\nBranch: %s\nCreatedAt: %s\nPurpose: provide a harmless diff for the prebuilt PipelineRun PR reporting test.\n",
		testNamespace, componentName, branchName, time.Now().UTC().Format(time.RFC3339))
	file, err := gitClient.CreateFile(reportingRepository, forgejoStatusReportingDummyPRFilePath, content, branchName)
	if err != nil {
		return 0, "", branchName, fmt.Errorf("failed to create Forgejo status-reporting PR dummy file %q on branch %q: %w", forgejoStatusReportingDummyPRFilePath, branchName, err)
	}

	pr, err := gitClient.CreatePullRequest(
		reportingRepository,
		fmt.Sprintf("E2E prebuilt PLR status reporting %s", testNamespace),
		"Temporary PR for integration-service Forgejo status-reporting e2e tests. This PR is closed automatically.",
		branchName,
		componentDefaultBranch,
	)
	if err != nil {
		return 0, "", branchName, fmt.Errorf("failed to create Forgejo status-reporting pull request from branch %q: %w", branchName, err)
	}

	sha := pr.HeadSHA
	if sha == "" && file != nil {
		sha = file.CommitSHA
	}
	if pr.Number == 0 || sha == "" {
		return pr.Number, sha, branchName, fmt.Errorf("created Forgejo status-reporting pull request from branch %q but PR number or head SHA was empty", branchName)
	}
	return pr.Number, sha, branchName, nil
}

func createForgejoComponentWithoutPACMergeRequest(f framework.Framework, testNamespace, applicationName, componentName, reportingRepoURL string) *appstudioApi.Component {
	componentObj := appstudioApi.ComponentSpec{
		ComponentName: componentName,
		Application:   applicationName,
		Source: appstudioApi.ComponentSource{
			ComponentSourceUnion: appstudioApi.ComponentSourceUnion{
				GitSource: &appstudioApi.GitSource{
					URL:           reportingRepoURL,
					Revision:      componentDefaultBranch,
					DockerfileURL: "Dockerfile",
				},
			},
		},
	}
	buildPipelineAnnotation := build.GetBuildPipelineBundleAnnotation(constants.DockerBuildOciTAMin)
	gitProviderAnnotation := map[string]string{"git-provider": "forgejo"}

	component, err := f.AsKubeAdmin.HasController.CreateComponentWithoutPACMergeRequest(componentObj, testNamespace, applicationName,
		utils.MergeMaps(buildPipelineAnnotation, gitProviderAnnotation))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return component
}

func applyPrebuiltForgejoStatusReportingBuildPLR(f *framework.Framework, namespace, applicationName, componentName string, prNumber int, prHeadSha, sourceBranch, reportingRepoURL, reportingRepository string, spec prebuiltBuildPLRSpec) (*pipeline.PipelineRun, error) {
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
		_, repo, ok := strings.Cut(reportingRepository, "/")
		if ok {
			spec.RepoName = repo
		}
	}
	if spec.RepoURL == "" {
		spec.RepoURL = reportingRepoURL
	}
	if spec.SourceRepoOrg == "" {
		spec.SourceRepoOrg = forgejoOrg
	}
	if spec.GitProvider == "" {
		spec.GitProvider = gitops.PipelineAsCodeForgejoProviderType
	}
	return applyPrebuiltBuildPLR(f, namespace, applicationName, componentName, spec)
}
