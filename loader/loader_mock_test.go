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
	toolkit "github.com/konflux-ci/operator-toolkit/loader"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	releasev1alpha1 "github.com/konflux-ci/release-service/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	resolutionv1beta1 "github.com/tektoncd/pipeline/pkg/apis/resolution/v1beta1"
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

	Context("When calling GetAllApplicationComponents", func() {
		It("returns resource and error from the context", func() {
			applicationComponents := []applicationapiv1alpha1.Component{}
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

	Context("When calling GetApplicationFromSnapshot", func() {
		It("returns resource and error from the context", func() {
			application := &applicationapiv1alpha1.Application{}
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

	Context("When calling GetComponentFromSnapshot", func() {
		It("returns resource and error from the context", func() {
			component := &applicationapiv1alpha1.Component{}
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
			component := &applicationapiv1alpha1.Component{}
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
			application := &applicationapiv1alpha1.Application{}
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
			application := &applicationapiv1alpha1.Application{}
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
			snapshot := &applicationapiv1alpha1.Snapshot{}
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

	Context("When calling GetAllSnapshotsForBuildPipelineRun", func() {
		It("returns resource and error from the context", func() {
			snapshots := []applicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AllSnapshotsForBuildPipelineRunContextKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetAllSnapshotsForBuildPipelineRun(mockContext, nil, nil)
			Expect(resource).To(Equal(&snapshots))
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

	Context("When calling GetRequiredIntegrationTestScenariosForSnapshot", func() {
		It("returns required integrationTestScenario and error from the context", func() {
			scenarios := []v1beta2.IntegrationTestScenario{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: RequiredIntegrationTestScenariosContextKey,
					Resource:   scenarios,
				},
			})
			resource, err := loader.GetRequiredIntegrationTestScenariosForSnapshot(mockContext, nil, nil, nil)
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
			snapshots := []applicationapiv1alpha1.Snapshot{}
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

	Context("When calling GetAutoReleasePlansForApplication", func() {
		It("returns snapshots and error from the context", func() {
			releasePlans := []releasev1alpha1.ReleasePlan{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: AutoReleasePlansContextKey,
					Resource:   releasePlans,
				},
			})
			resource, err := loader.GetAutoReleasePlansForApplication(mockContext, nil, nil)
			Expect(resource).To(Equal(&releasePlans))
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
			component := &applicationapiv1alpha1.Component{}
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
			resource, err := loader.GetPipelineRunsWithPRGroupHash(mockContext, nil, "", "", "")
			Expect(resource).To(Equal(&plrs))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetMatchingComponentSnapshotsForComponentAndPRGroupHash", func() {
		It("returns resource and error from the context", func() {
			snapshots := []applicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetComponentSnapshotsKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetMatchingComponentSnapshotsForComponentAndPRGroupHash(mockContext, nil, "", "", "", "")
			Expect(resource).To(Equal(&snapshots))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetMatchingComponentSnapshotsForPRGroupHash", func() {
		It("returns resource and error from the context", func() {
			snapshots := []applicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetPRSnapshotsKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetMatchingComponentSnapshotsForPRGroupHash(mockContext, nil, "", "", "")
			Expect(resource).To(Equal(&snapshots))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetComponentsFromSnapshotForPRGroup", func() {
		It("return resource and error from the context", func() {
			components := []string{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetComponentsFromSnapshotForPRGroupKey,
					Resource:   components,
				},
			})
			resource, err := loader.GetComponentsFromSnapshotForPRGroup(mockContext, nil, "", "", "", "")
			Expect(resource).To(Equal(components))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When calling GetMatchingGroupSnapshotsForPRGroupHash", func() {
		It("return resource and error from the context", func() {
			snapshots := []applicationapiv1alpha1.Snapshot{}
			mockContext := toolkit.GetMockedContext(ctx, []toolkit.MockData{
				{
					ContextKey: GetGroupSnapshotsKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetMatchingGroupSnapshotsForPRGroupHash(mockContext, nil, "", "", "")
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
})
