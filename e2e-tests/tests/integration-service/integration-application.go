package integration

import (
	"fmt"
	"time"

	"github.com/konflux-ci/operator-toolkit/metadata"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/clients/has"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/constants"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/framework"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils/build"
	"github.com/konflux-ci/integration-service/gitops"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	integrationv1beta2 "github.com/konflux-ci/integration-service/api/v1beta2"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"

	appstudioApi "github.com/konflux-ci/application-api/api/v1alpha1"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
)

var _ = framework.IntegrationServiceSuiteDescribe("Integration Service E2E tests", ginkgo.Label("integration-service", "applications"), func() {
	defer ginkgo.GinkgoRecover()

	var f *framework.Framework
	var err error

	var prHeadSha string
	var integrationTestScenario *integrationv1beta2.IntegrationTestScenario
	var newIntegrationTestScenario *integrationv1beta2.IntegrationTestScenario
	var skippedIntegrationTestScenario *integrationv1beta2.IntegrationTestScenario
	var timeout, interval time.Duration
	var originalComponent *appstudioApi.Component
	var pipelineRun *pipeline.PipelineRun
	var snapshot *appstudioApi.Snapshot
	var snapshotPush *appstudioApi.Snapshot
	var applicationName, componentName, componentBaseBranchName, pacBranchName, testNamespace string

	ginkgo.AfterEach(framework.ReportFailure(&f))

	ginkgo.Describe("[APPLICATION] with happy path for general flow of Integration service", ginkgo.Ordered, func() {
		ginkgo.BeforeAll(func() {
			// Initialize the tests controllers
			f, err = framework.NewFramework(utils.GetGeneratedNamespace("integration1"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			testNamespace = f.UserNamespace

			applicationName = createApp(*f, testNamespace)
			originalComponent, componentName, pacBranchName, componentBaseBranchName = createComponentApplication(*f, testNamespace, applicationName, componentRepoNameForGeneralIntegration, componentGitSourceURLForGeneralIntegration)

			integrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "", []string{"application"}, false)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			skippedIntegrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("skipped-its", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "", []string{"push"}, false)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.AfterAll(func() {
			if !ginkgo.CurrentSpecReport().Failed() {
				cleanup(*f, testNamespace, applicationName, componentName, snapshotPush, false)
			}

			// Delete new branches created by PaC and a testing branch used as a component's base branch
			err = f.AsKubeAdmin.CommonController.Github.DeleteRef(componentRepoNameForGeneralIntegration, pacBranchName)
			if err != nil {
				gomega.Expect(err.Error()).To(gomega.ContainSubstring(referenceDoesntExist))
			}
			err = f.AsKubeAdmin.CommonController.Github.DeleteRef(componentRepoNameForGeneralIntegration, componentBaseBranchName)
			if err != nil {
				gomega.Expect(err.Error()).To(gomega.ContainSubstring(referenceDoesntExist))
			}
		})

		ginkgo.When("[APPLICATION] a new Component is created", func() {
			ginkgo.It("[APPLICATION] triggers a build PipelineRun", ginkgo.Label("integration-service"), func() {
				pipelineRun, err = f.AsKubeDeveloper.IntegrationController.GetBuildPipelineRun(componentName, applicationName, testNamespace, false, "", false)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.It("[APPLICATION] verifies if the build PipelineRun contains the finalizer", ginkgo.Label("integration-service"), func() {
				gomega.Eventually(func() error {
					pipelineRun, err = f.AsKubeDeveloper.IntegrationController.GetBuildPipelineRun(componentName, applicationName, testNamespace, false, "", false)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
					if !controllerutil.ContainsFinalizer(pipelineRun, pipelinerunFinalizerByIntegrationService) {
						return fmt.Errorf("build pipelineRun %s/%s doesn't contain the finalizer: %s yet", pipelineRun.GetNamespace(), pipelineRun.GetName(), pipelinerunFinalizerByIntegrationService)
					}
					return nil
				}, 1*time.Minute, 1*time.Second).Should(gomega.Succeed(), "timeout when waiting for finalizer to be added")
			})

			ginkgo.It("[APPLICATION] waits for build PipelineRun to succeed", ginkgo.Label("integration-service"), func() {
				gomega.Expect(pipelineRun.Annotations[snapshotAnnotation]).To(gomega.Equal(""))
				gomega.Expect(f.AsKubeDeveloper.HasController.WaitForComponentPipelineToBeFinished(originalComponent, "", "", "",
					f.AsKubeAdmin.TektonController, &has.RetryOptions{Retries: 2, Always: true}, pipelineRun)).To(gomega.Succeed())
			})

			ginkgo.It("[APPLICATION] should have a related PaC init PR created", func() {
				gomega.Eventually(func() bool {
					prs, err := f.AsKubeAdmin.CommonController.Github.ListPullRequests(componentRepoNameForGeneralIntegration)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

					for _, pr := range prs {
						if pr.Head.GetRef() == pacBranchName {
							prHeadSha = pr.Head.GetSHA()
							return true
						}
					}
					return false
				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.BeTrue(), fmt.Sprintf("timed out when waiting for init PaC PR (branch name '%s') to be created in %s repository", pacBranchName, componentRepoNameForStatusReporting))

				// in case the first pipelineRun attempt has failed and was retried, we need to update the value of pipelineRun variable
				pipelineRun, err = f.AsKubeAdmin.HasController.GetComponentPipelineRun(componentName, testNamespace, prHeadSha)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})
		})

		ginkgo.When("[APPLICATION] the build pipelineRun run succeeded", func() {
			ginkgo.It("[APPLICATION] checks if the BuildPipelineRun have the annotation of chains signed", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, chainsSignedAnnotation, false)).To(gomega.Succeed())
			})

			ginkgo.It("[APPLICATION] checks if the Snapshot is created", func() {
				snapshot, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", "", componentName, testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.It("[APPLICATION] checks if the Build PipelineRun got annotated with Snapshot name", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, snapshotAnnotation, false)).To(gomega.Succeed())
			})

			ginkgo.It("[APPLICATION] verifies that the finalizer has been removed from the build pipelinerun", func() {
				timeout := "60s"
				interval := "1s"
				gomega.Eventually(func() error {
					pipelineRun, err = f.AsKubeDeveloper.IntegrationController.GetBuildPipelineRun(componentName, applicationName, testNamespace, false, "", false)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
					if controllerutil.ContainsFinalizer(pipelineRun, pipelinerunFinalizerByIntegrationService) {
						return fmt.Errorf("build pipelineRun %s/%s still contains the finalizer: %s", pipelineRun.GetNamespace(), pipelineRun.GetName(), pipelinerunFinalizerByIntegrationService)
					}
					return nil
				}, timeout, interval).Should(gomega.Succeed(), "timeout when waiting for finalizer to be removed")
			})

			ginkgo.It("[APPLICATION] checks if all of the integrationPipelineRuns passed", ginkgo.Label("slow"), func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForAllIntegrationPipelinesToBeFinished(testNamespace, applicationName, snapshot, []string{integrationTestScenario.Name}, false)).To(gomega.Succeed())
			})

			ginkgo.It("[APPLICATION] checks if the passed status of integration test is reported in the Snapshot", func() {
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

			ginkgo.It("[APPLICATION] checks if the skipped integration test is absent from the Snapshot's status annotation", func() {
				snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				statusDetail, err := f.AsKubeDeveloper.IntegrationController.GetIntegrationTestStatusDetailFromSnapshot(snapshot, skippedIntegrationTestScenario.Name)
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(err.Error()).To(gomega.ContainSubstring(skippedIntegrationTestScenario.Name))
				gomega.Expect(statusDetail).To(gomega.BeNil())
			})

			ginkgo.It("[APPLICATION] checks if the finalizer was removed from all of the related Integration pipelineRuns", func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForFinalizerToGetRemovedFromAllIntegrationPipelineRuns(testNamespace, applicationName, snapshot, []string{integrationTestScenario.Name}, false)).To(gomega.Succeed())
			})
		})

		ginkgo.It("[APPLICATION] creates a ReleasePlan", func() {
			_, err = f.AsKubeAdmin.ReleaseController.CreateReleasePlan(autoReleasePlan, testNamespace, applicationName, targetReleaseNamespace, "", nil, nil, nil, false)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			testScenarios, err := f.AsKubeAdmin.IntegrationController.GetIntegrationTestScenarios(applicationName, testNamespace, false)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			for _, testScenario := range *testScenarios {
				ginkgo.GinkgoWriter.Printf("IntegrationTestScenario %s is found\n", testScenario.Name)
			}
		})

		ginkgo.It("[APPLICATION] creates an snapshot of push event", func() {
			sampleImage := "quay.io/redhat-appstudio/sample-image@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
			snapshotPush, err = f.AsKubeAdmin.IntegrationController.CreateSnapshotWithImage(componentName, applicationName, testNamespace, sampleImage, "", false)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.When("[APPLICATION] An snapshot of push event is created", func() {
			ginkgo.It("[APPLICATION] checks if the global candidate is updated after push event", func() {
				gomega.Eventually(func() error {
					snapshotPush, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshotPush.Name, "", "", testNamespace)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

					component, err := f.AsKubeAdmin.HasController.GetComponentByApplicationName(applicationName, testNamespace)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
					gomega.Expect(component.Spec.ContainerImage).ToNot(gomega.Equal(originalComponent.Spec.ContainerImage))
					return nil

				}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.Succeed(), fmt.Sprintf("time out when waiting for updating the global candidate in %s namespace", testNamespace))
			})

			ginkgo.It("[APPLICATION] checks if all of the integrationPipelineRuns created by push event passed", ginkgo.Label("slow"), func() {
				gomega.Expect(f.AsKubeAdmin.IntegrationController.WaitForAllIntegrationPipelinesToBeFinished(testNamespace, applicationName, snapshotPush, []string{integrationTestScenario.Name}, false)).To(gomega.Succeed(), "Error when waiting for one of the integration pipelines to finish in %s namespace", testNamespace)
			})

			ginkgo.It("[APPLICATION] checks if a Release is created successfully", func() {
				timeout = time.Second * 60
				interval = time.Second * 5
				gomega.Eventually(func() error {
					_, err := f.AsKubeAdmin.ReleaseController.GetReleases(testNamespace)
					return err
				}, timeout, interval).Should(gomega.Succeed(), fmt.Sprintf("time out when waiting for release created for snapshot %s/%s", snapshotPush.GetNamespace(), snapshotPush.GetName()))
			})
		})
	})

	ginkgo.Describe("[APPLICATION] with an integration test fail", ginkgo.Ordered, func() {
		ginkgo.BeforeAll(func() {
			// Initialize the tests controllers
			f, err = framework.NewFramework(utils.GetGeneratedNamespace("integration2"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			testNamespace = f.UserNamespace

			applicationName = createApp(*f, testNamespace)
			originalComponent, componentName, pacBranchName, componentBaseBranchName = createComponentApplication(*f, testNamespace, applicationName, componentRepoNameForGeneralIntegration, componentGitSourceURLForGeneralIntegration)

			integrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoFail, "", []string{"pull_request"}, false)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			skippedIntegrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("skipped-its-fail", applicationName, testNamespace, gitURL, revision, pathInRepoFail, "", []string{"group"}, false)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.AfterAll(func() {
			if !ginkgo.CurrentSpecReport().Failed() {
				cleanup(*f, testNamespace, applicationName, componentName, snapshotPush, false)
			}

			// Delete new branches created by PaC and a testing branch used as a component's base branch
			err = f.AsKubeAdmin.CommonController.Github.DeleteRef(componentRepoNameForGeneralIntegration, pacBranchName)
			if err != nil {
				gomega.Expect(err.Error()).To(gomega.ContainSubstring(referenceDoesntExist))
			}
			err = f.AsKubeAdmin.CommonController.Github.DeleteRef(componentRepoNameForGeneralIntegration, componentBaseBranchName)
			if err != nil {
				gomega.Expect(err.Error()).To(gomega.ContainSubstring(referenceDoesntExist))
			}
		})

		ginkgo.It("[APPLICATION] triggers a build PipelineRun", ginkgo.Label("integration-service"), func() {
			pipelineRun, err = f.AsKubeDeveloper.IntegrationController.GetBuildPipelineRun(componentName, applicationName, testNamespace, false, "", false)
			gomega.Expect(pipelineRun.Annotations[snapshotAnnotation]).To(gomega.Equal(""))
			gomega.Expect(f.AsKubeDeveloper.HasController.WaitForComponentPipelineToBeFinished(originalComponent, "", "", "", f.AsKubeAdmin.TektonController,
				&has.RetryOptions{Retries: 2, Always: true}, pipelineRun)).To(gomega.Succeed())
		})

		ginkgo.It("[APPLICATION] should have a related PaC init PR created", func() {
			gomega.Eventually(func() bool {
				prs, err := f.AsKubeAdmin.CommonController.Github.ListPullRequests(componentRepoNameForGeneralIntegration)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

				for _, pr := range prs {
					if pr.Head.GetRef() == pacBranchName {
						prHeadSha = pr.Head.GetSHA()
						return true
					}
				}
				return false
			}, shortTimeout, constants.PipelineRunPollingInterval).Should(gomega.BeTrue(), fmt.Sprintf("timed out when waiting for init PaC PR (branch name '%s') to be created in %s repository", pacBranchName, componentRepoNameForStatusReporting))

			// in case the first pipelineRun attempt has failed and was retried, we need to update the value of pipelineRun variable
			pipelineRun, err = f.AsKubeAdmin.HasController.GetComponentPipelineRun(componentName, testNamespace, prHeadSha)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("[APPLICATION] checks if the BuildPipelineRun have the annotation of chains signed", func() {
			gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, chainsSignedAnnotation, false)).To(gomega.Succeed())
		})

		ginkgo.It("[APPLICATION] checks if the Snapshot is created", func() {
			snapshot, err = f.AsKubeDeveloper.IntegrationController.WaitForSnapshotToGetCreated("", pipelineRun.Name, componentName, testNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("[APPLICATION] checks if the Build PipelineRun got annotated with Snapshot name", func() {
			gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForBuildPipelineRunToGetAnnotated(testNamespace, applicationName, componentName, snapshotAnnotation, false)).To(gomega.Succeed())
		})

		ginkgo.It("[APPLICATION] checks if all of the integrationPipelineRuns finished", ginkgo.Label("slow"), func() {
			gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForAllIntegrationPipelinesToBeFinished(testNamespace, applicationName, snapshot, []string{integrationTestScenario.Name}, false)).To(gomega.Succeed())
		})

		ginkgo.It("[APPLICATION] checks if the failed status of integration test is reported in the Snapshot", func() {
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

		ginkgo.It("[APPLICATION] checks if the skipped integration test is absent from the Snapshot's status annotation", func() {
			snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			statusDetail, err := f.AsKubeDeveloper.IntegrationController.GetIntegrationTestStatusDetailFromSnapshot(snapshot, skippedIntegrationTestScenario.Name)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring(skippedIntegrationTestScenario.Name))
			gomega.Expect(statusDetail).To(gomega.BeNil())
		})

		ginkgo.It("[APPLICATION] checks if snapshot is marked as failed", ginkgo.FlakeAttempts(3), func() {
			snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect(gitops.HaveAppStudioTestsSucceeded(snapshot)).To(gomega.BeFalse(), "expected tests to fail for snapshot %s/%s", snapshot.GetNamespace(), snapshot.GetName())
		})

		ginkgo.It("[APPLICATION] checks if the finalizer was removed from all of the related Integration pipelineRuns", func() {
			gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForFinalizerToGetRemovedFromAllIntegrationPipelineRuns(testNamespace, applicationName, snapshot, []string{integrationTestScenario.Name}, false)).To(gomega.Succeed())
		})

		ginkgo.It("[APPLICATION] creates a new IntegrationTestScenario", func() {
			newIntegrationTestScenario, err = f.AsKubeAdmin.IntegrationController.CreateIntegrationTestScenario("", applicationName, testNamespace, gitURL, revision, pathInRepoPass, "", []string{}, false)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("[APPLICATION] updates the Snapshot with the re-run label for the new scenario", ginkgo.FlakeAttempts(3), func() {
			updatedSnapshot := snapshot.DeepCopy()
			err := metadata.AddLabels(updatedSnapshot, map[string]string{snapshotRerunLabel: newIntegrationTestScenario.Name})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect(f.AsKubeAdmin.IntegrationController.PatchSnapshot(snapshot, updatedSnapshot)).Should(gomega.Succeed())
			gomega.Expect(metadata.GetLabelsWithPrefix(updatedSnapshot, snapshotRerunLabel)).NotTo(gomega.BeEmpty())
		})

		ginkgo.When("[APPLICATION] An snapshot is updated with a re-run label for a given scenario", func() {
			ginkgo.It("[APPLICATION] checks if the new integration pipelineRun started", ginkgo.Label("slow"), func() {
				reRunPipelineRun, err := f.AsKubeDeveloper.IntegrationController.WaitForIntegrationPipelineToGetStarted(newIntegrationTestScenario.Name, snapshot.Name, testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				gomega.Expect(reRunPipelineRun).ShouldNot(gomega.BeNil())
			})

			ginkgo.It("[APPLICATION] checks if the re-run label was removed from the Snapshot", func() {
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

			ginkgo.It("[APPLICATION] checks if all integration pipelineRuns finished successfully", ginkgo.Label("slow"), func() {
				gomega.Expect(f.AsKubeDeveloper.IntegrationController.WaitForAllIntegrationPipelinesToBeFinished(testNamespace, applicationName, snapshot, []string{integrationTestScenario.Name, newIntegrationTestScenario.Name}, false)).To(gomega.Succeed())
			})

			ginkgo.It("[APPLICATION] checks if the name of the re-triggered pipelinerun is reported in the Snapshot", ginkgo.FlakeAttempts(3), func() {
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

			ginkgo.It("[APPLICATION] checks if snapshot is still marked as failed", func() {
				snapshot, err = f.AsKubeAdmin.IntegrationController.GetSnapshot(snapshot.Name, "", "", testNamespace)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				gomega.Expect(gitops.HaveAppStudioTestsSucceeded(snapshot)).To(gomega.BeFalse(), "expected tests to fail for snapshot %s/%s", snapshot.GetNamespace(), snapshot.GetName())
			})
		})

		ginkgo.It("[APPLICATION] creates an snapshot of push event", func() {
			sampleImage := "quay.io/redhat-appstudio/sample-image@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
			snapshotPush, err = f.AsKubeAdmin.IntegrationController.CreateSnapshotWithImage(componentName, applicationName, testNamespace, sampleImage, "", false)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.When("[APPLICATION] An snapshot of push event is created", func() {
			ginkgo.It("[APPLICATION] checks no Release CRs are created", func() {
				releases, err := f.AsKubeAdmin.ReleaseController.GetReleases(testNamespace)
				gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Error when fetching the Releases")
				gomega.Expect(releases.Items).To(gomega.BeEmpty(), "Expected no Release CRs to be present, but found some")
			})
		})
	})
})

func createApp(f framework.Framework, testNamespace string) string {
	applicationName := fmt.Sprintf("integ-app-%s", utils.GenerateRandomString(4))

	_, err := f.AsKubeAdmin.HasController.CreateApplication(applicationName, testNamespace)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return applicationName
}

func createComponentApplication(f framework.Framework, testNamespace, applicationName, componentRepoName, componentRepoURL string) (*appstudioApi.Component, string, string, string) {
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

	originalComponent, err := f.AsKubeAdmin.HasController.CreateComponentCheckImageRepository(componentObj, componentName, testNamespace, "", "", applicationName, false, utils.MergeMaps(utils.MergeMaps(constants.ComponentPaCRequestAnnotation, constants.ImageControllerAnnotationRequestPublicRepo), buildPipelineAnnotation))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return originalComponent, componentName, pacBranchName, componentBaseBranchName
}
