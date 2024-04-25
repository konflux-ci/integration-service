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
	"context"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta2"
	toolkit "github.com/redhat-appstudio/operator-toolkit/loader"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mockLoader struct {
	loader ObjectLoader
}

const (
	ApplicationContextKey toolkit.ContextKey = iota
	ComponentContextKey
	SnapshotContextKey
	SnapshotEnvironmentBindingContextKey
	DeploymentTargetContextKey
	DeploymentTargetClaimContextKey
	IntegrationTestScenarioContextKey
	TaskRunContextKey
	ApplicationComponentsContextKey
	SnapshotComponentsContextKey
	EnvironmentContextKey
	ReleaseContextKey
	PipelineRunsContextKey
	AllIntegrationTestScenariosContextKey
	RequiredIntegrationTestScenariosContextKey
	AllSnapshotsContextKey
	AutoReleasePlansContextKey
	GetScenarioContextKey
	AllEnvironmentsForScenarioContextKey
	AllSnapshotsForBuildPipelineRunContextKey
	AllTaskRunsWithMatchingPipelineRunLabelContextKey
	GetPipelineRunContextKey
	GetComponentContextKey
)

func NewMockLoader() ObjectLoader {
	return &mockLoader{
		loader: NewLoader(),
	}
}

// GetAllEnvironments returns the resource and error passed as values of the context.
func (l *mockLoader) GetAllEnvironments(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Environment, error) {
	if ctx.Value(EnvironmentContextKey) == nil {
		return l.loader.GetAllEnvironments(ctx, c, application)
	}
	environment, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, EnvironmentContextKey, &applicationapiv1alpha1.Environment{})
	return &[]applicationapiv1alpha1.Environment{*environment}, err
}

// GetReleasesWithSnapshot returns the resource and error passed as values of the context.
func (l *mockLoader) GetReleasesWithSnapshot(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot) (*[]releasev1alpha1.Release, error) {
	if ctx.Value(ReleaseContextKey) == nil {
		return l.loader.GetReleasesWithSnapshot(ctx, c, snapshot)
	}
	release, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, ReleaseContextKey, &releasev1alpha1.Release{})
	return &[]releasev1alpha1.Release{*release}, err
}

// GetAllApplicationComponents returns the resource and error passed as values of the context.
func (l *mockLoader) GetAllApplicationComponents(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Component, error) {
	if ctx.Value(ApplicationComponentsContextKey) == nil {
		return l.loader.GetAllApplicationComponents(ctx, c, application)
	}
	components, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, ApplicationComponentsContextKey, []applicationapiv1alpha1.Component{})
	return &components, err
}

// GetApplicationFromSnapshot returns the resource and error passed as values of the context.
func (l *mockLoader) GetApplicationFromSnapshot(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Application, error) {
	if ctx.Value(ApplicationContextKey) == nil {
		return l.loader.GetApplicationFromSnapshot(ctx, c, snapshot)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, ApplicationContextKey, &applicationapiv1alpha1.Application{})
}

// GetComponentFromSnapshot returns the resource and error passed as values of the context.
func (l *mockLoader) GetComponentFromSnapshot(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Component, error) {
	if ctx.Value(ComponentContextKey) == nil {
		return l.loader.GetComponentFromSnapshot(ctx, c, snapshot)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, ComponentContextKey, &applicationapiv1alpha1.Component{})
}

// GetComponentFromPipelineRun returns the resource and error passed as values of the context.
func (l *mockLoader) GetComponentFromPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.Component, error) {
	if ctx.Value(ComponentContextKey) == nil {
		return l.loader.GetComponentFromPipelineRun(ctx, c, pipelineRun)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, ComponentContextKey, &applicationapiv1alpha1.Component{})
}

// GetApplicationFromPipelineRun returns the resource and error passed as values of the context.
func (l *mockLoader) GetApplicationFromPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.Application, error) {
	if ctx.Value(ApplicationContextKey) == nil {
		return l.loader.GetApplicationFromPipelineRun(ctx, c, pipelineRun)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, ApplicationContextKey, &applicationapiv1alpha1.Application{})
}

