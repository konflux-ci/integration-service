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

package snapshot

import (
	"context"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/release"
	"github.com/redhat-appstudio/integration-service/tekton"
	"github.com/redhat-appstudio/operator-goodies/reconciler"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	snapshot    *applicationapiv1alpha1.Snapshot
	application *applicationapiv1alpha1.Application
	component   *applicationapiv1alpha1.Component
	logger      logr.Logger
	client      client.Client
	context     context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(snapshot *applicationapiv1alpha1.Snapshot, application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, logger logr.Logger, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		snapshot:    snapshot,
		application: application,
		component:   component,
		logger:      logger,
		client:      client,
		context:     context,
	}
}

// EnsureAllIntegrationTestPipelinesExist is an operation that will ensure that all Integration test pipelines
// associated with the Snapshot and the Application's IntegrationTestScenarios exist.
// Otherwise, it will create new Releases for each ReleasePlan.
func (a *Adapter) EnsureAllIntegrationTestPipelinesExist() (reconciler.OperationResult, error) {
	if gitops.HaveHACBSTestsFinished(a.snapshot) {
		a.logger.Info("The Snapshot has finished testing.")
		return reconciler.ContinueProcessing()
	}

	integrationTestScenarios, err := helpers.GetAllIntegrationTestScenariosForApplication(a.client, a.context, a.application)

	if err != nil {
		a.logger.Error(err, "Failed to get Integration test scenarios for following application",
			"Application.Name", a.application.Name,
			"Application.Namespace", a.application.Namespace)
	}

	if integrationTestScenarios != nil {
		a.logger.Info("Found IntegrationTestScenarios for application",
			"Application.Name", a.application.Name,
			"IntegrationTestScenarios", len(*integrationTestScenarios))
		for _, integrationTestScenario := range *integrationTestScenarios {
			integrationTestScenario := integrationTestScenario //G601
			integrationPipelineRuns, err := helpers.GetAllPipelineRunsForSnapshotAndScenario(a.client, a.context, a.snapshot, &integrationTestScenario)
			if err != nil {
				a.logger.Error(err, "Failed to get pipelineRuns for snapshot and scenario",
					"Snapshot.Name:", a.snapshot.Name,
					"IntegrationTestScenario.Name", integrationTestScenario.Name)
				return reconciler.RequeueOnErrorOrStop(err)
			}
			if integrationPipelineRuns != nil && len(*integrationPipelineRuns) > 0 {
				a.logger.Info("Found existing integrationPipelineRuns",
					"Snapshot.Name", a.snapshot.Name,
					"IntegrationTestScenario.Name", integrationTestScenario.Name,
					"len(integrationPipelineRuns)", len(*integrationPipelineRuns))
			} else {
				a.logger.Info("Creating new pipelinerun for integrationTestscenario",
					"IntegrationTestScenario.Name", integrationTestScenario.Name,
					"app name", a.application.Name,
					"namespace", a.application.Namespace)
				err := a.createIntegrationPipelineRun(a.application, &integrationTestScenario, a.snapshot)
				if err != nil {
					a.logger.Error(err, "Failed to create pipelineRun for snapshot and scenario")
					return reconciler.RequeueOnErrorOrStop(err)
				}
				updatedSnapshot, err := gitops.MarkSnapshotIntegrationStatusAsInProgress(a.client, a.context, a.snapshot, "Snapshot starts being tested by the integrationPipelineRun")
				if err != nil {
					a.logger.Error(err, "Failed to update integration status condition to in progress for snapshot",
						"Snapshot.Name", a.snapshot.Name,
						"Snapshot.Namespace", a.snapshot.Namespace)
				}
				a.logger.Info("IntegrationTestscenario pipeline has been created and integration status condition has been marked as in progress",
					"IntegrationTestScenario.Name", integrationTestScenario.Name,
					"App name", a.application.Name,
					"Snapshot name", updatedSnapshot.Name,
					"Namespace", a.application.Namespace)
			}
		}
	}

	requiredIntegrationTestScenarios, err := helpers.GetRequiredIntegrationTestScenariosForApplication(a.client, a.context, a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get all required IntegrationTestScenarios",
			"Application.Name", a.application.Name,
			"Application.Namespace", a.application.Namespace)
		patch := client.MergeFrom(a.snapshot.DeepCopy())
		gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to get all required IntegrationTestScenarios")
		return reconciler.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
	}
	if len(*requiredIntegrationTestScenarios) == 0 {
		updatedSnapshot, err := gitops.MarkSnapshotAsPassed(a.client, a.context, a.snapshot, "No required IntegrationTestScenarios found, skipped testing")
		if err != nil {
			a.logger.Error(err, "Failed to update Snapshot status",
				"Snapshot.Name", a.snapshot.Name,
				"Snapshot.Namespace", a.snapshot.Namespace)
			return reconciler.RequeueWithError(err)
		}
		a.logger.Info("No required IntegrationTestScenarios found, skipped testing and marked Snapshot as successful",
			"Snapshot.Name", updatedSnapshot.Name,
			"Snapshot.Namespace", updatedSnapshot.Namespace,
			"Snapshot.Status", updatedSnapshot.Status)
	}

	return reconciler.ContinueProcessing()
}

