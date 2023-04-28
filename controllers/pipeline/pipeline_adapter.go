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
	"strconv"
	"strings"
	"time"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	h "github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/status"
	"github.com/redhat-appstudio/integration-service/tekton"
	"github.com/redhat-appstudio/operator-goodies/reconciler"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Pipeline.
type Adapter struct {
	pipelineRun *tektonv1beta1.PipelineRun
	component   *applicationapiv1alpha1.Component
	application *applicationapiv1alpha1.Application
	logger      h.IntegrationLogger
	client      client.Client
	context     context.Context
	status      status.Status
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(pipelineRun *tektonv1beta1.PipelineRun, component *applicationapiv1alpha1.Component, application *applicationapiv1alpha1.Application, logger h.IntegrationLogger, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		pipelineRun: pipelineRun,
		component:   component,
		application: application,
		logger:      logger,
		client:      client,
		context:     context,
		status:      status.NewAdapter(logger.Logger, client),
	}
}

// EnsureSnapshotExists is an operation that will ensure that a pipeline Snapshot associated
// to the PipelineRun being processed exists. Otherwise, it will create a new pipeline Snapshot.
func (a *Adapter) EnsureSnapshotExists() (reconciler.OperationResult, error) {
	if !tekton.IsBuildPipelineRun(a.pipelineRun) || !h.HasPipelineRunSucceeded(a.pipelineRun) {
		return reconciler.ContinueProcessing()
	}

	if a.component == nil {
		a.logger.Info("The pipelineRun does not have any component associated with it, will not create a new Snapshot.")
		return reconciler.ContinueProcessing()
	}

	isLatest, err := a.isLatestSucceededPipelineRun()
	if err != nil {
		return reconciler.RequeueWithError(err)
	}
	if !isLatest {
		// not the last started pipeline that succeeded for current snapshot
		// this prevents deploying older pipeline run over new deployment
		a.logger.Info("The pipelineRun is not the latest succeded pipelineRun for the component, skipping creation of a new Snapshot ",
			"PipelineRun.Namespace", a.pipelineRun.Namespace,
			"PipelineRun.Name", a.pipelineRun.Name,
			"Component.Name", a.component.Name)
		return reconciler.ContinueProcessing()
	}

	expectedSnapshot, err := a.prepareSnapshotForPipelineRun(a.pipelineRun, a.component, a.application)
	if err != nil {
		return reconciler.RequeueWithError(err)
	}
	existingSnapshot, err := gitops.FindMatchingSnapshot(a.client, a.context, a.application, expectedSnapshot)
	if err != nil {
		return reconciler.RequeueWithError(err)
	}

	if existingSnapshot != nil {
		a.logger.Info("Found existing Snapshot",
			"Application.Name", a.application.Name,
			"Snapshot.Name", existingSnapshot.Name,
			"Snapshot.Spec.Components", existingSnapshot.Spec.Components)
		return reconciler.ContinueProcessing()
	}

	err = a.client.Create(a.context, expectedSnapshot)
	if err != nil {
		a.logger.Error(err, "Failed to create Snapshot",
			"Application.Name", a.application.Name, "Application.Namespace", a.application.Namespace)
		return reconciler.RequeueWithError(err)
	}

	a.logger.LogAuditEvent("Created new Snapshot", expectedSnapshot, h.LogActionAdd,
		"Application.Name", a.application.Name,
		"Snapshot.Name", expectedSnapshot.Name,
		"Snapshot.Spec.Components", expectedSnapshot.Spec.Components)

	return reconciler.ContinueProcessing()
}