// GetApplicationFromComponent returns the resource and error passed as values of the context.
func (l *mockLoader) GetApplicationFromComponent(ctx context.Context, c client.Client, component *applicationapiv1alpha1.Component) (*applicationapiv1alpha1.Application, error) {
	if ctx.Value(ApplicationContextKey) == nil {
		return l.loader.GetApplicationFromComponent(ctx, c, component)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, ApplicationContextKey, &applicationapiv1alpha1.Application{})
}

// GetEnvironmentFromIntegrationPipelineRun returns the resource and error passed as values of the context.
func (l *mockLoader) GetEnvironmentFromIntegrationPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.Environment, error) {
	if ctx.Value(EnvironmentContextKey) == nil {
		return l.loader.GetEnvironmentFromIntegrationPipelineRun(ctx, c, pipelineRun)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, EnvironmentContextKey, &applicationapiv1alpha1.Environment{})
}

// GetSnapshotFromPipelineRun returns the resource and error passed as values of the context.
func (l *mockLoader) GetSnapshotFromPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.Snapshot, error) {
	if ctx.Value(SnapshotContextKey) == nil {
		return l.loader.GetSnapshotFromPipelineRun(ctx, c, pipelineRun)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, SnapshotContextKey, &applicationapiv1alpha1.Snapshot{})
}

// GetAllIntegrationTestScenariosForApplication returns the resource and error passed as values of the context.
func (l *mockLoader) GetAllIntegrationTestScenariosForApplication(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]v1beta2.IntegrationTestScenario, error) {
	if ctx.Value(AllIntegrationTestScenariosContextKey) == nil {
		return l.loader.GetAllIntegrationTestScenariosForApplication(ctx, c, application)
	}
	integrationTestScenarios, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, AllIntegrationTestScenariosContextKey, []v1beta2.IntegrationTestScenario{})
	return &integrationTestScenarios, err
}

// GetRequiredIntegrationTestScenariosForApplication returns the resource and error passed as values of the context.
func (l *mockLoader) GetRequiredIntegrationTestScenariosForApplication(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]v1beta2.IntegrationTestScenario, error) {
	if ctx.Value(RequiredIntegrationTestScenariosContextKey) == nil {
		return l.loader.GetRequiredIntegrationTestScenariosForApplication(ctx, c, application)
	}
	integrationTestScenarios, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, RequiredIntegrationTestScenariosContextKey, []v1beta2.IntegrationTestScenario{})
	return &integrationTestScenarios, err
}

// GetDeploymentTargetClaimForEnvironment returns the resource and error passed as values of the context.
func (l *mockLoader) GetDeploymentTargetClaimForEnvironment(ctx context.Context, c client.Client, environment *applicationapiv1alpha1.Environment) (*applicationapiv1alpha1.DeploymentTargetClaim, error) {
	if ctx.Value(DeploymentTargetClaimContextKey) == nil {
		return l.loader.GetDeploymentTargetClaimForEnvironment(ctx, c, environment)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, DeploymentTargetClaimContextKey, &applicationapiv1alpha1.DeploymentTargetClaim{})
}

// GetDeploymentTargetForDeploymentTargetClaim returns the resource and error passed as values of the context.
func (l *mockLoader) GetDeploymentTargetForDeploymentTargetClaim(ctx context.Context, c client.Client, dtc *applicationapiv1alpha1.DeploymentTargetClaim) (*applicationapiv1alpha1.DeploymentTarget, error) {
	if ctx.Value(DeploymentTargetContextKey) == nil {
		return l.loader.GetDeploymentTargetForDeploymentTargetClaim(ctx, c, dtc)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, DeploymentTargetContextKey, &applicationapiv1alpha1.DeploymentTarget{})
}

// FindExistingSnapshotEnvironmentBinding returns the resource and error passed as values of the context.
func (l *mockLoader) FindExistingSnapshotEnvironmentBinding(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application, environment *applicationapiv1alpha1.Environment) (*applicationapiv1alpha1.SnapshotEnvironmentBinding, error) {
	if ctx.Value(SnapshotEnvironmentBindingContextKey) == nil {
		return l.loader.FindExistingSnapshotEnvironmentBinding(ctx, c, application, environment)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, SnapshotEnvironmentBindingContextKey, &applicationapiv1alpha1.SnapshotEnvironmentBinding{})
}

