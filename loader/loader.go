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

// Package loader contains functions used to load resource from the cluster
package loader

import (
	"context"
	"fmt"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/tekton"
	toolkit "github.com/konflux-ci/operator-toolkit/loader"
	releasev1alpha1 "github.com/konflux-ci/release-service/api/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ObjectLoader interface {
	GetReleasesWithSnapshot(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot) (*[]releasev1alpha1.Release, error)
	GetAllApplicationComponents(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Component, error)
	GetApplicationFromSnapshot(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Application, error)
	GetComponentFromSnapshot(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Component, error)
	GetComponentFromPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.Component, error)
	GetApplicationFromPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.Application, error)
	GetApplicationFromComponent(ctx context.Context, c client.Client, component *applicationapiv1alpha1.Component) (*applicationapiv1alpha1.Application, error)
	GetSnapshotFromPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.Snapshot, error)
	GetAllIntegrationTestScenariosForApplication(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]v1beta2.IntegrationTestScenario, error)
	GetRequiredIntegrationTestScenariosForSnapshot(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application, snapshot *applicationapiv1alpha1.Snapshot) (*[]v1beta2.IntegrationTestScenario, error)
	GetAllIntegrationTestScenariosForSnapshot(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application, snapshot *applicationapiv1alpha1.Snapshot) (*[]v1beta2.IntegrationTestScenario, error)
	GetAllPipelineRunsForSnapshotAndScenario(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot, integrationTestScenario *v1beta2.IntegrationTestScenario) (*[]tektonv1.PipelineRun, error)
	GetAllSnapshots(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Snapshot, error)
	GetAutoReleasePlansForApplication(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]releasev1alpha1.ReleasePlan, error)
	GetScenario(ctx context.Context, c client.Client, name, namespace string) (*v1beta2.IntegrationTestScenario, error)
	GetAllSnapshotsForBuildPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*[]applicationapiv1alpha1.Snapshot, error)
	GetAllSnapshotsForPR(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application, componentName, pullRequest string) (*[]applicationapiv1alpha1.Snapshot, error)
	GetAllTaskRunsWithMatchingPipelineRunLabel(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*[]tektonv1.TaskRun, error)
	GetPipelineRun(ctx context.Context, c client.Client, name, namespace string) (*tektonv1.PipelineRun, error)
	GetComponent(ctx context.Context, c client.Client, name, namespace string) (*applicationapiv1alpha1.Component, error)
	GetMatchingComponentSnapshotsForPRGroupHash(ctx context.Context, c client.Client, nameSpace, prGroupHash string) (*[]applicationapiv1alpha1.Snapshot, error)
	GetPipelineRunsWithPRGroupHash(ctx context.Context, c client.Client, namespace, prGroupHash string) (*[]tektonv1.PipelineRun, error)
	GetMatchingComponentSnapshotsForComponentAndPRGroupHash(ctx context.Context, c client.Client, snapshot, componentName, prGroupHash string) (*[]applicationapiv1alpha1.Snapshot, error)
	GetAllIntegrationPipelineRunsForSnapshot(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot) ([]tektonv1.PipelineRun, error)
	GetComponentsFromSnapshotForPRGroup(ctx context.Context, c client.Client, namespace, prGroup, prGroupHash string) ([]string, error)
	GetMatchingGroupSnapshotsForPRGroupHash(ctx context.Context, c client.Client, namespace, prGroupHash string) (*[]applicationapiv1alpha1.Snapshot, error)
}

type loader struct{}

func NewLoader() ObjectLoader {
	return &loader{}
}

// GetReleasesWithSnapshot returns all Releases associated with the given snapshot.
// In the case the List operation fails, an error will be returned.
func (l *loader) GetReleasesWithSnapshot(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot) (*[]releasev1alpha1.Release, error) {
	releases := &releasev1alpha1.ReleaseList{}
	opts := []client.ListOption{
		client.InNamespace(snapshot.Namespace),
		client.MatchingFields{"spec.snapshot": snapshot.Name},
	}

	err := c.List(ctx, releases, opts...)
	if err != nil {
		return nil, err
	}

	return &releases.Items, nil
}

