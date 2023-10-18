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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Release Adapter", Ordered, func() {
	var (
		loader ObjectLoader
	)

	BeforeAll(func() {
		loader = NewMockLoader()
	})

	Context("When calling getMockedResourceAndErrorFromContext", func() {
		contextErr := errors.New("error")
		contextResource := &releasev1alpha1.Release{
			ObjectMeta: v12.ObjectMeta{
				Name:      "pod",
				Namespace: "default",
			},
			Spec: releasev1alpha1.ReleaseSpec{
				ReleasePlan: "releasePlan",
				Snapshot:    "snapshot",
			},
		}

		It("returns resource from the context", func() {
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: ReleaseContextKey,
					Resource:   contextResource,
				},
			})
			resource, err := getMockedResourceAndErrorFromContext(mockContext, ReleaseContextKey, contextResource)
			Expect(err).To(BeNil())
			Expect(resource).To(Equal(contextResource))
		})

		It("returns error from the context", func() {
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: ReleaseContextKey,
					Err:        contextErr,
				},
			})
			resource, err := getMockedResourceAndErrorFromContext(mockContext, ReleaseContextKey, contextResource)
			Expect(err).To(Equal(contextErr))
			Expect(resource).To(BeNil())
		})

		It("returns resource and the error from the context", func() {
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: ReleaseContextKey,
					Resource:   contextResource,
					Err:        contextErr,
				},
			})
			resource, err := getMockedResourceAndErrorFromContext(mockContext, ReleaseContextKey, contextResource)
			Expect(err).To(Equal(contextErr))
			Expect(resource).To(Equal(contextResource))
		})

		It("should panic when the mocked data is not present", func() {
			Expect(func() {
				_, _ = getMockedResourceAndErrorFromContext(ctx, ReleaseContextKey, contextResource)
			}).To(Panic())
		})
	})

	Context("When calling GetAllEnvironments", func() {
		It("returns resource and error from the context", func() {
			environment := &applicationapiv1alpha1.Environment{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: EnvironmentContextKey,
					Resource:   environment,
				},
			})
			resource, err := loader.GetAllEnvironments(nil, mockContext, nil)
			Expect(resource).To(Equal(&[]applicationapiv1alpha1.Environment{*environment}))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetReleasesWithSnapshot", func() {
		It("returns resource and error from the context", func() {
			release := &releasev1alpha1.Release{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: ReleaseContextKey,
					Resource:   release,
				},
			})
			resource, err := loader.GetReleasesWithSnapshot(nil, mockContext, nil)
			Expect(resource).To(Equal(&[]releasev1alpha1.Release{*release}))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetAllApplicationComponents", func() {
		It("returns resource and error from the context", func() {
			applicationComponents := []applicationapiv1alpha1.Component{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: ApplicationComponentsContextKey,
					Resource:   applicationComponents,
				},
			})
			resource, err := loader.GetAllApplicationComponents(nil, mockContext, nil)
			Expect(resource).To(Equal(&applicationComponents))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetApplicationFromSnapshot", func() {
		It("returns resource and error from the context", func() {
			application := &applicationapiv1alpha1.Application{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: ApplicationContextKey,
					Resource:   application,
				},
			})
			resource, err := loader.GetApplicationFromSnapshot(nil, mockContext, nil)
			Expect(resource).To(Equal(application))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetComponentFromSnapshot", func() {
		It("returns resource and error from the context", func() {
			component := &applicationapiv1alpha1.Component{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: ComponentContextKey,
					Resource:   component,
				},
			})
			resource, err := loader.GetComponentFromSnapshot(nil, mockContext, nil)
			Expect(resource).To(Equal(component))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetComponentFromPipelineRun", func() {
		It("returns resource and error from the context", func() {
			component := &applicationapiv1alpha1.Component{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: ComponentContextKey,
					Resource:   component,
				},
			})
			resource, err := loader.GetComponentFromPipelineRun(nil, mockContext, nil)
			Expect(resource).To(Equal(component))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetApplicationFromPipelineRun", func() {
		It("returns resource and error from the context", func() {
			application := &applicationapiv1alpha1.Application{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: ApplicationContextKey,
					Resource:   application,
				},
			})
			resource, err := loader.GetApplicationFromPipelineRun(nil, mockContext, nil)
			Expect(resource).To(Equal(application))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetApplicationFromComponent", func() {
		It("returns resource and error from the context", func() {
			application := &applicationapiv1alpha1.Application{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: ApplicationContextKey,
					Resource:   application,
				},
			})
			resource, err := loader.GetApplicationFromComponent(nil, mockContext, nil)
			Expect(resource).To(Equal(application))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetEnvironmentFromIntegrationPipelineRun", func() {
		It("returns resource and error from the context", func() {
			environment := &applicationapiv1alpha1.Environment{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: EnvironmentContextKey,
					Resource:   environment,
				},
			})
			resource, err := loader.GetEnvironmentFromIntegrationPipelineRun(nil, mockContext, nil)
			Expect(resource).To(Equal(environment))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetSnapshotFromPipelineRun", func() {
		It("returns resource and error from the context", func() {
			snapshot := &applicationapiv1alpha1.Snapshot{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: SnapshotContextKey,
					Resource:   snapshot,
				},
			})
			resource, err := loader.GetSnapshotFromPipelineRun(nil, mockContext, nil)
			Expect(resource).To(Equal(snapshot))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling FindAvailableDeploymentTargetClass", func() {
		It("returns deploymentTargetClassre source and error from the context", func() {
			dtcls := &applicationapiv1alpha1.DeploymentTargetClass{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: DeploymentTargetClassContextKey,
					Resource:   dtcls,
				},
			})
			resource, err := loader.FindAvailableDeploymentTargetClass(nil, mockContext)
			Expect(resource).To(Equal(dtcls))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetAllIntegrationTestScenariosForApplication", func() {
		It("returns all integrationTestScenario and error from the context", func() {
			scenarios := []v1beta1.IntegrationTestScenario{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: AllIntegrationTestScenariosContextKey,
					Resource:   scenarios,
				},
			})
			resource, err := loader.GetAllIntegrationTestScenariosForApplication(nil, mockContext, nil)
			Expect(resource).To(Equal(&scenarios))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetRequiredIntegrationTestScenariosForApplication", func() {
		It("returns required integrationTestScenario and error from the context", func() {
			scenarios := []v1beta1.IntegrationTestScenario{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: RequiredIntegrationTestScenariosContextKey,
					Resource:   scenarios,
				},
			})
			resource, err := loader.GetRequiredIntegrationTestScenariosForApplication(nil, mockContext, nil)
			Expect(resource).To(Equal(&scenarios))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetDeploymentTargetClaimForEnvironment", func() {
		It("returns deploymentTargetClaim and error from the context", func() {
			dtc := &applicationapiv1alpha1.DeploymentTargetClaim{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: DeploymentTargetClaimContextKey,
					Resource:   dtc,
				},
			})
			resource, err := loader.GetDeploymentTargetClaimForEnvironment(nil, mockContext, nil)
			Expect(resource).To(Equal(dtc))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetDeploymentTargetForDeploymentTargetClaim", func() {
		It("returns deploymentTargetClaim and error from the context", func() {
			dt := &applicationapiv1alpha1.DeploymentTarget{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: DeploymentTargetContextKey,
					Resource:   dt,
				},
			})
			resource, err := loader.GetDeploymentTargetForDeploymentTargetClaim(nil, mockContext, nil)
			Expect(resource).To(Equal(dt))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling FindExistingSnapshotEnvironmentBinding", func() {
		It("returns existing snapshotEnvironmentBinding and error from the context", func() {
			binding := &applicationapiv1alpha1.SnapshotEnvironmentBinding{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: SnapshotEnvironmentBindingContextKey,
					Resource:   binding,
				},
			})
			resource, err := loader.FindExistingSnapshotEnvironmentBinding(nil, mockContext, nil, nil)
			Expect(resource).To(Equal(binding))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetAllPipelineRunsForSnapshotAndScenario", func() {
		It("returns pipelineRuns and error from the context", func() {
			prs := []tektonv1beta1.PipelineRun{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: PipelineRunsContextKey,
					Resource:   prs,
				},
			})
			resource, err := loader.GetAllPipelineRunsForSnapshotAndScenario(nil, mockContext, nil, nil)
			Expect(resource).To(Equal(&prs))
			Expect(err).To(BeNil())
		})
	})

	Context("When calling GetAllSnapshots", func() {
		It("returns snapshots and error from the context", func() {
			snapshots := []applicationapiv1alpha1.Snapshot{}
			mockContext := GetMockedContext(ctx, []MockData{
				{
					ContextKey: AllSnapshotsContextKey,
					Resource:   snapshots,
				},
			})
			resource, err := loader.GetAllSnapshots(nil, mockContext, nil)
			Expect(resource).To(Equal(&snapshots))
			Expect(err).To(BeNil())
		})
	})
})