// GetAllPipelineRunsForSnapshotAndScenario returns the resource and error passed as values of the context.
func (l *mockLoader) GetAllPipelineRunsForSnapshotAndScenario(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot, integrationTestScenario *v1beta2.IntegrationTestScenario) (*[]tektonv1.PipelineRun, error) {
	if ctx.Value(PipelineRunsContextKey) == nil {
		return l.loader.GetAllPipelineRunsForSnapshotAndScenario(ctx, c, snapshot, integrationTestScenario)
	}
	pipelineRuns, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, PipelineRunsContextKey, []tektonv1.PipelineRun{})
	return &pipelineRuns, err
}

// GetAllSnapshots returns the resource and error passed as values of the context.
func (l *mockLoader) GetAllSnapshots(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Snapshot, error) {
	if ctx.Value(AllSnapshotsContextKey) == nil {
		return l.loader.GetAllSnapshots(ctx, c, application)
	}
	snapshots, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, AllSnapshotsContextKey, []applicationapiv1alpha1.Snapshot{})
	return &snapshots, err
}

// GetAutoReleasePlansForApplication returns the resource and error passed as values of the context.
func (l *mockLoader) GetAutoReleasePlansForApplication(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]releasev1alpha1.ReleasePlan, error) {
	if ctx.Value(AutoReleasePlansContextKey) == nil {
		return l.loader.GetAutoReleasePlansForApplication(ctx, c, application)
	}
	autoReleasePlans, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, AutoReleasePlansContextKey, []releasev1alpha1.ReleasePlan{})
	return &autoReleasePlans, err
}

// GetScenario returns the resource and error passed as values of the context.
func (l *mockLoader) GetScenario(ctx context.Context, c client.Client, name, namespace string) (*v1beta2.IntegrationTestScenario, error) {
	if ctx.Value(GetScenarioContextKey) == nil {
		return l.loader.GetScenario(ctx, c, name, namespace)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, GetScenarioContextKey, &v1beta2.IntegrationTestScenario{})
}

func (l *mockLoader) GetAllEnvironmentsForScenario(ctx context.Context, c client.Client, integrationTestScenario *v1beta2.IntegrationTestScenario) (*[]applicationapiv1alpha1.Environment, error) {
	if ctx.Value(AllEnvironmentsForScenarioContextKey) == nil {
		return l.loader.GetAllEnvironmentsForScenario(ctx, c, integrationTestScenario)
	}
	environments, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, AllEnvironmentsForScenarioContextKey, []applicationapiv1alpha1.Environment{})
	return &environments, err
}

func (l *mockLoader) GetAllSnapshotsForBuildPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*[]applicationapiv1alpha1.Snapshot, error) {
	if ctx.Value(AllSnapshotsForBuildPipelineRunContextKey) == nil {
		return l.loader.GetAllSnapshotsForBuildPipelineRun(ctx, c, pipelineRun)
	}
	snapshots, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, AllSnapshotsForBuildPipelineRunContextKey, []applicationapiv1alpha1.Snapshot{})
	return &snapshots, err
}

func (l *mockLoader) GetAllTaskRunsWithMatchingPipelineRunLabel(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*[]tektonv1.TaskRun, error) {
	if ctx.Value(AllTaskRunsWithMatchingPipelineRunLabelContextKey) == nil {
		return l.loader.GetAllTaskRunsWithMatchingPipelineRunLabel(ctx, c, pipelineRun)
	}
	taskRuns, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, AllTaskRunsWithMatchingPipelineRunLabelContextKey, []tektonv1.TaskRun{})
	return &taskRuns, err
}

// GetPipelineRun returns the resource and error passed as values of the context.
func (l *mockLoader) GetPipelineRun(ctx context.Context, c client.Client, name, namespace string) (*tektonv1.PipelineRun, error) {
	if ctx.Value(GetPipelineRunContextKey) == nil {
		return l.loader.GetPipelineRun(ctx, c, name, namespace)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, GetPipelineRunContextKey, &tektonv1.PipelineRun{})
}

// GetComponent returns the resource and error passed as values of the context.
func (l *mockLoader) GetComponent(ctx context.Context, c client.Client, name, namespace string) (*applicationapiv1alpha1.Component, error) {
	if ctx.Value(GetComponentContextKey) == nil {
		return l.loader.GetComponent(ctx, c, name, namespace)
	}
	return toolkit.GetMockedResourceAndErrorFromContext(ctx, GetComponentContextKey, &applicationapiv1alpha1.Component{})
}
