package snapshot

import (
	"bytes"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/tonglil/buflogr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	integrationv1alpha1 "github.com/redhat-appstudio/integration-service/api/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Snapshot Adapter", Ordered, func() {
	var (
		adapter *Adapter

		testReleasePlan                   *releasev1alpha1.ReleasePlan
		hasApp                            *applicationapiv1alpha1.Application
		hasComp                           *applicationapiv1alpha1.Component
		hasSnapshot                       *applicationapiv1alpha1.Snapshot
		hasSnapshotPR                     *applicationapiv1alpha1.Snapshot
		deploymentTargetClass             *applicationapiv1alpha1.DeploymentTargetClass
		testpipelineRun                   *tektonv1beta1.PipelineRun
		integrationTestScenario           *integrationv1alpha1.IntegrationTestScenario
		integrationTestScenarioWithoutEnv *integrationv1alpha1.IntegrationTestScenario
		env                               *applicationapiv1alpha1.Environment
		sample_image                      string
		sample_revision                   string
	)
	const (
		SampleRepoLink = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
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

		integrationTestScenario = &integrationv1alpha1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: integrationv1alpha1.IntegrationTestScenarioSpec{
				Application: "application-sample",
				Bundle:      "quay.io/kpavic/test-bundle:component-pipeline-pass",
				Pipeline:    "component-pipeline-pass",
				Environment: integrationv1alpha1.TestEnvironment{
					Name: "envname",
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())

		integrationTestScenarioWithoutEnv = &integrationv1alpha1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass-withoutenv",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: integrationv1alpha1.IntegrationTestScenarioSpec{
				Application: "application-sample",
				Bundle:      "quay.io/kpavic/test-bundle:component-pipeline-pass",
				Pipeline:    "component-pipeline-pass",
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenarioWithoutEnv)).Should(Succeed())

		testReleasePlan = &releasev1alpha1.ReleasePlan{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-releaseplan-",
				Namespace:    "default",
				Labels: map[string]string{
					releasev1alpha1.AutoReleaseLabel: "true",
				},
			},
			Spec: releasev1alpha1.ReleasePlanSpec{
				Application: hasApp.Name,
				Target:      "default",
			},
		}
		Expect(k8sClient.Create(ctx, testReleasePlan)).Should(Succeed())

		env = &applicationapiv1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "envname",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.EnvironmentSpec{
				Type:               "POC",
				DisplayName:        "my-environment",
				DeploymentStrategy: applicationapiv1alpha1.DeploymentStrategy_Manual,
				ParentEnvironment:  "",
				Tags:               []string{},
				Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
					Env: []applicationapiv1alpha1.EnvVarPair{
						{
							Name:  "var_name",
							Value: "test",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, env)).Should(Succeed())

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-sample",
				Application:    "application-sample",
				ContainerImage: "",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: SampleRepoLink,
						},
					},
				},
			},
			Status: applicationapiv1alpha1.ComponentStatus{
				LastBuiltCommit: "",
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())

		deploymentTargetClass = &applicationapiv1alpha1.DeploymentTargetClass{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "dtcls" + "-",
			},
			Spec: applicationapiv1alpha1.DeploymentTargetClassSpec{
				Provisioner: applicationapiv1alpha1.Provisioner_Devsandbox,
			},
		}
		Expect(k8sClient.Create(ctx, deploymentTargetClass)).Should(Succeed())
	})

	BeforeEach(func() {
		sample_image = "quay.io/redhat-appstudio/sample-image"
		sample_revision = "random-value"

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      "component",
					gitops.SnapshotComponentLabel: "component-sample",
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
						ContainerImage: sample_image,
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{
									Revision: sample_revision,
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		hasSnapshotPR = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshotpr-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:            "component",
					gitops.SnapshotComponentLabel:       "component-sample",
					gitops.PipelineAsCodeEventTypeLabel: "pull_request",
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
						ContainerImage: sample_image,
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshotPR)).Should(Succeed())

		testpipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "build-pipelinerun" + "-",
				Namespace:    "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type": "build",
					"pipelines.openshift.io/used-by":        "build-cloud",
					"pipelines.openshift.io/runtime":        "nodejs",
					"pipelines.openshift.io/strategy":       "s2i",
					"appstudio.openshift.io/component":      "component-sample",
				},
				Annotations: map[string]string{
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
					"pipelinesascode.tekton.dev/installation-id":    "123",
				},
			},
			Spec: tektonv1beta1.PipelineRunSpec{
				PipelineRef: &tektonv1beta1.PipelineRef{
					Name:   "build-pipeline-pass",
					Bundle: "quay.io/kpavic/test-bundle:build-pipeline-pass",
				},
				Params: []tektonv1beta1.Param{
					{
						Name: "output-image",
						Value: tektonv1beta1.ArrayOrString{
							Type:      "string",
							StringVal: "quay.io/redhat-appstudio/sample-image",
						},
					},
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

		adapter = NewAdapter(hasSnapshot, hasApp, hasComp, ctrl.Log, k8sClient, ctx)
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshotPR)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, testpipelineRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, env)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, deploymentTargetClass)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenarioWithoutEnv)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, testReleasePlan)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

	})

	It("can create a new Adapter instance", func() {
		Expect(reflect.TypeOf(NewAdapter(hasSnapshot, hasApp, hasComp, ctrl.Log, k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("ensures the Applicationcomponents can be found ", func() {
		applicationComponents, err := adapter.getAllApplicationComponents(hasApp)
		Expect(err).To(BeNil())
		Expect(applicationComponents).NotTo(BeNil())
	})

	It("ensures the integrationTestPipelines are created", func() {
		Eventually(func() bool {
			result, err := adapter.EnsureAllIntegrationTestPipelinesExist()
			return !result.CancelRequest && err == nil
		}, time.Second*20).Should(BeTrue())

		requiredIntegrationTestScenarios, err := helpers.GetRequiredIntegrationTestScenariosForApplication(k8sClient, ctx, hasApp)
		Expect(err).To(BeNil())
		Expect(requiredIntegrationTestScenarios).NotTo(BeNil())
		if requiredIntegrationTestScenarios != nil {
			for _, requiredIntegrationTestScenario := range *requiredIntegrationTestScenarios {
				if !reflect.ValueOf(requiredIntegrationTestScenario.Spec.Environment).IsZero() {
					continue
				}
				requiredIntegrationTestScenario := requiredIntegrationTestScenario

				integrationPipelineRuns := &tektonv1beta1.PipelineRunList{}
				opts := []client.ListOption{
					client.InNamespace(hasApp.Namespace),
					client.MatchingLabels{
						"pipelines.appstudio.openshift.io/type": "test",
						"appstudio.openshift.io/snapshot":       hasSnapshot.Name,
						"test.appstudio.openshift.io/scenario":  requiredIntegrationTestScenario.Name,
					},
				}
				Eventually(func() bool {
					err := k8sClient.List(ctx, integrationPipelineRuns, opts...)
					return len(integrationPipelineRuns.Items) > 0 && err == nil
				}, time.Second*10).Should(BeTrue())

				Expect(k8sClient.Delete(ctx, &integrationPipelineRuns.Items[0])).Should(Succeed())
			}
		}
	})

	It("Ensure IntegrationPipelineRun can be created for scenario", func() {
		Eventually(func() bool {
			err := adapter.createIntegrationPipelineRun(hasApp, integrationTestScenario, hasSnapshot)
			return err == nil
		}, time.Second*20).Should(BeTrue())

		integrationPipelineRuns := &tektonv1beta1.PipelineRunList{}
		opts := []client.ListOption{
			client.InNamespace(hasApp.Namespace),
			client.MatchingLabels{
				"pipelines.appstudio.openshift.io/type": "test",
				"appstudio.openshift.io/snapshot":       hasSnapshot.Name,
				"test.appstudio.openshift.io/scenario":  integrationTestScenario.Name,
			},
		}
		Eventually(func() bool {
			err := k8sClient.List(ctx, integrationPipelineRuns, opts...)
			return len(integrationPipelineRuns.Items) > 0 && err == nil
		}, time.Second*10).Should(BeTrue())

		Expect(k8sClient.Delete(ctx, &integrationPipelineRuns.Items[0])).Should(Succeed())
	})

	It("ensures all Releases exists when AppStudio Tests succeeded", func() {
		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())

		Eventually(func() bool {
			result, err := adapter.EnsureAllReleasesExist()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		releases, err := adapter.getReleasesWithSnapshot(hasSnapshot)
		Expect(err).To(BeNil())
		Expect(releases).NotTo(BeNil())
		for _, release := range *releases {
			Expect(k8sClient.Delete(ctx, &release)).Should(Succeed())
		}
	})

	It("ensures global Component Image will not be updated in the PR context", func() {

		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshotPR, "test passed")
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshotPR)).To(BeTrue())

		Eventually(func() bool {
			result, err := adapter.EnsureGlobalCandidateImageUpdated()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		Expect(hasComp.Spec.ContainerImage).To(Equal(""))
		Expect(hasComp.Status.LastBuiltCommit).To(Equal(""))
	})

	It("ensures global Component Image updated when AppStudio Tests succeeded", func() {
		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())

		Eventually(func() bool {
			result, err := adapter.EnsureGlobalCandidateImageUpdated()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		Expect(hasComp.Spec.ContainerImage).To(Equal(sample_image))
		Expect(hasComp.Status.LastBuiltCommit).To(Equal(sample_revision))
	})

	It("no error from ensuring global Component Image updated when AppStudio Tests failed", func() {
		gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeFalse())
		result, err := adapter.EnsureGlobalCandidateImageUpdated()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.CancelRequest).To(BeFalse())
	})

	It("no action when EnsureAllReleasesExist function runs when AppStudio Tests failed and the snapshot is invalid", func() {
		var buf bytes.Buffer
		var log logr.Logger = buflogr.NewWithBuffer(&buf)

		// Set the snapshot up for failure by setting its status as failed and invalid
		// as well as marking it as PaC pull request event type
		updatedSnapshot, err := gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
		Expect(err).ShouldNot(HaveOccurred())
		gitops.SetSnapshotIntegrationStatusAsInvalid(updatedSnapshot, "snapshot invalid")
		hasSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodePullRequestType
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeFalse())
		Expect(gitops.IsSnapshotValid(hasSnapshot)).To(BeFalse())

		adapter = NewAdapter(hasSnapshot, hasApp, hasComp, log, k8sClient, ctx)
		Eventually(func() bool {
			result, err := adapter.EnsureAllReleasesExist()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		expectedLogEntry := "The Snapshot won't be released"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		expectedLogEntry = "the Snapshot hasn't passed all required integration tests"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		expectedLogEntry = "the Snapshot is invalid"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		expectedLogEntry = "the Snapshot was created for a PaC pull request event"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
	})

	It("no action when EnsureSnapshotEnvironmentBindingExist function runs when AppStudio Tests failed and the snapshot is invalid", func() {
		var buf bytes.Buffer
		var log logr.Logger = buflogr.NewWithBuffer(&buf)

		// Set the snapshot up for failure by setting its status as failed and invalid
		// as well as marking it as PaC pull request event type
		updatedSnapshot, err := gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
		Expect(err).ShouldNot(HaveOccurred())
		gitops.SetSnapshotIntegrationStatusAsInvalid(updatedSnapshot, "snapshot invalid")
		hasSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodePullRequestType
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeFalse())
		Expect(gitops.IsSnapshotValid(hasSnapshot)).To(BeFalse())

		adapter = NewAdapter(hasSnapshot, hasApp, hasComp, log, k8sClient, ctx)
		Eventually(func() bool {
			result, err := adapter.EnsureSnapshotEnvironmentBindingExist()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		expectedLogEntry := "The Snapshot won't be deployed"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		expectedLogEntry = "the Snapshot hasn't passed all required integration tests"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		expectedLogEntry = "the Snapshot is invalid"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		expectedLogEntry = "the Snapshot was created for a PaC pull request event"
		Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
	})

	It("ensures snapshot environmentBinding exist", func() {
		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
		Eventually(func() bool {
			result, err := adapter.EnsureSnapshotEnvironmentBindingExist()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		Eventually(func() bool {
			snapshotEnvironmentBinding, err := gitops.FindExistingSnapshotEnvironmentBinding(k8sClient, ctx, hasApp, env)
			return snapshotEnvironmentBinding != nil && err == nil
		}, time.Second*10).Should(BeTrue())
	})

	When("multiple components exist", func() {

		var (
			secondComp *applicationapiv1alpha1.Component
		)

		BeforeEach(func() {
			secondComp = &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "component-second-sample",
					Namespace: "default",
				},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ComponentName:  "component-second-sample",
					Application:    "application-sample",
					ContainerImage: "",
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{
								URL: SampleRepoLink,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, secondComp)).Should(Succeed())

		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, secondComp)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures updating existing snapshot works", func() {
			var snapshotEnvironmentBinding *applicationapiv1alpha1.SnapshotEnvironmentBinding

			// create snapshot environment
			components := []applicationapiv1alpha1.Component{
				*hasComp,
			}
			snapshotEnvironmentBinding, err := adapter.createSnapshotEnvironmentBindingForSnapshot(adapter.application, env, hasSnapshot, &components)
			Expect(err).To(BeNil())
			Expect(snapshotEnvironmentBinding).NotTo(BeNil())

			// update snapshot environment with new component
			componentsUpdate := []applicationapiv1alpha1.Component{
				*secondComp,
			}

			updatedSnapshotEnvironmentBinding, err := adapter.updateExistingSnapshotEnvironmentBindingWithSnapshot(snapshotEnvironmentBinding, hasSnapshot, &componentsUpdate)
			Expect(err).To(BeNil())
			Expect(updatedSnapshotEnvironmentBinding).NotTo(BeNil())
			Expect(len(updatedSnapshotEnvironmentBinding.Spec.Components) == 1)
			Expect(updatedSnapshotEnvironmentBinding.Spec.Components[0].Name == secondComp.Spec.ComponentName)

		})

		It("ensures the ephemeral copy Environment are created for IntegrationTestScenario", func() {
			Eventually(func() bool {
				result, err := adapter.EnsureCreationOfEnvironment()
				return !result.CancelRequest && err == nil
			}, time.Second*20).Should(BeTrue())

			deploymentTargetClaimList := &applicationapiv1alpha1.DeploymentTargetClaimList{}
			opts := []client.ListOption{
				client.InNamespace("default"),
			}
			Eventually(func() bool {
				err := k8sClient.List(ctx, deploymentTargetClaimList, opts...)
				return len(deploymentTargetClaimList.Items) > 0 && err == nil
			}, time.Second*10).Should(BeTrue())
			Expect(deploymentTargetClaimList).NotTo(BeNil())
			dtc := &deploymentTargetClaimList.Items[0]

			allEnvironments, err := adapter.getAllEnvironments()
			expected_environment := applicationapiv1alpha1.Environment{}
			for _, environment := range *allEnvironments {
				if environment.Spec.Configuration.Target.DeploymentTargetClaim.ClaimName == dtc.Name {
					expected_environment = environment
				}
			}
			Expect(err).To(BeNil())
			Expect(allEnvironments).NotTo(BeNil())
			Expect(expected_environment).NotTo(BeNil())
			Expect(k8sClient.Delete(ctx, &expected_environment)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, dtc)).Should(Succeed())
		})
	})

})