// EnsureGlobalComponentImageUpdated is an operation that ensure the ContainerImage in the Global Candidate List
// being updated when the Snapshot passed all the integration tests
func (a *Adapter) EnsureGlobalComponentImageUpdated() (reconciler.OperationResult, error) {
	if (a.component != nil) && gitops.HaveHACBSTestsSucceeded(a.snapshot) && !gitops.IsSnapshotCreatedByPACPullRequestEvent(a.snapshot) {
		patch := client.MergeFrom(a.component.DeepCopy())
		for _, component := range a.snapshot.Spec.Components {
			if component.Name == a.component.Name {
				a.component.Spec.ContainerImage = component.ContainerImage
				err := a.client.Patch(a.context, a.component, patch)
				if err != nil {
					a.logger.Error(err, "Failed to update Global Candidate for the Component",
						"Component.Name", a.component.Name)
					return reconciler.RequeueWithError(err)
				}
			}
		}
	}
	return reconciler.ContinueProcessing()
}

// EnsureAllReleasesExist is an operation that will ensure that all pipeline Releases associated
// to the Snapshot and the Application's ReleasePlans exist.
// Otherwise, it will create new Releases for each ReleasePlan.
func (a *Adapter) EnsureAllReleasesExist() (reconciler.OperationResult, error) {
	if !gitops.HaveHACBSTestsSucceeded(a.snapshot) {
		a.logger.Info("The Snapshot hasn't been marked as HACBSTestSucceeded, holding off on releasing.")
		return reconciler.ContinueProcessing()
	}

	if gitops.IsSnapshotCreatedByPACPullRequestEvent(a.snapshot) {
		a.logger.Info("The Snapshot won't be released because it was created for a PaC pull request event.")
		return reconciler.ContinueProcessing()
	}

	releasePlans, err := release.GetAutoReleasePlansForApplication(a.client, a.context, a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get all ReleasePlans",
			"Application.Name", a.application.Name,
			"Application.Namespace", a.application.Namespace)
		patch := client.MergeFrom(a.snapshot.DeepCopy())
		gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to get all ReleasePlans")
		return reconciler.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
	}

	err = a.createMissingReleasesForReleasePlans(a.application, releasePlans, a.snapshot)
	if err != nil {
		a.logger.Error(err, "Failed to create new Releases",
			"Snapshot.Name", a.snapshot.Name,
			"Snapshot.Namespace", a.snapshot.Namespace)
		patch := client.MergeFrom(a.snapshot.DeepCopy())
		gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to create new Releases")
		return reconciler.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
	}

	return reconciler.ContinueProcessing()
}

