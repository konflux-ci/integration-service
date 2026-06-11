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
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/has"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/framework"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils/build"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

var _ = framework.IntegrationServiceSuiteDescribe("Forgejo Status Reporting of Integration tests", ginkgo.Label("integration-service", "forgejo-status-reporting"), func() {
	defer ginkgo.GinkgoRecover()

	var f *framework.Framework
	var err error

	var mrID int
	var mrSha, forgejoToken string
	var snapshot *appstudioApi.Snapshot
	var component *appstudioApi.Component
	var buildPipelineRun, testPipelinerun *pipeline.PipelineRun
	var integrationTestScenarioPass *integrationv1beta2.IntegrationTestScenario
	var applicationName, componentName, componentBaseBranchName, pacBranchName, testNamespace string
	var mergeResultSha string

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

			applicationName = createApp(*f, testNamespace)

			gomega.Eventually(func() error {
				_, err := f.AsKubeAdmin.HasController.GetApplication(applicationName, testNamespace)
				return err
			}, time.Minute*2, time.Second*5).Should(gomega.Succeed(),
				fmt.Sprintf("Application %s should be available in namespace %s", applicationName, testNamespace))

			integrationTestScenarioPass, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "", []string{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			componentName = fmt.Sprintf("%s-%s", "test-comp-pac-forgejo", utils.GenerateRandomString(6))
			pacBranchName = constants.PaCPullRequestBranchPrefix + componentName
			componentBaseBranchName = fmt.Sprintf("base-forgejo-%s", utils.GenerateRandomString(6))

			forgejoToken = utils.GetEnv(constants.CODEBERG_BOT_TOKEN_ENV, "")
			gomega.Expect(forgejoToken).ShouldNot(gomega.BeEmpty(), fmt.Sprintf("'%s' env var is not set", constants.CODEBERG_BOT_TOKEN_ENV))
			gomega.Expect(f.AsKubeAdmin.CommonController.Forgejo).NotTo(gomega.BeNil())

			forgejoGitClient = git.NewForgejoClient(f.AsKubeAdmin.CommonController.Forgejo)
			gitClient = forgejoGitClient

			reportingRepository = fmt.Sprintf("%s/%s", forgejoOrg, forgejoComponentRepoName+"-"+utils.GenerateRandomString(6))
			err = gitClient.ForkRepository(forgejoProjectIDForStatusReporting, reportingRepository)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			reportingRepoURL = fmt.Sprintf("https://codeberg.org/%s", reportingRepository)

			err = build.CreateCodebergBuildSecret(f, "forgejo-build-secret", map[string]string{}, forgejoToken)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			err = gitClient.CreateBranch(reportingRepository, componentDefaultBranch, "", componentBaseBranchName)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.AfterAll(func() {
			if !ginkgo.CurrentSpecReport().Failed() {
				if mrID != 0 {
					_ = gitClient.DeleteBranchAndClosePullRequest(reportingRepository, mrID)
				}

				gomega.Expect(gitClient.DeleteRepositoryIfExists(reportingRepository)).To(gomega.Succeed())
			}

			if !ginkgo.CurrentSpecReport().Failed() {
				gomega.Expect(f.AsKubeAdmin.HasController.DeleteAllComponentsInASpecificNamespace(testNamespace, time.Minute*2)).To(gomega.Succeed())
				gomega.Expect(f.AsKubeAdmin.HasController.DeleteAllApplicationsInASpecificNamespace(testNamespace, time.Minute*2)).To(gomega.Succeed())
			}
		})

		ginkgo.When("a new Component with specified custom branch is created", ginkgo.Label("custom-branch"), func() {
			ginkgo.BeforeAll(func() {
				componentObj := appstudioApi.ComponentSpec{
					ComponentName: componentName,
					Application:   applicationName,
					Source: appstudioApi.ComponentSource{
						ComponentSourceUnion: appstudioApi.ComponentSourceUnion{
							GitSource: &appstudioApi.GitSource{
								URL:           reportingRepoURL,
								Revision:      componentBaseBranchName,
								DockerfileURL: "Dockerfile",
							},
						},
					},
				}
				buildPipelineAnnotation := build.GetBuildPipelineBundleAnnotation(constants.DockerBuildOciTAMin)
				gitProviderAnnotation := map[string]string{"git-provider": "forgejo"}

				component, err = f.AsKubeAdmin.HasController.CreateComponentCheckImageRepository(componentObj, testNamespace, "", "", applicationName, false,
					utils.MergeMaps(
						utils.MergeMaps(utils.MergeMaps(constants.ComponentPaCRequestAnnotation, constants.ImageControllerAnnotationRequestPublicRepo), buildPipelineAnnotation),
						gitProviderAnnotation,
					))
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				gomega.Eventually(func() error {
					_, getErr := f.AsKubeAdmin.TektonController.GetRepositoryParams(componentName, testNamespace)
					return getErr
				}, time.Minute*5, time.Second*5).Should(gomega.Succeed(),
					fmt.Sprintf("timed out waiting for PaC Repository CR for component %s in namespace %s", componentName, testNamespace))

				gomega.Eventually(func() bool {
					prs, listErr := git.ListPullRequestsWithRetry(gitClient, reportingRepository)
					if listErr != nil {
						ginkgo.GinkgoWriter.Printf("error listing pull requests while waiting for PaC init PR: %v\n", listErr)
						return false
					}
					for _, pr := range prs {
						if pr.SourceBranch == pacBranchName {
							mrID = pr.Number
							return true
						}
					}
					return false
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.BeTrue(),
					fmt.Sprintf("timed out waiting for PaC init PR (branch %s) to be created in %s", pacBranchName, reportingRepository))
			})

			ginkgo.It("triggers a Build PipelineRun", func() {
				gomega.Eventually(func() error {
					buildPipelineRun, err = f.AsKubeAdmin.HasController.GetComponentPipelineRun(componentName, applicationName, testNamespace, "")
					if err != nil {
						ginkgo.GinkgoWriter.Printf("Build PipelineRun has not been created yet for the component %s/%s\n", testNamespace, componentName)
						return err
					}
					if !buildPipelineRun.HasStarted() {
						return fmt.Errorf("build pipelinerun %s/%s hasn't started yet", buildPipelineRun.GetNamespace(), buildPipelineRun.GetName())
					}
					return nil
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), fmt.Sprintf("timed out when waiting for the build PipelineRun to start for the component %s/%s", testNamespace, componentName))
			})

			ginkgo.It("does not contain an annotation with a Snapshot Name", func() {
				gomega.Expect(buildPipelineRun.Annotations[snapshotAnnotation]).To(gomega.Equal(""))
			})

			ginkgo.It("should lead to build PipelineRun finishing successfully", func() {
				gomega.Expect(f.AsKubeAdmin.HasController.WaitForComponentPipelineToBeFinished(component, "", "",
					"", f.AsKubeAdmin.TektonController, &has.RetryOptions{Retries: 2, Always: true}, buildPipelineRun)).To(gomega.Succeed())
			})

			ginkgo.It("should have a related PaC init MR created", func() {
				gomega.Eventually(func() bool {
					prs, err := git.ListPullRequestsWithRetry(gitClient, reportingRepository)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

					for _, pr := range prs {
						if pr.SourceBranch == pacBranchName {
							mrID = pr.Number
							mrSha = pr.HeadSHA
							return true
						}
					}
					return false
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.BeTrue(), fmt.Sprintf("timed out when waiting for init PaC MR (branch name '%s') to be created in %s repository", pacBranchName, reportingRepository))

				buildPipelineRun, err = f.AsKubeAdmin.HasController.GetComponentPipelineRun(componentName, applicationName, testNamespace, mrSha)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.It(fmt.Sprintf("the PipelineRun should eventually finish successfully for component %s", componentName), func() {
				gomega.Expect(f.AsKubeAdmin.HasController.WaitForComponentPipelineToBeFinished(component, "", "", "",
					f.AsKubeAdmin.TektonController, &has.RetryOptions{Retries: 2, Always: true}, nil)).To(gomega.Succeed())
			})
		})

		ginkgo.When("the PaC build pipelineRun run succeeded", func() {
			ginkgo.It("checks if the BuildPipelineRun has the annotation of chains signed", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, chainsSignedAnnotation)).To(gomega.Succeed())
			})

			ginkgo.It("checks if the Snapshot is created", func() {
				snapshot, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", "", componentName, testNamespace)
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

			ginkgo.It("merging the PR should be successful", func() {
				var mergeResult *git.PullRequest
				gomega.Eventually(func() error {
					mergeResult, err = gitClient.MergePullRequest(reportingRepository, mrID)
					return err
				}, shortTimeout, constants.PipelineRunPollingInterval).ShouldNot(gomega.HaveOccurred(),
					fmt.Sprintf("error when merging PaC merge request ID #%d in repo %s", mrID, reportingRepository))

				mergeResultSha = mergeResult.MergeCommitSHA
				if mergeResultSha == "" {
					mergeResultSha = mergeResult.HeadSHA
				}
				ginkgo.GinkgoWriter.Printf("merged result sha: %s for MR #%d\n", mergeResultSha, mrID)
			})

			ginkgo.It("leads to triggering a push PipelineRun", func() {
				gomega.Eventually(func() error {
					pipelineRun, err := f.AsKubeAdmin.HasController.GetComponentPipelineRun(componentName, applicationName, testNamespace, mergeResultSha)
					if err != nil {
						ginkgo.GinkgoWriter.Printf("Push PipelineRun has not been created yet for the component %s/%s\n", testNamespace, componentName)
						return err
					}
					if !pipelineRun.HasStarted() {
						return fmt.Errorf("push pipelinerun %s/%s hasn't started yet", pipelineRun.GetNamespace(), pipelineRun.GetName())
					}
					return nil
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(),
					fmt.Sprintf("timed out when waiting for the push PipelineRun to start for the component %s/%s", testNamespace, componentName))
			})
		})

		ginkgo.When("Run integration tests after Merged MR", func() {
			ginkgo.It("should eventually complete successfully", func() {
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioPass, snapshot, testNamespace)).To(gomega.Succeed(),
					fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshot.GetName()))
			})

			ginkgo.It("eventually leads to the integration test PipelineRun's Pass status reported at MR commit status", func() {
				gomega.Expect(
					forgejoGitClient.GetCommitStatusConclusion(integrationTestScenarioPass.Name, reportingRepository, mrSha, mrID),
				).To(gomega.Equal(integrationPipelineRunCommitStatusSuccess))
			})
		})
	})
})