// GetAllApplicationComponents loads from the cluster all Components associated with the given Application.
// If the Application doesn't have any Components or this is not found in the cluster, an error will be returned.
func (l *loader) GetAllApplicationComponents(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Component, error) {
	applicationComponents := &applicationapiv1alpha1.ComponentList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := c.List(ctx, applicationComponents, opts...)
	if err != nil {
		return nil, err
	}

	return &applicationComponents.Items, nil
}

// GetApplicationFromSnapshot loads from the cluster the Application referenced in the given Snapshot.
// If the Snapshot doesn't specify an Component or this is not found in the cluster, an error will be returned.
func (l *loader) GetApplicationFromSnapshot(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Application, error) {
	application := &applicationapiv1alpha1.Application{}
	return application, toolkit.GetObject(snapshot.Spec.Application, snapshot.Namespace, c, ctx, application)
}

// GetComponentFromSnapshot loads from the cluster the Component referenced in the given Snapshot.
// If the Snapshot doesn't specify an Application or this is not found in the cluster, an error will be returned.
func (l *loader) GetComponentFromSnapshot(ctx context.Context, c client.Client, snapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Component, error) {
	if componentLabel, ok := snapshot.Labels[gitops.SnapshotComponentLabel]; ok {
		component := &applicationapiv1alpha1.Component{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: snapshot.Namespace,
			Name:      componentLabel,
		}, component)

		if err != nil {
			return nil, err
		}

		return component, nil
	} else {
		groupResource := schema.GroupResource{Group: "", Resource: ""}
		return nil, errors.NewNotFound(groupResource, fmt.Sprintf("Label '%s'", gitops.SnapshotComponentLabel))
	}
}

// GetComponentFromPipelineRun loads from the cluster the Component referenced in the given PipelineRun. If the PipelineRun doesn't
// specify a Component or this is not found in the cluster, an error will be returned.
func (l *loader) GetComponentFromPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.Component, error) {
	if componentName, found := pipelineRun.Labels[tekton.PipelineRunComponentLabel]; found {
		component := &applicationapiv1alpha1.Component{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      componentName,
		}, component)

		if err != nil {
			return nil, err
		}

		return component, nil
	}

	return nil, nil
}

// GetApplicationFromPipelineRun loads from the cluster the Application referenced in the given PipelineRun. If the PipelineRun doesn't
// specify an Application or this is not found in the cluster, an error will be returned.
func (l *loader) GetApplicationFromPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.Application, error) {
	if applicationName, found := pipelineRun.Labels[tekton.PipelineRunApplicationLabel]; found {
		application := &applicationapiv1alpha1.Application{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      applicationName,
		}, application)

		if err != nil {
			return nil, err
		}

		return application, nil
	}

	return nil, nil
}

// GetApplicationFromComponent loads from the cluster the Application referenced in the given Component. If the Component doesn't
// specify an Application or this is not found in the cluster, an error will be returned.
func (l *loader) GetApplicationFromComponent(ctx context.Context, c client.Client, component *applicationapiv1alpha1.Component) (*applicationapiv1alpha1.Application, error) {
	application := &applicationapiv1alpha1.Application{}
	err := c.Get(ctx, types.NamespacedName{
		Namespace: component.Namespace,
		Name:      component.Spec.Application,
	}, application)

	if err != nil {
		return nil, err
	}

	return application, nil
}

// GetSnapshotFromPipelineRun loads from the cluster the Snapshot referenced in the given PipelineRun.
// If the PipelineRun doesn't specify an Snapshot or this is not found in the cluster, an error will be returned.
func (l *loader) GetSnapshotFromPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*applicationapiv1alpha1.Snapshot, error) {
	if snapshotName, found := pipelineRun.Labels[tekton.SnapshotNameLabel]; found {
		snapshot := &applicationapiv1alpha1.Snapshot{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      snapshotName,
		}, snapshot)

		if err != nil {
			return nil, err
		}

		return snapshot, nil
	}

	return nil, fmt.Errorf("the pipeline has no snapshot associated with it")
}