// EnsureSnapshotPassedAllTests is an operation that will ensure that a pipeline Snapshot
// to the PipelineRun being processed passed all tests for all defined non-optional IntegrationTestScenarios.
func (a *Adapter) EnsureSnapshotPassedAllTests() (reconciler.OperationResult, error) {
	if !tekton.IsIntegrationPipelineRun(a.pipelineRun) || !h.HasPipelineRunSucceeded(a.pipelineRun) {
		return reconciler.ContinueProcessing()
	}

	existingSnapshot, err := a.getSnapshotFromPipelineRun(a.pipelineRun)
	if err != nil {
		return reconciler.RequeueWithError(err)
	}
	if existingSnapshot != nil {
		a.logger.Info("Found existing Snapshot",
			"Application.Name", a.application.Name,
			"Snapshot.Name", existingSnapshot.Name,
			"Snapshot.Spec.Components", existingSnapshot.Spec.Components)
	}

	// Get all integrationTestScenarios for the Application and then find the latest Succeeded Integration PipelineRuns
	// for the Snapshot
	integrationTestScenarios, err := h.GetRequiredIntegrationTestScenariosForApplication(a.client, a.context, a.application)
	if err != nil {
		return reconciler.RequeueWithError(err)
	}
	integrationPipelineRuns, err := a.getAllPipelineRunsForSnapshot(existingSnapshot, integrationTestScenarios)
	if err != nil {
		a.logger.Error(err, "Failed to get Integration PipelineRuns",
			"Snapshot.Name", existingSnapshot.Name)
		return reconciler.RequeueWithError(err)
	}

	// Skip doing anything if not all Integration PipelineRuns were found for all integrationTestScenarios
	if len(*integrationTestScenarios) != len(*integrationPipelineRuns) {
		a.logger.Info("Not all required Integration PipelineRuns finished",
			"Snapshot.Name", existingSnapshot.Name,
			"Snapshot.Spec.Components", existingSnapshot.Spec.Components)
		return reconciler.ContinueProcessing()
	}

	// Set the Snapshot Integration status as finished, but don't update the resource yet
	gitops.SetSnapshotIntegrationStatusAsFinished(existingSnapshot,
		"Snapshot integration status condition is finished since all testing pipelines completed")
	a.logger.LogAuditEvent("Snapshot integration status condition marked as finished, all testing pipelines completed",
		existingSnapshot, h.LogActionUpdate)

	// Go into the individual PipelineRun task results for each Integration PipelineRun
	// and determine if all of them passed (or were skipped)
	allIntegrationPipelineRunsPassed, err := a.determineIfAllIntegrationPipelinesPassed(integrationPipelineRuns)
	if err != nil {
		a.logger.Error(err, "Failed to determine outcomes for Integration PipelineRuns",
			"Snapshot.Name", existingSnapshot.Name)
		return reconciler.RequeueWithError(err)
	}

	// If the snapshot is a component type, check if the global component list changed in the meantime and
	// create a composite snapshot if it did. Does not apply for PAC pull request events.
	if a.component != nil && h.HasLabelWithValue(existingSnapshot, gitops.SnapshotTypeLabel, gitops.SnapshotComponentType) && !gitops.IsSnapshotCreatedByPACPullRequestEvent(existingSnapshot) {
		compositeSnapshot, err := a.createCompositeSnapshotsIfConflictExists(a.application, a.component, existingSnapshot)
		if err != nil {
			a.logger.Error(err, "Failed to determine if a composite snapshot needs to be created because of a conflict",
				"Snapshot.Name", existingSnapshot.Name)
			return reconciler.RequeueWithError(err)
		}

		if compositeSnapshot != nil {
			a.logger.Info("The global component list has changed in the meantime, marking snapshot as Invalid",
				"Application.Name", a.application.Name,
				"Snapshot.Name", existingSnapshot.Name)
			gitops.SetSnapshotIntegrationStatusAsInvalid(existingSnapshot,
				"The global component list has changed in the meantime, superseding with a composite snapshot")
			a.logger.LogAuditEvent("Snapshot integration status condition marked as invalid, the global component list has changed in the meantime",
				existingSnapshot, h.LogActionUpdate)
		}
	}

	// If all Integration Pipeline runs passed, mark the snapshot as succeeded, otherwise mark it as failed
	// This updates the Snapshot resource on the cluster
	if allIntegrationPipelineRunsPassed {
		existingSnapshot, err = gitops.MarkSnapshotAsPassed(a.client, a.context, existingSnapshot, "All Integration Pipeline tests passed")
		if err != nil {
			a.logger.Error(err, "Failed to Update Snapshot AppStudioTestSucceeded status")
			return reconciler.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("Snapshot integration status condition marked as passed, all Integration PipelineRuns succeeded",
			existingSnapshot, h.LogActionUpdate)
	} else {
		existingSnapshot, err = gitops.MarkSnapshotAsFailed(a.client, a.context, existingSnapshot, "Some Integration pipeline tests failed")
		if err != nil {
			a.logger.Error(err, "Failed to Update Snapshot AppStudioTestSucceeded status")
			return reconciler.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("Snapshot integration status condition marked as failed, some tests within Integration PipelineRuns failed",
			existingSnapshot, h.LogActionUpdate)
	}

	return reconciler.ContinueProcessing()
}

// EnsureStatusReported will ensure that integration PipelineRun status is reported to the git provider
// which (indirectly) triggered its execution.
func (a *Adapter) EnsureStatusReported() (reconciler.OperationResult, error) {
	if !tekton.IsIntegrationPipelineRun(a.pipelineRun) {
		return reconciler.ContinueProcessing()
	}

	reporters, err := a.status.GetReporters(a.pipelineRun)

	if err != nil {
		return reconciler.RequeueWithError(err)
	}

	for _, reporter := range reporters {
		if err := reporter.ReportStatus(a.client, a.context, a.pipelineRun); err != nil {
			return reconciler.RequeueWithError(err)
		}
	}

	return reconciler.ContinueProcessing()
}

// EnsureEphemeralEnvironmentsCleanedUp will ensure that ephemeral environment(s) associated with the
// integration PipelineRun are cleaned up.
func (a *Adapter) EnsureEphemeralEnvironmentsCleanedUp() (reconciler.OperationResult, error) {
	if !tekton.IsIntegrationPipelineRun(a.pipelineRun) || !h.HasPipelineRunFinished(a.pipelineRun) {
		return reconciler.ContinueProcessing()
	}

	testEnvironment, err := a.getEnvironmentFromIntegrationPipelineRun(a.context, a.pipelineRun)
	if err != nil {
		a.logger.Error(err, "Failed to find the environment for the pipelineRun")
		return reconciler.RequeueWithError(err)
	} else if testEnvironment == nil {
		a.logger.Info("The pipelineRun does not have any test Environments associated with it, skipping cleanup.")
		return reconciler.ContinueProcessing()
	}

	isEphemeral := false
	for _, tag := range testEnvironment.Spec.Tags {
		if tag == "ephemeral" {
			isEphemeral = true
			break
		}
	}

	if isEphemeral {
		dtc, err := gitops.GetDeploymentTargetClaimForEnvironment(a.client, a.context, testEnvironment)
		if err != nil || dtc == nil {
			a.logger.Error(err, "Failed to find deploymentTargetClaim defined in environment", "environment.Name", testEnvironment.Name)
			return reconciler.RequeueWithError(err)
		}

		dt, err := gitops.GetDeploymentTargetForDeploymentTargetClaim(a.client, a.context, dtc)
		if err != nil || dt == nil {
			a.logger.Error(err, "Failed to find deploymentTarget defined in deploymentTargetClaim", "deploymentTargetClaim.Name", dtc.Name)
			return reconciler.RequeueWithError(err)
		}

		binding, err := gitops.FindExistingSnapshotEnvironmentBinding(a.client, a.context, a.application, testEnvironment)
		if err != nil || binding == nil {
			a.logger.Error(err, "Failed to find snapshotEnvironmentBinding associated with environment", "environment.Name", testEnvironment.Name)
			return reconciler.RequeueWithError(err)
		}

		a.logger.Info("Deleting deploymentTarget", "deploymentTarget.Name", dt.Name)
		err = a.client.Delete(a.context, dt)
		if err != nil {
			a.logger.Error(err, "Failed to delete the deploymentTarget!")
			return reconciler.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("DeploymentTarget deleted", dt, h.LogActionDelete)

		a.logger.Info("Deleting deploymentTargetClaim", "deploymentTargetClaim.Name", dtc.Name)
		err = a.client.Delete(a.context, dtc)
		if err != nil {
			a.logger.Error(err, "Failed to delete the deploymentTargetClaim!")
			return reconciler.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("DeploymentTargetClaim deleted", dtc, h.LogActionDelete)

		a.logger.Info("Deleting environment", "environment.Name", testEnvironment.Name)
		err = a.client.Delete(a.context, testEnvironment)
		if err != nil {
			a.logger.Error(err, "Failed to delete the test ephemeral environment!")
			return reconciler.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("Ephemeral environment deleted", testEnvironment, h.LogActionDelete)

		a.logger.Info("Deleting snapshotEnvironmentBinding", "binding.Name", binding.Name)
		err = a.client.Delete(a.context, binding)
		if err != nil {
			a.logger.Error(err, "Failed to delete the snapshotEnvironmentBinding!")
			return reconciler.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("SnapshotEnvironmentBinding deleted", binding, h.LogActionDelete)
	} else {
		a.logger.Info("The pipelineRun test Environment is not ephemeral, skipping cleanup.")
	}

	return reconciler.ContinueProcessing()
}

// getEnvironmentFromIntegrationPipelineRun loads from the cluster the Environment referenced in the given PipelineRun.
// If the PipelineRun doesn't specify an Environment or this is not found in the cluster, an error will be returned.
func (a *Adapter) getEnvironmentFromIntegrationPipelineRun(context context.Context, pipelineRun *tektonv1beta1.PipelineRun) (*applicationapiv1alpha1.Environment, error) {
	if environmentLabel, ok := pipelineRun.Labels[tekton.EnvironmentNameLabel]; ok {
		environment := &applicationapiv1alpha1.Environment{}
		err := a.client.Get(context, types.NamespacedName{
			Namespace: pipelineRun.Namespace,
			Name:      environmentLabel,
		}, environment)

		if err != nil {
			return nil, err
		}

		return environment, nil
	} else {
		return nil, nil
	}
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
// In case the Image pullspec can't be composed, an error will be returned.
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

// getComponentSourceFromPipelineRun gets the component Git Source for the Component built in the given build PipelineRun,
// In case the Git Source can't be composed, an error will be returned.
func (a *Adapter) getComponentSourceFromPipelineRun(pipelineRun *tektonv1beta1.PipelineRun) (*applicationapiv1alpha1.ComponentSource, error) {
	componentSourceGitUrl, err := tekton.GetComponentSourceGitUrl(pipelineRun)
	if err != nil {
		return nil, err
	}
	componentSourceGitCommit, err := tekton.GetComponentSourceGitCommit(pipelineRun)
	if err != nil {
		return nil, err
	}
	componentSource := applicationapiv1alpha1.ComponentSource{
		ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
			GitSource: &applicationapiv1alpha1.GitSource{
				URL:      componentSourceGitUrl,
				Revision: componentSourceGitCommit,
			},
		},
	}

	return &componentSource, nil
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

// getComponentSourceFromSnapshotComponent gets the component source from the given Snapshot for the given Component,
func (a *Adapter) getComponentSourceFromSnapshotComponent(snapshot *applicationapiv1alpha1.Snapshot, component *applicationapiv1alpha1.Component) (*applicationapiv1alpha1.ComponentSource, error) {
	for _, snapshotComponent := range snapshot.Spec.Components {
		if snapshotComponent.Name == component.Name {
			return &snapshotComponent.Source, nil
		}
	}
	return nil, fmt.Errorf("couldn't find the requested component source info in the given Snapshot")
}

// getComponentSourceFromComponent gets the component source from the given Component as Revision
// and set Component.Status.LastBuiltCommit as Component.Source.GitSource.Revision if it is defined.
func (a *Adapter) getComponentSourceFromComponent(component *applicationapiv1alpha1.Component) *applicationapiv1alpha1.ComponentSource {
	componentSource := component.Spec.Source.DeepCopy()
	if component.Status.LastBuiltCommit != "" {
		componentSource.GitSource.Revision = component.Status.LastBuiltCommit
	}
	return componentSource
}

// determineIfAllIntegrationPipelinesPassed checks all Integration pipelines passed all of their test tasks.
// Returns an error if it can't get the PipelineRun outcomes
func (a *Adapter) determineIfAllIntegrationPipelinesPassed(integrationPipelineRuns *[]tektonv1beta1.PipelineRun) (bool, error) {
	allIntegrationPipelineRunsPassed := true
	for _, integrationPipelineRun := range *integrationPipelineRuns {
		integrationPipelineRun := integrationPipelineRun // G601
		pipelineRunOutcome, err := h.CalculateIntegrationPipelineRunOutcome(a.client, a.context, a.logger.Logger, &integrationPipelineRun)
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
			integrationPipelineRun, err := h.GetLatestPipelineRunForSnapshotAndScenario(a.client, a.context, snapshot, &integrationTestScenario)
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
func (a *Adapter) prepareSnapshot(application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, newContainerImage string, newComponentSource *applicationapiv1alpha1.ComponentSource) (*applicationapiv1alpha1.Snapshot, error) {
	applicationComponents, err := a.getAllApplicationComponents(application)
	if err != nil {
		return nil, fmt.Errorf("failed to get all Application Components for Application %s", a.application.Name)
	}

	var snapshotComponents []applicationapiv1alpha1.SnapshotComponent
	for _, applicationComponent := range *applicationComponents {
		applicationComponent := applicationComponent // G601
		containerImage := applicationComponent.Spec.ContainerImage

		var componentSource *applicationapiv1alpha1.ComponentSource
		if applicationComponent.Name == component.Name {
			containerImage = newContainerImage
			componentSource = newComponentSource
		} else {
			// Get ComponentSource for the component which is not built in this pipeline
			componentSource = a.getComponentSourceFromComponent(&applicationComponent)
		}

		// If containerImage is empty, we have run into a race condition in
		// which multiple components are being built in close succession.
		// We omit this not-yet-built component from the snapshot rather than
		// including a component that is incomplete.
		if containerImage == "" {
			continue
		}
		snapshotComponents = append(snapshotComponents, applicationapiv1alpha1.SnapshotComponent{
			Name:           applicationComponent.Name,
			ContainerImage: containerImage,
			Source:         *componentSource,
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
	componentSource, err := a.getComponentSourceFromPipelineRun(pipelineRun)
	if err != nil {
		return nil, err
	}

	snapshot, err := a.prepareSnapshot(application, component, newContainerImage, componentSource)
	if err != nil {
		return nil, err
	}

	if snapshot.Labels == nil {
		snapshot.Labels = make(map[string]string)
	}
	snapshot.Labels[gitops.SnapshotTypeLabel] = gitops.SnapshotComponentType
	snapshot.Labels[gitops.SnapshotComponentLabel] = a.component.Name
	snapshot.Labels[gitops.BuildPipelineRunNameLabel] = pipelineRun.Name
	if pipelineRun.Status.CompletionTime != nil {
		snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] = strconv.FormatInt(pipelineRun.Status.CompletionTime.Time.Unix(), 10)
	} else {
		snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] = strconv.FormatInt(time.Now().Unix(), 10)
	}

	// Copy PipelineRun PAC annotations/labels from Build to snapshot.
	// Modify the prefix so the PaC controller won't react to PipelineRuns generated from the snapshot.
	h.CopyLabelsByPrefix(&pipelineRun.ObjectMeta, &snapshot.ObjectMeta, "pipelinesascode.tekton.dev", gitops.PipelinesAsCodePrefix)
	h.CopyAnnotationsByPrefix(&pipelineRun.ObjectMeta, &snapshot.ObjectMeta, "pipelinesascode.tekton.dev", gitops.PipelinesAsCodePrefix)

	return snapshot, nil
}

// prepareCompositeSnapshot prepares the Snapshot for a given application,
// componentnew, containerImage and newContainerSource. In case the Snapshot can't be created, an error will be returned.
func (a *Adapter) prepareCompositeSnapshot(application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, newContainerImage string, newComponentSource *applicationapiv1alpha1.ComponentSource) (*applicationapiv1alpha1.Snapshot, error) {
	snapshot, err := a.prepareSnapshot(application, component, newContainerImage, newComponentSource)
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

	newComponentSource, err := a.getComponentSourceFromSnapshotComponent(testedSnapshot, component)
	if err != nil {
		return nil, err
	}

	compositeSnapshot, err := a.prepareCompositeSnapshot(application, component, newContainerImage, newComponentSource)
	if err != nil {
		return nil, err
	}

	// Copy PAC annotations/labels from testedSnapshot to compositeSnapshot.
	h.CopyLabelsByPrefix(&testedSnapshot.ObjectMeta, &compositeSnapshot.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
	h.CopyAnnotationsByPrefix(&testedSnapshot.ObjectMeta, &compositeSnapshot.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)

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
			a.logger.LogAuditEvent("CompositeSnapshot created", compositeSnapshot, h.LogActionAdd,
				"Application.Name", a.application.Name,
				"Snapshot.Spec.Components", compositeSnapshot.Spec.Components)
			return compositeSnapshot, nil
		}
	}

	return nil, nil
}

// isLatestSucceededPipelineRun return true if pipelineRun is the latest succeded pipelineRun
// for the component. Pipeline start timestamp is used for comparison because we care about
// time when pipeline was created.
func (a *Adapter) isLatestSucceededPipelineRun() (bool, error) {

	pipelineStartTime := a.pipelineRun.CreationTimestamp.Time

	pipelineRuns, err := h.GetSucceededBuildPipelineRunsForComponent(a.client, a.context, a.component)
	if err != nil {
		return false, err
	}
	for _, run := range *pipelineRuns {
		if a.pipelineRun.Name == run.Name {
			// it's the same pipeline
			continue
		}
		timestamp := run.CreationTimestamp.Time
		if pipelineStartTime.Before(timestamp) {
			// pipeline is not the latest
			// 1 second is minimal granularity, if both pipelines started at the same second, we cannot decide
			return false, nil
		}
	}
	return true, nil
}
