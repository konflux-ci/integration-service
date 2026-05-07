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
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	"github.com/konflux-ci/integration-service/tekton"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"
	"github.com/konflux-ci/operator-toolkit/metadata"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tonglil/buflogr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	v1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// nestedGroupsLoader wraps an ObjectLoader and overrides GetNestedComponentGroupsForComponentGroup
// to return per-ComponentGroup results, allowing tests to control the nesting graph precisely.
type nestedGroupsLoader struct {
	loader.ObjectLoader
	nestedGroupsByParent map[string][]v1beta2.ComponentGroup
	errByParent          map[string]error
}

func (l *nestedGroupsLoader) GetNestedComponentGroupsForComponentGroup(_ context.Context, _ client.Client, cg *v1beta2.ComponentGroup) ([]v1beta2.ComponentGroup, error) {
	if err, ok := l.errByParent[cg.Name]; ok {
		return nil, err
	}
	groups := l.nestedGroupsByParent[cg.Name]
	return groups, nil
}

var _ = Describe("Snapshot creation functions", Ordered, func() {
	var (
		buildPipelineRun  *tektonv1.PipelineRun
		successfulTaskRun *tektonv1.TaskRun
		hasCompGroup      *v1beta2.ComponentGroup
		hasAppSample      *applicationapiv1alpha1.Application
		hasCompSample     *applicationapiv1alpha1.Component
		logger            helpers.IntegrationLogger
		mockCtx           context.Context
	)
	const (
		SampleRepoLink           = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		SampleCommit             = "a2ba645d50e471d5f084b"
		SampleDigest             = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		SampleImageWithoutDigest = "quay.io/redhat-appstudio/sample-image"
		customLabel              = "custom.appstudio.openshift.io/custom-label"
		componentName            = "component-sample"
		componentName2           = "another-component-sample"
	)

	BeforeAll(func() {
		hasAppSample = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "application-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasAppSample)).Should(Succeed())

		hasCompSample = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  componentName,
				Application:    "application-sample",
				ContainerImage: "invalidImage",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: SampleCommit,
							Context:  "rpms/my-component",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasCompSample)).Should(Succeed())

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
					"appstudio.redhat.com/updateComponentOnSuccess":    "false",
					"pipelinesascode.tekton.dev/on-target-branch":      "[main,master]",
					"build.appstudio.openshift.io/repo":                "https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0",
					"foo":                                              "bar",
					"chains.tekton.dev/signed":                         "true",
					"pipelinesascode.tekton.dev/source-branch":         "sourceBranch",
					"pipelinesascode.tekton.dev/url-org":               "redhat",
					tektonconsts.PipelineRunComponentVersionAnnotation: "v1",
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

		hasCompGroup = &v1beta2.ComponentGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-group-sample",
				Namespace: "default",
			},
			Spec: v1beta2.ComponentGroupSpec{
				Components: []v1beta2.ComponentReference{
					v1beta2.ComponentReference{
						Name: componentName,
						ComponentVersion: v1beta2.ComponentVersionReference{
							Name:     "v1",
							Revision: "main",
						},
					},
					v1beta2.ComponentReference{
						Name: componentName2,
						ComponentVersion: v1beta2.ComponentVersionReference{
							Name:     "v1",
							Revision: "main",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasCompGroup)).Should(Succeed())

		hasCompGroup.Status = v1beta2.ComponentGroupStatus{
			GlobalCandidateList: []v1beta2.ComponentState{
				v1beta2.ComponentState{
					Name:                  componentName,
					Version:               "v1",
					URL:                   SampleRepoLink,
					LastPromotedImage:     fmt.Sprintf("%s@%s", SampleImageWithoutDigest, SampleDigest),
					LastPromotedCommit:    SampleCommit,
					LastPromotedBuildTime: nil,
				},
				v1beta2.ComponentState{
					Name:    componentName2,
					Version: "v1",
					URL:     "",
				},
				v1beta2.ComponentState{
					Name:    "deleted-component",
					Version: "v1",
				},
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

		logger = helpers.IntegrationLogger{Logger: log.FromContext(ctx)}

		mockCtx = toolkit.GetMockedContext(ctx, []toolkit.MockData{
			{
				ContextKey: loader.NestedComponentGroupsContextKey,
				Resource:   []v1beta2.ComponentGroup{},
			},
		})
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, buildPipelineRun)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasCompGroup)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasCompSample)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasAppSample)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	Context("Testing PrepareSnapshotForPipelineRun()", func() {
		It("ensures built component includes git context from Component CR", func() {
			expectedSnapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, buildPipelineRun, componentName, hasCompGroup, loader.NewMockLoader())
			Expect(err).ToNot(HaveOccurred())
			Expect(expectedSnapshot).NotTo(BeNil())
			var built *applicationapiv1alpha1.SnapshotComponent
			for i := range expectedSnapshot.Spec.Components {
				if expectedSnapshot.Spec.Components[i].Name == componentName {
					built = &expectedSnapshot.Spec.Components[i]
					break
				}
			}
			Expect(built).NotTo(BeNil())
			Expect(built.Source.GitSource).NotTo(BeNil())
			Expect(built.Source.GitSource.Context).To(Equal("rpms/my-component"))
		})

		It("ensures that snapshot has label pointing to build pipelinerun", func() {
			expectedSnapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, buildPipelineRun, componentName, hasCompGroup, loader.NewMockLoader())
			Expect(err).ToNot(HaveOccurred())
			Expect(expectedSnapshot).NotTo(BeNil())

			Expect(expectedSnapshot.Labels).NotTo(BeNil())
			Expect(expectedSnapshot.Labels).Should(HaveKeyWithValue(Equal(gitops.BuildPipelineRunNameLabel), Equal(buildPipelineRun.Name)))
			Expect(expectedSnapshot.Labels).Should(HaveKeyWithValue(Equal(gitops.ComponentGroupNameLabel), Equal(hasCompGroup.Name)))

			// Verify BuildPipelineRunStartTime annotation uses millisecond precision
			Expect(metadata.HasAnnotation(expectedSnapshot, gitops.BuildPipelineRunStartTime)).To(BeTrue())
			Expect(expectedSnapshot.Annotations[gitops.BuildPipelineRunStartTime]).NotTo(BeNil())

			// Verify annotation value is in milliseconds (should be > 1000000000000 for recent timestamps)
			startTimeMillis, err := strconv.ParseInt(expectedSnapshot.Annotations[gitops.BuildPipelineRunStartTime], 10, 64)
			Expect(err).ToNot(HaveOccurred())
			Expect(startTimeMillis).To(BeNumerically(">", 1000000000000)) // Millisecond timestamp

			// Verify snapshot name has timestamp format: prefix-YYYYMMDD-HHMMSS-mmm
			Expect(expectedSnapshot.Name).To(MatchRegexp(`^component-group-sample-\d{8}-\d{6}-\d{3}$`))

			// Verify the name matches the BuildPipelineRunStartTime annotation
			expectedName := gitops.GenerateSnapshotNameWithTimestamp(hasCompGroup.Name, startTimeMillis)
			Expect(expectedSnapshot.Name).To(Equal(expectedName))
		})

		It("ensures snapshot name is set with fallback timestamp when StartTime is nil", func() {
			buildPipelineRunNoStartTime := buildPipelineRun.DeepCopy()
			buildPipelineRunNoStartTime.Status.StartTime = nil

			expectedSnapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, buildPipelineRunNoStartTime, componentName, hasCompGroup, loader.NewMockLoader())
			Expect(err).ToNot(HaveOccurred())
			Expect(expectedSnapshot).NotTo(BeNil())

			// Should still have a valid name with timestamp format
			Expect(expectedSnapshot.Name).To(MatchRegexp(`^component-group-sample-\d{8}-\d{6}-\d{3}$`))

			// Should have BuildPipelineRunStartTime annotation set (from fallback)
			Expect(metadata.HasAnnotation(expectedSnapshot, gitops.BuildPipelineRunStartTime)).To(BeTrue())

			// Verify annotation value is in milliseconds (from fallback)
			startTimeMillis, err := strconv.ParseInt(expectedSnapshot.Annotations[gitops.BuildPipelineRunStartTime], 10, 64)
			Expect(err).ToNot(HaveOccurred())
			Expect(startTimeMillis).To(BeNumerically(">", 1000000000000)) // Millisecond timestamp

			// Verify the name matches the fallback timestamp
			expectedName := gitops.GenerateSnapshotNameWithTimestamp(hasCompGroup.Name, startTimeMillis)
			Expect(expectedSnapshot.Name).To(Equal(expectedName))
		})

		It("ensures that Labels and Annotations were copied to snapshot from pipelinerun", func() {
			copyToSnapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, buildPipelineRun, componentName, hasCompGroup, loader.NewMockLoader())
			Expect(err).ToNot(HaveOccurred())
			Expect(copyToSnapshot).NotTo(BeNil())

			prefixes := []string{gitops.BuildPipelineRunPrefix, gitops.CustomLabelPrefix, gitops.TestLabelPrefix}
			gitops.CopySnapshotLabelsAndAnnotations(&hasCompGroup.ObjectMeta, copyToSnapshot, componentName, &buildPipelineRun.ObjectMeta, prefixes, false)
			Expect(copyToSnapshot.Labels[gitops.SnapshotTypeLabel]).To(Equal(gitops.SnapshotComponentType))
			Expect(copyToSnapshot.Labels[gitops.SnapshotComponentLabel]).To(Equal(componentName))
			Expect(copyToSnapshot.Labels[gitops.ComponentGroupNameLabel]).To(Equal(hasCompGroup.Name))
			Expect(copyToSnapshot.Labels["build.appstudio.redhat.com/target_branch"]).To(Equal("main"))
			Expect(copyToSnapshot.Annotations["build.appstudio.openshift.io/repo"]).To(Equal("https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0"))
			Expect(copyToSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel]).To(Equal(buildPipelineRun.Labels["pipelinesascode.tekton.dev/event-type"]))
			Expect(copyToSnapshot.Labels[customLabel]).To(Equal(buildPipelineRun.Labels[customLabel]))

		})

		It("ensures that snapshot has Pull request label based on the merge queue's temporary source branch extracted from build pipelinerun", func() {
			mergeQueueBuildPipelineRun := buildPipelineRun.DeepCopy()
			mergeQueueBuildPipelineRun.Annotations[tektonconsts.PipelineAsCodeSourceBranchAnnotation] = "gh-readonly-queue/main/pr-2987-bda9b312bf224a6b5fb1e7ed6ae76dd9e6b1b75b"
			mergeQueueBuildPipelineRun.Labels[tektonconsts.PipelineAsCodeEventTypeLabel] = "push"
			mergeQueueBuildPipelineRun.Labels[tektonconsts.PipelineAsCodePullRequestLabel] = ""
			mergeQueueBuildPipelineRun.Annotations[tektonconsts.PipelineAsCodePullRequestLabel] = ""
			mergeQueueBuildPipelineRun.Name = buildPipelineRun.Name + "-merge"
			expectedSnapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, mergeQueueBuildPipelineRun, componentName, hasCompGroup, loader.NewMockLoader())
			Expect(err).ToNot(HaveOccurred())
			Expect(expectedSnapshot).NotTo(BeNil())

			Expect(expectedSnapshot.Labels).NotTo(BeNil())
			Expect(expectedSnapshot.Labels).Should(HaveKeyWithValue(Equal(gitops.BuildPipelineRunNameLabel), Equal(mergeQueueBuildPipelineRun.Name)))
			Expect(expectedSnapshot.Labels).Should(HaveKeyWithValue(Equal(gitops.ComponentGroupNameLabel), Equal(hasCompGroup.Name)))
			Expect(expectedSnapshot.Labels).Should(HaveKeyWithValue(Equal(gitops.PipelineAsCodePullRequestAnnotation), Equal("2987")))
			Expect(expectedSnapshot.Annotations).Should(HaveKeyWithValue(Equal(gitops.PipelineAsCodePullRequestAnnotation), Equal("2987")))
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
			expectedSnapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, buildPipelineRunNoSource, componentName, hasCompGroup, loader.NewMockLoader())
			Expect(expectedSnapshot).To(BeNil())
			Expect(err).To(HaveOccurred())
			err = tekton.AnnotateBuildPipelineRunWithCreateSnapshotAnnotation(ctx, buildPipelineRun, k8sClient, err)
			Expect(err).NotTo(HaveOccurred())
			Expect(buildPipelineRun.GetAnnotations()[helpers.CreateSnapshotAnnotationName]).ToNot(BeNil())
			err = json.Unmarshal([]byte(buildPipelineRun.GetAnnotations()[helpers.CreateSnapshotAnnotationName]), &info)
			Expect(err).NotTo(HaveOccurred())
			Expect(info["status"]).To(Equal("failed"))
			Expect(info["message"]).To(Equal("Failed to create snapshot. Error: " + messageError))
		})

		It("ensures pipelines as code labels and annotations are propagated to the snapshot", func() {
			snapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, buildPipelineRun, componentName, hasCompGroup, loader.NewMockLoader())
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
			snapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, buildPipelineRun, componentName, hasCompGroup, loader.NewMockLoader())
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
			snapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, buildPipelineRun, componentName, hasCompGroup, loader.NewMockLoader())
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
			snapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, buildPipelineRun, componentName, hasCompGroup, loader.NewMockLoader())
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

		It("ensures integration workflow annotation is set to 'pull-request' for pr events", func() {
			// default buildPipelineRun already has event-type set to pull_request
			snapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, buildPipelineRun, componentName, hasCompGroup, loader.NewMockLoader())
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).ToNot(BeNil())

			annotation, found := snapshot.GetAnnotations()[gitops.IntegrationWorkflowAnnotation]
			Expect(found).To(BeTrue())
			Expect(annotation).To(Equal(gitops.IntegrationWorkflowPullRequestValue))
			Expect(annotation).To(Equal("pull-request"))
		})

		It("ensures integration workflow annotation is set to 'push' for push events", func() {
			// copy buildPipelineRun and modify it to be a push event
			pushPipelineRun := buildPipelineRun.DeepCopy()
			pushPipelineRun.Labels["pipelinesascode.tekton.dev/event-type"] = "push"
			delete(pushPipelineRun.Labels, "pipelinesascode.tekton.dev/pull-request")

			snapshot, err := PrepareSnapshotForPipelineRun(mockCtx, k8sClient, pushPipelineRun, componentName, hasCompGroup, loader.NewMockLoader())
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).ToNot(BeNil())

			annotation, found := snapshot.GetAnnotations()[gitops.IntegrationWorkflowAnnotation]
			Expect(found).To(BeTrue())
			Expect(annotation).To(Equal(gitops.IntegrationWorkflowPushValue))
			Expect(annotation).To(Equal("push"))
		})
	})

	Context("testing creation of snapshotComponentsList", func() {
		It("Ensures valid and invalid snapshotComponents can be gathered from the GCL", func() {
			var buf bytes.Buffer
			readableLog := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}
			snapshotComponents, invalidComponents := getSnapshotComponentsFromGCL(hasCompGroup, readableLog)

			Expect(snapshotComponents).To(HaveLen(1))
			Expect(snapshotComponents).To(HaveKey(helpers.GetComponentVersionString(componentName, "v1")))

			Expect(invalidComponents).To(HaveLen(1))
			var invalidName string
			for k := range invalidComponents {
				invalidName = k.Name
			}
			Expect(invalidName).To(Equal(componentName2))

			Expect(buf.String()).To(ContainSubstring("componentVersion was deleted from spec.Components"))

		})

		It("Ensures built component can replace existing snapshotComponent", func() {
			newSnapshotComponent, err := getSnapshotComponentFromBuildPLR(buildPipelineRun, componentName, logger)
			Expect(err).NotTo(HaveOccurred())

			snapshotComponents, invalidComponents := getSnapshotComponentsFromGCL(hasCompGroup, logger)
			Expect(snapshotComponents).To(HaveLen(1))
			Expect(invalidComponents).To(HaveLen(1))

			upsertNewComponentImage(snapshotComponents, invalidComponents, newSnapshotComponent, logger)

			// The upserted image should replace the old image (same name+version key)
			Expect(snapshotComponents).To(HaveLen(1))
			Expect(snapshotComponents[helpers.GetComponentVersionString(componentName, "v1")].Name).To(Equal(componentName))
			Expect(invalidComponents).To(HaveLen(1))

		})

		It("Ensures built component is added under its version key when an unversioned entry exists", func() {
			newSnapshotComponent, err := getSnapshotComponentFromBuildPLR(buildPipelineRun, componentName, logger)
			Expect(err).NotTo(HaveOccurred())

			snapshotComponents, invalidComponents := getSnapshotComponentsFromGCL(hasCompGroup, logger)
			Expect(snapshotComponents).To(HaveLen(1))
			Expect(invalidComponents).To(HaveLen(1))

			// Simulate a version-less existing entry by re-keying the map
			versionedKey := helpers.GetComponentVersionString(componentName, "v1")
			comp := snapshotComponents[versionedKey]
			comp.Version = ""
			delete(snapshotComponents, versionedKey)
			snapshotComponents[helpers.GetComponentVersionString(componentName, "")] = comp

			upsertNewComponentImage(snapshotComponents, invalidComponents, newSnapshotComponent, logger)

			// The new versioned entry is added; the unversioned entry remains under its own key
			Expect(snapshotComponents).To(HaveLen(2))
			Expect(snapshotComponents).To(HaveKey(helpers.GetComponentVersionString(componentName, "v1")))
			Expect(invalidComponents).To(HaveLen(1))
		})

		It("Ensures built component can also be removed from invalidComponents", func() {
			// replace this with data from another-component-sample
			newSnapshotComponent, err := getSnapshotComponentFromBuildPLR(buildPipelineRun, componentName, logger)
			Expect(err).NotTo(HaveOccurred())
			newSnapshotComponent.Name = componentName2

			snapshotComponents, invalidComponents := getSnapshotComponentsFromGCL(hasCompGroup, logger)
			Expect(snapshotComponents).To(HaveLen(1))
			Expect(invalidComponents).To(HaveLen(1))

			upsertNewComponentImage(snapshotComponents, invalidComponents, newSnapshotComponent, logger)

			// The upserted image should exist in addition to the existing component
			Expect(snapshotComponents).To(HaveLen(2))
			Expect(snapshotComponents).To(HaveKey(helpers.GetComponentVersionString(componentName, "v1")))
			Expect(snapshotComponents).To(HaveKey(helpers.GetComponentVersionString(componentName2, "v1")))
			Expect(invalidComponents).To(BeEmpty())
		})
	})

	It("Ensures a result with invalid image digest will return an error", func() {
		// TODO: update or delete depending on result of slack discussion
		Expect(true).To(BeTrue())
	})

	Context("Testing flattenSnapshotComponentsMap()", func() {
		It("returns an empty slice for an empty map", func() {
			result := flattenSnapshotComponentsMap(map[string]applicationapiv1alpha1.SnapshotComponent{})
			Expect(result).To(BeEmpty())
		})

		It("returns a slice with one entry for a single-element map", func() {
			comp := applicationapiv1alpha1.SnapshotComponent{
				Name:           componentName,
				ContainerImage: fmt.Sprintf("%s@%s", SampleImageWithoutDigest, SampleDigest),
			}
			result := flattenSnapshotComponentsMap(map[string]applicationapiv1alpha1.SnapshotComponent{
				helpers.GetComponentVersionString(componentName, "v1"): comp,
			})
			Expect(result).To(HaveLen(1))
			Expect(result).To(ContainElement(comp))
		})

		It("returns all entries for a multi-element map", func() {
			comp1 := applicationapiv1alpha1.SnapshotComponent{Name: componentName, ContainerImage: "image1"}
			comp2 := applicationapiv1alpha1.SnapshotComponent{Name: componentName2, ContainerImage: "image2"}
			result := flattenSnapshotComponentsMap(map[string]applicationapiv1alpha1.SnapshotComponent{
				helpers.GetComponentVersionString(componentName, "v1"):  comp1,
				helpers.GetComponentVersionString(componentName2, "v1"): comp2,
			})
			Expect(result).To(HaveLen(2))
			Expect(result).To(ContainElements(comp1, comp2))
		})
	})

	Context("Testing getNestedSnapshotComponents()", func() {
		It("returns GCL components from the componentGroup itself when there are no nested groups", func() {
			snapshotComponents, invalidComponents, err := getNestedSnapshotComponents(hasCompGroup, []string{}, 0, loader.NewMockLoader(), mockCtx, k8sClient, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshotComponents).To(HaveLen(1))
			Expect(snapshotComponents).To(HaveKey(helpers.GetComponentVersionString(componentName, "v1")))
			// componentName2 has no containerImage so it ends up as invalid
			Expect(invalidComponents).To(HaveLen(1))
		})

		It("returns an error when a cycle is detected in nested componentGroups", func() {
			_, _, err := getNestedSnapshotComponents(hasCompGroup, []string{hasCompGroup.Name}, 0, loader.NewMockLoader(), mockCtx, k8sClient, logger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle found"))
		})

		It("returns an error when the loader fails to get nested component groups", func() {
			loaderErr := fmt.Errorf("simulated loader error")
			errLoader := &nestedGroupsLoader{
				ObjectLoader:         loader.NewMockLoader(),
				nestedGroupsByParent: map[string][]v1beta2.ComponentGroup{},
				errByParent:          map[string]error{hasCompGroup.Name: loaderErr},
			}
			_, _, err := getNestedSnapshotComponents(hasCompGroup, []string{}, 0, errLoader, ctx, k8sClient, logger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated loader error"))
		})

		It("merges components from a nested componentGroup with the parent's own GCL", func() {
			validImage := fmt.Sprintf("%s@%s", SampleImageWithoutDigest, SampleDigest)
			nestedCG := &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nested-component-group",
					Namespace: "default",
				},
				Spec: v1beta2.ComponentGroupSpec{
					Components: []v1beta2.ComponentReference{
						{
							Name:             "nested-component",
							ComponentVersion: v1beta2.ComponentVersionReference{Name: "v1"},
						},
					},
				},
				Status: v1beta2.ComponentGroupStatus{
					GlobalCandidateList: []v1beta2.ComponentState{
						{
							Name:               "nested-component",
							Version:            "v1",
							URL:                SampleRepoLink,
							LastPromotedImage:  validImage,
							LastPromotedCommit: SampleCommit,
						},
					},
				},
			}
			nestedLoader := &nestedGroupsLoader{
				ObjectLoader: loader.NewMockLoader(),
				nestedGroupsByParent: map[string][]v1beta2.ComponentGroup{
					hasCompGroup.Name: {*nestedCG},
					nestedCG.Name:     {},
				},
				errByParent: map[string]error{},
			}

			snapshotComponents, _, err := getNestedSnapshotComponents(hasCompGroup, []string{}, 0, nestedLoader, ctx, k8sClient, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshotComponents).To(HaveKey(helpers.GetComponentVersionString(componentName, "v1")))
			Expect(snapshotComponents).To(HaveKey(helpers.GetComponentVersionString("nested-component", "v1")))
		})

		It("parent componentGroup GCL overwrites duplicate entries from nested componentGroups", func() {
			childImage := fmt.Sprintf("quay.io/child-image@%s", SampleDigest)
			parentImage := fmt.Sprintf("%s@%s", SampleImageWithoutDigest, SampleDigest)
			nestedCG := &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nested-component-group-conflict",
					Namespace: "default",
				},
				Spec: v1beta2.ComponentGroupSpec{
					Components: []v1beta2.ComponentReference{
						{
							Name:             componentName,
							ComponentVersion: v1beta2.ComponentVersionReference{Name: "v1"},
						},
					},
				},
				Status: v1beta2.ComponentGroupStatus{
					GlobalCandidateList: []v1beta2.ComponentState{
						{
							Name:               componentName,
							Version:            "v1",
							URL:                SampleRepoLink,
							LastPromotedImage:  childImage,
							LastPromotedCommit: SampleCommit,
						},
					},
				},
			}
			nestedLoader := &nestedGroupsLoader{
				ObjectLoader: loader.NewMockLoader(),
				nestedGroupsByParent: map[string][]v1beta2.ComponentGroup{
					hasCompGroup.Name: {*nestedCG},
					nestedCG.Name:     {},
				},
				errByParent: map[string]error{},
			}

			snapshotComponents, _, err := getNestedSnapshotComponents(hasCompGroup, []string{}, 0, nestedLoader, ctx, k8sClient, logger)
			Expect(err).ToNot(HaveOccurred())
			key := helpers.GetComponentVersionString(componentName, "v1")
			Expect(snapshotComponents).To(HaveKey(key))
			Expect(snapshotComponents[key].ContainerImage).To(Equal(parentImage))
		})
	})

	Context("Testing PrepareSnapshot()", func() {
		It("ensures the Imagepullspec and ComponentSource from pipelinerun and prepare snapshot can be created", func() {
			newSnapshotComponent, err := getSnapshotComponentFromBuildPLR(buildPipelineRun, componentName, logger)
			Expect(err).NotTo(HaveOccurred())

			snapshot, err := PrepareSnapshot(mockCtx, k8sClient, hasCompGroup, newSnapshotComponent, loader.NewMockLoader(), logger)
			Expect(snapshot).NotTo(BeNil())
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot.Spec.Components).To(HaveLen(1), "One component should have been added to snapshot.  Other component should have been omited due to empty ContainerImage field or missing valid digest")
			Expect(snapshot.Spec.Components[0].Name).To(Equal(componentName), "The built component should have been added to the snapshot")
			Expect(snapshot.Annotations[helpers.CreateSnapshotAnnotationName]).To(Equal("Component(s) 'another-component-sample (version v1)' is(are) not included in snapshot due to missing valid containerImage or git source"))
		})
	})

	Context("Testing upsertMultipleComponentImages()", func() {
		var (
			existingSnapshotComponents    map[string]applicationapiv1alpha1.SnapshotComponent
			expectedNewSnapshotComponents map[string]applicationapiv1alpha1.SnapshotComponent
			componentsToInsert            []applicationapiv1alpha1.SnapshotComponent
		)
		BeforeAll(func() {
			existingSnapshotComponents = make(map[string]applicationapiv1alpha1.SnapshotComponent)
			existingSnapshotComponents["component1/main"] = applicationapiv1alpha1.SnapshotComponent{
				Name:           "component1",
				Version:        "main",
				ContainerImage: "quay.io/konflux/component1-main:old",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: SampleCommit,
						},
					},
				},
			}

			newSnapshotComponent1 := applicationapiv1alpha1.SnapshotComponent{
				Name:           "component1",
				Version:        "main",
				ContainerImage: "quay.io/konflux/component1-main@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: SampleCommit,
						},
					},
				},
			}
			newSnapshotComponent2 := applicationapiv1alpha1.SnapshotComponent{
				Name:           "component2",
				Version:        "main",
				ContainerImage: "quay.io/konflux/component2-main@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL:      SampleRepoLink,
							Revision: SampleCommit,
						},
					},
				},
			}
			componentsToInsert = []applicationapiv1alpha1.SnapshotComponent{newSnapshotComponent1, newSnapshotComponent2}

			expectedNewSnapshotComponents = make(map[string]applicationapiv1alpha1.SnapshotComponent)
			expectedNewSnapshotComponents["component1/main"] = newSnapshotComponent1
			expectedNewSnapshotComponents["component2/main"] = newSnapshotComponent2
		})

		It("Can upsert new componentVersions", func() {
			invalidComponents := make(map[v1beta2.ComponentState]InvalidComponentReason)
			upsertMultipleComponentImages(existingSnapshotComponents, invalidComponents, componentsToInsert, logger)
			Expect(existingSnapshotComponents).To(Equal(expectedNewSnapshotComponents))
			Expect(invalidComponents).To(BeEmpty())
		})

		It("Can upsert new componentVersions and replace invalid components", func() {
			invalidComponents := make(map[v1beta2.ComponentState]InvalidComponentReason)
			invalidComponents[v1beta2.ComponentState{
				Name:    "component2",
				Version: "main",
			}] = InvalidComponentReason{
				ComponentGroup: "",
				Reason:         "invalid digest in containerImage",
			}
			upsertMultipleComponentImages(existingSnapshotComponents, invalidComponents, componentsToInsert, logger)
			Expect(existingSnapshotComponents).To(Equal(expectedNewSnapshotComponents))
			Expect(invalidComponents).To(BeEmpty())
		})
	})

	Context("Testing PrepareParentSnapshot()", func() {
		var (
			childComponentSnapshot *applicationapiv1alpha1.Snapshot
			childImageURL          string
		)

		BeforeEach(func() {
			childImageURL = fmt.Sprintf("quay.io/child-image@%s", SampleDigest)
			childComponentSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "child-component-snapshot",
					Namespace: "default",
					Labels: map[string]string{
						gitops.SnapshotTypeLabel:      gitops.SnapshotComponentType,
						gitops.SnapshotComponentLabel: componentName,
					},
					Annotations: map[string]string{
						tektonconsts.PipelineRunComponentVersionAnnotation: "v1",
					},
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasAppSample.Name,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           componentName,
							Version:        "v1",
							ContainerImage: childImageURL,
							Source: applicationapiv1alpha1.ComponentSource{
								ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
									GitSource: &applicationapiv1alpha1.GitSource{
										URL:      SampleRepoLink,
										Revision: SampleCommit,
									},
								},
							},
						},
					},
				},
			}
		})

		It("returns a snapshot with the child component image overriding the GCL entry", func() {
			snapshot, err := PrepareParentSnapshot(mockCtx, k8sClient, loader.NewMockLoader(), logger, hasCompGroup, childComponentSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).NotTo(BeNil())

			var found *applicationapiv1alpha1.SnapshotComponent
			for i := range snapshot.Spec.Components {
				if snapshot.Spec.Components[i].Name == componentName {
					found = &snapshot.Spec.Components[i]
					break
				}
			}
			Expect(found).NotTo(BeNil())
			Expect(found.ContainerImage).To(Equal(childImageURL))
		})

		It("sets the SnapshotGitSourceRepoURLAnnotation from the child snapshot component's git source", func() {
			snapshot, err := PrepareParentSnapshot(mockCtx, k8sClient, loader.NewMockLoader(), logger, hasCompGroup, childComponentSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).NotTo(BeNil())
			Expect(snapshot.Annotations).To(HaveKeyWithValue(gitops.SnapshotGitSourceRepoURLAnnotation, SampleRepoLink))
		})

		It("sets the invalid component warning annotation when some GCL components remain invalid", func() {
			snapshot, err := PrepareParentSnapshot(mockCtx, k8sClient, loader.NewMockLoader(), logger, hasCompGroup, childComponentSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).NotTo(BeNil())
			// componentName2 has an empty containerImage in the GCL and is not provided by the child snapshot
			Expect(snapshot.Annotations).To(HaveKey(helpers.CreateSnapshotAnnotationName))
			Expect(snapshot.Annotations[helpers.CreateSnapshotAnnotationName]).To(ContainSubstring(componentName2))
		})

		It("sets the controller reference on the snapshot to the componentGroup", func() {
			snapshot, err := PrepareParentSnapshot(mockCtx, k8sClient, loader.NewMockLoader(), logger, hasCompGroup, childComponentSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapshot).NotTo(BeNil())
			ownerRef := metav1.GetControllerOf(snapshot)
			Expect(ownerRef).NotTo(BeNil())
			Expect(ownerRef.Name).To(Equal(hasCompGroup.Name))
		})

		It("returns a wrapped error when the loader fails to get nested component groups", func() {
			loaderErr := fmt.Errorf("simulated loader error")
			errLoader := &nestedGroupsLoader{
				ObjectLoader:         loader.NewMockLoader(),
				nestedGroupsByParent: map[string][]v1beta2.ComponentGroup{},
				errByParent:          map[string]error{hasCompGroup.Name: loaderErr},
			}
			snapshot, err := PrepareParentSnapshot(ctx, k8sClient, errLoader, logger, hasCompGroup, childComponentSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(snapshot).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("error getting nested snapshot components:"))
			Expect(err.Error()).To(ContainSubstring("simulated loader error"))
		})

		It("returns a MissingValidComponentError when no valid components remain after merging", func() {
			// A group whose entire GCL has no valid images, and a child snapshot that contributes nothing
			allInvalidGroup := &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "all-invalid-prep-group",
					Namespace: "default",
				},
				Spec: v1beta2.ComponentGroupSpec{
					Components: []v1beta2.ComponentReference{
						{
							Name:             componentName2,
							ComponentVersion: v1beta2.ComponentVersionReference{Name: "v1"},
						},
					},
				},
				Status: v1beta2.ComponentGroupStatus{
					GlobalCandidateList: []v1beta2.ComponentState{
						// empty LastPromotedImage → invalid, omitted from snapshot
						{Name: componentName2, Version: "v1"},
					},
				},
			}

			// Child snapshot is not a component/group/override type, so GetAllNewComponentsInSnapshot returns empty
			neutralChildSnapshot := &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "neutral-child-snapshot",
					Namespace: "default",
				},
				Spec: applicationapiv1alpha1.SnapshotSpec{
					Application: hasAppSample.Name,
				},
			}

			snapshot, err := PrepareParentSnapshot(mockCtx, k8sClient, loader.NewMockLoader(), logger, allInvalidGroup, neutralChildSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(snapshot).To(BeNil())
			Expect(helpers.IsMissingValidComponentError(err)).To(BeTrue())
		})
	})
})
