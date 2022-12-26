package snapshot

import (
	"reflect"
	"time"

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

		testReleasePlan         *releasev1alpha1.ReleasePlan
		hasApp                  *applicationapiv1alpha1.Application
		hasComp                 *applicationapiv1alpha1.Component
		hasSnapshot             *applicationapiv1alpha1.Snapshot
		hasSnapshotPR           *applicationapiv1alpha1.Snapshot
		testpipelineRun         *tektonv1beta1.PipelineRun
		integrationTestScenario *integrationv1alpha1.IntegrationTestScenario
		env                     applicationapiv1alpha1.Environment
		sample_image            string
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
					Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())

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

		env = applicationapiv1alpha1.Environment{
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
		Expect(k8sClient.Create(ctx, &env)).Should(Succeed())

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
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())
	})

	BeforeEach(func() {
		sample_image = "quay.io/redhat-appstudio/sample-image"

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
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, testReleasePlan)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

	})

	It("can create a new Adapter instance", func() {
		Expect(reflect.TypeOf(NewAdapter(hasSnapshot, hasApp, hasComp, ctrl.Log, k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("ensures the Applicationcomponents can be found ", func() {
		applicationComponents, err := adapter.getAllApplicationComponents(hasApp)
		Expect(err == nil).To(BeTrue())
		Expect(applicationComponents != nil).To(BeTrue())
	})

	It("ensures the integrationTestPipelines are created", func() {
		Eventually(func() bool {
			result, err := adapter.EnsureAllIntegrationTestPipelinesExist()
			return !result.CancelRequest && err == nil
		}, time.Second*20).Should(BeTrue())

		requiredIntegrationTestScenarios, err := helpers.GetRequiredIntegrationTestScenariosForApplication(k8sClient, ctx, hasApp)
		Expect(requiredIntegrationTestScenarios != nil && err == nil).To(BeTrue())
		if requiredIntegrationTestScenarios != nil {
			for _, requiredIntegrationTestScenario := range *requiredIntegrationTestScenarios {
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

	It("ensures all Releases exists when HACBSTests succeeded", func() {
		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshot)).To(BeTrue())

		Eventually(func() bool {
			result, err := adapter.EnsureAllReleasesExist()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		releases, err := adapter.getReleasesWithSnapshot(hasSnapshot)
		Expect(err == nil).To(BeTrue())
		Expect(releases != nil).To(BeTrue())
		for _, release := range *releases {
			Expect(k8sClient.Delete(ctx, &release)).Should(Succeed())
		}
	})

	It("ensures global Component Image will not be updated in the PR context", func() {

		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshotPR, "test passed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshotPR)).To(BeTrue())

		Eventually(func() bool {
			result, err := adapter.EnsureGlobalComponentImageUpdated()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		Expect(hasComp.Spec.ContainerImage).To(Equal(""))
	})

	It("ensures global Component Image updated when HACBSTests succeeded", func() {
		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshot)).To(BeTrue())

		Eventually(func() bool {
			result, err := adapter.EnsureGlobalComponentImageUpdated()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		Expect(hasComp.Spec.ContainerImage).To(Equal(sample_image))

	})

	It("no error from ensuring global Component Image updated when HACBSTests failed", func() {
		gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshot)).To(BeFalse())
		result, err := adapter.EnsureGlobalComponentImageUpdated()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(result.CancelRequest).To(BeFalse())
	})

	It("no error from ensuring all Releases exists function when HACBSTests failed", func() {
		gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshot)).To(BeFalse())
		Eventually(func() bool {
			result, err := adapter.EnsureAllReleasesExist()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())
	})

	It("ensures snapshot environmentBinding exist", func() {
		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveHACBSTestsSucceeded(hasSnapshot)).To(BeTrue())
		Eventually(func() bool {
			result, err := adapter.EnsureSnapshotEnvironmentBindingExist()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		Eventually(func() bool {
			snapshotEnvironmentBinding, err := gitops.FindExistingSnapshotEnvironmentBinding(k8sClient, ctx, hasApp, &env)
			return snapshotEnvironmentBinding != nil && err == nil
		}, time.Second*10).Should(BeTrue())
	})

})
