/*
Copyright 2023 Red Hat Inc.

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

package loader

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resolutionv1beta1 "github.com/tektoncd/pipeline/pkg/apis/resolution/v1beta1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	releasev1alpha1 "github.com/konflux-ci/release-service/api/v1alpha1"
)

var _ = Describe("Loader", Ordered, func() {
	var (
		loader                     ObjectLoader
		hasSnapshot                *applicationapiv1alpha1.Snapshot
		hasGroupSnapshot           *applicationapiv1alpha1.Snapshot
		hasApp                     *applicationapiv1alpha1.Application
		hasComp                    *applicationapiv1alpha1.Component
		integrationTestScenario    *v1beta2.IntegrationTestScenario
		integrationTestScenarioOpt *v1beta2.IntegrationTestScenario
		successfulTaskRun          *tektonv1.TaskRun
		taskRunSample              *tektonv1.TaskRun
		buildPipelineRun           *tektonv1.PipelineRun
		integrationPipelineRun     *tektonv1.PipelineRun
	)

	const (
		SampleRepoLink    = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		applicationName   = "application-sample"
		snapshotName      = "snapshot-sample"
		groupSnapshotName = "group-snapshot-sample"
		sample_image      = "quay.io/redhat-appstudio/sample-image"
		sample_revision   = "random-value"
		namespace         = "default"
		prGroupSha        = "featuresha"
		prGroup           = "feature"
	)

	BeforeAll(func() {
		loader = NewLoader()

		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-sample",
				Application:    applicationName,
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
				Name:      snapshotName,
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                   "component",
					gitops.SnapshotComponentLabel:              "component-sample",
					gitops.BuildPipelineRunNameLabel:           "pipelinerun-sample",
					gitops.PRGroupHashLabel:                    prGroupSha,
					gitops.PipelineAsCodeEventTypeLabel:        "pull_request",
					gitops.PipelineAsCodePullRequestAnnotation: "1",
					gitops.ApplicationNameLabel:                hasApp.Name,
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

		hasGroupSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      groupSnapshotName,
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:            "group",
					gitops.PRGroupHashLabel:             prGroupSha,
					gitops.PipelineAsCodeEventTypeLabel: "pull_request",
					gitops.ApplicationNameLabel:         hasApp.Name,
				},
				Annotations: map[string]string{
					gitops.PRGroupAnnotation: prGroup,
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
		Expect(k8sClient.Create(ctx, hasGroupSnapshot)).Should(Succeed())

		successfulTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-pass",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-pass",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/kpavic/test-bundle:component-pipeline-pass"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, successfulTaskRun)).Should(Succeed())

		now := time.Now()
		successfulTaskRun.Status = tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				Results: []tektonv1.TaskRunResult{
					{
						Name: "HACBS_TEST_OUTPUT",
						Value: *tektonv1.NewStructuredValues(`{
											"result": "SUCCESS",
											"timestamp": "2024-05-22T06:42:21+00:00",
											"failures": 0,
											"successes": 10,
											"warnings": 0
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, successfulTaskRun)).Should(Succeed())

		integrationTestScenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: hasApp.Name,
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "git",
					Params: []v1beta2.ResolverParameter{
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
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())

		integrationTestScenarioOpt = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass-optional",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "true",
				},
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: hasApp.Name,
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "git",
					Params: []v1beta2.ResolverParameter{
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
				Contexts: []v1beta2.TestContext{
					{Name: "component_non-existent-component", Description: "Single component testing"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenarioOpt)).Should(Succeed())

		buildPipelineRun = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type":    "build",
					"pipelines.openshift.io/used-by":           "build-cloud",
					"pipelines.openshift.io/runtime":           "nodejs",
					"pipelines.openshift.io/strategy":          "s2i",
					"appstudio.openshift.io/component":         "component-sample",
					"appstudio.openshift.io/application":       applicationName,
					"appstudio.openshift.io/snapshot":          snapshotName,
					"test.appstudio.openshift.io/scenario":     integrationTestScenario.Name,
					"pipelinesascode.tekton.dev/event-type":    "pull_request",
					"test.appstudio.openshift.io/pr-group-sha": prGroupSha,
				},
				Annotations: map[string]string{
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
					"appstudio.openshift.io/snapshot":               hasSnapshot.Name,
				},
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineRef: &tektonv1.PipelineRef{
					Name: "build-pipeline-pass",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/kpavic/test-bundle:component-pipeline-pass"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
				Params: []tektonv1.Param{
					{
						Name: "output-image",
						Value: tektonv1.ParamValue{
							Type:      tektonv1.ParamTypeString,
							StringVal: "quay.io/redhat-appstudio/sample-image",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, buildPipelineRun)).Should(Succeed())

		buildPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				ChildReferences: []tektonv1.ChildStatusReference{
					{
						Name:             successfulTaskRun.Name,
						PipelineTaskName: "task1",
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

		integrationPipelineRun = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-component-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type":           "test",
					"pac.test.appstudio.openshift.io/url-org":         "redhat-appstudio",
					"pac.test.appstudio.openshift.io/original-prname": "build-service-on-push",
					"pac.test.appstudio.openshift.io/url-repository":  "build-service",
					"pac.test.appstudio.openshift.io/repository":      "build-service-pac",
					"appstudio.openshift.io/snapshot":                 hasSnapshot.Name,
					"test.appstudio.openshift.io/scenario":            integrationTestScenario.Name,
					"appstudio.openshift.io/application":              hasApp.Name,
					"appstudio.openshift.io/component":                hasComp.Name,
				},
				Annotations: map[string]string{
					"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
				},
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineRef: &tektonv1.PipelineRef{
					Name: "component-pipeline-pass",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/test-bundle:component-pipeline-pass"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, integrationPipelineRun)).Should(Succeed())

		taskRunSample = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "taskrun-sample",
				Namespace: "default",
				Labels: map[string]string{
					"tekton.dev/pipelineRun": buildPipelineRun.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, taskRunSample)).Should(Succeed())
	})

	AfterAll(func() {
		_ = k8sClient.Delete(ctx, hasSnapshot)
		_ = k8sClient.Delete(ctx, buildPipelineRun)
		_ = k8sClient.Delete(ctx, integrationPipelineRun)
		_ = k8sClient.Delete(ctx, successfulTaskRun)
		_ = k8sClient.Delete(ctx, integrationTestScenario)
		_ = k8sClient.Delete(ctx, hasApp)
		_ = k8sClient.Delete(ctx, hasComp)
		_ = k8sClient.Delete(ctx, taskRunSample)
	})

	createReleasePlan := func(releasePlan *releasev1alpha1.ReleasePlan) {
		Expect(k8sClient.Create(ctx, releasePlan)).Should(Succeed())

		// Wait for the release plan to be created
		Eventually(func() bool {
			tmpPlan := &releasev1alpha1.ReleasePlan{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: releasePlan.Namespace,
				Name:      releasePlan.Name,
			}, tmpPlan)
			return err == nil
		}, time.Second*10).Should(BeTrue())
	}

	deleteReleasePlan := func(releasePlan *releasev1alpha1.ReleasePlan) {
		err := k8sClient.Delete(ctx, releasePlan)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())

		// Wait for the release plan to be removed
		Eventually(func() bool {
			tmpPlan := &releasev1alpha1.ReleasePlan{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: releasePlan.Namespace,
				Name:      releasePlan.Name,
			}, tmpPlan)
			return k8serrors.IsNotFound(err)
		}, time.Second*10).Should(BeTrue())
	}

	It("ensures all Releases exists when HACBSTests succeeded", func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(ctx).NotTo(BeNil())
		Expect(hasSnapshot).NotTo(BeNil())
		err := gitops.MarkSnapshotAsPassed(ctx, k8sClient, hasSnapshot, "test passed")
		Expect(err).To(Succeed())
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())

		// Normally we would Ensure that releases exist here, but that requires
		// importing the snapshot package which causes an import cycle

		releases, err := loader.GetReleasesWithSnapshot(ctx, k8sClient, hasSnapshot)
		Expect(err).ToNot(HaveOccurred())
		Expect(releases).NotTo(BeNil())
		for _, release := range *releases {
			Expect(k8sClient.Delete(ctx, &release)).Should(Succeed())
		}
	})

	It("ensures the Application Components can be found ", func() {
		applicationComponents, err := loader.GetAllApplicationComponents(ctx, k8sClient, hasApp)
		Expect(err).ToNot(HaveOccurred())
		Expect(applicationComponents).NotTo(BeNil())
	})

	It("ensures we can get an Application from a Snapshot ", func() {
		app, err := loader.GetApplicationFromSnapshot(ctx, k8sClient, hasSnapshot)
		Expect(err).ToNot(HaveOccurred())
		Expect(app).NotTo(BeNil())
		Expect(app.ObjectMeta).To(Equal(hasApp.ObjectMeta))
	})

	It("ensures we can get a Component from a Snapshot ", func() {
		comp, err := loader.GetComponentFromSnapshot(ctx, k8sClient, hasSnapshot)
		Expect(err).ToNot(HaveOccurred())
		Expect(comp).NotTo(BeNil())
		Expect(comp.ObjectMeta).To(Equal(hasComp.ObjectMeta))
	})

	It("ensures a non-nil error is returned when we cannot get a Component from a Snapshot if the label is missing", func() {
		// Temporarily remove the component label from Snapshot
		delete(hasSnapshot.Labels, gitops.SnapshotComponentLabel)

		comp, err := loader.GetComponentFromSnapshot(ctx, k8sClient, hasSnapshot)
		Expect(err).To(HaveOccurred())
		Expect(comp).To(BeNil())

		// Restore the component label
		hasSnapshot.Labels[gitops.SnapshotComponentLabel] = hasComp.Name
	})

	It("ensures we can get a Component from a Pipeline Run ", func() {
		comp, err := loader.GetComponentFromPipelineRun(ctx, k8sClient, buildPipelineRun)
		Expect(err).ToNot(HaveOccurred())
		Expect(comp).NotTo(BeNil())
		Expect(comp.ObjectMeta).To(Equal(hasComp.ObjectMeta))
	})

	It("ensures we can get the application from the Pipeline Run", func() {
		app, err := loader.GetApplicationFromPipelineRun(ctx, k8sClient, buildPipelineRun)
		Expect(err).ToNot(HaveOccurred())
		Expect(app).NotTo(BeNil())
		Expect(app.ObjectMeta).To(Equal(hasApp.ObjectMeta))
	})

	It("ensures we can get the Application from a Component", func() {
		app, err := loader.GetApplicationFromComponent(ctx, k8sClient, hasComp)
		Expect(err).ToNot(HaveOccurred())
		Expect(app).NotTo(BeNil())
		Expect(app.ObjectMeta).To(Equal(hasApp.ObjectMeta))
	})

	It("ensures we can get the Snapshot from a Pipeline Run", func() {
		snapshot, err := loader.GetSnapshotFromPipelineRun(ctx, k8sClient, buildPipelineRun)
		Expect(err).ToNot(HaveOccurred())
		Expect(snapshot).NotTo(BeNil())
		Expect(snapshot.ObjectMeta).To(Equal(hasSnapshot.ObjectMeta))
	})

	It("ensures we can get the Snapshot from a Pipeline Run", func() {
		snapshots, err := loader.GetAllSnapshotsForBuildPipelineRun(ctx, k8sClient, buildPipelineRun)
		Expect(err).ToNot(HaveOccurred())
		Expect(snapshots).NotTo(BeNil())
		Expect(*snapshots).To(HaveLen(1))
		Expect((*snapshots)[0].Name).To(Equal(hasSnapshot.Name))
	})

	It("can fetch all pipelineRuns for snapshot and scenario", func() {
		pipelineRuns, err := loader.GetAllPipelineRunsForSnapshotAndScenario(ctx, k8sClient, hasSnapshot, integrationTestScenario)
		Expect(err).ToNot(HaveOccurred())
		Expect(pipelineRuns).NotTo(BeNil())
		Expect(*pipelineRuns).To(HaveLen(1))
		Expect((*pipelineRuns)[0].Name).To(Equal(integrationPipelineRun.Name))
	})

	It("can fetch all integrationTestScenario for application", func() {
		integrationTestScenarios, err := loader.GetAllIntegrationTestScenariosForApplication(ctx, k8sClient, hasApp)
		Expect(err).ToNot(HaveOccurred())
		Expect(integrationTestScenarios).NotTo(BeNil())
		Expect(*integrationTestScenarios).To(HaveLen(2))
	})

	It("can fetch required integrationTestScenario for application", func() {
		integrationTestScenarios, err := loader.GetRequiredIntegrationTestScenariosForSnapshot(ctx, k8sClient, hasApp, hasSnapshot)
		Expect(err).ToNot(HaveOccurred())
		Expect(integrationTestScenarios).NotTo(BeNil())
		Expect(*integrationTestScenarios).To(HaveLen(1))
		Expect((*integrationTestScenarios)[0].Name).To(Equal(integrationTestScenario.Name))
	})

	It("can fetch all integrationTestScenario for Snapshot, based on the context", func() {
		integrationTestScenarios, err := loader.GetAllIntegrationTestScenariosForSnapshot(ctx, k8sClient, hasApp, hasSnapshot)
		Expect(err).ToNot(HaveOccurred())
		Expect(integrationTestScenarios).NotTo(BeNil())
		Expect(*integrationTestScenarios).To(HaveLen(1))
		Expect((*integrationTestScenarios)[0].Name).To(Equal(integrationTestScenario.Name))
	})

	It("ensures that all Snapshots for a given application can be found", func() {
		snapshots, err := loader.GetAllSnapshots(ctx, k8sClient, hasApp)
		Expect(err).ToNot(HaveOccurred())
		Expect(*snapshots).To(HaveLen(2))
	})

	It("ensures the ReleasePlan can be gotten for Application", func() {
		gottenReleasePlanItems, err := loader.GetAutoReleasePlansForApplication(ctx, k8sClient, hasApp, hasSnapshot)
		Expect(err).ToNot(HaveOccurred())
		Expect(gottenReleasePlanItems).NotTo(BeNil())

	})

	It("can get all TaskRuns present in the cluster that are associated with the given pipelineRun", func() {
		taskRuns, err := loader.GetAllTaskRunsWithMatchingPipelineRunLabel(ctx, k8sClient, buildPipelineRun)
		Expect(err).ToNot(HaveOccurred())
		Expect(*taskRuns).To(HaveLen(1))
		Expect((*taskRuns)[0].Name).To(Equal(taskRunSample.Name))
	})

	When("release plan with auto-release label is created", func() {

		const (
			releasePlanName = "test-release-plan-with-label"
		)

		var (
			releasePlanWithLabel *releasev1alpha1.ReleasePlan
		)

		BeforeEach(func() {
			// Create ReleasePlan with "auto-release" label set to true
			releasePlanWithLabel = &releasev1alpha1.ReleasePlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      releasePlanName,
					Namespace: hasApp.Namespace,
					Labels: map[string]string{
						"release.appstudio.openshift.io/auto-release": "true",
					},
				},
				Spec: releasev1alpha1.ReleasePlanSpec{
					Application: hasApp.Name,
					Target:      "default",
				},
			}
			createReleasePlan(releasePlanWithLabel)
		})

		AfterEach(func() {
			deleteReleasePlan(releasePlanWithLabel)
		})

		It("ensures the auto-release plans for application are returned correctly when the auto-release label is set to true", func() {
			// Get auto-release plans for application
			autoReleasePlans, err := loader.GetAutoReleasePlansForApplication(ctx, k8sClient, hasApp, hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(autoReleasePlans).ToNot(BeNil())
			Expect(*autoReleasePlans).To(HaveLen(1))
			Expect((*autoReleasePlans)[0].Name).To(Equal(releasePlanWithLabel.Name))

		})
	})

	When("release plan without auto-release label is created", func() {

		const (
			releasePlanName = "test-release-plan-no-label"
		)

		var (
			releasePlanNoLabel *releasev1alpha1.ReleasePlan
		)

		BeforeEach(func() {
			// Create ReleasePlan without "auto-release" label
			releasePlanNoLabel = &releasev1alpha1.ReleasePlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      releasePlanName,
					Namespace: hasApp.Namespace,
				},
				Spec: releasev1alpha1.ReleasePlanSpec{
					Application: hasApp.Name,
					Target:      "default",
				},
			}
			createReleasePlan(releasePlanNoLabel)
		})

		AfterEach(func() {
			deleteReleasePlan(releasePlanNoLabel)
		})

		It("ensures the auto-release plans for application are returned correctly when the auto-release label is missing", func() {
			// Get auto-release plans for application
			autoReleasePlans, err := loader.GetAutoReleasePlansForApplication(ctx, k8sClient, hasApp, hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(autoReleasePlans).ToNot(BeNil())
			Expect(*autoReleasePlans).To(HaveLen(1))
			Expect((*autoReleasePlans)[0].Name).To(Equal(releasePlanNoLabel.Name))

		})
	})

	When("release plan with auto-release label is set to false", func() {

		const (
			releasePlanName = "test-release-plan-false-label"
		)

		var (
			releasePlanFalseLabel *releasev1alpha1.ReleasePlan
		)

		BeforeEach(func() {
			// Create ReleasePlan with the "auto-release" label set to false
			releasePlanFalseLabel = &releasev1alpha1.ReleasePlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      releasePlanName,
					Namespace: hasApp.Namespace,
					Labels: map[string]string{
						"release.appstudio.openshift.io/auto-release": "false",
					},
				},
				Spec: releasev1alpha1.ReleasePlanSpec{
					Application: hasApp.Name,
					Target:      "default",
				},
			}
			createReleasePlan(releasePlanFalseLabel)
		})

		AfterEach(func() {
			deleteReleasePlan(releasePlanFalseLabel)
		})

		It("ensures the auto-release plans for application are returned correctly when the auto-release label is set to false", func() {
			// Get auto-release plans for application
			autoReleasePlans, err := loader.GetAutoReleasePlansForApplication(ctx, k8sClient, hasApp, hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(autoReleasePlans).ToNot(BeNil())
			Expect(*autoReleasePlans).To(BeEmpty())
		})

		It("Can fetch integration test scenario", func() {
			fetchedScenario, err := loader.GetScenario(ctx, k8sClient, integrationTestScenario.Name, integrationTestScenario.Namespace)
			Expect(err).To(Succeed())
			Expect(fetchedScenario.Name).To(Equal(integrationTestScenario.Name))
			Expect(fetchedScenario.Namespace).To(Equal(integrationTestScenario.Namespace))
			Expect(fetchedScenario.Spec).To(Equal(integrationTestScenario.Spec))
		})

		It("Can fetch pipelineRun", func() {
			fetchedBuildPipelineRun, err := loader.GetPipelineRun(ctx, k8sClient, buildPipelineRun.Name, buildPipelineRun.Namespace)
			Expect(err).To(Succeed())
			Expect(fetchedBuildPipelineRun.Name).To(Equal(buildPipelineRun.Name))
			Expect(fetchedBuildPipelineRun.Namespace).To(Equal(buildPipelineRun.Namespace))
			Expect(fetchedBuildPipelineRun.Spec).To(Equal(buildPipelineRun.Spec))
		})

		It("Can fetch component", func() {
			fetchedBuildComponent, err := loader.GetComponent(ctx, k8sClient, hasComp.Spec.ComponentName, hasComp.Namespace)
			Expect(err).To(Succeed())
			Expect(fetchedBuildComponent.Name).To(Equal(hasComp.Spec.ComponentName))
			Expect(fetchedBuildComponent.Namespace).To(Equal(hasComp.Namespace))
			Expect(fetchedBuildComponent.Spec).To(Equal(hasComp.Spec))
		})

		It("Can get build plr with pr group hash", func() {
			fetchedBuildPLRs, err := loader.GetPipelineRunsWithPRGroupHash(ctx, k8sClient, hasSnapshot.Namespace, prGroupSha, hasApp.Name)
			Expect(err).To(Succeed())
			Expect((*fetchedBuildPLRs)[0].Name).To(Equal(buildPipelineRun.Name))
			Expect((*fetchedBuildPLRs)[0].Namespace).To(Equal(buildPipelineRun.Namespace))
			Expect((*fetchedBuildPLRs)[0].Spec).To(Equal(buildPipelineRun.Spec))
		})

		It("Can get matching snapshot for component and pr group hash", func() {
			fetchedSnapshots, err := loader.GetMatchingComponentSnapshotsForComponentAndPRGroupHash(ctx, k8sClient, hasSnapshot.Namespace, hasComp.Name, prGroupSha, hasApp.Name)
			Expect(err).To(Succeed())
			Expect((*fetchedSnapshots)[0].Name).To(Equal(hasSnapshot.Name))
			Expect((*fetchedSnapshots)[0].Namespace).To(Equal(hasSnapshot.Namespace))
			Expect((*fetchedSnapshots)[0].Spec).To(Equal(hasSnapshot.Spec))
		})

		It("Can get matching snapshot for pr group hash", func() {
			fetchedSnapshots, err := loader.GetMatchingComponentSnapshotsForPRGroupHash(ctx, k8sClient, "", prGroupSha, hasApp.Name)
			Expect(err).To(Succeed())
			Expect((*fetchedSnapshots)[0].Name).To(Equal(hasSnapshot.Name))
			Expect((*fetchedSnapshots)[0].Namespace).To(Equal(hasSnapshot.Namespace))
			Expect((*fetchedSnapshots)[0].Spec).To(Equal(hasSnapshot.Spec))
		})

		It("Can get matching group snapshot for pr group hash", func() {
			fetchedSnapshots, err := loader.GetMatchingGroupSnapshotsForPRGroupHash(ctx, k8sClient, "", prGroupSha, hasApp.Name)
			Expect(err).To(Succeed())
			Expect((*fetchedSnapshots)[0].Name).To(Equal(hasGroupSnapshot.Name))
			Expect((*fetchedSnapshots)[0].Namespace).To(Equal(hasGroupSnapshot.Namespace))
			Expect((*fetchedSnapshots)[0].Spec).To(Equal(hasGroupSnapshot.Spec))
		})

		It("Can get matching components from snapshots for pr group hash", func() {
			components, err := loader.GetComponentsFromSnapshotForPRGroup(ctx, k8sClient, "", "", prGroupSha, hasApp.Name)
			Expect(err).To(Succeed())
			Expect((components)[0]).To(Equal(hasComp.Name))

		})

		It("Can get all integration pipelineruns for snapshot", func() {
			plrs, err := loader.GetAllIntegrationPipelineRunsForSnapshot(ctx, k8sClient, hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(plrs).NotTo(BeNil())
			Expect(plrs).To(HaveLen(1))
			Expect((plrs)[0].Name).To(Equal(integrationPipelineRun.Name))
		})

		It("Can get a resolutionrequest", func() {
			reqName := "sample-resolutionrequest"
			reqNamespace := "default"
			rr := &resolutionv1beta1.ResolutionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      reqName,
					Namespace: reqNamespace,
				},
				Spec: resolutionv1beta1.ResolutionRequestSpec{
					Params: []tektonv1.Param{
						{
							Name: "url",
							Value: tektonv1.ParamValue{
								Type:      tektonv1.ParamTypeString,
								StringVal: "https://github.com/konflux-ci/integration-examples.git",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).Should(Succeed())

			request, err := loader.GetResolutionRequest(ctx, k8sClient, reqNamespace, reqName)
			Expect(err).NotTo(HaveOccurred())
			Expect(request).NotTo(BeNil())
			Expect(request.Name).To(Equal(reqName))
		})
	})
})
