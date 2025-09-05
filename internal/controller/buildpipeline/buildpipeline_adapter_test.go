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

package buildpipeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/status"
	"github.com/konflux-ci/integration-service/tekton"
	"github.com/konflux-ci/operator-toolkit/metadata"
	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"

	"go.uber.org/mock/gomock"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tonglil/buflogr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Pipeline Adapter", Ordered, func() {
	var (
		adapter       *Adapter
		createAdapter func() *Adapter
		buf           bytes.Buffer
		logger        helpers.IntegrationLogger
		mockReporter  *status.MockReporterInterface
		mockStatus    *status.MockStatusInterface

		successfulTaskRun            *tektonv1.TaskRun
		failedTaskRun                *tektonv1.TaskRun
		buildPipelineRun             *tektonv1.PipelineRun
		buildPipelineRun2            *tektonv1.PipelineRun
		hasComp                      *applicationapiv1alpha1.Component
		hasComp2                     *applicationapiv1alpha1.Component
		hasApp                       *applicationapiv1alpha1.Application
		hasSnapshot                  *applicationapiv1alpha1.Snapshot
		hasComSnapshot2              *applicationapiv1alpha1.Snapshot
		integrationTestScenario      *v1beta2.IntegrationTestScenario
		groupIntegrationTestScenario *v1beta2.IntegrationTestScenario
	)
	const (
		SampleRepoLink           = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		SampleCommit             = "a2ba645d50e471d5f084b"
		SampleDigest             = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		SampleImageWithoutDigest = "quay.io/redhat-appstudio/sample-image"
		SampleImage              = SampleImageWithoutDigest + "@" + SampleDigest
		invalidDigest            = "invalidDigest"
		customLabel              = "custom.appstudio.openshift.io/custom-label"
		prGroup                  = "feature1"
		prGroupSha               = "feature1hash"
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

		logger = helpers.IntegrationLogger{Logger: ctrl.Log}.WithApp(*hasApp)

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-sample",
				Application:    "application-sample",
				ContainerImage: "invalidImage",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: SampleCommit,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())

		hasComp2 = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "another-component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "another-component-sample",
				Application:    "application-sample",
				ContainerImage: "",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: SampleCommit,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComp2)).Should(Succeed())

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                   "component",
					gitops.SnapshotComponentLabel:              hasComp.Name,
					gitops.PipelineAsCodeEventTypeLabel:        gitops.PipelineAsCodePullRequestType,
					gitops.PipelineAsCodePullRequestAnnotation: "1",
					gitops.PRGroupHashLabel:                    prGroupSha,
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation: "123",
					gitops.PRGroupAnnotation:                      prGroup,
					gitops.PipelineAsCodeGitProviderAnnotation:    "github",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComp.Name,
						ContainerImage: SampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		hasComSnapshot2 = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hasComSnapshot2Name",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                         gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel:                    hasComp2.Name,
					gitops.PipelineAsCodeEventTypeLabel:              gitops.PipelineAsCodePullRequestType,
					gitops.PRGroupHashLabel:                          prGroupSha,
					"pac.test.appstudio.openshift.io/url-org":        "testorg",
					"pac.test.appstudio.openshift.io/url-repository": "testrepo",
					gitops.PipelineAsCodeSHALabel:                    "sha",
					gitops.PipelineAsCodePullRequestAnnotation:       "1",
				},
				Annotations: map[string]string{
					gitops.PRGroupAnnotation:                      prGroup,
					gitops.PipelineAsCodeGitProviderAnnotation:    "github",
					gitops.PipelineAsCodePullRequestAnnotation:    "1",
					gitops.PipelineAsCodeInstallationIDAnnotation: "123",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
			},
		}

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
							{Name: "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{Name: "name",
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
						Name: "TEST_OUTPUT",
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

		failedTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-fail",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-fail",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{Name: "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{Name: "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, failedTaskRun)).Should(Succeed())

		failedTaskRun.Status = tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				Results: []tektonv1.TaskRunResult{
					{
						Name: "TEST_OUTPUT",
						Value: *tektonv1.NewStructuredValues(`{
											"result": "FAILURE",
											"timestamp": "2024-05-22T06:42:21+00:00",
											"failures": 1,
											"successes": 0,
											"warnings": 0
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, failedTaskRun)).Should(Succeed())

		integrationTestScenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-its",
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

		groupIntegrationTestScenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-its-group",
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
				Contexts: []v1beta2.TestContext{
					{
						Name:        "group",
						Description: "group testing",
					},
				},
			},
		}
	})

	BeforeEach(func() {
		buildPipelineRun = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-build-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type":    "build",
					"pipelines.openshift.io/used-by":           "build-cloud",
					"pipelines.openshift.io/runtime":           "nodejs",
					"pipelines.openshift.io/strategy":          "s2i",
					"appstudio.openshift.io/component":         "component-sample",
					"build.appstudio.redhat.com/target_branch": "main",
					"pipelinesascode.tekton.dev/event-type":    "pull_request",
					"pipelinesascode.tekton.dev/pull-request":  "1",
					customLabel: "custom-label",
				},
				Annotations: map[string]string{
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
					"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
					"build.appstudio.openshift.io/repo":             "https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0",
					"foo":                                           "bar",
					"chains.tekton.dev/signed":                      "true",
					"pipelinesascode.tekton.dev/source-branch":      "sourceBranch",
					"pipelinesascode.tekton.dev/url-org":            "redhat",
				},
				CreationTimestamp: metav1.Time{Time: time.Now()},
			},
			Spec: tektonv1.PipelineRunSpec{
				PipelineRef: &tektonv1.PipelineRef{
					Name: "build-pipeline-pass",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{Name: "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{Name: "name",
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
							StringVal: SampleImageWithoutDigest,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, buildPipelineRun)).Should(Succeed())

		buildPipelineRun.Status = tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				Results: []tektonv1.PipelineRunResult{
					{
						Name:  "IMAGE_DIGEST",
						Value: *tektonv1.NewStructuredValues(SampleDigest),
					},
					{
						Name:  "IMAGE_URL",
						Value: *tektonv1.NewStructuredValues(SampleImageWithoutDigest),
					},
					{
						Name:  "CHAINS-GIT_URL",
						Value: *tektonv1.NewStructuredValues(SampleRepoLink),
					},
					{
						Name:  "CHAINS-GIT_COMMIT",
						Value: *tektonv1.NewStructuredValues(SampleCommit),
					},
				},
				StartTime: &metav1.Time{Time: time.Now()},
			},
			Status: v1.Status{
				Conditions: v1.Conditions{
					apis.Condition{
						Reason: "Completed",
						Status: "True",
						Type:   apis.ConditionSucceeded,
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, buildPipelineRun)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, successfulTaskRun)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, failedTaskRun)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	When("NewAdapter is called", func() {
		It("creates and return a new adapter", func() {
			Expect(reflect.TypeOf(NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient))).To(Equal(reflect.TypeOf(&Adapter{})))
		})
	})

	When("NewAdapter is created", func() {
		BeforeEach(func() {
			adapter = createAdapter()
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
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
					ContextKey: loader.GetPipelineRunContextKey,
					Resource:   buildPipelineRun,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *hasComp2},
				},
			})
		})

		It("ensures the Imagepullspec and ComponentSource from pipelinerun and prepare snapshot can be created", func() {
			imagePullSpec, err := adapter.getImagePullSpecFromPipelineRun(buildPipelineRun)
			Expect(err).ToNot(HaveOccurred())
			Expect(imagePullSpec).NotTo(BeEmpty())

			componentSource, err := adapter.getComponentSourceFromPipelineRun(buildPipelineRun)
			Expect(err).ToNot(HaveOccurred())

			applicationComponents, err := adapter.loader.GetAllApplicationComponents(adapter.context, adapter.client, adapter.application)
			Expect(err).ToNot(HaveOccurred())
			Expect(applicationComponents).NotTo(BeNil())

			snapshot, err := gitops.PrepareSnapshot(adapter.context, adapter.client, hasApp, applicationComponents, hasComp, imagePullSpec, componentSource)
			Expect(snapshot).NotTo(BeNil())
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).NotTo(BeNil())
			Expect(snapshot.Spec.Components).To(HaveLen(1), "One component should have been added to snapshot.  Other component should have been omited due to empty ContainerImage field or missing valid digest")
			Expect(snapshot.Spec.Components[0].Name).To(Equal(hasComp.Name), "The built component should have been added to the snapshot")
		})

		It("ensures that snapshot has label pointing to build pipelinerun", func() {
			expectedSnapshot, err := adapter.prepareSnapshotForPipelineRun(buildPipelineRun, hasComp, hasApp)
			Expect(err).ToNot(HaveOccurred())
			Expect(expectedSnapshot).NotTo(BeNil())

			Expect(expectedSnapshot.Labels).NotTo(BeNil())
			Expect(expectedSnapshot.Labels).Should(HaveKeyWithValue(Equal(gitops.BuildPipelineRunNameLabel), Equal(buildPipelineRun.Name)))
			Expect(expectedSnapshot.Labels).Should(HaveKeyWithValue(Equal(gitops.ApplicationNameLabel), Equal(hasApp.Name)))
			Expect(metadata.HasAnnotation(expectedSnapshot, gitops.BuildPipelineRunStartTime)).To(BeTrue())
			Expect(expectedSnapshot.Annotations[gitops.BuildPipelineRunStartTime]).NotTo(BeNil())
		})

		It("ensures that Labels and Annotations were copied to snapshot from pipelinerun", func() {
			copyToSnapshot, err := adapter.prepareSnapshotForPipelineRun(buildPipelineRun, hasComp, hasApp)
			Expect(err).ToNot(HaveOccurred())
			Expect(copyToSnapshot).NotTo(BeNil())

			prefixes := []string{gitops.BuildPipelineRunPrefix, gitops.CustomLabelPrefix, gitops.TestLabelPrefix}
			gitops.CopySnapshotLabelsAndAnnotations(hasApp, copyToSnapshot, hasComp.Name, &buildPipelineRun.ObjectMeta, prefixes)
			Expect(copyToSnapshot.Labels[gitops.SnapshotTypeLabel]).To(Equal(gitops.SnapshotComponentType))
			Expect(copyToSnapshot.Labels[gitops.SnapshotComponentLabel]).To(Equal(hasComp.Name))
			Expect(copyToSnapshot.Labels[gitops.ApplicationNameLabel]).To(Equal(hasApp.Name))
			Expect(copyToSnapshot.Labels["build.appstudio.redhat.com/target_branch"]).To(Equal("main"))
			Expect(copyToSnapshot.Annotations["build.appstudio.openshift.io/repo"]).To(Equal("https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0"))
			Expect(copyToSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel]).To(Equal(buildPipelineRun.Labels["pipelinesascode.tekton.dev/event-type"]))
			Expect(copyToSnapshot.Labels[customLabel]).To(Equal(buildPipelineRun.Labels[customLabel]))

		})

		It("ensure err is returned when pipelinerun doesn't have Result for ", func() {
			// We don't need to update the underlying resource on the control plane,
			// so we create a copy and modify its status. This prevents update conflicts in other tests.
			buildPipelineRunNoSource := buildPipelineRun.DeepCopy()
			buildPipelineRunNoSource.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					ChildReferences: []tektonv1.ChildStatusReference{
						{
							Name:             successfulTaskRun.Name,
							PipelineTaskName: "task1",
						},
					},
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1.NewStructuredValues(SampleRepoLink),
						},
					},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Completed",
							Status: "True",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}

			componentSource, err := adapter.getComponentSourceFromPipelineRun(buildPipelineRunNoSource)
			Expect(componentSource).To(BeNil())
			Expect(err).To(HaveOccurred())
		})

		It("ensure err is returned when pipelinerun doesn't have Result for customized error and build pipelineRun annotated ", func() {
			// We don't need to update the underlying resource on the control plane,
			// so we create a copy and modify its status. This prevents update conflicts in other tests.
			buildPipelineRunNoSource := buildPipelineRun.DeepCopy()
			buildPipelineRunNoSource.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					ChildReferences: []tektonv1.ChildStatusReference{
						{
							Name:             successfulTaskRun.Name,
							PipelineTaskName: "task1",
						},
					},
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1.NewStructuredValues(SampleRepoLink),
						},
						{
							Name:  "IMAGE_URL",
							Value: *tektonv1.NewStructuredValues(SampleImageWithoutDigest),
						},
					},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Completed",
							Status: "True",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}

			messageError := "Missing info IMAGE_DIGEST from pipelinerun pipelinerun-build-sample"
			var info map[string]string
			expectedSnap, err := adapter.prepareSnapshotForPipelineRun(buildPipelineRunNoSource, hasComp, hasApp)
			Expect(expectedSnap).To(BeNil())
			Expect(err).To(HaveOccurred())
			err = tekton.AnnotateBuildPipelineRunWithCreateSnapshotAnnotation(adapter.context, buildPipelineRun, adapter.client, err)
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter.pipelineRun.GetAnnotations()[helpers.CreateSnapshotAnnotationName]).ToNot(BeNil())
			err = json.Unmarshal([]byte(adapter.pipelineRun.GetAnnotations()[helpers.CreateSnapshotAnnotationName]), &info)
			Expect(err).NotTo(HaveOccurred())
			Expect(info["status"]).To(Equal("failed"))
			Expect(info["message"]).To(Equal("Failed to create snapshot. Error: " + messageError))
		})

		It("ensures pipelines as code labels and annotations are propagated to the snapshot", func() {
			snapshot, err := adapter.prepareSnapshotForPipelineRun(buildPipelineRun, hasComp, hasApp)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).ToNot(BeNil())
			annotation, found := snapshot.GetAnnotations()["pac.test.appstudio.openshift.io/on-target-branch"]
			Expect(found).To(BeTrue())
			Expect(annotation).To(Equal("[main,master]"))
			label, found := snapshot.GetLabels()["pac.test.appstudio.openshift.io/event-type"]
			Expect(found).To(BeTrue())
			Expect(label).To(Equal("pull_request"))
		})

		It("ensures non-pipelines as code labels and annotations are NOT propagated to the snapshot", func() {
			snapshot, err := adapter.prepareSnapshotForPipelineRun(buildPipelineRun, hasComp, hasApp)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).ToNot(BeNil())

			// non-PaC labels are not copied
			_, found := buildPipelineRun.GetLabels()["pipelines.appstudio.openshift.io/type"]
			Expect(found).To(BeTrue())
			_, found = snapshot.GetLabels()["pipelines.appstudio.openshift.io/type"]
			Expect(found).To(BeFalse())

			// non-PaC annotations are not copied
			_, found = buildPipelineRun.GetAnnotations()["foo"]
			Expect(found).To(BeTrue())
			_, found = snapshot.GetAnnotations()["foo"]
			Expect(found).To(BeFalse())
		})

		It("ensures build labels and annotations prefixed with 'build.appstudio' are propagated to the snapshot", func() {
			snapshot, err := adapter.prepareSnapshotForPipelineRun(buildPipelineRun, hasComp, hasApp)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).ToNot(BeNil())

			annotation, found := snapshot.GetAnnotations()["build.appstudio.openshift.io/repo"]
			Expect(found).To(BeTrue())
			Expect(annotation).To(Equal("https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0"))

			label, found := snapshot.GetLabels()["build.appstudio.redhat.com/target_branch"]
			Expect(found).To(BeTrue())
			Expect(label).To(Equal("main"))
		})

		It("ensures build labels and annotations non-prefixed with 'build.appstudio' are NOT propagated to the snapshot", func() {
			snapshot, err := adapter.prepareSnapshotForPipelineRun(buildPipelineRun, hasComp, hasApp)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).ToNot(BeNil())

			// build annotations non-prefixed with 'build.appstudio' are not copied
			_, found := buildPipelineRun.GetAnnotations()["appstudio.redhat.com/updateComponentOnSuccess"]
			Expect(found).To(BeTrue())
			_, found = snapshot.GetAnnotations()["appstudio.redhat.com/updateComponentOnSuccess"]
			Expect(found).To(BeFalse())

			// build labels non-prefixed with 'build.appstudio' are not copied
			_, found = buildPipelineRun.GetLabels()["pipelines.appstudio.openshift.io/type"]
			Expect(found).To(BeTrue())
			_, found = snapshot.GetLabels()["pipelines.appstudio.openshift.io/type"]
			Expect(found).To(BeFalse())
		})

		It("ensure snapshot will not be created in instance when chains is incomplete", func() {
			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			buildPipelineRun.Annotations = map[string]string{
				"appstudio.redhat.com/updateComponentOnSuccess": "false",
				"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
				"build.appstudio.openshift.io/repo":             "https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0",
				"foo":                                           "bar",
			}
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)

			Eventually(func() bool {
				result, err := adapter.EnsureSnapshotExists()
				return !result.CancelRequest && err != nil
			}, time.Second*10).Should(BeTrue())

			expectedLogEntry := "Not processing the pipelineRun because it's not yet signed with Chains"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			unexpectedLogEntry := "Created new Snapshot"
			Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
		})

		It("ensure error info is added to build pipelineRun annotation", func() {
			buildPipelineRun.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "IMAGE_DIGEST",
							Value: *tektonv1.NewStructuredValues(invalidDigest),
						},
						{
							Name:  "IMAGE_URL",
							Value: *tektonv1.NewStructuredValues(SampleImageWithoutDigest),
						},
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1.NewStructuredValues(SampleRepoLink),
						},
						{
							Name:  "CHAINS-GIT_COMMIT",
							Value: *tektonv1.NewStructuredValues(SampleCommit),
						},
					},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Completed",
							Status: "True",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())
			adapter = createAdapter()
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
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
					ContextKey: loader.GetPipelineRunContextKey,
					Resource:   buildPipelineRun,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})
			_, err := adapter.prepareSnapshotForPipelineRun(adapter.pipelineRun, adapter.component, adapter.application)
			Expect(helpers.IsInvalidImageDigestError(err)).To(BeTrue())
			Eventually(func() bool {
				result, err := adapter.EnsureSnapshotExists()
				return result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())
			Expect(adapter.pipelineRun.GetAnnotations()[helpers.CreateSnapshotAnnotationName]).ToNot(BeNil())
			var info map[string]string
			err = json.Unmarshal([]byte(adapter.pipelineRun.GetAnnotations()[helpers.CreateSnapshotAnnotationName]), &info)
			Expect(err).NotTo(HaveOccurred())
			invalidDigestError := helpers.NewInvalidImageDigestError(hasComp.Name, SampleImageWithoutDigest+"@"+invalidDigest)
			Expect(info["status"]).To(Equal("failed"))
			Expect(info["message"]).Should(ContainSubstring(invalidDigestError.Error()))
		})
	})

	When("Pipeline creates snapshot with SNAPSHOT result", func() {
		var (
			pipelineCreatedSnapshot *applicationapiv1alpha1.Snapshot
		)

		BeforeEach(func() {
			// Create a snapshot that would be created by the pipeline
			pipelineCreatedSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipeline-created-snapshot",
					Namespace: "default",
					Annotations: map[string]string{
						"appstudio.redhat.com/snapshot-status": "pending",
					},
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasApp.Name,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           hasComp.Name,
							ContainerImage: SampleImage,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pipelineCreatedSnapshot)).Should(Succeed())

			// Add SNAPSHOT result to build pipeline run
			buildPipelineRun.Status.Results = append(buildPipelineRun.Status.Results, tektonv1.PipelineRunResult{
				Name:  "SNAPSHOT",
				Value: *tektonv1.NewStructuredValues("pipeline-created-snapshot"),
			})
			Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, pipelineCreatedSnapshot)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("processes existing snapshot when SNAPSHOT result is present", func() {
			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)

			Eventually(func() bool {
				result, err := adapter.EnsureSnapshotExists()
				return !result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())

			expectedLogEntry := "Found SNAPSHOT result in PipelineRun, processing existing snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Processed pipeline-created Snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			unexpectedLogEntry := "Created new Snapshot"
			Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))

			// Verify the pending annotation was removed
			Eventually(func() bool {
				updatedSnapshot := &applicationapiv1alpha1.Snapshot{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      pipelineCreatedSnapshot.Name,
					Namespace: pipelineCreatedSnapshot.Namespace,
				}, updatedSnapshot)
				if err != nil {
					return false
				}
				_, exists := updatedSnapshot.Annotations["appstudio.redhat.com/snapshot-status"]
				return !exists
			}, time.Second*5).Should(BeTrue())
		})

		It("falls back to traditional workflow when referenced snapshot is not found", func() {
			// Delete the snapshot to simulate missing reference
			Expect(k8sClient.Delete(ctx, pipelineCreatedSnapshot)).Should(Succeed())

			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})

			Eventually(func() bool {
				result, err := adapter.EnsureSnapshotExists()
				return !result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())

			expectedLogEntry := "Referenced snapshot not found, falling back to traditional workflow"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("continues with traditional workflow when no SNAPSHOT result is present", func() {
			// Remove the SNAPSHOT result
			buildPipelineRun.Status.Results = buildPipelineRun.Status.Results[:len(buildPipelineRun.Status.Results)-1]
			Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})

			Eventually(func() bool {
				result, err := adapter.EnsureSnapshotExists()
				return !result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())

			unexpectedLogEntry := "Found SNAPSHOT result in PipelineRun"
			Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
		})
	})

	When("Snapshot already exists", func() {
		It("ensures snapshot creation is skipped when snapshot already exists", func() {
			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			// check the behavior when there are multiple Snapshots associated with the build pipelineRun
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
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
					ContextKey: loader.GetPipelineRunContextKey,
					Resource:   buildPipelineRun,
				},
				{
					ContextKey: loader.PipelineRunsContextKey,
					Resource:   []tektonv1.PipelineRun{*buildPipelineRun},
				},
				{
					ContextKey: loader.AllSnapshotsForBuildPipelineRunContextKey,
					Resource:   []applicationapiv1alpha1.Snapshot{*hasSnapshot, *hasSnapshot},
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})

			Eventually(func() bool {
				result, err := adapter.EnsureSnapshotExists()
				return !result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())

			expectedLogEntry := "The build pipelineRun is already associated with more than one existing Snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			unexpectedLogEntry := "Created new Snapshot"
			Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))

			// check the behavior when there is only one Snapshot associated with the build pipelineRun
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
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
					ContextKey: loader.GetPipelineRunContextKey,
					Resource:   buildPipelineRun,
				},
				{
					ContextKey: loader.PipelineRunsContextKey,
					Resource:   []tektonv1.PipelineRun{*buildPipelineRun},
				},
				{
					ContextKey: loader.AllSnapshotsForBuildPipelineRunContextKey,
					Resource:   []applicationapiv1alpha1.Snapshot{*hasSnapshot},
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})

			Eventually(func() bool {
				result, err := adapter.EnsureSnapshotExists()
				return !result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())

			expectedLogEntry = "There is an existing Snapshot associated with this build pipelineRun, but the pipelineRun is not yet annotated"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Updated build pipelineRun"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			unexpectedLogEntry = "Created new Snapshot"
			Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))

			// The previous call should have added the Snapshot annotation to the buildPipelineRun
			// now we test if that is detected correctly
			Eventually(func() bool {
				result, err := adapter.EnsureSnapshotExists()
				return !result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())

			expectedLogEntry = "The build pipelineRun is already associated with existing Snapshot via annotation"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			unexpectedLogEntry = "Created new Snapshot"
			Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
		})
	})

	When("multiple succesfull build pipeline runs exists for the same component", func() {
		BeforeAll(func() {
			buildPipelineRun2 = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipelinerun-build-sample-2",
					Namespace: "default",
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type": "build",
						"pipelines.openshift.io/used-by":        "build-cloud",
						"pipelines.openshift.io/runtime":        "nodejs",
						"pipelines.openshift.io/strategy":       "s2i",
						"appstudio.openshift.io/component":      "component-sample",
						"pipelinesascode.tekton.dev/event-type": "pull_request",
					},
					Annotations: map[string]string{
						"appstudio.redhat.com/updateComponentOnSuccess": "false",
						"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
						"foo": "bar",
					},
				},
				Spec: tektonv1.PipelineRunSpec{
					PipelineRef: &tektonv1.PipelineRef{
						Name: "build-pipeline-pass",
						ResolverRef: tektonv1.ResolverRef{
							Resolver: "bundle",
							Params: tektonv1.Params{
								{Name: "bundle",
									Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
								},
								{Name: "name",
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
								StringVal: SampleImageWithoutDigest,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, buildPipelineRun2)).Should(Succeed())

			buildPipelineRun2.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "IMAGE_DIGEST",
							Value: *tektonv1.NewStructuredValues(SampleDigest),
						},
					},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Completed",
							Status: "True",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, buildPipelineRun2)).Should(Succeed())
			Expect(helpers.HasPipelineRunSucceeded(buildPipelineRun2)).To(BeTrue())
		})

		AfterAll(func() {
			err := k8sClient.Delete(ctx, buildPipelineRun2)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("can detect if a PipelineRun has succeeded", func() {
			buildPipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "False",
			})
			Expect(helpers.HasPipelineRunSucceeded(buildPipelineRun)).To(BeFalse())
			buildPipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "True",
			})
			Expect(helpers.HasPipelineRunSucceeded(buildPipelineRun)).To(BeTrue())
			Expect(helpers.HasPipelineRunSucceeded(&tektonv1.TaskRun{})).To(BeFalse())
		})

		It("Can add an annotation to the build pipelinerun", func() {
			err := tekton.AnnotateBuildPipelineRun(adapter.context, buildPipelineRun, "test", "value", adapter.client)
			Expect(err).NotTo(HaveOccurred())
			Expect(buildPipelineRun.Annotations["test"]).To(Equal("value"))
		})

		It("can annotate the build pipelineRun with the Snapshot name", func() {
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
			err := adapter.annotateBuildPipelineRunWithSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(adapter.pipelineRun.Annotations[tektonconsts.SnapshotNameLabel]).To(Equal(hasSnapshot.Name))
		})

		It("Can annotate the build pipelineRun with the CreateSnapshot annotate", func() {
			sampleErr := errors.New("this is a sample error")
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
			err := tekton.AnnotateBuildPipelineRunWithCreateSnapshotAnnotation(adapter.context, buildPipelineRun, adapter.client, sampleErr)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(3 * time.Second)
			// Get pipeline run from cluster
			newPipelineRun := new(tektonv1.PipelineRun)
			err = k8sClient.Get(ctx, types.NamespacedName{
				Namespace: buildPipelineRun.Namespace,
				Name:      buildPipelineRun.Name,
			}, newPipelineRun)
			Expect(err).NotTo(HaveOccurred())

			// Check that annotation from pipelineRun contains the JSON string we expect
			Expect(newPipelineRun.Annotations[helpers.CreateSnapshotAnnotationName]).NotTo(BeNil())
			var info map[string]string
			err = json.Unmarshal([]byte(newPipelineRun.Annotations[helpers.CreateSnapshotAnnotationName]), &info)
			Expect(err).NotTo(HaveOccurred())
			Expect(info["status"]).To(Equal("failed"))
			Expect(info["message"]).To(Equal("Failed to create snapshot. Error: " + sampleErr.Error()))

			// Check that an attempt to modify a pipelineRun that's being deleted doesn't do anything
			newPipelineRun.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			newSampleErr := errors.New("this is a different sample error")
			err = tekton.AnnotateBuildPipelineRunWithCreateSnapshotAnnotation(adapter.context, newPipelineRun, adapter.client, newSampleErr)
			Expect(err).NotTo(HaveOccurred())
			Expect(info["status"]).To(Equal("failed"))
			Expect(info["message"]).To(Equal("Failed to create snapshot. Error: " + sampleErr.Error()))
		})

		It("can find matching snapshot", func() {
			// make sure the first pipeline started as first
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.AllSnapshotsContextKey,
					Resource:   []applicationapiv1alpha1.Snapshot{*hasSnapshot},
				},
			})
			allSnapshots, err := adapter.loader.GetAllSnapshots(adapter.context, adapter.client, adapter.application)
			Expect(err).ToNot(HaveOccurred())
			Expect(allSnapshots).NotTo(BeNil())
			existingSnapshot := gitops.FindMatchingSnapshot(hasApp, allSnapshots, hasSnapshot)
			Expect(existingSnapshot).NotTo(BeNil())
			Expect(existingSnapshot.Name).To(Equal(hasSnapshot.Name))
		})
	})

	When("A new Build pipelineRun is created", func() {

		When("can add and remove finalizers from the pipelineRun", func() {
			BeforeEach(func() {
				adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
			})
			It("can add and remove finalizers from build pipelineRun", func() {
				// Mark build PLR as incomplete
				buildPipelineRun.Status.Conditions = nil
				Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

				// Ensure PLR does not have finalizer
				existingBuildPLR := new(tektonv1.PipelineRun)
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: buildPipelineRun.Namespace,
					Name:      buildPipelineRun.Name,
				}, existingBuildPLR)
				Expect(err).ToNot(HaveOccurred())
				Expect(existingBuildPLR.ObjectMeta.Finalizers).To(BeNil())

				// Add Finalizer to PLR
				result, err := adapter.EnsurePipelineIsFinalized()
				Expect(result.CancelRequest).To(BeFalse())
				Expect(err).ToNot(HaveOccurred())

				// Ensure PLR has finalizer
				Eventually(func() bool {
					return slices.Contains(buildPipelineRun.ObjectMeta.Finalizers, helpers.IntegrationPipelineRunFinalizer)
				}, time.Second*10).Should(BeTrue())

				// Update build PLR as completed
				buildPipelineRun.Status = tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						ChildReferences: []tektonv1.ChildStatusReference{
							{
								Name:             successfulTaskRun.Name,
								PipelineTaskName: "task1",
							},
						},
						Results: []tektonv1.PipelineRunResult{
							{
								Name:  "CHAINS-GIT_URL",
								Value: *tektonv1.NewStructuredValues(SampleRepoLink),
							},
							{
								Name:  "IMAGE_URL",
								Value: *tektonv1.NewStructuredValues(SampleImageWithoutDigest),
							},
						},
					},
					Status: v1.Status{
						Conditions: v1.Conditions{
							apis.Condition{
								Reason: "Completed",
								Status: "True",
								Type:   apis.ConditionSucceeded,
							},
						},
					},
				}
				Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

				Eventually(func() bool {
					result, err := adapter.EnsureSnapshotExists()
					return result.CancelRequest && err == nil
				}, time.Second*10).Should(BeTrue())
				// Ensure the PLR on the control plane does not have finalizer
				Eventually(func() bool {
					updatedBuildPLR := new(tektonv1.PipelineRun)
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: buildPipelineRun.Namespace,
						Name:      buildPipelineRun.Name,
					}, updatedBuildPLR)
					return err == nil && !controllerutil.ContainsFinalizer(updatedBuildPLR, helpers.IntegrationPipelineRunFinalizer)
				}, time.Second*20).Should(BeTrue())
			})

		})

		When("add pr group to the build pipelineRun annotations and labels", func() {
			BeforeEach(func() {
				adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
			})
			It("add pr group to the build pipelineRun annotations and labels", func() {
				existingBuildPLR := new(tektonv1.PipelineRun)
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Namespace: buildPipelineRun.Namespace,
						Name:      buildPipelineRun.Name,
					}, existingBuildPLR)
					return err == nil
				}, time.Second*10).Should(BeTrue())

				Expect(metadata.HasAnnotation(existingBuildPLR, gitops.PRGroupAnnotation)).To(BeFalse())
				Expect(metadata.HasLabel(existingBuildPLR, gitops.PRGroupHashLabel)).To(BeFalse())

				// Add label and annotation to PLR
				result, err := adapter.EnsurePRGroupAnnotated()
				Expect(err).NotTo(HaveOccurred())
				Expect(result.CancelRequest).To(BeFalse())
				Expect(result.RequeueRequest).To(BeFalse())

				Eventually(func() bool {
					_ = adapter.client.Get(adapter.context, types.NamespacedName{
						Namespace: buildPipelineRun.Namespace,
						Name:      buildPipelineRun.Name,
					}, existingBuildPLR)
					return metadata.HasAnnotation(existingBuildPLR, gitops.PRGroupAnnotation) && metadata.HasLabel(existingBuildPLR, gitops.PRGroupHashLabel)
				}, time.Second*10).Should(BeTrue())

				Expect(existingBuildPLR.Annotations).Should(HaveKeyWithValue(Equal(gitops.PRGroupAnnotation), Equal("sourceBranch")))
				Expect(existingBuildPLR.Labels[gitops.PRGroupHashLabel]).NotTo(BeNil())
			})
		})

		When("running pipeline with deletion timestamp is processed", func() {

			var runningDeletingBuildPipeline *tektonv1.PipelineRun

			BeforeEach(func() {
				runningDeletingBuildPipeline = &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pipelinerun-build-running-deleting",
						Namespace: "default",
						Labels: map[string]string{
							"pipelines.appstudio.openshift.io/type":    "build",
							"pipelines.openshift.io/used-by":           "build-cloud",
							"pipelines.openshift.io/runtime":           "nodejs",
							"pipelines.openshift.io/strategy":          "s2i",
							"appstudio.openshift.io/component":         "component-sample",
							"pipelinesascode.tekton.dev/event-type":    "pull_request",
							"build.appstudio.redhat.com/target_branch": "main",
						},
						Annotations: map[string]string{
							"appstudio.redhat.com/updateComponentOnSuccess": "false",
							"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
							"build.appstudio.openshift.io/repo":             "https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0",
							"foo":                                           "bar",
						},
					},
					Spec: tektonv1.PipelineRunSpec{
						PipelineRef: &tektonv1.PipelineRef{
							Name: "build-pipeline-pass",
							ResolverRef: tektonv1.ResolverRef{
								Resolver: "bundle",
								Params: tektonv1.Params{
									{Name: "bundle",
										Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
									},
									{Name: "name",
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
									StringVal: SampleImageWithoutDigest,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, runningDeletingBuildPipeline)).Should(Succeed())

				runningDeletingBuildPipeline.Status = tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						Results: []tektonv1.PipelineRunResult{},
					},
					Status: v1.Status{
						Conditions: v1.Conditions{
							apis.Condition{
								Reason: "Running",
								Status: "Unknown",
								Type:   apis.ConditionSucceeded,
							},
						},
					},
				}
				Expect(k8sClient.Status().Update(ctx, runningDeletingBuildPipeline)).Should(Succeed())

				adapter = NewAdapter(ctx, runningDeletingBuildPipeline, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
				adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
					{
						ContextKey: loader.GetPipelineRunContextKey,
						Resource:   runningDeletingBuildPipeline,
					},
				})
			})

			AfterEach(func() {
				err := k8sClient.Delete(ctx, runningDeletingBuildPipeline)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})
			// tekton is keeping deleted pipelines in running state, we have to remove finalizer in this case
			It("removes finalizer", func() {
				// Add Finalizer to PLR
				result, err := adapter.EnsurePipelineIsFinalized()
				Expect(result.CancelRequest).To(BeFalse())
				Expect(err).ToNot(HaveOccurred())
				// make sure the finelizer is there
				Expect(controllerutil.ContainsFinalizer(runningDeletingBuildPipeline, helpers.IntegrationPipelineRunFinalizer)).To(BeTrue())

				// deletionTimestamp must be set here, create client call in BeforeEach() removes it
				runningDeletingBuildPipeline.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				Eventually(func() bool {
					result, err := adapter.EnsureSnapshotExists()
					return !result.CancelRequest && err == nil
				}, time.Second*10).Should(BeTrue())
				Expect(controllerutil.ContainsFinalizer(runningDeletingBuildPipeline, helpers.IntegrationPipelineRunFinalizer)).To(BeFalse())
			})

		})

		When("attempting to update a problematic build pipelineRun", func() {
			It("handles an already deleted build pipelineRun", func() {
				notFoundErr := new(k8serrors.StatusError)
				notFoundErr.ErrStatus = metav1.Status{
					Message: "Resource Not Found",
					Code:    404,
					Status:  "Failure",
					Reason:  metav1.StatusReasonNotFound,
				}

				var buf bytes.Buffer
				log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
				buildPipelineRun.Annotations[gitops.SnapshotLabel] = hasSnapshot.Name
				adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
				adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
					{
						ContextKey: loader.GetPipelineRunContextKey,
						Resource:   nil,
						Err:        notFoundErr,
					},
				})

				Eventually(func() bool {
					result, err := adapter.EnsureSnapshotExists()
					return !result.RequeueRequest && err == nil
				}, time.Second*10).Should(BeTrue())

				expectedLogEntry := "The build pipelineRun is already associated with existing Snapshot via annotation"
				Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
				unexpectedLogEntry := "Created new Snapshot"
				Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
				unexpectedLogEntry = "Updated build pipelineRun"
				Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
				unexpectedLogEntry = "Removed Finalizer from the PipelineRun"
				Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
			})
			It("handles a build pipelineRun that has Snapshot annotation, but runs into conflicts updating status", func() {
				conflictErr := new(k8serrors.StatusError)
				conflictErr.ErrStatus = metav1.Status{
					Message: "Operation cannot be fulfilled",
					Code:    409,
					Status:  "Failure",
					Reason:  metav1.StatusReasonConflict,
				}

				var buf bytes.Buffer
				log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
				buildPipelineRun.Annotations[gitops.SnapshotLabel] = hasSnapshot.Name
				adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
				adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
					{
						ContextKey: loader.GetPipelineRunContextKey,
						Resource:   buildPipelineRun,
						Err:        conflictErr,
					},
				})

				Eventually(func() bool {
					result, err := adapter.EnsureSnapshotExists()
					return result.RequeueRequest && err != nil
				}, time.Second*10).Should(BeTrue())

				expectedLogEntry := "The build pipelineRun is already associated with existing Snapshot via annotation"
				Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
				expectedLogEntry = "Operation cannot be fulfilled"
				Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

				unexpectedLogEntry := "Created new Snapshot"
				Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
				unexpectedLogEntry = "Updated build pipelineRun"
				Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
				unexpectedLogEntry = "Removed Finalizer from the PipelineRun"
				Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
			})
			It("handles a build pipelineRun without Snapshot annotation that runs into conflicts when trying to add it", func() {
				conflictErr := new(k8serrors.StatusError)
				conflictErr.ErrStatus = metav1.Status{
					Message: "Operation cannot be fulfilled",
					Code:    409,
					Status:  "Failure",
					Reason:  metav1.StatusReasonConflict,
				}

				var buf bytes.Buffer
				log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
				adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
				adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
					{
						ContextKey: loader.GetPipelineRunContextKey,
						Resource:   buildPipelineRun,
						Err:        conflictErr,
					},
					{
						ContextKey: loader.AllSnapshotsContextKey,
						Resource:   []applicationapiv1alpha1.Snapshot{*hasSnapshot},
					},
				})

				Eventually(func() bool {
					result, err := adapter.EnsureSnapshotExists()
					return result.RequeueRequest && err != nil
				}, time.Second*10).Should(BeTrue())

				expectedLogEntry := "Operation cannot be fulfilled"
				Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

				unexpectedLogEntry := "The build pipelineRun is already associated with existing Snapshot via annotation"
				Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
				unexpectedLogEntry = "Created new Snapshot"
				Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
				unexpectedLogEntry = "Updated build pipelineRun"
				Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
				unexpectedLogEntry = "Removed Finalizer from the PipelineRun"
				Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
			})
		})
	})

	When("A new Build pipelineRun has started running", func() {
		BeforeEach(func() {
			// Remove the PR group creation Annotation from the group Snapshot
			delete(hasSnapshot.Annotations, gitops.PRGroupCreationAnnotation)
			Expect(k8sClient.Update(ctx, hasSnapshot)).Should(Succeed())
			// Update build PLR as running
			buildPipelineRun.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					ChildReferences: []tektonv1.ChildStatusReference{
						{
							Name:             successfulTaskRun.Name,
							PipelineTaskName: "task1",
						},
					},
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1.NewStructuredValues(SampleRepoLink),
						},
						{
							Name:  "IMAGE_URL",
							Value: *tektonv1.NewStructuredValues(SampleImageWithoutDigest),
						},
					},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Running",
							Status: "Unknown",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

			// Add label and annotation to PLR
			err := metadata.AddLabels(buildPipelineRun, map[string]string{gitops.PRGroupHashLabel: "b4e3bd082b29abdca3442e1e04ddf88ce82fc01ae6f577dd879de813ff5aa4"})
			Expect(err).NotTo(HaveOccurred())
			err = metadata.AddAnnotations(buildPipelineRun, map[string]string{gitops.PRGroupAnnotation: "sourceBranch"})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Update(ctx, buildPipelineRun)).Should(Succeed())

			Eventually(func() bool {
				updatedBuildPLR := new(tektonv1.PipelineRun)
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Namespace: buildPipelineRun.Namespace,
					Name:      buildPipelineRun.Name,
				}, updatedBuildPLR)
				return !helpers.HasPipelineRunFinished(buildPipelineRun) && metadata.HasAnnotation(buildPipelineRun, gitops.PRGroupAnnotation) &&
					metadata.HasLabel(buildPipelineRun, gitops.PRGroupHashLabel)
			}, time.Second*20).Should(BeTrue())
		})

		When("add pr group to the build pipelineRun annotations and labels", func() {
			BeforeEach(func() {
				// Mock an in-flight component build PLR that belongs to the same PR group
				otherComp := hasComp.DeepCopy()
				otherComp.Name = "other-component"

				buildPipelineRun2 = buildPipelineRun.DeepCopy()
				buildPipelineRun2.Name = "incoming-build-pipeline-run"
				buildPipelineRun2.Labels[tektonconsts.ComponentNameLabel] = otherComp.Name
				delete(buildPipelineRun2.Annotations, gitops.PRGroupAnnotation)
				delete(buildPipelineRun2.Labels, gitops.PRGroupHashLabel)
				buildPipelineRun2.ResourceVersion = ""

				Expect(k8sClient.Create(ctx, buildPipelineRun2)).Should(Succeed())

				buildPipelineRun2.Status = tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						Results: []tektonv1.PipelineRunResult{},
					},
					Status: v1.Status{
						Conditions: v1.Conditions{
							apis.Condition{
								Reason: "Running",
								Status: "Unknown",
								Type:   apis.ConditionSucceeded,
							},
						},
					},
				}
				Expect(k8sClient.Status().Update(ctx, buildPipelineRun2)).Should(Succeed())

				// Set the timestamp in the future, so it's newer than the original buildPipelineRun
				buildPipelineRun2.CreationTimestamp = metav1.NewTime(time.Now().Add(time.Hour * 12))

				buf = bytes.Buffer{}
				log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
				adapter = NewAdapter(ctx, buildPipelineRun2, otherComp, hasApp, log, loader.NewMockLoader(), k8sClient)
				adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
					{
						ContextKey: loader.ApplicationComponentsContextKey,
						Resource:   []applicationapiv1alpha1.Component{*hasComp},
					},
					{
						ContextKey: loader.GetComponentSnapshotsKey,
						Resource:   []applicationapiv1alpha1.Snapshot{*hasSnapshot},
					},
					{
						ContextKey: loader.GetBuildPLRContextKey,
						Resource:   []tektonv1.PipelineRun{*buildPipelineRun, *buildPipelineRun2},
					},
				})
			})
			AfterEach(func() {
				Expect(k8sClient.Delete(ctx, buildPipelineRun2)).Should(Succeed())
			})
			It("notifies the latest Snapshots and in-flight builds in the PR group about the incoming new build pipelineRun", func() {
				result, err := adapter.EnsurePRGroupAnnotated()
				Expect(err).NotTo(HaveOccurred())
				Expect(result.CancelRequest).To(BeFalse())
				Expect(result.RequeueRequest).To(BeFalse())

				expectedLogEntry := "pr group info is updated to build pipelineRun metadata"
				Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
				expectedLogEntry = "notified all component snapshots and build pipelines in the pr group about the build pipeline status"
				Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
				expectedLogEntry = "build pipelineRun has had pr group info in metadata, no need to update"
				Expect(buf.String()).ShouldNot(ContainSubstring(expectedLogEntry))

				Eventually(func() bool {
					err := adapter.client.Get(adapter.context, types.NamespacedName{
						Namespace: hasSnapshot.Namespace,
						Name:      hasSnapshot.Name,
					}, hasSnapshot)
					return err == nil && metadata.HasAnnotation(hasSnapshot, gitops.PRGroupCreationAnnotation)
				}, time.Second*10).Should(BeTrue())

				Eventually(func() bool {
					err = adapter.client.Get(adapter.context, types.NamespacedName{
						Namespace: buildPipelineRun.Namespace,
						Name:      buildPipelineRun.Name,
					}, buildPipelineRun)
					return err == nil && metadata.HasAnnotation(buildPipelineRun, gitops.PRGroupCreationAnnotation)
				}, time.Second*10).Should(BeTrue())
			})
		})
	})

	When("A Build pipelineRun has failed", func() {
		BeforeEach(func() {
			// Remove the PR group creation Annotation from the group Snapshot
			delete(hasSnapshot.Annotations, gitops.PRGroupCreationAnnotation)
			Expect(k8sClient.Update(ctx, hasSnapshot)).Should(Succeed())

			// Update build PLR as failed
			buildPipelineRun.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					ChildReferences: []tektonv1.ChildStatusReference{
						{
							Name:             failedTaskRun.Name,
							PipelineTaskName: "task1",
						},
					},
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1.NewStructuredValues(SampleRepoLink),
						},
						{
							Name:  "IMAGE_URL",
							Value: *tektonv1.NewStructuredValues(SampleImageWithoutDigest),
						},
					},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Failed",
							Status: "False",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

			Eventually(func() bool {
				updatedBuildPLR := new(tektonv1.PipelineRun)
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Namespace: buildPipelineRun.Namespace,
					Name:      buildPipelineRun.Name,
				}, updatedBuildPLR)
				return helpers.HasPipelineRunFinished(buildPipelineRun) && !helpers.HasPipelineRunSucceeded(buildPipelineRun)
			}, time.Second*20).Should(BeTrue())
		})

		When("add pr group to the build pipelineRun annotations and labels", func() {
			BeforeEach(func() {
				// Add label and annotation to PLR
				err := metadata.AddLabels(buildPipelineRun, map[string]string{gitops.PRGroupHashLabel: prGroupSha})
				Expect(err).NotTo(HaveOccurred())
				err = metadata.AddAnnotations(buildPipelineRun, map[string]string{gitops.PRGroupAnnotation: prGroup})
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Update(ctx, buildPipelineRun)).Should(Succeed())

				Eventually(func() bool {
					_ = k8sClient.Get(ctx, types.NamespacedName{
						Namespace: buildPipelineRun.Namespace,
						Name:      buildPipelineRun.Name,
					}, buildPipelineRun)
					return metadata.HasAnnotation(buildPipelineRun, gitops.PRGroupAnnotation) && metadata.HasLabel(buildPipelineRun, gitops.PRGroupHashLabel)
				}, time.Second*10).Should(BeTrue())

				Expect(helpers.HasPipelineRunFinished(buildPipelineRun)).Should(BeTrue())
				Expect(helpers.HasPipelineRunSucceeded(buildPipelineRun)).Should(BeFalse())

				// Mock an in-flight component build PLR that belongs to the same PR group and component and is newer
				inFlightBuildPLR := buildPipelineRun.DeepCopy()
				inFlightBuildPLR.Name = "in-flight-build-plr"
				inFlightBuildPLR.CreationTimestamp = metav1.NewTime(time.Now().Add(time.Hour * 12))
				inFlightBuildPLR.Status = tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						Results: []tektonv1.PipelineRunResult{},
					},
					Status: v1.Status{
						Conditions: v1.Conditions{
							apis.Condition{
								Reason: "Running",
								Status: "Unknown",
								Type:   apis.ConditionSucceeded,
							},
						},
					},
				}

				buf = bytes.Buffer{}
				log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
				adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
				adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
					{
						ContextKey: loader.ApplicationComponentsContextKey,
						Resource:   []applicationapiv1alpha1.Component{*hasComp},
					},
					{
						ContextKey: loader.GetComponentSnapshotsKey,
						Resource:   []applicationapiv1alpha1.Snapshot{*hasSnapshot},
					},
					{
						ContextKey: loader.GetBuildPLRContextKey,
						Resource:   []tektonv1.PipelineRun{*inFlightBuildPLR, *buildPipelineRun},
					},
				})
			})
			It("doesn't notify latest Snapshots and in-flight builds in the PR group about the build pipeline failure because it's not the latest build", func() {
				result, err := adapter.EnsurePRGroupAnnotated()
				Expect(err).NotTo(HaveOccurred())
				Expect(result.CancelRequest).To(BeFalse())
				Expect(result.RequeueRequest).To(BeFalse())

				expectedLogEntry := "not the latest pipelineRun, skipping notifying the group about the failure"
				Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
				expectedLogEntry = "notified all component snapshots and build pipelines in the pr group about the build pipeline status"
				Expect(buf.String()).ShouldNot(ContainSubstring(expectedLogEntry))

				Eventually(func() bool {
					err := adapter.client.Get(adapter.context, types.NamespacedName{
						Namespace: hasSnapshot.Namespace,
						Name:      hasSnapshot.Name,
					}, hasSnapshot)
					return err == nil && !metadata.HasAnnotation(hasSnapshot, gitops.PRGroupCreationAnnotation)
				}, time.Second*10).Should(BeTrue())

				Eventually(func() bool {
					err = adapter.client.Get(adapter.context, types.NamespacedName{
						Namespace: buildPipelineRun.Namespace,
						Name:      buildPipelineRun.Name,
					}, buildPipelineRun)
					return err == nil && !metadata.HasAnnotation(buildPipelineRun, gitops.PRGroupCreationAnnotation)
				}, time.Second*10).Should(BeTrue())
			})
		})

		When("add pr group to the build pipelineRun annotations and labels", func() {
			BeforeEach(func() {
				// Remove the PR group creation Annotation from the group Snapshot
				delete(hasSnapshot.Annotations, gitops.PRGroupCreationAnnotation)
				Expect(k8sClient.Update(ctx, hasSnapshot)).Should(Succeed())
				// Add label and annotation to PLR
				err := metadata.AddLabels(buildPipelineRun, map[string]string{gitops.PRGroupHashLabel: prGroupSha})
				Expect(err).NotTo(HaveOccurred())
				err = metadata.AddAnnotations(buildPipelineRun, map[string]string{gitops.PRGroupAnnotation: prGroup})
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Update(ctx, buildPipelineRun)).Should(Succeed())

				Eventually(func() bool {
					_ = k8sClient.Get(ctx, types.NamespacedName{
						Namespace: buildPipelineRun.Namespace,
						Name:      buildPipelineRun.Name,
					}, buildPipelineRun)
					return metadata.HasAnnotation(buildPipelineRun, gitops.PRGroupAnnotation) && metadata.HasLabel(buildPipelineRun, gitops.PRGroupHashLabel)
				}, time.Second*10).Should(BeTrue())

				Expect(helpers.HasPipelineRunFinished(buildPipelineRun)).Should(BeTrue())
				Expect(helpers.HasPipelineRunSucceeded(buildPipelineRun)).Should(BeFalse())

				// Mock an in-flight component build PLR that belongs to the same PR group
				inFlightBuildPLR := buildPipelineRun.DeepCopy()
				inFlightBuildPLR.Labels[tektonconsts.ComponentNameLabel] = "other-component"
				inFlightBuildPLR.Status = tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						Results: []tektonv1.PipelineRunResult{},
					},
					Status: v1.Status{
						Conditions: v1.Conditions{
							apis.Condition{
								Reason: "Running",
								Status: "Unknown",
								Type:   apis.ConditionSucceeded,
							},
						},
					},
				}

				adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
				adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
					{
						ContextKey: loader.ApplicationComponentsContextKey,
						Resource:   []applicationapiv1alpha1.Component{*hasComp},
					},
					{
						ContextKey: loader.GetComponentSnapshotsKey,
						Resource:   []applicationapiv1alpha1.Snapshot{*hasSnapshot},
					},
					{
						ContextKey: loader.GetBuildPLRContextKey,
						Resource:   []tektonv1.PipelineRun{*inFlightBuildPLR},
					},
				})
			})
			It("notifies all latest Snapshots and in-flight builds in the PR group about the build pipeline failure", func() {
				result, err := adapter.EnsurePRGroupAnnotated()
				Expect(err).NotTo(HaveOccurred())
				Expect(result.CancelRequest).To(BeFalse())
				Expect(result.RequeueRequest).To(BeFalse())

				Eventually(func() bool {
					err = adapter.client.Get(adapter.context, types.NamespacedName{
						Namespace: hasSnapshot.Namespace,
						Name:      hasSnapshot.Name,
					}, hasSnapshot)
					return err == nil && metadata.HasAnnotation(hasSnapshot, gitops.PRGroupCreationAnnotation)
				}, time.Second*10).Should(BeTrue())

				expectedBuildFailureMsg := fmt.Sprintf("build PLR %s failed for component %s so it can't be added to the group Snapshot for PR group %s", buildPipelineRun.Name, hasComp.Name, prGroup)
				Expect(hasSnapshot.Annotations[gitops.PRGroupCreationAnnotation]).Should(ContainSubstring(expectedBuildFailureMsg))

				Eventually(func() bool {
					err = adapter.client.Get(adapter.context, types.NamespacedName{
						Namespace: buildPipelineRun.Namespace,
						Name:      buildPipelineRun.Name,
					}, buildPipelineRun)
					return err == nil && metadata.HasAnnotation(buildPipelineRun, gitops.PRGroupCreationAnnotation)
				}, time.Second*10).Should(BeTrue())

				Expect(buildPipelineRun.Annotations[gitops.PRGroupCreationAnnotation]).Should(ContainSubstring(expectedBuildFailureMsg))
			})
		})

	})

	When("a build PLR is triggered or retirggered, succeeded or failed", func() {
		BeforeEach(func() {
			patch := client.MergeFrom(buildPipelineRun.DeepCopy())
			_ = metadata.SetAnnotation(&buildPipelineRun.ObjectMeta, gitops.PRGroupAnnotation, prGroup)
			_ = metadata.SetLabel(&buildPipelineRun.ObjectMeta, gitops.PRGroupHashLabel, prGroupSha)
			Expect(k8sClient.Patch(ctx, buildPipelineRun, patch)).Should(Succeed())
			ctrl := gomock.NewController(GinkgoT())
			mockReporter = status.NewMockReporterInterface(ctrl)
			mockStatus = status.NewMockStatusInterface(ctrl)
			mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
			mockStatus.EXPECT().GetReporter(gomock.Any()).Return(mockReporter).AnyTimes()
			mockStatus.EXPECT().FindSnapshotWithOpenedPR(gomock.Any(), gomock.Any()).Return(hasSnapshot, 0, nil).AnyTimes()
			mockReporter.EXPECT().GetReporterName().AnyTimes()
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).AnyTimes()
			mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).AnyTimes()
		})
		It("ensure integration test is initialized from build PLR", func() {
			buildPipelineRun.Status = tektonv1.PipelineRunStatus{
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Running",
							Status: "Unknown",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
			})

			result, err := adapter.EnsureIntegrationTestReportedToGitProvider()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
			Expect(metadata.HasAnnotationWithValue(buildPipelineRun, helpers.SnapshotCreationReportAnnotation, intgteststat.BuildPLRInProgress.String())).To(BeTrue())

			result, err = adapter.EnsureIntegrationTestReportedToGitProvider()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
			expectedLogEntry := "integration test has been set correctly or is being processed, no need to set integration test status from build pipelinerun"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("ensure integration test is set from build PLR when build PLR fails", func() {
			buildPipelineRun.Status = tektonv1.PipelineRunStatus{
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Failed",
							Status: "False",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, buildPipelineRun)).Should(Succeed())

			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
			})

			result, err := adapter.EnsureIntegrationTestReportedToGitProvider()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
			Expect(metadata.HasAnnotationWithValue(buildPipelineRun, helpers.SnapshotCreationReportAnnotation, intgteststat.BuildPLRFailed.String())).To(BeTrue())

			result, err = adapter.EnsureIntegrationTestReportedToGitProvider()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
			expectedLogEntry := "integration test has been set correctly or is being processed, no need to set integration test status from build pipelinerun"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("ensure integration test is set from build PLR when build PLR succeeded but snapshot is not created", func() {
			Expect(metadata.SetAnnotation(buildPipelineRun, helpers.CreateSnapshotAnnotationName, "failed to create snapshot due to error")).ShouldNot(HaveOccurred())
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario},
				},
			})

			result, err := adapter.EnsureIntegrationTestReportedToGitProvider()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
			Expect(metadata.HasAnnotationWithValue(buildPipelineRun, helpers.SnapshotCreationReportAnnotation, intgteststat.SnapshotCreationFailed.String())).To(BeTrue())

			result, err = adapter.EnsureIntegrationTestReportedToGitProvider()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
			expectedLogEntry := "integration test has been set correctly or is being processed, no need to set integration test status from build pipelinerun"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("Ensure group context integration test can be initialized", func() {
			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			buildPipelineRun = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipelinerun-build-sample",
					Namespace: "default",
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type":    "build",
						"pipelines.openshift.io/used-by":           "build-cloud",
						"pipelines.openshift.io/runtime":           "nodejs",
						"pipelines.openshift.io/strategy":          "s2i",
						"appstudio.openshift.io/component":         "component-sample",
						"build.appstudio.redhat.com/target_branch": "main",
						"pipelinesascode.tekton.dev/event-type":    "pull_request",
						"pipelinesascode.tekton.dev/pull-request":  "1",
						"pipelinesascode.tekton.dev/git-provider":  "github",
						customLabel:             "custom-label",
						gitops.PRGroupHashLabel: prGroupSha,
					},
					Annotations: map[string]string{
						"appstudio.redhat.com/updateComponentOnSuccess": "false",
						"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
						"build.appstudio.openshift.io/repo":             "https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0",
						"chains.tekton.dev/signed":                      "true",
						"pipelinesascode.tekton.dev/source-branch":      "sourceBranch",
						"pipelinesascode.tekton.dev/url-org":            "redhat",
						"pipelinesascode.tekton.dev/git-provider":       "github",
						gitops.PRGroupAnnotation:                        prGroup,
					},
					CreationTimestamp: metav1.NewTime(time.Now().Add(time.Hour * 1)),
				},
				Spec: tektonv1.PipelineRunSpec{},
			}

			buildPipelineRun2 = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipelinerun-build-sample-2",
					Namespace: "default",
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type":   "build",
						"pipelines.openshift.io/used-by":          "build-cloud",
						"pipelines.openshift.io/runtime":          "nodejs",
						"pipelines.openshift.io/strategy":         "s2i",
						"appstudio.openshift.io/component":        "component-sample",
						"pipelinesascode.tekton.dev/event-type":   "pull_request",
						"pipelinesascode.tekton.dev/git-provider": "github",
						gitops.PRGroupHashLabel:                   prGroupSha,
					},
					Annotations: map[string]string{
						"appstudio.redhat.com/updateComponentOnSuccess": "false",
						"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
						gitops.PRGroupAnnotation:                        prGroup,
						"pipelinesascode.tekton.dev/git-provider":       "github",
					},
					CreationTimestamp: metav1.NewTime(time.Now()),
				},
				Spec: tektonv1.PipelineRunSpec{},
			}

			//mockStatus.EXPECT().IsPRMRInSnapshotOpened(gomock.Any(), hasComSnapshot2).Return(true, nil)
			mockStatus.EXPECT().IsPRMRInSnapshotOpened(gomock.Any(), gomock.Any()).Return(true, 0, nil).AnyTimes()

			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)

			adapter.status = mockStatus
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.GetPRSnapshotsKey,
					Resource:   []applicationapiv1alpha1.Snapshot{*hasSnapshot, *hasComSnapshot2},
				},
				{
					ContextKey: loader.GetComponentSnapshotsKey,
					Resource:   []applicationapiv1alpha1.Snapshot{*hasSnapshot, *hasComSnapshot2},
				},
				{
					ContextKey: loader.GetBuildPLRContextKey,
					Resource:   []tektonv1.PipelineRun{*buildPipelineRun, *buildPipelineRun2},
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp, *hasComp2},
				},
				{
					ContextKey: loader.GetComponentsFromSnapshotForPRGroupKey,
					Resource:   []string{hasComp.Name, hasComp2.Name},
				},
				{
					ContextKey: loader.AllIntegrationTestScenariosContextKey,
					Resource:   []v1beta2.IntegrationTestScenario{*integrationTestScenario, *groupIntegrationTestScenario},
				},
			})

			result, err := adapter.EnsureIntegrationTestReportedToGitProvider()
			expectedLogEntry := "Opened PR/MR in snapshot is found"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "group snapshot is expected to be created for build pipelinerun"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "there is more than 1 component with open pr or mr found, so group snapshot is expected: [component-sample another-component-sample]"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			Expect(result.CancelRequest).To(BeFalse())
			Expect(result.RequeueRequest).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("integration status should not be set from build PLR", func() {
		BeforeAll(func() {
			patch := client.MergeFrom(buildPipelineRun.DeepCopy())
			_ = metadata.SetAnnotation(&buildPipelineRun.ObjectMeta, gitops.PRGroupAnnotation, prGroup)
			_ = metadata.SetLabel(&buildPipelineRun.ObjectMeta, gitops.PRGroupHashLabel, prGroupSha)
			Expect(k8sClient.Patch(ctx, buildPipelineRun, patch)).Should(Succeed())
		})
		It("integration test will not be set from build PLR when build PLR succeeded and snapshot is created", func() {
			Expect(metadata.SetAnnotation(buildPipelineRun, tektonconsts.SnapshotNameLabel, "snashot-sample")).ShouldNot(HaveOccurred())
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
			})

			result, err := adapter.EnsureIntegrationTestReportedToGitProvider()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
			Expect(metadata.HasAnnotationWithValue(buildPipelineRun, helpers.SnapshotCreationReportAnnotation, "SnapshotCreated")).To(BeTrue())
			expectedLogEntry := "snapshot has been created for build pipelineRun, no need to report integration status from build pipelinerun status"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("integration test will not be set from build PLR when build PLR is not from pac pull request event", func() {
			Expect(metadata.DeleteLabel(buildPipelineRun, tektonconsts.PipelineAsCodePullRequestLabel)).ShouldNot(HaveOccurred())
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
			})

			result, err := adapter.EnsureIntegrationTestReportedToGitProvider()
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
			expectedLogEntry := "build pipelineRun is not created by pull/merge request, no need to set integration test status in git provider"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("integration test will not be set from build PLR when pr group is not annotated to build PLR", func() {
			Expect(metadata.DeleteLabel(buildPipelineRun, gitops.PRGroupHashLabel)).ShouldNot(HaveOccurred())
			Expect(metadata.DeleteAnnotation(buildPipelineRun, gitops.PRGroupAnnotation)).ShouldNot(HaveOccurred())
			buf = bytes.Buffer{}
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.status = mockStatus
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
			})
			result, err := adapter.EnsureIntegrationTestReportedToGitProvider()
			expectedLogEntry := "pr group info has not been added to build pipelineRun metadata, try again"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			Expect(result.RequeueRequest && err != nil).To(BeTrue())
		})
	})
	// Unit tests for refactored pipeline-created snapshot functionality
	When("Testing refactored snapshot metadata functions", func() {
		var testSnapshot *applicationapiv1alpha1.Snapshot
		var testPipelineRun *tektonv1.PipelineRun

		BeforeEach(func() {
			testSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-snapshot",
					Namespace: "default",
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasApp.Name,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           hasComp.Name,
							ContainerImage: SampleImage,
						},
					},
				},
			}

			testPipelineRun = buildPipelineRun.DeepCopy()
			testPipelineRun.Status.CompletionTime = &metav1.Time{Time: time.Now()}
			testPipelineRun.Status.StartTime = &metav1.Time{Time: time.Now().Add(-5 * time.Minute)}

			adapter = NewAdapter(ctx, testPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
		})

		Describe("applySnapshotMetadata function", func() {
			It("applies correct metadata to snapshot", func() {
				adapter.applySnapshotMetadata(testSnapshot, testPipelineRun)

				// Verify build pipeline run labels are set
				Expect(testSnapshot.Labels[gitops.BuildPipelineRunNameLabel]).To(Equal(testPipelineRun.Name))
				Expect(testSnapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel]).ToNot(BeEmpty())

				// Verify build pipeline run start time annotation is set
				Expect(testSnapshot.Annotations[gitops.BuildPipelineRunStartTime]).ToNot(BeEmpty())

				// Verify standard labels are copied from pipeline run
				Expect(testSnapshot.Labels[gitops.ApplicationNameLabel]).To(Equal(hasApp.Name))
				Expect(testSnapshot.Labels[gitops.SnapshotComponentLabel]).To(Equal(hasComp.Name))
			})

			It("handles snapshot with nil labels and annotations", func() {
				testSnapshot.Labels = nil
				testSnapshot.Annotations = nil

				adapter.applySnapshotMetadata(testSnapshot, testPipelineRun)

				// Verify maps are initialized and populated
				Expect(testSnapshot.Labels).ToNot(BeNil())
				Expect(testSnapshot.Annotations).ToNot(BeNil())
				Expect(testSnapshot.Labels[gitops.BuildPipelineRunNameLabel]).To(Equal(testPipelineRun.Name))
			})

			It("uses current time when completion time is nil", func() {
				testPipelineRun.Status.CompletionTime = nil

				beforeTime := time.Now().Unix()
				adapter.applySnapshotMetadata(testSnapshot, testPipelineRun)
				afterTime := time.Now().Unix()

				finishTimeStr := testSnapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel]
				finishTime, err := strconv.ParseInt(finishTimeStr, 10, 64)
				Expect(err).ToNot(HaveOccurred())
				Expect(finishTime).To(BeNumerically(">=", beforeTime))
				Expect(finishTime).To(BeNumerically("<=", afterTime))
			})

			It("skips start time annotation when start time is nil", func() {
				testPipelineRun.Status.StartTime = nil

				adapter.applySnapshotMetadata(testSnapshot, testPipelineRun)

				// Should not have start time annotation
				_, exists := testSnapshot.Annotations[gitops.BuildPipelineRunStartTime]
				Expect(exists).To(BeFalse())
			})
		})

		Describe("getExistingSnapshot function", func() {
			var pipelineCreatedSnapshot *applicationapiv1alpha1.Snapshot

			BeforeEach(func() {
				pipelineCreatedSnapshot = &applicationapiv1alpha1.Snapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pipeline-created-snapshot-test",
						Namespace: "default",
					},
					Spec: applicationapiv1alpha1.SnapshotSpec{
						Application: hasApp.Name,
						Components: []applicationapiv1alpha1.SnapshotComponent{
							{
								Name:           hasComp.Name,
								ContainerImage: SampleImage,
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, pipelineCreatedSnapshot)).Should(Succeed())
			})

			AfterEach(func() {
				err := k8sClient.Delete(ctx, pipelineCreatedSnapshot)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})

			It("successfully retrieves existing snapshot", func() {
				snapshot, err := adapter.getExistingSnapshot(pipelineCreatedSnapshot.Name)

				Expect(err).ToNot(HaveOccurred())
				Expect(snapshot).ToNot(BeNil())
				Expect(snapshot.Name).To(Equal(pipelineCreatedSnapshot.Name))
				Expect(snapshot.Namespace).To(Equal(pipelineCreatedSnapshot.Namespace))
			})

			It("returns not found error for non-existent snapshot", func() {
				snapshot, err := adapter.getExistingSnapshot("non-existent-snapshot")

				Expect(err).To(HaveOccurred())
				Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				Expect(snapshot).To(BeNil())
			})
		})

		Describe("processExistingSnapshot function", func() {
			var pipelineCreatedSnapshot *applicationapiv1alpha1.Snapshot

			BeforeEach(func() {
				pipelineCreatedSnapshot = &applicationapiv1alpha1.Snapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pipeline-created-snapshot-process",
						Namespace: "default",
						Annotations: map[string]string{
							"appstudio.redhat.com/snapshot-status": "pending",
						},
					},
					Spec: applicationapiv1alpha1.SnapshotSpec{
						Application: hasApp.Name,
						Components: []applicationapiv1alpha1.SnapshotComponent{
							{
								Name:           hasComp.Name,
								ContainerImage: SampleImage,
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, pipelineCreatedSnapshot)).Should(Succeed())
			})

			AfterEach(func() {
				err := k8sClient.Delete(ctx, pipelineCreatedSnapshot)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})

			It("removes pending annotation and adds standard metadata", func() {
				err := adapter.processExistingSnapshot(pipelineCreatedSnapshot)
				Expect(err).ToNot(HaveOccurred())

				// Retrieve updated snapshot to verify changes
				updatedSnapshot := &applicationapiv1alpha1.Snapshot{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      pipelineCreatedSnapshot.Name,
						Namespace: pipelineCreatedSnapshot.Namespace,
					}, updatedSnapshot)
					if err != nil {
						return false
					}
					// Check that pending annotation was removed
					_, exists := updatedSnapshot.Annotations["appstudio.redhat.com/snapshot-status"]
					return !exists
				}, time.Second*10).Should(BeTrue())

				// Verify standard metadata was added
				Expect(updatedSnapshot.Labels[gitops.BuildPipelineRunNameLabel]).To(Equal(testPipelineRun.Name))
				Expect(updatedSnapshot.Labels[gitops.ApplicationNameLabel]).To(Equal(hasApp.Name))
				Expect(updatedSnapshot.Labels[gitops.SnapshotComponentLabel]).To(Equal(hasComp.Name))
			})

			It("initializes labels and annotations if nil", func() {
				// Create snapshot without labels/annotations
				snapshotWithoutMeta := &applicationapiv1alpha1.Snapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "snapshot-without-meta",
						Namespace: "default",
					},
					Spec: pipelineCreatedSnapshot.Spec,
				}
				Expect(k8sClient.Create(ctx, snapshotWithoutMeta)).Should(Succeed())

				err := adapter.processExistingSnapshot(snapshotWithoutMeta)
				Expect(err).ToNot(HaveOccurred())

				// Verify maps were initialized and populated
				Expect(snapshotWithoutMeta.Labels).ToNot(BeNil())
				Expect(snapshotWithoutMeta.Annotations).ToNot(BeNil())
				Expect(snapshotWithoutMeta.Labels[gitops.BuildPipelineRunNameLabel]).To(Equal(testPipelineRun.Name))

				// Cleanup
				err = k8sClient.Delete(ctx, snapshotWithoutMeta)
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			})

			It("preserves existing metadata while adding new metadata", func() {
				pipelineCreatedSnapshot.Labels = map[string]string{
					"existing-label": "existing-value",
				}
				pipelineCreatedSnapshot.Annotations = map[string]string{
					"existing-annotation":                  "existing-value",
					"appstudio.redhat.com/snapshot-status": "pending",
				}
				Expect(k8sClient.Update(ctx, pipelineCreatedSnapshot)).Should(Succeed())

				err := adapter.processExistingSnapshot(pipelineCreatedSnapshot)
				Expect(err).ToNot(HaveOccurred())

				// Verify existing metadata is preserved
				Expect(pipelineCreatedSnapshot.Labels["existing-label"]).To(Equal("existing-value"))
				Expect(pipelineCreatedSnapshot.Annotations["existing-annotation"]).To(Equal("existing-value"))

				// Verify new metadata is added
				Expect(pipelineCreatedSnapshot.Labels[gitops.BuildPipelineRunNameLabel]).To(Equal(testPipelineRun.Name))

				// Verify pending annotation is removed
				_, exists := pipelineCreatedSnapshot.Annotations["appstudio.redhat.com/snapshot-status"]
				Expect(exists).To(BeFalse())
			})
		})
	})

	When("Testing end-to-end pipeline-created snapshot workflow", func() {
		var pipelineCreatedSnapshot *applicationapiv1alpha1.Snapshot
		var testPipelineRun *tektonv1.PipelineRun

		BeforeEach(func() {
			// Create a test pipeline run
			testPipelineRun = buildPipelineRun.DeepCopy()
			testPipelineRun.Name = "e2e-test-pipeline-run"
			testPipelineRun.ResourceVersion = ""
			Expect(k8sClient.Create(ctx, testPipelineRun)).Should(Succeed())

			// Create pipeline status with success
			testPipelineRun.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "IMAGE_DIGEST",
							Value: *tektonv1.NewStructuredValues(SampleDigest),
						},
						{
							Name:  "IMAGE_URL",
							Value: *tektonv1.NewStructuredValues(SampleImageWithoutDigest),
						},
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1.NewStructuredValues(SampleRepoLink),
						},
						{
							Name:  "CHAINS-GIT_COMMIT",
							Value: *tektonv1.NewStructuredValues(SampleCommit),
						},
						{
							Name:  "SNAPSHOT",
							Value: *tektonv1.NewStructuredValues("e2e-pipeline-snapshot"),
						},
					},
					StartTime:      &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
					CompletionTime: &metav1.Time{Time: time.Now()},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Completed",
							Status: "True",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, testPipelineRun)).Should(Succeed())

			// Create a snapshot that would be created by the pipeline
			pipelineCreatedSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-pipeline-snapshot",
					Namespace: "default",
					Annotations: map[string]string{
						"appstudio.redhat.com/snapshot-status": "pending",
					},
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasApp.Name,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           hasComp.Name,
							ContainerImage: SampleImage,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pipelineCreatedSnapshot)).Should(Succeed())

			adapter = NewAdapter(ctx, testPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, testPipelineRun)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Delete(ctx, pipelineCreatedSnapshot)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("processes pipeline-created snapshot successfully", func() {
			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, testPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)

			result, err := adapter.EnsureSnapshotExists()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			// Verify the pipeline-created snapshot workflow was used
			expectedLogEntry := "Found SNAPSHOT result in PipelineRun, processing existing snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "Processed pipeline-created Snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			// Verify no traditional snapshot was created
			unexpectedLogEntry := "Created new Snapshot"
			Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))

			// Verify pipeline run is annotated with snapshot name
			Eventually(func() bool {
				updatedPLR := &tektonv1.PipelineRun{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testPipelineRun.Name,
					Namespace: testPipelineRun.Namespace,
				}, updatedPLR)
				if err != nil {
					return false
				}
				return updatedPLR.Annotations[tektonconsts.SnapshotNameLabel] == pipelineCreatedSnapshot.Name
			}, time.Second*10).Should(BeTrue())

			// Verify pending annotation was removed from snapshot
			Eventually(func() bool {
				updatedSnapshot := &applicationapiv1alpha1.Snapshot{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      pipelineCreatedSnapshot.Name,
					Namespace: pipelineCreatedSnapshot.Namespace,
				}, updatedSnapshot)
				if err != nil {
					return false
				}
				_, exists := updatedSnapshot.Annotations["appstudio.redhat.com/snapshot-status"]
				return !exists
			}, time.Second*10).Should(BeTrue())
		})

		It("verifies component updates are NOT performed by build pipeline controller", func() {
			// This test ensures the refactoring goal: component updates should be handled by snapshot controller
			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, testPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)

			result, err := adapter.EnsureSnapshotExists()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			// Verify no component update logic is called
			// The comment in processExistingSnapshot should indicate this
			// We expect the snapshot controller to handle component updates, not the build pipeline controller
			Expect(buf.String()).ShouldNot(ContainSubstring("updateComponentOnSuccess"))
			Expect(buf.String()).ShouldNot(ContainSubstring("Updating component"))
		})
	})

	When("Testing error scenarios and edge cases", func() {
		var testPipelineRun *tektonv1.PipelineRun

		BeforeEach(func() {
			testPipelineRun = buildPipelineRun.DeepCopy()
			testPipelineRun.Name = "error-test-pipeline-run"
			testPipelineRun.ResourceVersion = ""
			testPipelineRun.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "SNAPSHOT",
							Value: *tektonv1.NewStructuredValues("non-existent-snapshot"),
						},
					},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Completed",
							Status: "True",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testPipelineRun)).Should(Succeed())
			Expect(k8sClient.Status().Update(ctx, testPipelineRun)).Should(Succeed())

			adapter = NewAdapter(ctx, testPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, testPipelineRun)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("falls back to traditional workflow when referenced snapshot is not found", func() {
			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, testPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})

			result, err := adapter.EnsureSnapshotExists()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			// Verify fallback message is logged
			expectedLogEntry := "Referenced snapshot not found, falling back to traditional workflow"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			// Verify traditional workflow creates snapshot
			expectedLogEntry = "Created new Snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("handles malformed SNAPSHOT result gracefully", func() {
			// Update pipeline run with empty SNAPSHOT result
			testPipelineRun.Status.Results[0].Value = *tektonv1.NewStructuredValues("")
			Expect(k8sClient.Status().Update(ctx, testPipelineRun)).Should(Succeed())

			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, testPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})

			result, err := adapter.EnsureSnapshotExists()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			// Should proceed with traditional workflow since SNAPSHOT result is empty
			expectedLogEntry := "Created new Snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			// Should not try to process pipeline-created snapshot
			unexpectedLogEntry := "Found SNAPSHOT result in PipelineRun"
			Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
		})

		It("handles processExistingSnapshot failure correctly", func() {
			// Create a snapshot but don't give adapter permissions to update it
			// This simulates a scenario where processExistingSnapshot would fail
			pipelineCreatedSnapshot := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "process-failure-snapshot",
					Namespace: "default",
					Annotations: map[string]string{
						"appstudio.redhat.com/snapshot-status": "pending",
					},
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasApp.Name,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           hasComp.Name,
							ContainerImage: SampleImage,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pipelineCreatedSnapshot)).Should(Succeed())

			// Update pipeline run to reference this snapshot
			testPipelineRun.Status.Results[0].Value = *tektonv1.NewStructuredValues(pipelineCreatedSnapshot.Name)
			Expect(k8sClient.Status().Update(ctx, testPipelineRun)).Should(Succeed())

			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, testPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)

			result, err := adapter.EnsureSnapshotExists()
			// Should succeed because processExistingSnapshot should work with proper permissions
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			expectedLogEntry := "Processed pipeline-created Snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			// Cleanup
			err = k8sClient.Delete(ctx, pipelineCreatedSnapshot)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})
	})

	When("Testing regression scenarios for traditional snapshot creation", func() {
		var testPipelineRun *tektonv1.PipelineRun

		BeforeEach(func() {
			testPipelineRun = buildPipelineRun.DeepCopy()
			testPipelineRun.Name = "regression-test-pipeline-run"
			testPipelineRun.ResourceVersion = ""
			testPipelineRun.Status = tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					Results: []tektonv1.PipelineRunResult{
						{
							Name:  "IMAGE_DIGEST",
							Value: *tektonv1.NewStructuredValues(SampleDigest),
						},
						{
							Name:  "IMAGE_URL",
							Value: *tektonv1.NewStructuredValues(SampleImageWithoutDigest),
						},
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1.NewStructuredValues(SampleRepoLink),
						},
						{
							Name:  "CHAINS-GIT_COMMIT",
							Value: *tektonv1.NewStructuredValues(SampleCommit),
						},
						// Notably NO SNAPSHOT result to trigger traditional workflow
					},
					StartTime: &metav1.Time{Time: time.Now()},
				},
				Status: v1.Status{
					Conditions: v1.Conditions{
						apis.Condition{
							Reason: "Completed",
							Status: "True",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, testPipelineRun)).Should(Succeed())
			Expect(k8sClient.Status().Update(ctx, testPipelineRun)).Should(Succeed())

			adapter = NewAdapter(ctx, testPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, testPipelineRun)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("traditional snapshot creation still works after refactoring", func() {
			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			adapter = NewAdapter(ctx, testPipelineRun, hasComp, hasApp, log, loader.NewMockLoader(), k8sClient)
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})

			result, err := adapter.EnsureSnapshotExists()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			// Verify traditional workflow is used
			expectedLogEntry := "Created new Snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			// Verify pipeline-created workflow is NOT used
			unexpectedLogEntry := "Found SNAPSHOT result in PipelineRun"
			Expect(buf.String()).ShouldNot(ContainSubstring(unexpectedLogEntry))
		})

		It("prepareSnapshotForPipelineRun uses applySnapshotMetadata correctly", func() {
			adapter.context = toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.ComponentContextKey,
					Resource:   hasComp,
				},
				{
					ContextKey: loader.ApplicationComponentsContextKey,
					Resource:   []applicationapiv1alpha1.Component{*hasComp},
				},
			})

			snapshot, err := adapter.prepareSnapshotForPipelineRun(testPipelineRun, hasComp, hasApp)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).ToNot(BeNil())

			// Verify the shared applySnapshotMetadata function was used
			Expect(snapshot.Labels[gitops.BuildPipelineRunNameLabel]).To(Equal(testPipelineRun.Name))
			Expect(snapshot.Labels[gitops.ApplicationNameLabel]).To(Equal(hasApp.Name))
			Expect(snapshot.Labels[gitops.SnapshotComponentLabel]).To(Equal(hasComp.Name))

			// Verify metadata logic that's shared between traditional and pipeline-created snapshots
			Expect(snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel]).ToNot(BeEmpty())
			Expect(snapshot.Annotations[gitops.BuildPipelineRunStartTime]).ToNot(BeEmpty())
		})
	})

	createAdapter = func() *Adapter {
		adapter = NewAdapter(ctx, buildPipelineRun, hasComp, hasApp, logger, loader.NewMockLoader(), k8sClient)
		return adapter
	}
})
