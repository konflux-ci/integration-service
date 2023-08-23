package snapshot

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/tonglil/buflogr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/loader"
	"github.com/redhat-appstudio/integration-service/status"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	releasemetadata "github.com/redhat-appstudio/release-service/metadata"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockStatusAdapter struct {
	Reporter          *MockStatusReporter
	GetReportersError error
}

type MockStatusReporter struct {
	Called            bool
	ReportStatusError error
}

func (r *MockStatusReporter) ReportStatusForPipelineRun(client.Client, context.Context, *tektonv1beta1.PipelineRun) error {
	r.Called = true
	return r.ReportStatusError
}

func (r *MockStatusReporter) ReportStatusForSnapshot(client.Client, context.Context, *applicationapiv1alpha1.Snapshot, string, gitops.IntegrationTestStatus) error {
	r.Called = true
	return r.ReportStatusError
}

func (a *MockStatusAdapter) GetReporters(object client.Object) ([]status.Reporter, error) {
	return []status.Reporter{a.Reporter}, a.GetReportersError
}

var _ = Describe("Snapshot Adapter", Ordered, func() {
	var (
		adapter        *Adapter
		logger         helpers.IntegrationLogger
		statusAdapter  *MockStatusAdapter
		statusReporter *MockStatusReporter

		testReleasePlan                   *releasev1alpha1.ReleasePlan
		hasApp                            *applicationapiv1alpha1.Application
		hasComp                           *applicationapiv1alpha1.Component
		hasSnapshot                       *applicationapiv1alpha1.Snapshot
		hasSnapshotPR                     *applicationapiv1alpha1.Snapshot
		deploymentTargetClass             *applicationapiv1alpha1.DeploymentTargetClass
		integrationPipelineRun            *tektonv1beta1.PipelineRun
		integrationTestScenario           *v1beta1.IntegrationTestScenario
		integrationTestScenarioWithoutEnv *v1beta1.IntegrationTestScenario
		env                               *applicationapiv1alpha1.Environment
	)
	const (
		SampleRepoLink  = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		sample_image    = "quay.io/redhat-appstudio/sample-image"
		sample_revision = "random-value"
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

		integrationTestScenario = &v1beta1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta1.IntegrationTestScenarioSpec{
				Application: "application-sample",
				ResolverRef: v1beta1.ResolverRef{
					Resolver: "git",
					Params: []v1beta1.ResolverParameter{
						{
							Name:  "url",
							Value: "https://github.com/redhat-appstudio/integration-examples.git",
						},
						{
							Name:  "revision",
							Value: "main",
						},
						{
							Name:  "pathInRepo",
							Value: "pipelineruns/integration_pipelinerun_pass.yaml",
						},
					},
				},
				Environment: v1beta1.TestEnvironment{
					Name: "envname",
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())

		integrationTestScenarioWithoutEnv = &v1beta1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass-without-env",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta1.IntegrationTestScenarioSpec{
				Application: "application-sample",
				ResolverRef: v1beta1.ResolverRef{
					Resolver: "git",
					Params: []v1beta1.ResolverParameter{
						{
							Name:  "url",
							Value: "https://github.com/redhat-appstudio/integration-examples.git",
						},
						{
							Name:  "revision",
							Value: "main",
						},
						{
							Name:  "pathInRepo",
							Value: "pipelineruns/integration_pipelinerun_pass.yaml",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenarioWithoutEnv)).Should(Succeed())

		testReleasePlan = &releasev1alpha1.ReleasePlan{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-releaseplan-",
				Namespace:    "default",
				Labels: map[string]string{
					releasemetadata.AutoReleaseLabel: "true",
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

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:              "component",
					gitops.SnapshotComponentLabel:         "component-sample",
					"build.appstudio.redhat.com/pipeline": "enterprise-contract",
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation:   "123",
					"build.appstudio.redhat.com/commit_sha":         "6c65b2fcaea3e1a0a92476c8b5dc89e92a85f025",
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
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

		integrationPipelineRun = &tektonv1beta1.PipelineRun{
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
						Value: tektonv1beta1.ParamValue{
							Type:      tektonv1beta1.ParamTypeString,
							StringVal: "quay.io/redhat-appstudio/sample-image",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationPipelineRun)).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasSnapshot.Name,
				Namespace: "default",
			}, hasSnapshot)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasSnapshotPR)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationPipelineRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasApp)
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

	When("adapter is created", func() {
		var buf bytes.Buffer

		It("can create a new Adapter instance", func() {
			Expect(reflect.TypeOf(NewAdapter(hasSnapshot, hasApp, hasComp, logger, loader.NewMockLoader(), k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensures the integrationTestPipelines are created", func() {
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(hasSnapshot, hasApp, hasComp, log, loader.NewMockLoader(), k8sClient, ctx)
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasSnapshot,
				},
				{
					ContextKey: loader.EnvironmentContextKey,
					Resource:   env,
				},
				{
					ContextKey: loader.SnapshotComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioWithoutEnv},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioWithoutEnv},
				},
			})
			result, err := adapter.EnsureAllIntegrationTestPipelinesExist()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			requiredIntegrationTestScenarios, err := adapter.loader.GetRequiredIntegrationTestScenariosForApplication(k8sClient, adapter.context, hasApp)
			Expect(err).To(BeNil())
			Expect(requiredIntegrationTestScenarios).NotTo(BeNil())
			expectedLogEntry := "IntegrationTestScenario has environment defined,"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "IntegrationTestscenario pipeline has been created namespace default name"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("Ensure IntegrationPipelineRun can be created for scenario", func() {
			_, err := adapter.createIntegrationPipelineRun(hasApp, integrationTestScenario, hasSnapshot)
			Expect(err == nil).To(BeTrue())

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
				err := k8sClient.List(adapter.context, integrationPipelineRuns, opts...)
				return len(integrationPipelineRuns.Items) > 0 && err == nil
			}, time.Second*10).Should(BeTrue())

			Expect(k8sClient.Delete(adapter.context, &integrationPipelineRuns.Items[0])).Should(Succeed())
		})

		It("ensures global Component Image will not be updated in the PR context", func() {
			gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshotPR, "test passed")
			Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshotPR)).To(BeTrue())
			adapter.snapshot = hasSnapshotPR

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
			adapter.snapshot = hasSnapshot

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
			adapter.snapshot = hasSnapshot
			result, err := adapter.EnsureGlobalCandidateImageUpdated()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())
		})

		It("no action when EnsureAllReleasesExist function runs when AppStudio Tests failed and the snapshot is invalid", func() {
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			// Set the snapshot up for failure by setting its status as failed and invalid
			// as well as marking it as PaC pull request event type
			updatedSnapshot, err := gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
			Expect(err).ShouldNot(HaveOccurred())
			gitops.SetSnapshotIntegrationStatusAsInvalid(updatedSnapshot, "snapshot invalid")
			hasSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodePullRequestType
			Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeFalse())
			Expect(gitops.IsSnapshotValid(hasSnapshot)).To(BeFalse())

			adapter = NewAdapter(hasSnapshot, hasApp, hasComp, log, loader.NewMockLoader(), k8sClient, ctx)
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
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			// Set the snapshot up for failure by setting its status as failed and invalid
			// as well as marking it as PaC pull request event type
			updatedSnapshot, err := gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
			Expect(err).ShouldNot(HaveOccurred())
			gitops.SetSnapshotIntegrationStatusAsInvalid(updatedSnapshot, "snapshot invalid")
			hasSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodePullRequestType
			Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeFalse())
			Expect(gitops.IsSnapshotValid(hasSnapshot)).To(BeFalse())

			adapter = NewAdapter(hasSnapshot, hasApp, hasComp, log, loader.NewMockLoader(), k8sClient, ctx)
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

		It("ensures snapshot environmentBinding exists", func() {
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
			hasSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodePushType
			Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
			adapter = NewAdapter(hasSnapshot, hasApp, hasComp, log, loader.NewMockLoader(), k8sClient, ctx)
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasSnapshot,
				},
				{
					ContextKey: loader.EnvironmentContextKey,
					Resource:   env,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
				{
					ContextKey: loader.SnapshotEnvironmentBindingContextKey,
					Resource:   nil,
				},
			})
			result, err := adapter.EnsureSnapshotEnvironmentBindingExist()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			expectedLogEntry := "SnapshotEnvironmentBinding created for Snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			bindingList := &applicationapiv1alpha1.SnapshotEnvironmentBindingList{}
			opts := &client.ListOptions{
				Namespace: hasApp.Namespace,
			}
			Eventually(func() bool {
				_ = adapter.client.List(adapter.context, bindingList, opts)
				return len(bindingList.Items) > 0 && bindingList.Items[0].Spec.Snapshot == hasSnapshot.Name
			}, time.Second*10).Should(BeTrue())
			binding := bindingList.Items[0]

			Expect(binding.Spec.Application).To(Equal(hasApp.Name))
			Expect(binding.Spec.Environment).To(Equal(env.Name))

			owners := binding.GetOwnerReferences()
			Expect(len(owners) == 1).To(BeTrue())
			Expect(owners[0].Name).To(Equal(hasApp.Name))

			err = k8sClient.Delete(ctx, &binding)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures build labels/annotations prefixed with 'build.appstudio' are propagated from snapshot to Integration test PLR", func() {
			pipelineRun, err := adapter.createIntegrationPipelineRun(hasApp, integrationTestScenario, hasSnapshot)
			Expect(err).To(BeNil())
			Expect(pipelineRun).ToNot(BeNil())

			annotation, found := pipelineRun.GetAnnotations()["build.appstudio.redhat.com/commit_sha"]
			Expect(found).To(BeTrue())
			Expect(annotation).To(Equal("6c65b2fcaea3e1a0a92476c8b5dc89e92a85f025"))

			label, found := pipelineRun.GetLabels()["build.appstudio.redhat.com/pipeline"]
			Expect(found).To(BeTrue())
			Expect(label).To(Equal("enterprise-contract"))
		})

		It("ensures build labels/annotations non-prefixed with 'build.appstudio' are NOT propagated from snapshot to Integration test PLR", func() {
			pipelineRun, err := adapter.createIntegrationPipelineRun(hasApp, integrationTestScenario, hasSnapshot)
			Expect(err).To(BeNil())
			Expect(pipelineRun).ToNot(BeNil())

			// build annotations non-prefixed with 'build.appstudio' are not copied
			_, found := hasSnapshot.GetAnnotations()["appstudio.redhat.com/updateComponentOnSuccess"]
			Expect(found).To(BeTrue())
			_, found = pipelineRun.GetAnnotations()["appstudio.redhat.com/updateComponentOnSuccess"]
			Expect(found).To(BeFalse())

			// build labels non-prefixed with 'build.appstudio' are not copied
			_, found = hasSnapshot.GetLabels()[gitops.SnapshotTypeLabel]
			Expect(found).To(BeTrue())
			_, found = pipelineRun.GetLabels()[gitops.SnapshotTypeLabel]
			Expect(found).To(BeFalse())

		})
	})

	It("fails when EnsureAllReleasesExist is called and GetAutoReleasePlansForApplication returns an error", func() {
		var buf bytes.Buffer
		log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
		adapter = NewAdapter(hasSnapshot, hasApp, hasComp, log, loader.NewMockLoader(), k8sClient, ctx)

		// Mock the context with error for AutoReleasePlansContextKey
		adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
			{
				ContextKey: loader.AutoReleasePlansContextKey,
				Err:        fmt.Errorf("Failed to get all ReleasePlans"),
			},
		})

		result, err := adapter.EnsureAllReleasesExist()
		Expect(result.CancelRequest).To(BeTrue())
		Expect(result.RequeueRequest).To(BeFalse())
		Expect(err).NotTo(HaveOccurred())
		Expect(buf.String()).Should(ContainSubstring("Snapshot integration status marked as Invalid. Failed to get all ReleasePlans"))
	})

	When("multiple components exist", func() {

		var (
			secondComp *applicationapiv1alpha1.Component
			buf        bytes.Buffer
		)

		BeforeAll(func() {
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

			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(hasSnapshotPR, hasApp, hasComp, log, loader.NewMockLoader(), k8sClient, ctx)
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasSnapshotPR,
				},
				{
					ContextKey: loader.EnvironmentContextKey,
					Resource:   env,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *secondComp},
				},
				{
					ContextKey: loader.DeploymentTargetClassContextKey,
					Resource:   deploymentTargetClass,
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioWithoutEnv},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioWithoutEnv},
				},
			})
		})

		AfterAll(func() {
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
			result, err := adapter.EnsureCreationOfEnvironment()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			expectedLogEntry := "An ephemeral Environment is created for integrationTestScenario"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "A snapshotEnvironmentbinding is created"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			expectedLogEntry = "DeploymentTargetClaim is created for environment"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("ensures status is reported if ephemeral Environment provision failed", func() {
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasSnapshotPR,
				},
				{
					ContextKey: loader.EnvironmentContextKey,
					Resource:   env,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *secondComp},
				},
				{
					ContextKey: loader.DeploymentTargetClassContextKey,
					Resource:   nil,
					Err:        fmt.Errorf("error"),
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioWithoutEnv},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioWithoutEnv},
				},
			})
			statusReporter = &MockStatusReporter{}
			statusAdapter = &MockStatusAdapter{Reporter: statusReporter}
			adapter.status = statusAdapter
			result, err := adapter.EnsureCreationOfEnvironment()
			Expect(statusReporter.Called).To(BeTrue())
			Expect(!result.CancelRequest && err != nil).To(BeTrue())
		})

		It("ensure binding with scenario label created", func() {
			bindingList := &applicationapiv1alpha1.SnapshotEnvironmentBindingList{}
			opts := &client.ListOptions{
				Namespace: hasApp.Namespace,
			}
			Eventually(func() bool {
				_ = adapter.client.List(adapter.context, bindingList, opts)
				return len(bindingList.Items) > 0 && bindingList.Items[0].ObjectMeta.Labels[gitops.SnapshotTestScenarioLabel] == integrationTestScenario.Name
			}, time.Second*10).Should(BeTrue())
			binding := bindingList.Items[0]
			Expect(binding.Spec.Application).To(Equal(hasApp.Name))
			Expect(binding.Spec.Snapshot).To(Equal(hasSnapshotPR.Name))
			Expect(binding.Spec.Environment).NotTo(BeNil())

			env := &applicationapiv1alpha1.Environment{}
			err := adapter.client.Get(adapter.context, types.NamespacedName{
				Name:      binding.Spec.Environment,
				Namespace: binding.Namespace,
			}, env)
			Expect(err).To(BeNil())
			Expect(env).NotTo(BeNil())

			dtc := &applicationapiv1alpha1.DeploymentTargetClaim{}
			err = adapter.client.Get(adapter.context, types.NamespacedName{
				Name:      env.Spec.Configuration.Target.DeploymentTargetClaim.ClaimName,
				Namespace: binding.Namespace,
			}, dtc)
			Expect(err).To(BeNil())
			Expect(dtc).NotTo(BeNil())

			err = k8sClient.Delete(ctx, env)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Delete(ctx, &binding)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Delete(ctx, dtc)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})
	})

	When("An ephemeral Environment environment exists already but binding doesn't exist", func() {
		var (
			ephemeralEnv *applicationapiv1alpha1.Environment
			buf          bytes.Buffer
		)

		BeforeAll(func() {
			ephemeralEnv = &applicationapiv1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "ephemeral-env-",
					Namespace:    "default",
					Labels: map[string]string{
						gitops.SnapshotLabel:             hasSnapshotPR.Name,
						gitops.SnapshotTestScenarioLabel: integrationTestScenario.Name,
					},
				},
				Spec: applicationapiv1alpha1.EnvironmentSpec{
					Type:               "POC",
					DisplayName:        "ephemeral-environment",
					DeploymentStrategy: applicationapiv1alpha1.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{"ephemeral"},
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
			Expect(k8sClient.Create(ctx, ephemeralEnv)).Should(Succeed())

			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(hasSnapshotPR, hasApp, hasComp, log, loader.NewMockLoader(), k8sClient, ctx)
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasSnapshotPR,
				},
				{
					ContextKey: loader.EnvironmentContextKey,
					Resource:   ephemeralEnv,
				},
				{
					ContextKey: loader.SnapshotEnvironmentBindingContextKey,
					Resource:   nil,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioWithoutEnv},
				},
			})
		})

		AfterAll(func() {
			err := k8sClient.Delete(ctx, ephemeralEnv)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures the ephemeral copy Environment will not be created again for IntegrationTestScenario", func() {
			result, err := adapter.EnsureCreationOfEnvironment()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			expectedLogEntry := "Environment already exists and contains snapshot and scenario"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "A snapshotEnvironmentbinding is created"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("ensure binding with scenario label created", func() {
			bindingList := &applicationapiv1alpha1.SnapshotEnvironmentBindingList{}
			opts := &client.ListOptions{
				Namespace: hasApp.Namespace,
			}
			Eventually(func() bool {
				_ = adapter.client.List(adapter.context, bindingList, opts)
				return len(bindingList.Items) > 0 && bindingList.Items[0].ObjectMeta.Labels[gitops.SnapshotTestScenarioLabel] == integrationTestScenario.Name
			}, time.Second*10).Should(BeTrue())
			binding := bindingList.Items[0]
			Expect(binding.Spec.Application).To(Equal(hasApp.Name))
			Expect(binding.Spec.Snapshot).To(Equal(hasSnapshotPR.Name))
			Expect(binding.Spec.Environment).To(Equal(ephemeralEnv.Name))

			owners := binding.GetOwnerReferences()
			Expect(len(owners) == 1).To(BeTrue())
			Expect(owners[0].Name).To(Equal(ephemeralEnv.Name))
		})
	})

})