// EnsureSnapshotEnvironmentBindingExist is an operation that will ensure that all
// SnapshotEnvironmentBindings for non-ephemeral root environments point to the newly constructed snapshot.
// If the bindings don't already exist, it will create new ones for each of the environments.
func (a *Adapter) EnsureSnapshotEnvironmentBindingExist() (reconciler.OperationResult, error) {
	if !gitops.HaveHACBSTestsSucceeded(a.snapshot) {
		a.logger.Info("The Snapshot hasn't been marked as HACBSTestSucceeded, holding off on deploying.")
		return reconciler.ContinueProcessing()
	}

	if gitops.IsSnapshotCreatedByPACPullRequestEvent(a.snapshot) {
		a.logger.Info("The Snapshot won't be deployed because it was created for a PaC pull request event.")
		return reconciler.ContinueProcessing()
	}

	availableEnvironments, err := a.findAvailableEnvironments()
	if err != nil {
		return reconciler.RequeueWithError(err)
	}

	components, err := a.getAllApplicationComponents(a.application)
	if err != nil {
		return reconciler.RequeueWithError(err)
	}

	for _, availableEnvironment := range *availableEnvironments {
		availableEnvironment := availableEnvironment // G601
		snapshotEnvironmentBinding, err := gitops.FindExistingSnapshotEnvironmentBinding(a.client, a.context, a.application, &availableEnvironment)
		if err != nil {
			return reconciler.RequeueWithError(err)
		}
		if snapshotEnvironmentBinding != nil {
			snapshotEnvironmentBinding, err = a.updateExistingSnapshotEnvironmentBindingWithSnapshot(snapshotEnvironmentBinding, a.snapshot, components)
			if err != nil {
				a.logger.Error(err, "Failed to update SnapshotEnvironmentBinding",
					"SnapshotEnvironmentBinding.Application", snapshotEnvironmentBinding.Spec.Application,
					"SnapshotEnvironmentBinding.Environment", snapshotEnvironmentBinding.Spec.Environment,
					"SnapshotEnvironmentBinding.Snapshot", snapshotEnvironmentBinding.Spec.Snapshot)
				patch := client.MergeFrom(a.snapshot.DeepCopy())
				gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to update SnapshotEnvironmentBinding")
				return reconciler.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
			}
		} else {
			snapshotEnvironmentBinding, err = a.createSnapshotEnvironmentBindingForSnapshot(a.application, &availableEnvironment, a.snapshot, components)
			if err != nil {
				a.logger.Error(err, "Failed to create SnapshotEnvironmentBinding",
					"SnapshotEnvironmentBinding.Application", snapshotEnvironmentBinding.Spec.Application,
					"SnapshotEnvironmentBinding.Environment", snapshotEnvironmentBinding.Spec.Environment,
					"SnapshotEnvironmentBinding.Snapshot", snapshotEnvironmentBinding.Spec.Snapshot)
				patch := client.MergeFrom(a.snapshot.DeepCopy())
				gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to create SnapshotEnvironmentBinding")
				return reconciler.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
			}

		}
		a.logger.Info("Created/updated SnapshotEnvironmentBinding",
			"SnapshotEnvironmentBinding.Application", snapshotEnvironmentBinding.Spec.Application,
			"SnapshotEnvironmentBinding.Environment", snapshotEnvironmentBinding.Spec.Environment,
			"SnapshotEnvironmentBinding.Snapshot", snapshotEnvironmentBinding.Spec.Snapshot)
	}
	return reconciler.ContinueProcessing()
}

// getReleasesWithSnapshot returns all Releases associated with the given snapshot.
// In the case the List operation fails, an error will be returned.
func (a *Adapter) getReleasesWithSnapshot(snapshot *applicationapiv1alpha1.Snapshot) (*[]releasev1alpha1.Release, error) {
	releases := &releasev1alpha1.ReleaseList{}
	opts := []client.ListOption{
		client.InNamespace(snapshot.Namespace),
		client.MatchingFields{"spec.snapshot": snapshot.Name},
	}

	err := a.client.List(a.context, releases, opts...)
	if err != nil {
		return nil, err
	}

	return &releases.Items, nil
}

