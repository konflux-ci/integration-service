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
	"errors"
	"reflect"

	toolkit "github.com/konflux-ci/operator-toolkit/loader"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	oldapplicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	konfluxv1beta2 "github.com/konflux-ci/integration-service/api/konflux/v1beta2"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	releasev1alpha1 "github.com/konflux-ci/release-service/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resolutionv1beta1 "github.com/tektoncd/pipeline/pkg/apis/resolution/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

var _ = Describe("Release Adapter", Ordered, func() {
	var (
		loader ObjectLoader
	)

	BeforeAll(func() {
		loader = NewMockLoader()
	})

	Context("When calling GetReleasesWithSnapshot", func() {
		It("returns resource and error from the context", func() {
			release := &releasev1alpha1.Release{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ReleaseContextKey,
					Resource:   release,
				},
			})
			resource, err := loader.GetReleasesWithSnapshot(mockContext, nil, nil)
			Expect(resource).To(Equal(&[]releasev1alpha1.Release{*release}))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllApplicationComponents [APPLICATION]", func() {
		It("returns resource and error from the context", func() {
			applicationComponents := []oldapplicationapiv1alpha1.Component{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ApplicationComponentsContextKey,
					Resource:   applicationComponents,
				},
			})
			resource, err := loader.GetAllApplicationComponents(mockContext, nil, nil)
			Expect(resource).To(Equal(&applicationComponents))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllComponentsInNamespace", func() {
		It("returns resource and error from the context", func() {
			namespaceComponents := []oldapplicationapiv1alpha1.Component{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: NamespaceComponentsContextKey,
					Resource:   namespaceComponents,
				},
			})
			resource, err := loader.GetAllComponentsInNamespace(mockContext, nil, "default")
			Expect(resource).To(Equal(&namespaceComponents))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetApplicationFromSnapshot [APPLICATION]", func() {
		It("returns resource and error from the context", func() {
			application := &oldapplicationapiv1alpha1.Application{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ApplicationContextKey,
					Resource:   application,
				},
			})
			resource, err := loader.GetApplicationFromSnapshot(mockContext, nil, nil)
			Expect(resource).To(Equal(application))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetComponentGroupFromSnapshot", func() {
		It("returns resource and error from the context", func() {
			componentGroup := &konfluxv1beta2.ComponentGroup{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ComponentGroupContextKey,
					Resource:   componentGroup,
				},
			})
			resource, err := loader.GetComponentGroupFromSnapshot(mockContext, nil, nil)
			Expect(resource).To(Equal(componentGroup))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetComponentFromSnapshot", func() {
		It("returns resource and error from the context", func() {
			component := &oldapplicationapiv1alpha1.Component{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ComponentContextKey,
					Resource:   component,
				},
			})
			resource, err := loader.GetComponentFromSnapshot(mockContext, nil, nil)
			Expect(resource).To(Equal(component))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetComponentFromPipelineRun", func() {
		It("returns resource and error from the context", func() {
			component := &oldapplicationapiv1alpha1.Component{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ComponentContextKey,
					Resource:   component,
				},
			})
			resource, err := loader.GetComponentFromPipelineRun(mockContext, nil, nil)
			Expect(resource).To(Equal(component))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetApplicationFromPipelineRun", func() {
		It("returns resource and error from the context", func() {
			application := &oldapplicationapiv1alpha1.Application{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ApplicationContextKey,
					Resource:   application,
				},
			})
			resource, err := loader.GetApplicationFromPipelineRun(mockContext, nil, nil)
			Expect(resource).To(Equal(application))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetApplicationFromComponent", func() {
		It("returns resource and error from the context", func() {
			application := &oldapplicationapiv1alpha1.Application{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ApplicationContextKey,
					Resource:   application,
				},
			})
			resource, err := loader.GetApplicationFromComponent(mockContext, nil, nil)
			Expect(resource).To(Equal(application))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetSnapshotFromPipelineRun", func() {
		It("returns resource and error from the context", func() {
			snapshot := &oldapplicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: SnapshotContextKey,
					Resource:   snapshot,
				},
			})
			resource, err := loader.GetSnapshotFromPipelineRun(mockContext, nil, nil)
			Expect(resource).To(Equal(snapshot))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllSnapshotsForBuildPipelineRun [APPLICATION]", func() {
		It("returns resource and error from the context", func() {
			snapshots := []oldapplicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllSnapshotsForBuildPipelineRunApplicationContextKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetAllSnapshotsForBuildPipelineRunApplication(mockContext, nil, nil)
			Expect(resource).To(Equal(&snapshots))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllSnapshotsForBuildPipelineRun", func() {
		It("returns resource and error from the context", func() {
			mappedSnapshots := map[string][]oldapplicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllSnapshotsForBuildPipelineRunContextKey,
					Resource:   mappedSnapshots,
				},
			})
			resource, err := loader.GetAllSnapshotsForBuildPipelineRun(mockContext, nil, nil, nil)
			Expect(resource).To(Equal(&mappedSnapshots))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllIntegrationTestScenariosForApplication", func() {
		It("returns all integrationTestScenario and error from the context", func() {
			scenarios := []v1beta2.IntegrationTestScenario{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllIntegrationTestScenariosContextKey,
					Resource:   scenarios,
				},
			})
			resource, err := loader.GetAllIntegrationTestScenariosForApplication(mockContext, nil, nil)
			Expect(resource).To(Equal(&scenarios))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllIntegrationTestScenariosForComponentGroup", func() {
		It("returns all integrationTestScenario and error from the context", func() {
			scenarios := []v1beta2.IntegrationTestScenario{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllIntegrationTestScenariosForComponentGroupContextKey,
					Resource:   scenarios,
				},
			})
			resource, err := loader.GetAllIntegrationTestScenariosForComponentGroup(mockContext, nil, nil)
			Expect(resource).To(Equal(&scenarios))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllIntegrationTestScenariosForComponentGroups", func() {
		It("returns all integrationTestScenario and error from the context", func() {
			scenarios := []v1beta2.IntegrationTestScenario{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllIntegrationTestScenariosForComponentGroupContextKey,
					Resource:   scenarios,
				},
				{
					ContextKey: AllIntegrationTestScenariosForComponentGroupsContextKey,
					Resource:   scenarios,
				},
			})
			resource, err := loader.GetAllIntegrationTestScenariosForComponentGroups(mockContext, nil, nil)
			Expect(resource).To(Equal(&scenarios))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetRequiredIntegrationTestScenariosForSnapshotApplication", func() {
		It("returns required integrationTestScenario and error from the context", func() {
			scenarios := []v1beta2.IntegrationTestScenario{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   scenarios,
				},
			})
			resource, err := loader.GetRequiredIntegrationTestScenariosForSnapshotApplication(mockContext, nil, nil, nil)
			Expect(resource).To(Equal(&scenarios))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllIntegrationTestScenariosForSnapshotApplication", func() {
		It("returns all integrationTestScenario and error from the context", func() {
			scenarios := []v1beta2.IntegrationTestScenario{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllIntegrationTestScenariosForSnapshotContextKey,
					Resource:   scenarios,
				},
			})
			resource, err := loader.GetAllIntegrationTestScenariosForSnapshotApplication(mockContext, nil, nil, nil)
			Expect(resource).To(Equal(&scenarios))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllIntegrationTestScenariosForSnapshot", func() {
		It("returns all integrationTestScenario and error from the context", func() {
			scenarios := []v1beta2.IntegrationTestScenario{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllIntegrationTestScenariosForSnapshotContextKey,
					Resource:   scenarios,
				},
			})
			resource, err := loader.GetAllIntegrationTestScenariosForSnapshot(mockContext, nil, nil, nil)
			Expect(resource).To(Equal(&scenarios))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetRequiredIntegrationTestScenariosForSnapshot", func() {
		It("returns required integrationTestScenario and error from the context", func() {
			scenarios := []v1beta2.IntegrationTestScenario{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: RequiredIntegrationTestScenariosForSnapshotContextKey,
					Resource:   scenarios,
				},
			})
			resource, err := loader.GetRequiredIntegrationTestScenariosForSnapshot(mockContext, nil, nil, nil)
			Expect(resource).To(Equal(&scenarios))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllPipelineRunsForSnapshotAndScenario", func() {
		It("returns pipelineRuns and error from the context", func() {
			prs := []tektonv1.PipelineRun{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: PipelineRunsContextKey,
					Resource:   prs,
				},
			})
			resource, err := loader.GetAllPipelineRunsForSnapshotAndScenario(mockContext, nil, nil, nil)
			Expect(resource).To(Equal(&prs))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllSnapshots", func() {
		It("returns snapshots and error from the context", func() {
			snapshots := []oldapplicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllSnapshotsContextKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetAllSnapshots(mockContext, nil, nil)
			Expect(resource).To(Equal(&snapshots))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAutoReleasePlansForApplication [APPLICATION]", func() {
		It("returns snapshots and error from the context", func() {
			releasePlans := []releasev1alpha1.ReleasePlan{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AutoReleasePlansContextKey,
					Resource:   releasePlans,
				},
			})
			resource, err := loader.GetAutoReleasePlansForApplication(mockContext, nil, nil, nil, true)
			Expect(resource).To(Equal(&releasePlans))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAutoReleasePlansForComponentGroup", func() {
		It("returns snapshots and error from the context", func() {
			releasePlans := []releasev1alpha1.ReleasePlan{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AutoReleasePlansContextKey,
					Resource:   releasePlans,
				},
			})
			resource, err := loader.GetAutoReleasePlansForApplication(mockContext, nil, nil, nil, true)
			Expect(resource).To(Equal(&releasePlans))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetScenario", func() {
		It("returns the integrationTestScenario and error from the context", func() {
			scenario := &v1beta2.IntegrationTestScenario{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetScenarioContextKey,
					Resource:   scenario,
				},
			})
			resource, err := loader.GetScenario(mockContext, nil, "scenario", "namespace")
			Expect(resource).To(Equal(scenario))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllTaskRunsWithMatchingPipelineRunLabel", func() {
		It("returns TaskRuns and error from the context", func() {
			taskRuns := []tektonv1.TaskRun{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   taskRuns,
				},
			})
			resource, err := loader.GetAllTaskRunsWithMatchingPipelineRunLabel(mockContext, nil, nil)
			Expect(resource).To(Equal(&taskRuns))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetTaskRun", func() {
		It("returns the TaskRun and error from the context", func() {
			taskRun := &tektonv1.TaskRun{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: TaskRunContextKey,
					Resource:   taskRun,
				},
			})
			resource, err := loader.GetTaskRun(mockContext, nil, "", "")
			Expect(resource).To(Equal(taskRun))
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the TaskRun matching the requested name", func() {
			first := &tektonv1.TaskRun{ObjectMeta: metav1.ObjectMeta{Name: "task-a", Namespace: "default"}}
			second := &tektonv1.TaskRun{ObjectMeta: metav1.ObjectMeta{Name: "task-b", Namespace: "default"}}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: TaskRunContextKey,
					Resource:   []*tektonv1.TaskRun{first, second},
				},
			})

			resource, err := loader.GetTaskRun(mockContext, nil, second.Name, second.Namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(resource).To(BeIdenticalTo(second))

			resource, err = loader.GetTaskRun(mockContext, nil, "absent-taskrun", "default")
			Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			Expect(resource).To(BeNil())
		})

		It("returns an error when the context has neither a TaskRun nor an error", func() {
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: TaskRunContextKey,
				},
			})
			resource, err := loader.GetTaskRun(mockContext, nil, "absent-taskrun", "default")
			Expect(resource).To(BeNil())
			Expect(err).To(MatchError(`TaskRun context for "absent-taskrun" has no resource or error`))
		})

		It("returns the error from the context when no TaskRun is set", func() {
			readErr := errors.New("taskrun read failed")
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: TaskRunContextKey,
					Err:        readErr,
				},
			})
			resource, err := loader.GetTaskRun(mockContext, nil, "absent-taskrun", "default")
			Expect(resource).To(BeNil())
			Expect(err).To(MatchError(readErr))
		})
	})

	Context("When calling GetTaskRunsReferencedByPipelineRun", func() {
		pipelineRunWithChildren := func(children ...tektonv1.ChildStatusReference) *tektonv1.PipelineRun {
			return &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "referenced-plr",
					Namespace: "default",
				},
				Status: tektonv1.PipelineRunStatus{
					PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
						ChildReferences: children,
					},
				},
			}
		}

		It("uses the TaskRuns from the context and ignores CustomRuns and extra TaskRuns", func() {
			cached := tektonv1.TaskRun{ObjectMeta: metav1.ObjectMeta{Name: "cached-taskrun"}}
			extra := tektonv1.TaskRun{ObjectMeta: metav1.ObjectMeta{Name: "retry-taskrun"}}
			customRun := tektonv1.ChildStatusReference{
				Name:             "custom-run",
				PipelineTaskName: "custom-task",
			}
			customRun.Kind = "CustomRun"
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{cached, extra},
				},
			})

			resolved, missing, err := loader.GetTaskRunsReferencedByPipelineRun(mockContext, nil, pipelineRunWithChildren(
				tektonv1.ChildStatusReference{Name: cached.Name, PipelineTaskName: "task1"},
				customRun,
			))
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(HaveKey(cached.Name))
			Expect(resolved).NotTo(HaveKey(extra.Name))
			Expect(resolved).NotTo(HaveKey(customRun.Name))
			Expect(missing).To(BeEmpty())
		})

		It("uses the TaskRun from the context when the name is missing from the list", func() {
			live := &tektonv1.TaskRun{ObjectMeta: metav1.ObjectMeta{Name: "live-taskrun"}}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{},
				},
				{
					ContextKey: TaskRunContextKey,
					Resource:   live,
				},
			})

			resolved, missing, err := loader.GetTaskRunsReferencedByPipelineRun(mockContext, nil, pipelineRunWithChildren(
				tektonv1.ChildStatusReference{Name: "live-taskrun", PipelineTaskName: "task1"},
			))
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(HaveKeyWithValue("live-taskrun", live))
			Expect(missing).To(BeEmpty())
		})

		It("resolves each missing TaskRun by name", func() {
			first := &tektonv1.TaskRun{ObjectMeta: metav1.ObjectMeta{Name: "task-a"}}
			second := &tektonv1.TaskRun{ObjectMeta: metav1.ObjectMeta{Name: "task-b"}}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{},
				},
				{
					ContextKey: TaskRunContextKey,
					Resource:   []*tektonv1.TaskRun{first, second},
				},
			})

			resolved, missing, err := loader.GetTaskRunsReferencedByPipelineRun(mockContext, nil, pipelineRunWithChildren(
				tektonv1.ChildStatusReference{Name: first.Name, PipelineTaskName: "task1"},
				tektonv1.ChildStatusReference{Name: second.Name, PipelineTaskName: "task2"},
				tektonv1.ChildStatusReference{Name: "absent-taskrun", PipelineTaskName: "task3"},
			))
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(HaveKeyWithValue(first.Name, first))
			Expect(resolved).To(HaveKeyWithValue(second.Name, second))
			Expect(missing).To(Equal([]string{"absent-taskrun"}))
		})

		It("returns the list error from the context", func() {
			listErr := errors.New("taskrun list failed")
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Err:        listErr,
				},
			})

			resolved, missing, err := loader.GetTaskRunsReferencedByPipelineRun(mockContext, nil, &tektonv1.PipelineRun{})
			Expect(err).To(MatchError(listErr))
			Expect(resolved).To(BeNil())
			Expect(missing).To(BeNil())
		})

		It("returns the GetTaskRun error from the context", func() {
			readErr := errors.New("taskrun read failed")
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllTaskRunsWithMatchingPipelineRunLabelContextKey,
					Resource:   []tektonv1.TaskRun{},
				},
				{
					ContextKey: TaskRunContextKey,
					Err:        readErr,
				},
			})

			resolved, missing, err := loader.GetTaskRunsReferencedByPipelineRun(mockContext, nil, pipelineRunWithChildren(
				tektonv1.ChildStatusReference{Name: "live-taskrun", PipelineTaskName: "task1"},
			))
			Expect(err).To(MatchError(readErr))
			Expect(resolved).To(BeNil())
			Expect(missing).To(BeNil())
		})
	})

	Context("When calling GetPipelineRun", func() {
		It("returns resource and error from the context", func() {
			pipelineRun := &tektonv1.PipelineRun{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetPipelineRunContextKey,
					Resource:   pipelineRun,
				},
			})
			resource, err := loader.GetPipelineRun(mockContext, nil, "", "")
			Expect(resource).To(Equal(pipelineRun))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetComponent", func() {
		It("returns resource and error from the context", func() {
			component := &oldapplicationapiv1alpha1.Component{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetComponentContextKey,
					Resource:   component,
				},
			})
			resource, err := loader.GetComponent(mockContext, nil, "", "")
			Expect(resource).To(Equal(component))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetPipelineRunsWithPRGroupHash", func() {
		It("returns resource and error from the context", func() {
			plrs := []tektonv1.PipelineRun{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetBuildPLRContextKey,
					Resource:   plrs,
				},
			})
			resource, err := loader.GetPipelineRunsWithPRGroupHash(mockContext, nil, "", "")
			Expect(resource).To(Equal(&plrs))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetPipelineRunsWithPRGroupHashForApplication", func() {
		It("returns resource and error from the context", func() {
			plrs := []tektonv1.PipelineRun{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetBuildPLRContextKey,
					Resource:   plrs,
				},
			})
			resource, err := loader.GetPipelineRunsWithPRGroupHashForApplication(mockContext, nil, "", "", "")
			Expect(resource).To(Equal(&plrs))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetMatchingComponentSnapshotsForComponentAndPRGroupHash", func() {
		It("returns resource and error from the context", func() {
			snapshots := []oldapplicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetComponentSnapshotsKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetMatchingComponentSnapshotsForComponentAndPRGroupHash(mockContext, nil, "", "", "", "", "", "")
			Expect(resource).To(Equal(&snapshots))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetMatchingComponentSnapshotsForPRGroupHash", func() {
		It("returns resource and error from the context", func() {
			snapshots := []oldapplicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetPRSnapshotsKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetMatchingComponentSnapshotsForPRGroupHash(mockContext, nil, "", "", "", "")
			Expect(resource).To(Equal(&snapshots))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetComponentsFromSnapshotForPRGroup", func() {
		It("return resource and error from the context", func() {
			components := []Tuple{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetComponentsFromSnapshotForPRGroupKey,
					Resource:   components,
				},
			})
			resource, err := loader.GetComponentsFromSnapshotForPRGroup(mockContext, nil, "", "", "", "")
			Expect(reflect.DeepEqual(components, resource)).To(BeTrue())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetMatchingGroupSnapshotsForPRGroupHash", func() {
		It("return resource and error from the context", func() {
			snapshots := []oldapplicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetGroupSnapshotsKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetMatchingGroupSnapshotsForPRGroupHash(mockContext, nil, "", "", "", "")
			Expect(resource).To(Equal(&snapshots))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllIntegrationPipelineRunsForSnapshot", func() {
		It("return resource and error from the context", func() {
			plrs := []tektonv1.PipelineRun{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetPipelineRunforSnapshotsKey,
					Resource:   plrs,
				},
			})
			resource, err := loader.GetAllIntegrationPipelineRunsForSnapshot(mockContext, nil, nil)
			Expect(resource).To(Equal(plrs))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetResolutionRequest", func() {
		It("return resource and error from the context", func() {
			rr := resolutionv1beta1.ResolutionRequest{
				Status: resolutionv1beta1.ResolutionRequestStatus{
					ResolutionRequestStatusFields: resolutionv1beta1.ResolutionRequestStatusFields{
						Data: "SomeMockData",
					},
				},
			}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ResolutionRequestContextKey,
					Resource:   rr,
				},
			})
			resource, err := loader.GetResolutionRequest(mockContext, nil, "", "")
			Expect(resource).To(Equal(rr))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetPRComponentSnapshotsForComponent", func() {
		It("returns resource and error from the context", func() {
			snapshots := []oldapplicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetPRComponentSnapshotsForComponentContextKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetPRComponentSnapshotsForComponent(mockContext, nil, []string{""}, "", "", "")
			Expect(resource).To(Equal(&snapshots))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllComponentGroupsInNamespace", func() {
		It("returns resource and error from the context", func() {
			componentGroups := []konfluxv1beta2.ComponentGroup{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ComponentGroupsContextKey,
					Resource:   componentGroups,
				},
			})
			resource, err := loader.GetAllComponentGroupsInNamespace(mockContext, nil, "")
			Expect(resource).To(Equal(componentGroups))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetComponentGroupsContainingComponentGroup", func() {
		It("returns resource and error from the context", func() {
			componentGroups := []konfluxv1beta2.ComponentGroup{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ComponentGroupsContextKey,
					Resource:   componentGroups,
				},
			})
			resource, err := loader.GetComponentGroupsContainingComponentGroup(mockContext, nil, nil)
			Expect(resource).To(Equal(componentGroups))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetNestedComponentGroupsForComponentGroup", func() {
		It("returns resource and error from the context", func() {
			componentGroups := []konfluxv1beta2.ComponentGroup{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: NestedComponentGroupsContextKey,
					Resource:   componentGroups,
				},
			})
			resource, err := loader.GetNestedComponentGroupsForComponentGroup(mockContext, nil, nil)
			Expect(resource).To(Equal(componentGroups))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetSecret", func() {
		It("returns resource and error from the context", func() {
			secret := &corev1.Secret{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: SecretContextKey,
					Resource:   secret,
				},
			})
			resource, err := loader.GetSecret(mockContext, nil, "", "")
			Expect(resource).To(Equal(secret))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetServiceAccount", func() {
		It("returns resource and error from the context", func() {
			serviceAccount := &corev1.ServiceAccount{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: ServiceAccountContextKey,
					Resource:   serviceAccount,
				},
			})
			resource, err := loader.GetServiceAccount(mockContext, nil, "", "")
			Expect(resource).To(Equal(serviceAccount))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetAllRepositoriesInNamespace", func() {
		It("returns resource and error from the context", func() {
			repositories := []pacv1alpha1.Repository{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: RepositoriesContextKey,
					Resource:   repositories,
				},
			})
			resource, err := loader.GetAllRepositoriesInNamespace(mockContext, nil, "")
			Expect(resource).To(Equal(&repositories))
			Expect(err).ToNot(HaveOccurred())
		})
	})

})
