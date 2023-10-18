/*
Copyright 2022 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package snapshot

import (
	"bytes"
	"fmt"
	"reflect"
	"time"

	"github.com/tonglil/buflogr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/loader"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	releasemetadata "github.com/redhat-appstudio/release-service/metadata"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	intgteststat "github.com/redhat-appstudio/integration-service/pkg/integrationteststatus"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Snapshot Adapter", Ordered, func() {
	var (
		adapter *Adapter
		logger  helpers.IntegrationLogger

		testReleasePlan                       *releasev1alpha1.ReleasePlan
		hasApp                                *applicationapiv1alpha1.Application
		hasComp                               *applicationapiv1alpha1.Component
		hasSnapshot                           *applicationapiv1alpha1.Snapshot
		hasSnapshotPR                         *applicationapiv1alpha1.Snapshot
		deploymentTargetClass                 *applicationapiv1alpha1.DeploymentTargetClass
		integrationTestScenario               *v1beta1.IntegrationTestScenario
		integrationTestScenarioWithoutEnv     *v1beta1.IntegrationTestScenario
		integrationTestScenarioWithoutEnvCopy *v1beta1.IntegrationTestScenario
		env                                   *applicationapiv1alpha1.Environment
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

				Annotations: map[string]string{
					"test.appstudio.openshift.io/kind": "kind",
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
		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:              "component",
					gitops.SnapshotComponentLabel:         "component-sample",
					"build.appstudio.redhat.com/pipeline": "enterprise-contract",
					gitops.PipelineAsCodeEventTypeLabel:   "push",
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
		err = k8sClient.Delete(ctx, hasSnapshot)
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

	When("adapter is created", func() {
		var buf bytes.Buffer

		It("can create a new Adapter instance", func() {
			Expect(reflect.TypeOf(NewAdapter(hasSnapshot, hasApp, hasComp, logger, loader.NewMockLoader(), k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensures the integrationTestPipelines are created", func() {
			integrationTestScenarioWithoutEnvCopy = &v1beta1.IntegrationTestScenario{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-pass-without-env-copy",
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
			Expect(k8sClient.Create(ctx, integrationTestScenarioWithoutEnvCopy)).Should(Succeed())

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
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioWithoutEnv, *integrationTestScenarioWithoutEnvCopy},
				},
				{
					ContextKey: loader.RequiredIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenario, *integrationTestScenarioWithoutEnv, *integrationTestScenarioWithoutEnvCopy},
				},
			})
			result, err := adapter.EnsureStaticIntegrationPipelineRunsExist()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			requiredIntegrationTestScenarios, err := adapter.loader.GetRequiredIntegrationTestScenariosForApplication(k8sClient, adapter.context, hasApp)
			Expect(err).To(BeNil())
			Expect(requiredIntegrationTestScenarios).NotTo(BeNil())
			expectedLogEntry := "IntegrationTestScenario has environment defined,"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Creating new pipelinerun for integrationTestscenario integrationTestScenario.Name example-pass-without-env"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Creating new pipelinerun for integrationTestscenario integrationTestScenario.Name example-pass-without-env-copy"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Snapshot integration status marked as In Progress. Snapshot starts being tested by the integrationPipelineRun"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			// Snapshot must have InProgress tests
			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
			Expect(err).To(BeNil())
			detail, ok := statuses.GetScenarioStatus(integrationTestScenarioWithoutEnv.Name)
			Expect(ok).To(BeTrue())
			Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusInProgress))
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
			Eventually(func() error {
				if err := k8sClient.List(adapter.context, integrationPipelineRuns, opts...); err != nil {
					return err
				}

				if expected, got := 1, len(integrationPipelineRuns.Items); expected != got {
					return fmt.Errorf("found %d PipelineRuns, expected: %d", expected, got)
				}

				return nil
			}, time.Second*10).Should(BeNil())

			Expect(integrationPipelineRuns.Items).To(HaveLen(1))
			Expect(integrationPipelineRuns.Items[0].Annotations).To(Equal(map[string]string{
				gitops.PipelineAsCodeInstallationIDAnnotation: "123",
				"build.appstudio.redhat.com/commit_sha":       "6c65b2fcaea3e1a0a92476c8b5dc89e92a85f025",
				"test.appstudio.openshift.io/kind":            "kind",
			}))

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

		It("no error from ensuring global Component Image updated when AppStudio Tests failed", func() {
			gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "test failed")
			Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeFalse())
			adapter.snapshot = hasSnapshot
			result, err := adapter.EnsureGlobalCandidateImageUpdated()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			Expect(hasComp.Spec.ContainerImage).To(Equal(""))
			Expect(hasComp.Status.LastBuiltCommit).To(Equal(""))
		})

		It("ensures global Component Image updated when AppStudio Tests succeeded", func() {
			gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
			Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
			adapter.snapshot = hasSnapshot

			result, err := adapter.EnsureGlobalCandidateImageUpdated()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			Expect(hasComp.Spec.ContainerImage).To(Equal(sample_image))
			Expect(hasComp.Status.LastBuiltCommit).To(Equal(sample_revision))

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      hasSnapshot.Name,
					Namespace: "default",
				}, hasSnapshot)
				return err == nil && gitops.IsSnapshotMarkedAsAddedToGlobalCandidateList(hasSnapshot)
			}, time.Second*10).Should(BeTrue())

			// Check if the adapter function detects that it already promoted the snapshot component
			result, err = adapter.EnsureGlobalCandidateImageUpdated()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			expectedLogEntry := "The Snapshot's component was previously added to the global candidate list, skipping adding it."
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("ensures Release created successfully", func() {
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			gitops.SetSnapshotIntegrationStatusAsFinished(hasSnapshot, "Snapshot integration status condition is finished since all testing pipelines completed")
			gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
			Expect(gitops.HaveAppStudioTestsFinished(hasSnapshot)).To(BeTrue())
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
					ContextKey: loader.AllIntegrationTestScenariosContextKey,
					Resource:   []v1beta1.IntegrationTestScenario{*integrationTestScenarioWithoutEnv},
				},
				{
					ContextKey: loader.AutoReleasePlansContextKey,
					Resource:   []releasev1alpha1.ReleasePlan{*testReleasePlan},
				},
				{
					ContextKey: loader.ReleaseContextKey,
					Resource:   &releasev1alpha1.Release{},
				},
			})

			Eventually(func() bool {
				result, err := adapter.EnsureAllReleasesExist()
				return !result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      hasSnapshot.Name,
					Namespace: "default",
				}, hasSnapshot)
				return err == nil && gitops.IsSnapshotMarkedAsAutoReleased(hasSnapshot)
			}, time.Second*10).Should(BeTrue())

			// Check if the adapter function detects that it already released the snapshot
			result, err := adapter.EnsureAllReleasesExist()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			expectedLogEntry := "The Snapshot was previously auto-released, skipping auto-release."
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
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

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      hasSnapshot.Name,
					Namespace: "default",
				}, hasSnapshot)
				return err == nil && gitops.IsSnapshotMarkedAsDeployedToRootEnvironments(hasSnapshot)
			}, time.Second*10).Should(BeTrue())

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
			Expect(owners[0].Name).To(Equal(hasSnapshot.Name))

			err = k8sClient.Delete(ctx, &binding)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

			// Check if the adapter function detects that it already released the snapshot
			result, err = adapter.EnsureSnapshotEnvironmentBindingExist()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			expectedLogEntry = "The Snapshot was previously deployed to all root environments, skipping deployment."
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
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
				{
					ContextKey: loader.SnapshotEnvironmentBindingContextKey,
					Resource:   nil,
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
			newLabels := map[string]string{}
			newLabels[gitops.SnapshotTestScenarioLabel] = integrationTestScenario.Name
			snapshotEnvironmentBinding, err := adapter.createSnapshotEnvironmentBindingForSnapshot(adapter.application, env, hasSnapshot, newLabels)
			Expect(err).To(BeNil())
			Expect(snapshotEnvironmentBinding).NotTo(BeNil())
			Expect(snapshotEnvironmentBinding.Labels[gitops.SnapshotTestScenarioLabel]).To(Equal(integrationTestScenario.Name))

			otherSnapshot := hasSnapshot.DeepCopy()
			otherSnapshot.Name = "other-snapshot"
			otherSnapshot.Spec.Components = []applicationapiv1alpha1.SnapshotComponent{{Name: secondComp.Name, ContainerImage: sample_image}}

			err = adapter.updateExistingSnapshotEnvironmentBindingWithSnapshot(snapshotEnvironmentBinding, otherSnapshot)
			Expect(err).To(BeNil())

			// get fresh copy to make sure that SEB was updated in k8s
			updatedSEB := &applicationapiv1alpha1.SnapshotEnvironmentBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: snapshotEnvironmentBinding.Namespace,
					Name:      snapshotEnvironmentBinding.Name,
				}, updatedSEB)
				return err == nil && len(updatedSEB.Spec.Components) == 1
			}, time.Second*10).Should(BeTrue())
			Expect(updatedSEB.Spec.Snapshot == otherSnapshot.Name)
			Expect(updatedSEB.Spec.Components[0].Name == secondComp.Name)

			err = k8sClient.Delete(ctx, snapshotEnvironmentBinding)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures the ephemeral copy Environment are created for IntegrationTestScenario", func() {
			result, err := adapter.EnsureCreationOfEphemeralEnvironments()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())

			expectedLogEntry := "Ephemeral environment is created for integrationTestScenario"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "A snapshotEnvironmentbinding is created"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			expectedLogEntry = "DeploymentTargetClaim is created for environment"
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
			result, err := adapter.EnsureCreationOfEphemeralEnvironments()
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

	Describe("shouldScenarioRunInEphemeralEnv", func() {
		It("returns true when env is defined in scenario", func() {
			Expect(shouldScenarioRunInEphemeralEnv(integrationTestScenario)).To(BeTrue())
		})

		It("returns false when env is NOT defined in scenario", func() {
			Expect(shouldScenarioRunInEphemeralEnv(integrationTestScenarioWithoutEnv)).To(BeFalse())
		})
	})

})