// createMissingReleasesForReleasePlans checks if there's existing Releases for a given list of ReleasePlans and creates
// new ones if they are missing. In case the Releases can't be created, an error will be returned.
func (a *Adapter) createMissingReleasesForReleasePlans(application *applicationapiv1alpha1.Application, releasePlans *[]releasev1alpha1.ReleasePlan, snapshot *applicationapiv1alpha1.Snapshot) error {
	releases, err := a.getReleasesWithSnapshot(snapshot)
	if err != nil {
		return err
	}

	for _, releasePlan := range *releasePlans {
		releasePlan := releasePlan // G601
		existingRelease := release.FindMatchingReleaseWithReleasePlan(releases, releasePlan)
		if existingRelease != nil {
			a.logger.Info("Found existing Release",
				"Snapshot.Name", snapshot.Name,
				"ReleasePlan.Name", releasePlan.Name,
				"Release.Name", existingRelease.Name)
		} else {
			newRelease := release.NewReleaseForReleasePlan(&releasePlan, snapshot)
			// Propagate annotations/labels from snapshot to Release
			helpers.CopyAnnotationsByPrefix(&snapshot.ObjectMeta, &newRelease.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
			helpers.CopyLabelsByPrefix(&snapshot.ObjectMeta, &newRelease.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)

			err := ctrl.SetControllerReference(application, newRelease, a.client.Scheme())
			if err != nil {
				return err
			}
			err = a.client.Create(a.context, newRelease)
			if err != nil {
				return err
			}
			a.logger.Info("Created new Release",
				"Application.Name", a.application.Name,
				"ReleasePlan.Name", releasePlan.Name,
				"Release.Name", newRelease.Name)
		}
	}
	return nil
}

// getAllEnvironments gets all environments in the namespace
func (a *Adapter) getAllEnvironments() (*[]applicationapiv1alpha1.Environment, error) {

	environmentList := &applicationapiv1alpha1.EnvironmentList{}
	opts := []client.ListOption{
		client.InNamespace(a.application.Namespace),
	}
	err := a.client.List(a.context, environmentList, opts...)
	return &environmentList.Items, err
}

// findAvailableEnvironments gets all environments that don't have a ParentEnvironment and are not tagged as ephemeral.
func (a *Adapter) findAvailableEnvironments() (*[]applicationapiv1alpha1.Environment, error) {
	allEnvironments, err := a.getAllEnvironments()
	if err != nil {
		return nil, err
	}
	availableEnvironments := []applicationapiv1alpha1.Environment{}
	for _, environment := range *allEnvironments {
		if environment.Spec.ParentEnvironment == "" {
			isEphemeral := false
			for _, tag := range environment.Spec.Tags {
				if tag == "ephemeral" {
					isEphemeral = true
					break
				}
			}
			if !isEphemeral {
				availableEnvironments = append(availableEnvironments, environment)
			}
		}
	}
	return &availableEnvironments, nil
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

// createIntegrationPipelineRun creates and returns a new integration PipelineRun. The Pipeline information and the parameters to it
// will be extracted from the given integrationScenario. The integration's Snapshot will also be passed to the integration PipelineRun.
func (a *Adapter) createIntegrationPipelineRun(application *applicationapiv1alpha1.Application, integrationTestScenario *v1alpha1.IntegrationTestScenario, snapshot *applicationapiv1alpha1.Snapshot) error {
	pipelineRun := tekton.NewIntegrationPipelineRun(snapshot.Name, application.Namespace, *integrationTestScenario).
		WithSnapshot(snapshot).
		WithIntegrationLabels(integrationTestScenario).
		WithApplicationAndComponent(a.application, a.component).
		AsPipelineRun()
	// copy PipelineRun PAC annotations/labels from snapshot to integration test PipelineRuns
	helpers.CopyAnnotationsByPrefix(&snapshot.ObjectMeta, &pipelineRun.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
	helpers.CopyLabelsByPrefix(&snapshot.ObjectMeta, &pipelineRun.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
	err := ctrl.SetControllerReference(snapshot, pipelineRun, a.client.Scheme())
	if err != nil {
		return err
	}
	err = a.client.Create(a.context, pipelineRun)
	if err != nil {
		return err
	}

	return nil
}

// createSnapshotEnvironmentBindingForSnapshot creates and returns a new snapshotEnvironmentBinding
// for the given application, environment, snapshot, and components.
// If it's not possible to create it and set the application as the owner, an error will be returned
func (a *Adapter) createSnapshotEnvironmentBindingForSnapshot(application *applicationapiv1alpha1.Application,
	environment *applicationapiv1alpha1.Environment, snapshot *applicationapiv1alpha1.Snapshot,
	components *[]applicationapiv1alpha1.Component) (*applicationapiv1alpha1.SnapshotEnvironmentBinding, error) {
	bindingName := application.Name + "-" + environment.Name + "-" + "binding"

	snapshotEnvironmentBinding := gitops.NewSnapshotEnvironmentBinding(
		bindingName, application.Namespace, application.Name,
		environment.Name,
		snapshot, *components)

	err := ctrl.SetControllerReference(application, snapshotEnvironmentBinding, a.client.Scheme())
	if err != nil {
		return nil, err
	}

	err = a.client.Create(a.context, snapshotEnvironmentBinding)
	if err != nil {
		return nil, err
	}

	return snapshotEnvironmentBinding, nil
}

// updateExistingSnapshotEnvironmentBindingWithSnapshot updates and returns snapshotEnvironmentBinding
// with the given snapshot and components. If it's not possible to patch, an error will be returned.
func (a *Adapter) updateExistingSnapshotEnvironmentBindingWithSnapshot(snapshotEnvironmentBinding *applicationapiv1alpha1.SnapshotEnvironmentBinding,
	snapshot *applicationapiv1alpha1.Snapshot,
	components *[]applicationapiv1alpha1.Component) (*applicationapiv1alpha1.SnapshotEnvironmentBinding, error) {

	patch := client.MergeFrom(snapshotEnvironmentBinding.DeepCopy())

	snapshotEnvironmentBinding.Spec.Snapshot = snapshot.Name
	snapshotComponents := gitops.NewBindingComponents(*components)
	snapshotEnvironmentBinding.Spec.Components = *snapshotComponents

	err := a.client.Patch(a.context, snapshotEnvironmentBinding, patch)
	if err != nil {
		return nil, err
	}

	return snapshotEnvironmentBinding, nil
}
