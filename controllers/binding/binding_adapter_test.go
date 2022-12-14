package binding

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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Binding Adapter", Ordered, func() {
	var (
		adapter *Adapter

		testReleasePlan         *releasev1alpha1.ReleasePlan
		hasApp                  *applicationapiv1alpha1.Application
		hasComp                 *applicationapiv1alpha1.Component
		hasSnapshot             *applicationapiv1alpha1.Snapshot
		finishedSnapshot        *applicationapiv1alpha1.Snapshot
		integrationTestScenario *integrationv1alpha1.IntegrationTestScenario
		hasEnv                  *applicationapiv1alpha1.Environment
		hasBinding              *applicationapiv1alpha1.SnapshotEnvironmentBinding
	)
	const (
		SampleRepoLink = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		sampleImage    = "quay.io/redhat-appstudio/sample-image"
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

		hasEnv = &applicationapiv1alpha1.Environment{
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
				UnstableConfigurationFields: &applicationapiv1alpha1.UnstableEnvironmentConfiguration{
					KubernetesClusterCredentials: applicationapiv1alpha1.KubernetesClusterCredentials{
						TargetNamespace:          "example-pass-9664d7b0-54f5-409d-9abf-de9a8bbde59f",
						APIURL:                   "https://api.sample.lab.upshift.rdu2.redhat.com:6443",
						ClusterCredentialsSecret: "example-managed-environment-secret",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasEnv)).Should(Succeed())

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

		finishedSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample-finished",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:            "component",
					gitops.SnapshotComponentLabel:       hasComp.Name,
					gitops.PipelineAsCodeEventTypeLabel: "push",
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation: "123",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComp.Name,
						ContainerImage: sampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, finishedSnapshot)).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      finishedSnapshot.Name,
				Namespace: "default",
			}, finishedSnapshot)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		finishedSnapshot, err := gitops.MarkSnapshotAsPassed(k8sClient, ctx, finishedSnapshot, "Snapshot passed")
		Expect(err == nil).To(BeTrue())
		Expect(gitops.HaveHACBSTestsFinished(finishedSnapshot)).To(BeTrue())
	})

	BeforeEach(func() {
		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:            "component",
					gitops.SnapshotComponentLabel:       hasComp.Name,
					gitops.PipelineAsCodeEventTypeLabel: "push",
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
						ContainerImage: sampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		hasBinding = &applicationapiv1alpha1.SnapshotEnvironmentBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-binding-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTestScenarioLabel: integrationTestScenario.Name,
				},
			},
			Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
				Application: hasApp.Name,
				Snapshot:    hasSnapshot.Name,
				Environment: hasEnv.Name,
				Components:  []applicationapiv1alpha1.BindingComponent{},
			},
		}
		Expect(k8sClient.Create(ctx, hasBinding)).Should(Succeed())

		hasBinding.Status = applicationapiv1alpha1.SnapshotEnvironmentBindingStatus{
			BindingConditions: []metav1.Condition{
				{
					Reason:             "Completed",
					Status:             "True",
					Type:               gitops.BindingDeploymentStatusConditionType,
					LastTransitionTime: metav1.Time{Time: time.Now()},
				},
			},
		}

		Expect(k8sClient.Status().Update(ctx, hasBinding)).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasBinding.Name,
				Namespace: "default",
			}, hasBinding)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		adapter = NewAdapter(hasBinding, hasSnapshot, hasEnv, hasApp, integrationTestScenario, ctrl.Log, k8sClient, ctx)
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasBinding)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasEnv)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, testReleasePlan)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, finishedSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

	})

	It("can create a new Adapter instance", func() {
		Expect(reflect.TypeOf(NewAdapter(hasBinding, hasSnapshot, hasEnv, hasApp, integrationTestScenario, ctrl.Log, k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("ensures the integrationTestPipelines are created for a deployed SnapshotEnvironment binding", func() {
		Eventually(func() bool {
			result, err := adapter.EnsureIntegrationTestPipelineForScenarioExists()
			return !result.CancelRequest && err == nil
		}, time.Second*20).Should(BeTrue())

		integrationPipelineRuns := &tektonv1beta1.PipelineRunList{}
		opts := []client.ListOption{
			client.InNamespace(hasApp.Namespace),
			client.MatchingLabels{
				"pipelines.appstudio.openshift.io/type": "test",
				"appstudio.openshift.io/snapshot":       hasSnapshot.Name,
				"test.appstudio.openshift.io/scenario":  integrationTestScenario.Name,
				"appstudio.openshift.io/environment":    hasEnv.Name,
			},
		}
		Eventually(func() bool {
			err := k8sClient.List(ctx, integrationPipelineRuns, opts...)
			return len(integrationPipelineRuns.Items) > 0 && err == nil
		}, time.Second*20).Should(BeTrue())

		integrationPipelineRun := integrationPipelineRuns.Items[0]

		Expect(integrationPipelineRun.Labels["appstudio.openshift.io/environment"] == hasEnv.Name).To(BeTrue())
		Expect(integrationPipelineRun.Spec.Workspaces != nil).To(BeTrue())
		Expect(len(integrationPipelineRun.Spec.Workspaces) > 0).To(BeTrue())
		Expect(len(integrationPipelineRun.Spec.Params) > 0).To(BeTrue())

		Expect(k8sClient.Delete(ctx, &integrationPipelineRuns.Items[0])).Should(Succeed())

	})

	It("ensures the integrationTestPipelines are NOT created for a Snapshot that finished testing", func() {
		finishedAdapter := NewAdapter(hasBinding, finishedSnapshot, hasEnv, hasApp, integrationTestScenario, ctrl.Log, k8sClient, ctx)
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

		Eventually(func() bool {
			result, err := finishedAdapter.EnsureIntegrationTestPipelineForScenarioExists()
			return !result.CancelRequest && err == nil
		}, time.Second*20).Should(BeTrue())

		integrationPipelineRuns := &tektonv1beta1.PipelineRunList{}
		opts := []client.ListOption{
			client.InNamespace(hasApp.Namespace),
			client.MatchingLabels{
				"pipelines.appstudio.openshift.io/type": "test",
				"appstudio.openshift.io/snapshot":       finishedSnapshot.Name,
				"test.appstudio.openshift.io/scenario":  integrationTestScenario.Name,
				"appstudio.openshift.io/environment":    hasEnv.Name,
			},
		}
		Eventually(func() bool {
			err := k8sClient.List(ctx, integrationPipelineRuns, opts...)
			return len(integrationPipelineRuns.Items) == 0 && err == nil
		}, time.Second*10).Should(BeTrue())
	})

})