// GetAllIntegrationTestScenariosForApplication returns all IntegrationTestScenarios used by the application being processed.
func (l *loader) GetAllIntegrationTestScenariosForApplication(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]v1beta2.IntegrationTestScenario, error) {
	integrationList := &v1beta2.IntegrationTestScenarioList{}

	opts := &client.ListOptions{
		Namespace:     application.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.application", application.Name),
	}

	err := c.List(ctx, integrationList, opts)
	if err != nil {
		return nil, err
	}

	return &integrationList.Items, nil
}

// GetRequiredIntegrationTestScenariosForSnapshot returns the IntegrationTestScenarios used by the application and snapshot being processed.
// An IntegrationTestScenarios will only be returned if it has the test.appstudio.openshift.io/optional
// label not set to true or if it is missing the label entirely, and have the correct context for the defined snapshot.
func (l *loader) GetRequiredIntegrationTestScenariosForSnapshot(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application, snapshot *applicationapiv1alpha1.Snapshot) (*[]v1beta2.IntegrationTestScenario, error) {
	integrationList := &v1beta2.IntegrationTestScenarioList{}
	labelRequirement, err := labels.NewRequirement("test.appstudio.openshift.io/optional", selection.NotIn, []string{"true"})
	if err != nil {
		return nil, err
	}
	labelSelector := labels.NewSelector().Add(*labelRequirement)

	opts := &client.ListOptions{
		Namespace:     application.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.application", application.Name),
		LabelSelector: labelSelector,
	}

	err = c.List(ctx, integrationList, opts)
	if err != nil {
		return nil, err
	}

	integrationTestScenarios := gitops.FilterIntegrationTestScenariosWithContext(&integrationList.Items, snapshot)
	return integrationTestScenarios, nil
}

// GetAllIntegrationTestScenariosForSnapshot returns the IntegrationTestScenarios used by the application and snapshot being processed.
// All the IntegrationTestScenarios will be returned regardless of whether it has the test.appstudio.openshift.io/optional
// label not set to true or if it is missing the label entirely, but they will have the correct context for the defined snapshot.
func (l *loader) GetAllIntegrationTestScenariosForSnapshot(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application, snapshot *applicationapiv1alpha1.Snapshot) (*[]v1beta2.IntegrationTestScenario, error) {
	integrationList, err := l.GetAllIntegrationTestScenariosForApplication(ctx, c, application)
	if err != nil {
		return nil, err
	}

	integrationTestScenarios := gitops.FilterIntegrationTestScenariosWithContext(integrationList, snapshot)
	return integrationTestScenarios, nil
}

// GetAllPipelineRunsForSnapshotAndScenario returns all Integration PipelineRun for the
// associated Snapshot and IntegrationTestScenario. In the case the List operation fails,
// an error will be returned.
func (l *loader) GetAllPipelineRunsForSnapshotAndScenario(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, integrationTestScenario *v1beta2.IntegrationTestScenario) (*[]tektonv1.PipelineRun, error) {
	integrationPipelineRuns := &tektonv1.PipelineRunList{}
	opts := []client.ListOption{
		client.InNamespace(snapshot.Namespace),
		client.MatchingLabels{
			"pipelines.appstudio.openshift.io/type": "test",
			"appstudio.openshift.io/snapshot":       snapshot.Name,
			"test.appstudio.openshift.io/scenario":  integrationTestScenario.Name,
		},
	}

	err := adapterClient.List(ctx, integrationPipelineRuns, opts...)
	if err != nil {
		return nil, err
	}
	return &integrationPipelineRuns.Items, nil
}

