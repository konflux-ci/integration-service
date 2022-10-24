package helpers_test

import (
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Pipeline Adapter", Ordered, func() {
	var (
		testpipelineRun *tektonv1beta1.PipelineRun
		hasApp          *applicationapiv1alpha1.Application
		hasSnapshot     *applicationapiv1alpha1.ApplicationSnapshot
		logger          logr.Logger
		sample_image    string
	)

	BeforeAll(func() {
		logger = logf.Log.WithName("helpers_test")

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
	})

	BeforeEach(func() {
		sample_image = "quay.io/redhat-appstudio/sample-image"

		hasSnapshot = &applicationapiv1alpha1.ApplicationSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.ApplicationSnapshotTypeLabel:      "component",
					gitops.ApplicationSnapshotComponentLabel: "component-sample",
				},
			},
			Spec: applicationapiv1alpha1.ApplicationSnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.ApplicationSnapshotComponent{
					{
						Name:           "component-sample",
						ContainerImage: sample_image,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		testpipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-component-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelinesascode.tekton.dev/url-org":         "redhat-appstudio",
					"pipelinesascode.tekton.dev/original-prname": "build-service-on-push",
					"pipelinesascode.tekton.dev/url-repository":  "build-service",
					"pipelinesascode.tekton.dev/repository":      "build-service-pac",
					"test.appstudio.openshift.io/snapshot":       "snapshot-sample",
				},
				Annotations: map[string]string{
					"pipelinesascode.tekton.dev/on-target-branch": "[main]",
				},
			},
			Spec: tektonv1beta1.PipelineRunSpec{
				PipelineRef: &tektonv1beta1.PipelineRef{
					Name:   "component-pipeline-pass",
					Bundle: "quay.io/kpavic/test-bundle:component-pipeline-pass",
				},
			},
		}
		Expect(k8sClient.Create(ctx, testpipelineRun)).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasSnapshot.Name,
				Namespace: "default",
			}, hasSnapshot)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())
	})
	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, testpipelineRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("should return all IntegrationTestScenarios used by the application", func() {
		updatedSnapshot, err := gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(err == nil && updatedSnapshot != nil).To(BeTrue())
		Expect(gitops.HaveHACBSTestsFinished(hasSnapshot)).To(BeTrue())

		integrationTestScenarios, err := helpers.GetAllIntegrationTestScenariosForApplication(k8sClient, ctx, hasApp)
		Expect(err == nil && integrationTestScenarios != nil).To(BeTrue())

		for _, integrationTestScenario := range *integrationTestScenarios {
			integrationTestScenario := integrationTestScenario //G601
			integrationPipelineRun, err := helpers.GetLatestPipelineRunForApplicationSnapshotAndScenario(k8sClient, ctx, hasApp, hasSnapshot, &integrationTestScenario)
			Expect(err != nil && integrationPipelineRun == nil)
		}
		pipelineRunOutcome, err := helpers.CalculateIntegrationPipelineRunOutcome(logger, testpipelineRun)
		Expect(err == nil && pipelineRunOutcome).To(BeTrue())
	})

	It("should return all the required IntegrationTestScenarios used by the application", func() {
		updatedSnapshot, err := gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(err == nil && updatedSnapshot != nil).To(BeTrue())
		Expect(gitops.HaveHACBSTestsFinished(hasSnapshot)).To(BeTrue())

		requiredIntegrationTestScenarios, err := helpers.GetRequiredIntegrationTestScenariosForApplication(k8sClient, ctx, hasApp)
		Expect(err == nil && requiredIntegrationTestScenarios != nil).To(BeTrue())
		if requiredIntegrationTestScenarios != nil {
			for _, requiredIntegrationTestScenario := range *requiredIntegrationTestScenarios {
				requiredIntegrationTestScenario := requiredIntegrationTestScenario

				integrationPipelineRuns := &tektonv1beta1.PipelineRunList{}
				opts := []client.ListOption{
					client.InNamespace(hasApp.Namespace),
					client.MatchingLabels{
						"pipelines.appstudio.openshift.io/type": "test",
						"test.appstudio.openshift.io/snapshot":  hasSnapshot.Name,
						"test.appstudio.openshift.io/scenario":  requiredIntegrationTestScenario.Name,
					},
				}
				Eventually(func() bool {
					err := k8sClient.List(ctx, integrationPipelineRuns, opts...)
					return len(integrationPipelineRuns.Items) > 0 && err == nil
				}, time.Second*10).Should(BeTrue())

				integrationPipelineRun, err := helpers.GetLatestPipelineRunForApplicationSnapshotAndScenario(k8sClient, ctx, hasApp, hasSnapshot, &requiredIntegrationTestScenario)
				Expect(err != nil && integrationPipelineRun == nil)

				pipelineRunOutcome, err := helpers.CalculateIntegrationPipelineRunOutcome(logger, integrationPipelineRun)
				Expect(err == nil && pipelineRunOutcome).To(BeTrue())

				Expect(k8sClient.Delete(ctx, &integrationPipelineRuns.Items[0])).Should(Succeed())
			}
		}
	})

	It("ensures multiple task pipelinerun outcome when HACBSTests succeeded", func() {
		testpipelineRun.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				TaskRuns: map[string]*tektonv1beta1.PipelineRunTaskRunStatus{
					"task1": &tektonv1beta1.PipelineRunTaskRunStatus{
						PipelineTaskName: "task-skipped",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "HACBS_TEST_OUTPUT",
										Value: "{\"result\":\"SKIPPED\",\"timestamp\":\"1665405318\",\"failures\":[],\"successes\":0}",
									},
								},
							},
						},
					},
					"task2": &tektonv1beta1.PipelineRunTaskRunStatus{
						PipelineTaskName: "task-success",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "HACBS_TEST_OUTPUT",
										Value: "{\"result\":\"SUCCESS\",\"timestamp\":\"1665405318\",\"failures\":[],\"successes\":5}",
									},
								},
							},
						},
					},
					"task3": &tektonv1beta1.PipelineRunTaskRunStatus{
						PipelineTaskName: "task-success",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "HACBS_TEST_OUTPUT",
										Value: "{\"result\":\"SUCCESS\",\"timestamp\":\"1665565318\",\"failures\":[],\"successes\":10}",
									},
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, testpipelineRun)).Should(Succeed())

		pipelineRunOutcome, err := helpers.CalculateIntegrationPipelineRunOutcome(logger, testpipelineRun)
		Expect(err == nil && pipelineRunOutcome).To(BeTrue())

		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshot)).To(BeTrue())
	})

	It("no error from pipelinrun when HACBSTests failed", func() {
		testpipelineRun.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				TaskRuns: map[string]*tektonv1beta1.PipelineRunTaskRunStatus{
					"index1": &tektonv1beta1.PipelineRunTaskRunStatus{
						PipelineTaskName: "task-failure",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "HACBS_TEST_OUTPUT",
										Value: "{\"result\":\"FAILURE\",\"timestamp\":\"1665405317\",\"failures\":1,\"successes\":0}",
									},
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, testpipelineRun)).Should(Succeed())

		pipelineRunOutcome, err := helpers.CalculateIntegrationPipelineRunOutcome(logger, testpipelineRun)
		Expect(err == nil).To(BeTrue())
		Expect(pipelineRunOutcome).To(BeFalse())

		gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshot)).To(BeFalse())
	})

	It("ensure No Task pipelinerun passed when HACBSTests passed", func() {

		testpipelineRun.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				TaskRuns: map[string]*tektonv1beta1.PipelineRunTaskRunStatus{
					"task1": &tektonv1beta1.PipelineRunTaskRunStatus{
						PipelineTaskName: "no-task-1",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "TEST_OUTPUT",
										Value: "TEST_VALUE",
									},
								},
							},
						},
					},
					"task2": &tektonv1beta1.PipelineRunTaskRunStatus{
						PipelineTaskName: "no-task-2",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, testpipelineRun)).Should(Succeed())

		pipelineRunOutcome, err := helpers.CalculateIntegrationPipelineRunOutcome(logger, testpipelineRun)
		Expect(err == nil).To(BeTrue())
		Expect(pipelineRunOutcome).To(BeTrue())

		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshot)).To(BeTrue())
	})

	It("can handle malformed HACBS_TEST_OUTPUT result", func() {
		testpipelineRun.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				TaskRuns: map[string]*tektonv1beta1.PipelineRunTaskRunStatus{
					"task1": {
						PipelineTaskName: "task-malformed-result",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "HACBS_TEST_OUTPUT",
										Value: "invalid json",
									},
								},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Status().Update(ctx, testpipelineRun)).Should(Succeed())
		result, err := helpers.CalculateIntegrationPipelineRunOutcome(logr.Discard(), testpipelineRun)
		Expect(err).ToNot(BeNil())
		Expect(result).To(BeFalse())
	})
})
