/*
Copyright 2022.

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

package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/controllers/results"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/status"
	"github.com/redhat-appstudio/integration-service/tekton"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	pipelineRun *tektonv1beta1.PipelineRun
	component   *applicationapiv1alpha1.Component
	application *applicationapiv1alpha1.Application
	logger      logr.Logger
	client      client.Client
	context     context.Context
	status      status.Status
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(pipelineRun *tektonv1beta1.PipelineRun, component *applicationapiv1alpha1.Component, application *applicationapiv1alpha1.Application, logger logr.Logger, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		pipelineRun: pipelineRun,
		component:   component,
		application: application,
		logger:      logger,
		client:      client,
		context:     context,
		status:      status.NewAdapter(logger, client),
	}
}

// EnsureSnapshotExists is an operation that will ensure that a pipeline Snapshot associated
// to the PipelineRun being processed exists. Otherwise, it will create a new pipeline Snapshot.
func (a *Adapter) EnsureSnapshotExists() (results.OperationResult, error) {
	if !tekton.IsBuildPipelineRun(a.pipelineRun) || !tekton.HasPipelineRunSucceeded(a.pipelineRun) {
		return results.ContinueProcessing()
	}

	if a.component == nil {
		a.logger.Info("The pipelineRun does not have any component associated with it, will not create a new Snapshot.")
		return results.ContinueProcessing()
	}

	expectedSnapshot, err := a.prepareSnapshotForPipelineRun(a.pipelineRun, a.component, a.application)
	if err != nil {
		return results.RequeueWithError(err)
	}
	existingSnapshot, err := gitops.FindMatchingSnapshot(a.client, a.context, a.application, expectedSnapshot)
	if err != nil {
		return results.RequeueWithError(err)
	}

	if existingSnapshot != nil {
		a.logger.Info("Found existing Snapshot",
			"Application.Name", a.application.Name,
			"Snapshot.Name", existingSnapshot.Name,
			"Snapshot.Spec.Components", existingSnapshot.Spec.Components)
		return results.ContinueProcessing()
	}

	err = a.client.Create(a.context, expectedSnapshot)
	if err != nil {
		a.logger.Error(err, "Failed to create Snapshot",
			"Application.Name", a.application.Name, "Application.Namespace", a.application.Namespace)
		return results.RequeueWithError(err)
	}

	a.logger.Info("Created new Snapshot",
		"Application.Name", a.application.Name,
		"Snapshot.Name", expectedSnapshot.Name,
		"Snapshot.Spec.Components", expectedSnapshot.Spec.Components)

	return results.ContinueProcessing()
}

// EnsureSnapshotPassedAllTests is an operation that will ensure that a pipeline Snapshot
// to the PipelineRun being processed passed all tests for all defined non-optional IntegrationTestScenarios.
func (a *Adapter) EnsureSnapshotPassedAllTests() (results.OperationResult, error) {
	if !tekton.IsIntegrationPipelineRun(a.pipelineRun) || !tekton.HasPipelineRunSucceeded(a.pipelineRun) {
		return results.ContinueProcessing()
	}

	existingSnapshot, err := a.getSnapshotFromPipelineRun(a.pipelineRun)
	if err != nil {
		return results.RequeueWithError(err)
	}
	if existingSnapshot != nil {
		a.logger.Info("Found existing Snapshot",
			"Application.Name", a.application.Name,
			"Snapshot.Name", existingSnapshot.Name,
			"Snapshot.Spec.Components", existingSnapshot.Spec.Components)
	}

	// Get all integrationTestScenarios for the Application and then find the latest Succeeded Integration PipelineRuns
	// for the Snapshot
	integrationTestScenarios, err := helpers.GetRequiredIntegrationTestScenariosForApplication(a.client, a.context, a.application)
	if err != nil {
		return results.RequeueWithError(err)
	}
	integrationPipelineRuns, err := a.getAllPipelineRunsForSnapshot(existingSnapshot, integrationTestScenarios)
	if err != nil {
		a.logger.Error(err, "Failed to get Integration PipelineRuns",
			"Snapshot.Name", existingSnapshot.Name)
		return results.RequeueWithError(err)
	}

	// Skip doing anything if not all Integration PipelineRuns were found for all integrationTestScenarios
	if len(*integrationTestScenarios) != len(*integrationPipelineRuns) {
		a.logger.Info("Not all required Integration PipelineRuns finished",
			"Snapshot.Name", existingSnapshot.Name,
			"Snapshot.Spec.Components", existingSnapshot.Spec.Components)
		return results.ContinueProcessing()
	}

	// Go into the individual PipelineRun task results for each Integration PipelineRun
	// and determine if all of them passed (or were skipped)
	allIntegrationPipelineRunsPassed, err := a.determineIfAllIntegrationPipelinesPassed(integrationPipelineRuns)
	if err != nil {
		a.logger.Error(err, "Failed to determine outcomes for Integration PipelineRuns",
			"Snapshot.Name", existingSnapshot.Name)
		return results.RequeueWithError(err)
	}

	// If the snapshot is a component type, check if the global component list changed in the meantime and
	// create a composite snapshot if it did. Does not apply for PAC pull request events.
	if a.component != nil && helpers.HasLabelWithValue(existingSnapshot, gitops.SnapshotTypeLabel, gitops.SnapshotComponentType) && !gitops.IsSnapshotCreatedByPACPullRequestEvent(existingSnapshot) {
		compositeSnapshot, err := a.createCompositeSnapshotsIfConflictExists(a.application, a.component, existingSnapshot)
		if err != nil {
			a.logger.Error(err, "Failed to determine if a composite snapshot needs to be created because of a conflict",
				"Snapshot.Name", existingSnapshot.Name)
			return results.RequeueWithError(err)
		}
		if compositeSnapshot != nil {
			existingSnapshot, err := gitops.MarkSnapshotAsFailed(a.client, a.context, existingSnapshot,
				"The global component list has changed in the meantime, superseding with a composite snapshot")
			if err != nil {
				a.logger.Error(err, "Failed to Update Snapshot StonesoupTestSucceeded status")
				return results.RequeueWithError(err)
			}
			a.logger.Info("The global component list has changed in the meantime, marking snapshot as failed",
				"Application.Name", a.application.Name,
				"Snapshot.Name", existingSnapshot.Name)
			return results.ContinueProcessing()
		}
	}

	// If all Integration Pipeline runs passed, mark the snapshot as succeeded, otherwise mark it as failed
	if allIntegrationPipelineRunsPassed {
		existingSnapshot, err = gitops.MarkSnapshotAsPassed(a.client, a.context, existingSnapshot, "All Integration Pipeline tests passed")
		if err != nil {
			a.logger.Error(err, "Failed to Update Snapshot StonesoupTestSucceeded status")
			return results.RequeueWithError(err)
		}
		a.logger.Info("All Integration PipelineRuns succeeded, marking Snapshot as succeeded",
			"Application.Name", a.application.Name,
			"Snapshot.Name", existingSnapshot.Name)
	} else {
		existingSnapshot, err = gitops.MarkSnapshotAsFailed(a.client, a.context, existingSnapshot, "Some Integration pipeline tests failed")
		if err != nil {
			a.logger.Error(err, "Failed to Update Snapshot StonesoupTestSucceeded status")
			return results.RequeueWithError(err)
		}
		a.logger.Info("Some tests within Integration PipelineRuns failed, marking Snapshot as failed",
			"Application.Name", a.application.Name,
			"Snapshot.Name", existingSnapshot.Name)
	}

	return results.ContinueProcessing()
}

// EnsureStatusReported will ensure that integration PipelineRun status is reported to the git provider
// which (indirectly) triggered its execution.
func (a *Adapter) EnsureStatusReported() (results.OperationResult, error) {
	if !tekton.IsIntegrationPipelineRun(a.pipelineRun) {
		return results.ContinueProcessing()
	}

	reporters, err := a.status.GetReporters(a.pipelineRun)

	if err != nil {
		return results.RequeueWithError(err)
	}

	for _, reporter := range reporters {
		if err := reporter.ReportStatus(a.context, a.pipelineRun); err != nil {
			return results.RequeueWithError(err)
		}
	}

	return results.ContinueProcessing()
}

// getAllApplicationComponents loads from the cluster all Components associated with the given Application.
// If the Application doesn't have any Components or this is not found in the cluster, an error will be returned.
func (a *Adapter) getAllApplicationComponents(application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Component, error) {
	applicationComponents := &applicationapiv1alpha1.ComponentList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := a.client.List(a.context, applicationComponents, opts...)
	if err != nil {
		return nil, err
	}

	return &applicationComponents.Items, nil
}

// getImagePullSpecFromPipelineRun gets the full image pullspec from the given build PipelineRun,
// In case the Image pullspec can't be can't be composed, an error will be returned.
func (a *Adapter) getImagePullSpecFromPipelineRun(pipelineRun *tektonv1beta1.PipelineRun) (string, error) {
	outputImage, err := tekton.GetOutputImage(pipelineRun)
	if err != nil {
		return "", err
	}
	imageDigest, err := tekton.GetOutputImageDigest(pipelineRun)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%s", strings.Split(outputImage, ":")[0], imageDigest), nil
}

// getImagePullSpecFromSnapshotComponent gets the full image pullspec from the given Snapshot Component,
func (a *Adapter) getImagePullSpecFromSnapshotComponent(snapshot *applicationapiv1alpha1.Snapshot, component *applicationapiv1alpha1.Component) (string, error) {
	for _, snapshotComponent := range snapshot.Spec.Components {
		if snapshotComponent.Name == component.Name {
			return snapshotComponent.ContainerImage, nil
		}
	}
	return "", fmt.Errorf("couldn't find the requested component info in the given Snapshot")
}

// determineIfAllIntegrationPipelinesPassed checks all Integration pipelines passed all of their test tasks.
// Returns an error if it can't get the PipelineRun outcomes
func (a *Adapter) determineIfAllIntegrationPipelinesPassed(integrationPipelineRuns *[]tektonv1beta1.PipelineRun) (bool, error) {
	allIntegrationPipelineRunsPassed := true
	for _, integrationPipelineRun := range *integrationPipelineRuns {
		integrationPipelineRun := integrationPipelineRun // G601
		pipelineRunOutcome, err := helpers.CalculateIntegrationPipelineRunOutcome(a.logger, &integrationPipelineRun)
		if err != nil {
			a.logger.Error(err, "Failed to get Integration PipelineRun outcome",
				"PipelineRun.Name", integrationPipelineRun.Name, "PipelineRun.Namespace", integrationPipelineRun.Namespace)
			return false, err
		}
		if !pipelineRunOutcome {
			a.logger.Info("Integration PipelineRun did not pass all tests",
				"PipelineRun.Name", integrationPipelineRun.Name, "PipelineRun.Namespace", integrationPipelineRun.Namespace)
			allIntegrationPipelineRunsPassed = false
		}
	}
	return allIntegrationPipelineRunsPassed, nil
}

// getSnapshotFromPipelineRun loads from the cluster the Snapshot referenced in the given PipelineRun.
// If the PipelineRun doesn't specify an Snapshot or this is not found in the cluster, an error will be returned.
func (a *Adapter) getSnapshotFromPipelineRun(pipelineRun *tektonv1beta1.PipelineRun) (*applicationapiv1alpha1.Snapshot, error) {
	if snapshotName, found := pipelineRun.Labels[tekton.SnapshotNameLabel]; found {
		snapshot := &applicationapiv1alpha1.Snapshot{}
		err := a.client.Get(a.context, types.NamespacedName{
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

// getAllPipelineRunsForSnapshot loads from the cluster all Integration PipelineRuns for each IntegrationTestScenario
// associated with the Snapshot. If the Application doesn't have any IntegrationTestScenarios associated with it,
// an error will be returned.
func (a *Adapter) getAllPipelineRunsForSnapshot(snapshot *applicationapiv1alpha1.Snapshot, integrationTestScenarios *[]v1alpha1.IntegrationTestScenario) (*[]tektonv1beta1.PipelineRun, error) {
	var integrationPipelineRuns []tektonv1beta1.PipelineRun
	for _, integrationTestScenario := range *integrationTestScenarios {
		integrationTestScenario := integrationTestScenario // G601
		if a.pipelineRun.Labels[tekton.ScenarioNameLabel] != integrationTestScenario.Name {
			integrationPipelineRun, err := helpers.GetLatestPipelineRunForSnapshotAndScenario(a.client, a.context, snapshot, &integrationTestScenario)
			if err != nil {
				return nil, err
			}
			if integrationPipelineRun != nil {
				a.logger.Info("Found existing integrationPipelineRun",
					"IntegrationTestScenario.Name", integrationTestScenario.Name,
					"integrationPipelineRun.Name", integrationPipelineRun.Name)
				integrationPipelineRuns = append(integrationPipelineRuns, *integrationPipelineRun)
			}
		} else {
			integrationPipelineRuns = append(integrationPipelineRuns, *a.pipelineRun)
			a.logger.Info("The current integrationPipelineRun matches the integration test scenario",
				"IntegrationTestScenario.Name", integrationTestScenario.Name,
				"integrationPipelineRun.Name", a.pipelineRun.Name)
		}
	}

	return &integrationPipelineRuns, nil
}

// prepareSnapshot prepares the Snapshot for a given application and the updated component (if any).
// In case the Snapshot can't be created, an error will be returned.
func (a *Adapter) prepareSnapshot(application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, newContainerImage string) (*applicationapiv1alpha1.Snapshot, error) {
	applicationComponents, err := a.getAllApplicationComponents(application)
	if err != nil {
		return nil, fmt.Errorf("failed to get all Application Components for Application %s", a.application.Name)
	}

	var snapshotComponents []applicationapiv1alpha1.SnapshotComponent
	for _, applicationComponent := range *applicationComponents {
		containerImage := applicationComponent.Spec.ContainerImage
		if applicationComponent.Name == component.Name {
			containerImage = newContainerImage
		}
		snapshotComponents = append(snapshotComponents, applicationapiv1alpha1.SnapshotComponent{
			Name:           applicationComponent.Name,
			ContainerImage: containerImage,
		})
	}

	snapshot := gitops.NewSnapshot(application, &snapshotComponents)

	err = ctrl.SetControllerReference(application, snapshot, a.client.Scheme())
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

// prepareSnapshotForPipelineRun prepares the Snapshot for a given PipelineRun,
// component and application. In case the Snapshot can't be created, an error will be returned.
func (a *Adapter) prepareSnapshotForPipelineRun(pipelineRun *tektonv1beta1.PipelineRun, component *applicationapiv1alpha1.Component, application *applicationapiv1alpha1.Application) (*applicationapiv1alpha1.Snapshot, error) {
	newContainerImage, err := a.getImagePullSpecFromPipelineRun(pipelineRun)
	if err != nil {
		return nil, err
	}

	snapshot, err := a.prepareSnapshot(application, component, newContainerImage)
	if err != nil {
		return nil, err
	}

	if snapshot.Labels == nil {
		snapshot.Labels = make(map[string]string)
	}
	snapshot.Labels[gitops.SnapshotTypeLabel] = gitops.SnapshotComponentType
	snapshot.Labels[gitops.SnapshotComponentLabel] = a.component.Name

	// Copy PipelineRun PAC annotations/labels from Build to snapshot.
	// Modify the prefix so the PaC controller won't react to PipelineRuns generated from the snapshot.
	helpers.CopyLabelsByPrefix(&pipelineRun.ObjectMeta, &snapshot.ObjectMeta, "pipelinesascode.tekton.dev", gitops.PipelinesAsCodePrefix)
	helpers.CopyAnnotationsByPrefix(&pipelineRun.ObjectMeta, &snapshot.ObjectMeta, "pipelinesascode.tekton.dev", gitops.PipelinesAsCodePrefix)

	return snapshot, nil
}

// prepareSnapshotForPipelineRun prepares the Snapshot for a given PipelineRun,
// component and application. In case the Snapshot can't be created, an error will be returned.
func (a *Adapter) prepareCompositeSnapshot(application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, newContainerImage string) (*applicationapiv1alpha1.Snapshot, error) {
	snapshot, err := a.prepareSnapshot(application, component, newContainerImage)
	if err != nil {
		return nil, err
	}

	if snapshot.Labels == nil {
		snapshot.Labels = map[string]string{}
	}
	snapshot.Labels[gitops.SnapshotTypeLabel] = gitops.SnapshotCompositeType

	return snapshot, nil
}

// createCompositeSnapshotsIfConflictExists checks if the component Snapshot is good to release by checking if any
// of the other components containerImages changed in the meantime. If any of them changed, it creates a new composite snapshot.
func (a *Adapter) createCompositeSnapshotsIfConflictExists(application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, testedSnapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Snapshot, error) {
	newContainerImage, err := a.getImagePullSpecFromSnapshotComponent(testedSnapshot, component)
	if err != nil {
		return nil, err
	}

	compositeSnapshot, err := a.prepareCompositeSnapshot(application, component, newContainerImage)
	if err != nil {
		return nil, err
	}

	// Copy PAC annotations/labels from testedSnapshot to compositeSnapshot.
	helpers.CopyLabelsByPrefix(&testedSnapshot.ObjectMeta, &compositeSnapshot.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
	helpers.CopyAnnotationsByPrefix(&testedSnapshot.ObjectMeta, &compositeSnapshot.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)

	// Mark tested snapshot as failed and create the new composite snapshot if it doesn't exist already
	if !gitops.CompareSnapshots(compositeSnapshot, testedSnapshot) {
		existingCompositeSnapshot, err := gitops.FindMatchingSnapshot(a.client, a.context, a.application, compositeSnapshot)
		if err != nil {
			return nil, err
		}

		if existingCompositeSnapshot != nil {
			a.logger.Info("Found existing composite Snapshot",
				"Application.Name", a.application.Name,
				"Snapshot.Name", existingCompositeSnapshot.Name,
				"Snapshot.Spec.Components", existingCompositeSnapshot.Spec.Components)
			return existingCompositeSnapshot, nil
		} else {
			err = a.client.Create(a.context, compositeSnapshot)
			if err != nil {
				return nil, err
			}
			a.logger.Info("Created a new composite Snapshot",
				"Application.Name", a.application.Name,
				"Snapshot.Name", compositeSnapshot.Name,
				"Snapshot.Spec.Components", compositeSnapshot.Spec.Components)
			return compositeSnapshot, nil
		}
	}

	return nil, nil
}
