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

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"
	releasev1alpha1 "github.com/konflux-ci/release-service/api/v1alpha1"
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
	AllIntegrationTestScenariosForSnapshotContextKey
	AllSnapshotsContextKey
	AutoReleasePlansContextKey
	GetScenarioContextKey
	AllEnvironmentsForScenarioContextKey
	AllSnapshotsForBuildPipelineRunContextKey
	AllSnapshotsForGivenPRContextKey
	AllTaskRunsWithMatchingPipelineRunLabelContextKey
	GetPipelineRunContextKey
	GetComponentContextKey
	GetBuildPLRContextKey
	GetComponentSnapshotsKey
	GetPRSnapshotsKey
	GetPipelineRunforSnapshotsKey
	GetComponentsFromSnapshotForPRGroupKey
)

func NewMockLoader() ObjectLoader {
	return &mockLoader{
		loader: NewLoader(),
	}
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

// GetRequiredIntegrationTestScenariosForSnapshot returns the resource and error passed as values of the context.
func (l *mockLoader) GetRequiredIntegrationTestScenariosForSnapshot(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application, snapshot *applicationapiv1alpha1.Snapshot) (*[]v1beta2.IntegrationTestScenario, error) {
	if ctx.Value(RequiredIntegrationTestScenariosContextKey) == nil {
		return l.loader.GetRequiredIntegrationTestScenariosForSnapshot(ctx, c, application, snapshot)
	}
	integrationTestScenarios, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, RequiredIntegrationTestScenariosContextKey, []v1beta2.IntegrationTestScenario{})
	return &integrationTestScenarios, err
}

// GetAllIntegrationTestScenariosForSnapshot returns the resource and error passed as values of the context.
func (l *mockLoader) GetAllIntegrationTestScenariosForSnapshot(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application, snapshot *applicationapiv1alpha1.Snapshot) (*[]v1beta2.IntegrationTestScenario, error) {
	if ctx.Value(AllIntegrationTestScenariosForSnapshotContextKey) == nil {
		return l.loader.GetAllIntegrationTestScenariosForSnapshot(ctx, c, application, snapshot)
	}
	integrationTestScenarios, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, AllIntegrationTestScenariosForSnapshotContextKey, []v1beta2.IntegrationTestScenario{})
	return &integrationTestScenarios, err
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

func (l *mockLoader) GetAllSnapshotsForBuildPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*[]applicationapiv1alpha1.Snapshot, error) {
	if ctx.Value(AllSnapshotsForBuildPipelineRunContextKey) == nil {
		return l.loader.GetAllSnapshotsForBuildPipelineRun(ctx, c, pipelineRun)
	}
	snapshots, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, AllSnapshotsForBuildPipelineRunContextKey, []applicationapiv1alpha1.Snapshot{})
	return &snapshots, err
}

func (l *mockLoader) GetAllSnapshotsForPR(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application, pullRequest string) (*[]applicationapiv1alpha1.Snapshot, error) {
	if ctx.Value(AllSnapshotsForGivenPRContextKey) == nil {
		return l.loader.GetAllSnapshotsForPR(ctx, c, application, pullRequest)
	}
	snapshots, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, AllSnapshotsForGivenPRContextKey, []applicationapiv1alpha1.Snapshot{})
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

// GetPipelineRunsWithPRGroupHash returns the resource and error passed as values of the context.
func (l *mockLoader) GetPipelineRunsWithPRGroupHash(ctx context.Context, c client.Client, namespace, prGroupHash string) (*[]tektonv1.PipelineRun, error) {
	if ctx.Value(GetBuildPLRContextKey) == nil {
		return l.loader.GetPipelineRunsWithPRGroupHash(ctx, c, namespace, prGroupHash)
	}
	pipelineRuns, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, GetBuildPLRContextKey, []tektonv1.PipelineRun{})
	return &pipelineRuns, err
}

// GetMatchingComponentSnapshotsForComponentAndPRGroupHash returns the resource and error passed as values of the context
func (l *mockLoader) GetMatchingComponentSnapshotsForComponentAndPRGroupHash(ctx context.Context, c client.Client, namespace, componentName, prGroupHash string) (*[]applicationapiv1alpha1.Snapshot, error) {
	if ctx.Value(GetComponentSnapshotsKey) == nil {
		return l.loader.GetMatchingComponentSnapshotsForComponentAndPRGroupHash(ctx, c, namespace, componentName, prGroupHash)
	}
	snapshots, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, GetComponentSnapshotsKey, []applicationapiv1alpha1.Snapshot{})
	return &snapshots, err
}

// GetMatchingComponentSnapshotsForPRGroupHash returns the resource and error passed as values of the context
func (l *mockLoader) GetMatchingComponentSnapshotsForPRGroupHash(ctx context.Context, c client.Client, nameSpace, prGroupHash string) (*[]applicationapiv1alpha1.Snapshot, error) {
	if ctx.Value(GetPRSnapshotsKey) == nil {
		return l.loader.GetMatchingComponentSnapshotsForPRGroupHash(ctx, c, nameSpace, prGroupHash)
	}
	snapshots, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, GetPRSnapshotsKey, []applicationapiv1alpha1.Snapshot{})
	return &snapshots, err
}

// GetAllIntegrationPipelineRunsForSnapshot returns the resource and error passed as values of the context
func (l *mockLoader) GetAllIntegrationPipelineRunsForSnapshot(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot) ([]tektonv1.PipelineRun, error) {
	if ctx.Value(GetPipelineRunforSnapshotsKey) == nil {
		return l.loader.GetAllIntegrationPipelineRunsForSnapshot(ctx, adapterClient, snapshot)
	}
	pipelineRuns, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, GetPipelineRunforSnapshotsKey, []tektonv1.PipelineRun{})
	return pipelineRuns, err
}

func (l *mockLoader) GetComponentsFromSnapshotForPRGroup(ctx context.Context, c client.Client, namespace, prGroup, prGroupHash string) ([]string, error) {
	if ctx.Value(GetComponentsFromSnapshotForPRGroupKey) == nil {
		return l.loader.GetComponentsFromSnapshotForPRGroup(ctx, c, namespace, prGroup, prGroupHash)
	}
	components, err := toolkit.GetMockedResourceAndErrorFromContext(ctx, GetComponentsFromSnapshotForPRGroupKey, []string{})
	return components, err
}