// GetAllSnapshots returns all Snapshots in the Application's namespace nil if it's not found.
// In the case the List operation fails, an error will be returned.
func (l *loader) GetAllSnapshots(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Snapshot, error) {
	snapshots := &applicationapiv1alpha1.SnapshotList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := c.List(ctx, snapshots, opts...)
	if err != nil {
		return nil, err
	}

	return &snapshots.Items, nil
}

// GetAutoReleasePlansForApplication returns the ReleasePlans used by the application being processed. If matching
// ReleasePlans are not found, an error will be returned. A ReleasePlan will only be returned if it has the
// release.appstudio.openshift.io/auto-release label set to true or if it is missing the label entirely.
func (l *loader) GetAutoReleasePlansForApplication(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application) (*[]releasev1alpha1.ReleasePlan, error) {
	releasePlans := &releasev1alpha1.ReleasePlanList{}
	labelRequirement, err := labels.NewRequirement("release.appstudio.openshift.io/auto-release", selection.NotIn, []string{"false"})
	if err != nil {
		return nil, err
	}
	labelSelector := labels.NewSelector().Add(*labelRequirement)

	opts := &client.ListOptions{
		Namespace:     application.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.application", application.Name),
		LabelSelector: labelSelector,
	}

	err = c.List(ctx, releasePlans, opts)
	if err != nil {
		return nil, err
	}

	return &releasePlans.Items, nil
}

// GetScenario returns integration test scenario requested by name and namespace
func (l *loader) GetScenario(ctx context.Context, c client.Client, name, namespace string) (*v1beta2.IntegrationTestScenario, error) {
	scenario := &v1beta2.IntegrationTestScenario{}
	return scenario, toolkit.GetObject(name, namespace, c, ctx, scenario)
}

// GetAllSnapshotsForBuildPipelineRun returns all Snapshots for the associated build pipelineRun.
// In the case the List operation fails, an error will be returned.
func (l *loader) GetAllSnapshotsForBuildPipelineRun(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*[]applicationapiv1alpha1.Snapshot, error) {
	snapshots := &applicationapiv1alpha1.SnapshotList{}
	opts := []client.ListOption{
		client.InNamespace(pipelineRun.Namespace),
		client.MatchingLabels{
			gitops.BuildPipelineRunNameLabel: pipelineRun.Name,
		},
	}

	err := c.List(ctx, snapshots, opts...)
	if err != nil {
		return nil, err
	}
	return &snapshots.Items, nil
}

// GetAllSnapshotsForPR returns all Snapshots for the associated Pull Request.
// In the case the List operation fails, an error will be returned.
// gitops.PipelineAsCodePullRequestAnnotation is also a label
func (l *loader) GetAllSnapshotsForPR(ctx context.Context, c client.Client, application *applicationapiv1alpha1.Application, componentName, pullRequest string) (*[]applicationapiv1alpha1.Snapshot, error) {
	snapshots := &applicationapiv1alpha1.SnapshotList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingLabels{
			gitops.PipelineAsCodePullRequestAnnotation: pullRequest,
			gitops.SnapshotComponentLabel:              componentName,
		},
	}

	err := c.List(ctx, snapshots, opts...)
	if err != nil {
		return nil, err
	}
	return &snapshots.Items, nil
}

// GetAllTaskRunsWithMatchingPipelineRunLabel finds all Child TaskRuns
// whose "tekton.dev/pipeline" label points to the given PipelineRun
func (l *loader) GetAllTaskRunsWithMatchingPipelineRunLabel(ctx context.Context, c client.Client, pipelineRun *tektonv1.PipelineRun) (*[]tektonv1.TaskRun, error) {
	taskRuns := &tektonv1.TaskRunList{}
	opts := []client.ListOption{
		client.InNamespace(pipelineRun.Namespace),
		client.MatchingLabels{
			"tekton.dev/pipelineRun": pipelineRun.Name,
		},
	}

	err := c.List(ctx, taskRuns, opts...)
	if err != nil {
		return nil, err
	}

	return &taskRuns.Items, nil
}

