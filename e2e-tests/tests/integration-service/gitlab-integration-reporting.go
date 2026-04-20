package integration

import (
	"fmt"
	"os"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils/build"
	"gitlab.com/gitlab-org/api/client-go"

	"strings"
	"time"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/has"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/framework"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"

	appstudioApi "github.com/konflux-ci/application-api/api/v1alpha1"
	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

var _ = framework.IntegrationServiceSuiteDescribe("Gitlab Status Reporting of Integration tests", ginkgo.Label("integration-service", "gitlab-status-reporting"), func() {
	defer ginkgo.GinkgoRecover()

	var f *framework.Framework
	var err error

	var mrID int
	var mrSha, projectID, gitlabToken string
	var snapshot *appstudioApi.Snapshot
	var component *appstudioApi.Component
	var buildPipelineRun, testPipelinerun *pipeline.PipelineRun
	var integrationTestScenarioPass, integrationTestScenarioFail *integrationv1beta2.IntegrationTestScenario
	var applicationName, componentName, componentBaseBranchName, pacBranchName, testNamespace string
	var mergeResult *gitlab.MergeRequest
	var mergeResultSha string

	ginkgo.AfterEach(framework.ReportFailure(&f))

	ginkgo.Describe("Gitlab with status reporting of Integration tests in the assosiated merge request", ginkgo.Ordered, func() {
		ginkgo.BeforeAll(func() {
			if os.Getenv(constants.SKIP_PAC_TESTS_ENV) == "true" {
				ginkgo.Skip("Skipping this test due to configuration issue with Spray proxy")
			}

			f, err = framework.NewFramework(utils.GetGeneratedNamespace("gitlab-rep"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			testNamespace = f.UserNamespace

			if utils.IsPrivateHostname(f.OpenshiftConsoleHost) {
				ginkgo.Skip("Using private cluster (not reachable from Github), skipping...")
			}

			applicationName = createApp(*f, testNamespace)

			// Wait for application to be available before creating integration test scenarios
			gomega.Eventually(func() error {
				_, err := f.AsKubeAdmin.HasController.GetApplication(applicationName, testNamespace)
				return err
			}, time.Minute*2, time.Second*5).Should(gomega.Succeed(),
				fmt.Sprintf("Application %s should be available in namespace %s", applicationName, testNamespace))

			integrationTestScenarioPass, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "", []string{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			integrationTestScenarioFail, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoFail, "", []string{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			componentName = fmt.Sprintf("%s-%s", "test-comp-pac-gitlab", utils.GenerateRandomString(6))
			pacBranchName = constants.PaCPullRequestBranchPrefix + componentName
			componentBaseBranchName = fmt.Sprintf("base-gitlab-%s", utils.GenerateRandomString(6))

			projectID = gitlabProjectIDForStatusReporting

			gitlabToken = utils.GetEnv(constants.GITLAB_BOT_TOKEN_ENV, "")
			gomega.Expect(gitlabToken).ShouldNot(gomega.BeEmpty(), fmt.Sprintf("'%s' env var is not set", constants.GITLAB_BOT_TOKEN_ENV))

			err = f.AsKubeAdmin.CommonController.Gitlab.EnsureBranchExists(projectID, componentDefaultBranch, fallbackBranchName)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			err = f.AsKubeAdmin.CommonController.Gitlab.CreateGitlabNewBranch(projectID, componentBaseBranchName, componentRevision, componentDefaultBranch)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			secretAnnotations := map[string]string{}

			err = build.CreateGitlabBuildSecret(f, "gitlab-build-secret", secretAnnotations, gitlabToken)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.AfterAll(func() {

			// Cleanup test: close MR if opened, delete created branch, delete associated Webhooks
			gomega.Expect(f.AsKubeAdmin.CommonController.Gitlab.CloseMergeRequest(projectID, mrID)).To(gomega.Succeed())
			gomega.Expect(f.AsKubeAdmin.CommonController.Gitlab.DeleteBranch(projectID, componentBaseBranchName)).NotTo(gomega.HaveOccurred())
			gomega.Expect(f.AsKubeAdmin.CommonController.Gitlab.DeleteWebhooks(projectID, f.ClusterAppDomain)).NotTo(gomega.HaveOccurred())

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
								URL:           gitlabComponentGitSourceURLForStatusReporting,
								Revision:      componentBaseBranchName,
								DockerfileURL: "Dockerfile",
							},
						},
					},
				}
				// get the build pipeline bundle annotation
				buildPipelineAnnotation := build.GetBuildPipelineBundleAnnotation(constants.DockerBuild)
				// Create a component with Git Source URL, a specified git branch
				component, err = f.AsKubeAdmin.HasController.CreateComponentCheckImageRepository(componentObj, testNamespace, "", "", applicationName, false, utils.MergeMaps(utils.MergeMaps(constants.ComponentPaCRequestAnnotation, constants.ImageControllerAnnotationRequestPublicRepo), buildPipelineAnnotation))
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
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

			ginkgo.It("should have a related PaC init MR is created", func() {
				gomega.Eventually(func() bool {
					mrs, err := f.AsKubeAdmin.CommonController.Gitlab.GetMergeRequests(projectID)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

					for _, mr := range mrs {
						if mr.SourceBranch == pacBranchName {
							mrID = mr.IID
							mrSha = mr.SHA
							return true
						}
					}
					return false
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.BeTrue(), fmt.Sprintf("timed out when waiting for init PaC MR (branch name '%s') to be created in %s repository", pacBranchName, componentRepoNameForStatusReporting))

				// in case the first pipelineRun attempt has failed and was retried, we need to update the value of pipelineRun variable
				buildPipelineRun, err = f.AsKubeAdmin.HasController.GetComponentPipelineRun(componentName, applicationName, testNamespace, mrSha)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})
			ginkgo.It(fmt.Sprintf("the PipelineRun should eventually finish successfully for component %s", componentName), func() {
				gomega.Expect(f.AsKubeAdmin.HasController.WaitForComponentPipelineToBeFinished(component, "", "", "",
					f.AsKubeAdmin.TektonController, &has.RetryOptions{Retries: 2, Always: true}, nil)).To(gomega.Succeed())
			})
		})

		ginkgo.When("the PaC build pipelineRun run succeeded", func() {
			ginkgo.It("checks if the BuildPipelineRun have the annotation of chains signed", func() {
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

				gomega.Expect(testPipelinerun.Labels[snapshotAnnotation]).To(gomega.ContainSubstring(snapshot.Name))

			})
		})

		ginkgo.When("Integration PipelineRun is created", func() {

			ginkgo.It("should eventually complete successfully", func() {
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioPass, snapshot, testNamespace)).To(gomega.Succeed(), fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshot.GetName()))
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioFail, snapshot, testNamespace)).To(gomega.Succeed(), fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshot.GetName()))
			})

			ginkgo.It("validates the Integration test scenario PipelineRun is reported to merge request CommitStatus, and it pass", func() {
				gomega.Eventually(func() string {
					commitStatuses, _, err := f.AsKubeAdmin.CommonController.Gitlab.GetClient().Commits.GetCommitStatuses(projectID, mrSha, nil)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

					for _, commitStatus := range commitStatuses {
						commitStatusNames := strings.Split(commitStatus.Name, "/")
						if len(commitStatusNames) > 2 {
							if strings.Contains(integrationTestScenarioPass.Name, strings.TrimSpace(commitStatusNames[1])) {
								return commitStatus.Status
							}
						}
					}
					return ""
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Equal(integrationPipelineRunCommitStatusSuccess), fmt.Sprintf("timed out when waiting for expected commitStatus to be created for sha %s in %s repository", mrSha, componentRepoNameForStatusReporting))
			})

			ginkgo.It("eventually leads to the integration test PipelineRun's Pass status reported at MR commit status", func() {
				gomega.Expect(f.AsKubeAdmin.HasController.GitLab.GetCommitStatusConclusion(integrationTestScenarioPass.Name, projectID, mrSha, mrID)).To(gomega.Equal(integrationPipelineRunCommitStatusSuccess))
			})

			ginkgo.It("validates the Integration test scenario PipelineRun is reported to merge request CommitStatus, and it fails", func() {
				gomega.Eventually(func() string {
					commitStatuses, _, err := f.AsKubeAdmin.CommonController.Gitlab.GetClient().Commits.GetCommitStatuses(projectID, mrSha, nil)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

					for _, commitStatus := range commitStatuses {
						commitStatusNames := strings.Split(commitStatus.Name, "/")
						if len(commitStatusNames) > 2 {
							if strings.Contains(integrationTestScenarioFail.Name, strings.TrimSpace(commitStatusNames[1])) {
								return commitStatus.Status
							}
						}
					}
					return ""
				}, longTimeout, constants.PipelineRunPollingInterval).Should(gomega.Equal(integrationPipelineRunCommitStatusFail), fmt.Sprintf("timed out when waiting for expected commitStatus to be created for sha %s in %s repository", mrSha, componentRepoNameForStatusReporting))
			})

			ginkgo.It("eventually leads to the integration test PipelineRun's Fail status reported at MR commit status", func() {
				gomega.Expect(f.AsKubeAdmin.HasController.GitLab.GetCommitStatusConclusion(integrationTestScenarioFail.Name, projectID, mrSha, mrID)).To(gomega.Equal(integrationPipelineRunCommitStatusFail))
			})

			ginkgo.It("validates at least one MR note contains the final integration test result", func() {
				gomega.Eventually(func() bool {
					notes, _, err := f.AsKubeAdmin.CommonController.Gitlab.GetClient().Notes.ListMergeRequestNotes(projectID, mrID, nil)
					if err != nil {
						ginkgo.GinkgoWriter.Printf("failed to list MR notes: %v\n", err)
						return false
					}
					for _, note := range notes {
						if note.System {
							continue
						}
						body := note.Body
						if strings.Contains(body, integrationTestScenarioPass.Name) || strings.Contains(body, integrationTestScenarioFail.Name) {
							return true
						}
						if strings.Contains(body, "integration") &&
							(strings.Contains(body, "pass") || strings.Contains(body, "fail") || strings.Contains(body, "success") || strings.Contains(body, "failed")) {
							return true
						}
					}
					return false
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.BeTrue(), fmt.Sprintf("no MR note found containing integration test result for MR #%d in project %s", mrID, componentRepoNameForStatusReporting))
			})

			ginkgo.It("merging the PR should be successful", func() {
				gomega.Eventually(func() error {
					mergeResult, err = f.AsKubeAdmin.CommonController.Gitlab.AcceptMergeRequest(projectID, mrID)
					return err
				}, shortTimeout, constants.PipelineRunPollingInterval).ShouldNot(gomega.HaveOccurred(), fmt.Sprintf("error when merging PaC merge request ID #%d in ProjectID %s", mrID, projectID))

				mergeResultSha = mergeResult.SHA
				ginkgo.GinkgoWriter.Printf("merged result sha: %s for MR #%d\n", mergeResultSha, mrID)

			})
			ginkgo.It("leads to triggering on push PipelineRun", func() {
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
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), fmt.Sprintf("timed out when waiting for the PipelineRun to start for the component %s/%s", testNamespace, componentName))
			})
		})
		ginkgo.When("Run integration tests after Merged MR", func() {
			ginkgo.It("should eventually complete successfully", func() {
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioPass, snapshot, testNamespace)).To(gomega.Succeed(), fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshot.GetName()))
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForIntegrationPipelineToBeFinished(integrationTestScenarioFail, snapshot, testNamespace)).To(gomega.Succeed(), fmt.Sprintf("Error when waiting for an integration pipelinerun for snapshot %s/%s to finish", testNamespace, snapshot.GetName()))
			})

			ginkgo.It("validates the Integration test scenario PipelineRun is reported to merge request CommitStatus, and it pass", func() {
				gomega.Eventually(func() string {
					commitStatuses, _, err := f.AsKubeAdmin.CommonController.Gitlab.GetClient().Commits.GetCommitStatuses(projectID, mrSha, nil)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

					for _, commitStatus := range commitStatuses {
						commitStatusNames := strings.Split(commitStatus.Name, "/")
						if len(commitStatusNames) > 2 {
							if strings.Contains(integrationTestScenarioPass.Name, strings.TrimSpace(commitStatusNames[1])) {
								return commitStatus.Status
							}
						}
					}
					return ""
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Equal(integrationPipelineRunCommitStatusSuccess), fmt.Sprintf("timed out when waiting for expected commitStatus to be created for sha %s in %s repository", mrSha, componentRepoNameForStatusReporting))
			})

			ginkgo.It("eventually leads to the integration test PipelineRun's Pass status reported at MR commit status", func() {
				gomega.Expect(f.AsKubeAdmin.HasController.GitLab.GetCommitStatusConclusion(integrationTestScenarioPass.Name, projectID, mrSha, mrID)).To(gomega.Equal(constants.CheckrunConclusionSuccess))
			})

			ginkgo.It("validates the Integration test scenario PipelineRun is reported to merge request CommitStatus, and it fails", func() {
				gomega.Eventually(func() string {
					commitStatuses, _, err := f.AsKubeAdmin.CommonController.Gitlab.GetClient().Commits.GetCommitStatuses(projectID, mrSha, nil)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

					for _, commitStatus := range commitStatuses {
						commitStatusNames := strings.Split(commitStatus.Name, "/")
						if len(commitStatusNames) > 2 {
							if strings.Contains(integrationTestScenarioFail.Name, strings.TrimSpace(commitStatusNames[1])) {
								return commitStatus.Status
							}
						}
					}
					return ""
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Equal(integrationPipelineRunCommitStatusFail), fmt.Sprintf("timed out when waiting for expected commitStatus to be created for sha %s in %s repository", mrSha, componentRepoNameForStatusReporting))
			})

			ginkgo.It("eventually leads to the integration test PipelineRun's Fail status reported at MR commit status", func() {
				gomega.Expect(f.AsKubeAdmin.HasController.GitLab.GetCommitStatusConclusion(integrationTestScenarioFail.Name, projectID, mrSha, mrID)).To(gomega.Equal(integrationPipelineRunCommitStatusFail))
			})
		})
	})
})