// GetPipelineRun returns Tekton pipelineRun requested by name and namespace
func (l *loader) GetPipelineRun(ctx context.Context, c client.Client, name, namespace string) (*tektonv1.PipelineRun, error) {
	pipelineRun := &tektonv1.PipelineRun{}
	return pipelineRun, toolkit.GetObject(name, namespace, c, ctx, pipelineRun)
}

// GetComponent returns application component requested by name and namespace
func (l *loader) GetComponent(ctx context.Context, c client.Client, name, namespace string) (*applicationapiv1alpha1.Component, error) {
	component := &applicationapiv1alpha1.Component{}
	return component, toolkit.GetObject(name, namespace, c, ctx, component)
}

// GetPipelineRunsWithPRGroupHash gets the build pipelineRun with the given pr group hash string and the same namespace with the given snapshot
func (l *loader) GetPipelineRunsWithPRGroupHash(ctx context.Context, adapterClient client.Client, namespace, prGroupHash string) (*[]tektonv1.PipelineRun, error) {
	buildPipelineRuns := &tektonv1.PipelineRunList{}

	evnentTypeLabelRequirement, err := labels.NewRequirement("pipelinesascode.tekton.dev/event-type", selection.NotIn, []string{"push", "Push"})
	if err != nil {
		return nil, err
	}
	prGroupLabelRequirement, err := labels.NewRequirement("test.appstudio.openshift.io/pr-group-sha", selection.In, []string{prGroupHash})
	if err != nil {
		return nil, err
	}
	plrTypeLabelRequirement, err := labels.NewRequirement("pipelines.appstudio.openshift.io/type", selection.In, []string{"build"})
	if err != nil {
		return nil, err
	}

	labelSelector := labels.NewSelector().
		Add(*evnentTypeLabelRequirement).
		Add(*prGroupLabelRequirement).
		Add(*plrTypeLabelRequirement)

	opts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	err = adapterClient.List(ctx, buildPipelineRuns, opts)
	if err != nil {
		return nil, err
	}
	return &buildPipelineRuns.Items, nil
}

// GetMatchingComponentSnapshotsForComponentAndPRGroupHash gets the component snapshot with the given pr group hash string and the the same namespace with the given snapshot
func (l *loader) GetMatchingComponentSnapshotsForComponentAndPRGroupHash(ctx context.Context, c client.Client, namespace, componentName, prGroupHash string) (*[]applicationapiv1alpha1.Snapshot, error) {
	snapshots := &applicationapiv1alpha1.SnapshotList{}

	eventTypeLabelRequirement, err := labels.NewRequirement("pac.test.appstudio.openshift.io/event-type", selection.NotIn, []string{"push", "Push"})
	if err != nil {
		return nil, err
	}
	componentLabelRequirement, err := labels.NewRequirement("appstudio.openshift.io/component", selection.In, []string{componentName})
	if err != nil {
		return nil, err
	}
	prGroupLabelRequirement, err := labels.NewRequirement("test.appstudio.openshift.io/pr-group-sha", selection.In, []string{prGroupHash})
	if err != nil {
		return nil, err
	}
	snapshotTypeLabelRequirement, err := labels.NewRequirement("test.appstudio.openshift.io/type", selection.In, []string{"component"})
	if err != nil {
		return nil, err
	}

	labelSelector := labels.NewSelector().
		Add(*eventTypeLabelRequirement).
		Add(*componentLabelRequirement).
		Add(*prGroupLabelRequirement).
		Add(*snapshotTypeLabelRequirement)

	opts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	err = c.List(ctx, snapshots, opts)
	if err != nil {
		return nil, err
	}
	return &snapshots.Items, nil
}

// GetMatchingGroupSnapshotsForPRGroupHash gets the group snapshots with the given pr group hash string and the the same namespace
func (l *loader) GetMatchingGroupSnapshotsForPRGroupHash(ctx context.Context, c client.Client, namespace, prGroupHash string) (*[]applicationapiv1alpha1.Snapshot, error) {
	snapshots := &applicationapiv1alpha1.SnapshotList{}

	eventTypeLabelRequirement, err := labels.NewRequirement("pac.test.appstudio.openshift.io/event-type", selection.NotIn, []string{"push", "Push"})
	if err != nil {
		return nil, err
	}
	prGroupLabelRequirement, err := labels.NewRequirement("test.appstudio.openshift.io/pr-group-sha", selection.In, []string{prGroupHash})
	if err != nil {
		return nil, err
	}
	snapshotTypeLabelRequirement, err := labels.NewRequirement("test.appstudio.openshift.io/type", selection.In, []string{"group"})
	if err != nil {
		return nil, err
	}

	labelSelector := labels.NewSelector().
		Add(*eventTypeLabelRequirement).
		Add(*prGroupLabelRequirement).
		Add(*snapshotTypeLabelRequirement)

	opts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	err = c.List(ctx, snapshots, opts)
	if err != nil {
		return nil, err
	}
	return &snapshots.Items, nil
}

// GetMatchingComponentSnapshotsForPRGroupHash gets the component snapshot with the given pr group hash string and the the same namespace with the given snapshot
func (l *loader) GetMatchingComponentSnapshotsForPRGroupHash(ctx context.Context, c client.Client, namespace, prGroupHash string) (*[]applicationapiv1alpha1.Snapshot, error) {
	snapshots := &applicationapiv1alpha1.SnapshotList{}

	eventTypeLabelRequirement, err := labels.NewRequirement("pac.test.appstudio.openshift.io/event-type", selection.NotIn, []string{"push", "Push"})
	if err != nil {
		return nil, err
	}
	prGroupLabelRequirement, err := labels.NewRequirement("test.appstudio.openshift.io/pr-group-sha", selection.In, []string{prGroupHash})
	if err != nil {
		return nil, err
	}
	snapshotTypeLabelRequirement, err := labels.NewRequirement("test.appstudio.openshift.io/type", selection.In, []string{"component"})
	if err != nil {
		return nil, err
	}

	labelSelector := labels.NewSelector().
		Add(*eventTypeLabelRequirement).
		Add(*prGroupLabelRequirement).
		Add(*snapshotTypeLabelRequirement)

	opts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	err = c.List(ctx, snapshots, opts)
	if err != nil {
		return nil, err
	}
	return &snapshots.Items, nil
}

// function GetAllIntegrationPipelineRunsForSnapshot returns list of integration pipelineruns that matches the required labels specified in the function
func (l *loader) GetAllIntegrationPipelineRunsForSnapshot(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot) ([]tektonv1.PipelineRun, error) {
	integrationPipelineRuns := &tektonv1.PipelineRunList{}
	opts := []client.ListOption{
		client.InNamespace(snapshot.Namespace),
		client.MatchingLabels{
			"pipelines.appstudio.openshift.io/type": "test",
			"appstudio.openshift.io/snapshot":       snapshot.Name,
		},
	}
	err := adapterClient.List(ctx, integrationPipelineRuns, opts...)
	if err != nil {
		return nil, err
	}

	return integrationPipelineRuns.Items, err
}

// GetComponentsFromSnapshotForPRGroup returns the component names affected by the given pr group hash
func (l *loader) GetComponentsFromSnapshotForPRGroup(ctx context.Context, client client.Client, namespace, prGroup, prGroupHash string) ([]string, error) {
	snapshots, err := l.GetMatchingComponentSnapshotsForPRGroupHash(ctx, client, namespace, prGroupHash)
	if err != nil {
		return nil, err
	}

	var componentNames []string
	for _, snapshot := range *snapshots {
		componentName := snapshot.Labels[gitops.SnapshotComponentLabel]
		if slices.Contains(componentNames, componentName) {
			continue
		}
		componentNames = append(componentNames, componentName)
	}
	return componentNames, nil
}
