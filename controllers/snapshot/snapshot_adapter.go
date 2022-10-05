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
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/controllers/results"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/release"
	"github.com/redhat-appstudio/integration-service/tekton"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	snapshot    *applicationapiv1alpha1.ApplicationSnapshot
	application *applicationapiv1alpha1.Application
	component   *applicationapiv1alpha1.Component
	logger      logr.Logger
	client      client.Client
	context     context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(snapshot *applicationapiv1alpha1.ApplicationSnapshot, application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, logger logr.Logger, client client.Client,
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
// associated with the ApplicationSnapshot and the Application's IntegrationTestScenarios exist.
// Otherwise, it will create new Releases for each ReleasePlan.
func (a *Adapter) EnsureAllIntegrationTestPipelinesExist() (results.OperationResult, error) {
	if gitops.HaveHACBSTestsFinished(a.snapshot) {
		a.logger.Info("The Snapshot has finished testing.")
		return results.ContinueProcessing()
	}

	integrationTestScenarios, err := helpers.GetAllIntegrationTestScenariosForApplication(a.client, a.context, a.application)

	if err != nil {
		a.logger.Error(err, "Failed to get Integration test scenarios for following application",
			"Application.Name", a.application.Name,
			"Application.Namespace", a.application.Namespace)
	}

	for _, integrationTestScenario := range *integrationTestScenarios {
		integrationTestScenario := integrationTestScenario //G601
		integrationPipelineRun, err := helpers.GetLatestPipelineRunForApplicationSnapshotAndScenario(a.client, a.context, a.application, a.snapshot, &integrationTestScenario)
		if err != nil {
			a.logger.Error(err, "Failed to get latest pipelineRun for application snapshot and scenario",
				"integrationPipelineRun:", integrationPipelineRun)
			return results.RequeueOnErrorOrStop(err)
		}
		if integrationPipelineRun != nil {
			a.logger.Info("Found existing integrationPipelineRun",
				"IntegrationTestScenario.Name", integrationTestScenario.Name,
				"integrationPipelineRun.Name", integrationPipelineRun.Name)
		} else {
			a.logger.Info("Creating new pipelinerun for integrationTestscenario",
				"IntegrationTestScenario.Name", integrationTestScenario.Name,
				"app name", a.application.Name,
				"namespace", a.application.Namespace)
			err := a.createIntegrationPipelineRun(a.application, &integrationTestScenario, a.snapshot)
			if err != nil {
				a.logger.Error(err, "Failed to create pipelineRun for application snapshot and scenario")
				return results.RequeueOnErrorOrStop(err)
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
		return results.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
	}
	if len(*requiredIntegrationTestScenarios) == 0 {
		updatedSnapshot, err := gitops.MarkSnapshotAsPassed(a.client, a.context, a.snapshot, "No required IntegrationTestScenarios found, skipped testing")
		if err != nil {
			a.logger.Error(err, "Failed to update Snapshot status",
				"ApplicationSnapshot.Name", a.snapshot.Name,
				"ApplicationSnapshot.Namespace", a.snapshot.Namespace)
			return results.RequeueWithError(err)
		}
		a.logger.Info("No required IntegrationTestScenarios found, skipped testing and marked Snapshot as successful",
			"ApplicationSnapshot.Name", updatedSnapshot.Name,
			"ApplicationSnapshot.Namespace", updatedSnapshot.Namespace,
			"ApplicationSnapshot.Status", updatedSnapshot.Status)
	}

	return results.ContinueProcessing()
}

// EnsureGlobalComponentImageUpdated is an operation that ensure the ContainerImage in the Global Candidate List
// being updated when the ApplicationSnapshot passed all the integration tests
func (a *Adapter) EnsureGlobalComponentImageUpdated() (results.OperationResult, error) {
	if (a.component != nil) && gitops.HaveHACBSTestsSucceeded(a.snapshot) {
		patch := client.MergeFrom(a.component.DeepCopy())
		for _, component := range a.snapshot.Spec.Components {
			if component.Name == a.component.Name {
				a.component.Spec.ContainerImage = component.ContainerImage
				err := a.client.Patch(a.context, a.component, patch)
				if err != nil {
					a.logger.Error(err, "Failed to update Global Candidate for the Component",
						"Component.Name", a.component.Name)
					return results.RequeueWithError(err)
				}
			}
		}
	}
	return results.ContinueProcessing()
}

// EnsureAllReleasesExist is an operation that will ensure that all pipeline Releases associated
// to the ApplicationSnapshot and the Application's ReleasePlans exist.
// Otherwise, it will create new Releases for each ReleasePlan.
func (a *Adapter) EnsureAllReleasesExist() (results.OperationResult, error) {
	if !gitops.HaveHACBSTestsSucceeded(a.snapshot) {
		a.logger.Info("The Snapshot hasn't been marked as HACBSTestSucceeded, holding off on releasing.")
		return results.ContinueProcessing()
	}

	releasePlans, err := release.GetAutoReleasePlansForApplication(a.client, a.context, a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get all ReleasePlans",
			"Application.Name", a.application.Name,
			"Application.Namespace", a.application.Namespace)
		patch := client.MergeFrom(a.snapshot.DeepCopy())
		gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to get all ReleasePlans")
		return results.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
	}

	err = a.createMissingReleasesForReleasePlans(a.application, releasePlans, a.snapshot)
	if err != nil {
		a.logger.Error(err, "Failed to create new Releases",
			"ApplicationSnapshot.Name", a.snapshot.Name,
			"ApplicationSnapshot.Namespace", a.snapshot.Namespace)
		patch := client.MergeFrom(a.snapshot.DeepCopy())
		gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to create new Releases")
		return results.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
	}

	return results.ContinueProcessing()
}

// EnsureApplicationSnapshotEnvironmentBindingExist is an operation that will ensure that all
// ApplicationSnapshotEnvironmentBindings for non-ephemeral root environments point to the newly constructed snapshot.
// If the bindings don't already exist, it will create new ones for each of the environments.
func (a *Adapter) EnsureApplicationSnapshotEnvironmentBindingExist() (results.OperationResult, error) {
	if !gitops.HaveHACBSTestsSucceeded(a.snapshot) {
		a.logger.Info("The Snapshot hasn't been marked as HACBSTestSucceeded, holding off on deploying.")
		return results.ContinueProcessing()
	}

	availableEnvironments, err := a.findAvailableEnvironments()
	if err != nil {
		return results.RequeueWithError(err)
	}

	components, err := a.getAllApplicationComponents(a.application)
	if err != nil {
		return results.RequeueWithError(err)
	}

	for _, availableEnvironment := range *availableEnvironments {
		availableEnvironment := availableEnvironment // G601
		applicationSnapshotEnvironmentBinding, err := gitops.FindExistingApplicationSnapshotEnvironmentBinding(a.client, a.context, a.application, &availableEnvironment)
		if err != nil {
			return results.RequeueWithError(err)
		}
		if applicationSnapshotEnvironmentBinding != nil {
			applicationSnapshotEnvironmentBinding, err = a.updateExistingApplicationSnapshotEnvironmentBindingWithSnapshot(applicationSnapshotEnvironmentBinding, a.snapshot, components)
			if err != nil {
				a.logger.Error(err, "Failed to update ApplicationSnapshotEnvironmentBinding",
					"ApplicationSnapshotEnvironmentBinding.Application", applicationSnapshotEnvironmentBinding.Spec.Application,
					"ApplicationSnapshotEnvironmentBinding.Environment", applicationSnapshotEnvironmentBinding.Spec.Environment,
					"ApplicationSnapshotEnvironmentBinding.Snapshot", applicationSnapshotEnvironmentBinding.Spec.Snapshot)
				patch := client.MergeFrom(a.snapshot.DeepCopy())
				gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to update ApplicationSnapshotEnvironmentBinding")
				return results.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
			}
		} else {
			applicationSnapshotEnvironmentBinding, err = a.createApplicationSnapshotEnvironmentBindingForSnapshot(a.application, &availableEnvironment, a.snapshot, components)
			if err != nil {
				a.logger.Error(err, "Failed to create ApplicationSnapshotEnvironmentBinding",
					"ApplicationSnapshotEnvironmentBinding.Application", applicationSnapshotEnvironmentBinding.Spec.Application,
					"ApplicationSnapshotEnvironmentBinding.Environment", applicationSnapshotEnvironmentBinding.Spec.Environment,
					"ApplicationSnapshotEnvironmentBinding.Snapshot", applicationSnapshotEnvironmentBinding.Spec.Snapshot)
				patch := client.MergeFrom(a.snapshot.DeepCopy())
				gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to create ApplicationSnapshotEnvironmentBinding")
				return results.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
			}

		}
		a.logger.Info("Created/updated ApplicationSnapshotEnvironmentBinding",
			"ApplicationSnapshotEnvironmentBinding.Application", applicationSnapshotEnvironmentBinding.Spec.Application,
			"ApplicationSnapshotEnvironmentBinding.Environment", applicationSnapshotEnvironmentBinding.Spec.Environment,
			"ApplicationSnapshotEnvironmentBinding.Snapshot", applicationSnapshotEnvironmentBinding.Spec.Snapshot)
	}
	return results.ContinueProcessing()
}

// getReleasesWithApplicationSnapshot returns all Releases associated with the given applicationSnapshot.
// In the case the List operation fails, an error will be returned.
func (a *Adapter) getReleasesWithApplicationSnapshot(applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot) (*[]releasev1alpha1.Release, error) {
	releases := &releasev1alpha1.ReleaseList{}
	opts := []client.ListOption{
		client.InNamespace(applicationSnapshot.Namespace),
		client.MatchingFields{"spec.applicationSnapshot": applicationSnapshot.Name},
	}

	err := a.client.List(a.context, releases, opts...)
	if err != nil {
		return nil, err
	}

	return &releases.Items, nil
}

// createMissingReleasesForReleasePlans checks if there's existing Releases for a given list of ReleasePlans and creates
// new ones if they are missing. In case the Releases can't be created, an error will be returned.
func (a *Adapter) createMissingReleasesForReleasePlans(application *applicationapiv1alpha1.Application, releasePlans *[]releasev1alpha1.ReleasePlan, applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot) error {
	releases, err := a.getReleasesWithApplicationSnapshot(applicationSnapshot)
	if err != nil {
		return err
	}

	for _, releasePlan := range *releasePlans {
		releasePlan := releasePlan // G601
		existingRelease := release.FindMatchingReleaseWithReleasePlan(releases, releasePlan)
		if existingRelease != nil {
			a.logger.Info("Found existing Release",
				"ApplicationSnapshot.Name", applicationSnapshot.Name,
				"ReleasePlan.Name", releasePlan.Name,
				"Release.Name", existingRelease.Name)
		} else {
			newRelease := release.CreateReleaseForReleasePlan(&releasePlan, applicationSnapshot)
			// copy PipelineRun PAC annotations/labels from applicaitonSnapshot to Release
			if applicationSnapshot.Annotations != nil {
				if newRelease.ObjectMeta.Annotations == nil {
					newRelease.ObjectMeta.Annotations = make(map[string]string)
				}
				for key, value := range applicationSnapshot.Annotations {
					if strings.Contains(key, "pipelinesascode.tekton.dev") {
						newRelease.ObjectMeta.Annotations[key] = value
					}
				}
			}
			if applicationSnapshot.Labels != nil {
				if newRelease.ObjectMeta.Labels == nil {
					newRelease.ObjectMeta.Labels = make(map[string]string)
				}
				for key, value := range applicationSnapshot.Labels {
					if strings.Contains(key, "pipelinesascode.tekton.dev") {
						newRelease.ObjectMeta.Labels[key] = value
					}
				}
			}
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
// will be extracted from the given integrationScenario. The integration's ApplicationSnapshot will also be passed to the
// integration PipelineRun.
func (a *Adapter) createIntegrationPipelineRun(application *applicationapiv1alpha1.Application, integrationTestScenario *v1alpha1.IntegrationTestScenario, applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot) error {
	pipelineRun := tekton.NewIntegrationPipelineRun(applicationSnapshot.Name, application.Namespace, *integrationTestScenario).
		WithApplicationSnapshot(applicationSnapshot).
		WithIntegrationLabels(integrationTestScenario).
		WithApplicationAndComponent(a.application, a.component).
		AsPipelineRun()
	// copy PipelineRun PAC annotations/labels from applicaitonSnapshot to integration test PipelineRuns
	if applicationSnapshot.Annotations != nil {
		if pipelineRun.ObjectMeta.Annotations == nil {
			pipelineRun.ObjectMeta.Annotations = make(map[string]string)
		}
		for key, value := range applicationSnapshot.Annotations {
			if strings.Contains(key, "pipelinesascode.tekton.dev") {
				pipelineRun.ObjectMeta.Annotations[key] = value
			}
		}
	}
	if applicationSnapshot.Labels != nil {
		for key, value := range applicationSnapshot.Labels {
			if strings.Contains(key, "pipelinesascode.tekton.dev") {
				pipelineRun.ObjectMeta.Labels[key] = value
			}
		}
	}
	err := ctrl.SetControllerReference(applicationSnapshot, pipelineRun, a.client.Scheme())
	if err != nil {
		return err
	}
	err = a.client.Create(a.context, pipelineRun)
	if err != nil {
		return err
	}

	return nil
}

// createApplicationSnapshotEnvironmentBindingForSnapshot creates and returns a new applicationSnapshotEnvironmentBinding
// for the given application, environment, applicationSnapshot, and components.
// If it's not possible to create it and set the application as the owner, an error will be returned
func (a *Adapter) createApplicationSnapshotEnvironmentBindingForSnapshot(application *applicationapiv1alpha1.Application,
	environment *applicationapiv1alpha1.Environment, applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot,
	components *[]applicationapiv1alpha1.Component) (*applicationapiv1alpha1.ApplicationSnapshotEnvironmentBinding, error) {
	bindingName := application.Name + "-" + environment.Name + "-" + "binding"

	applicationSnapshotEnvironmentBinding := gitops.CreateApplicationSnapshotEnvironmentBinding(
		bindingName, application.Namespace, application.Name,
		environment.Name,
		applicationSnapshot, *components)

	err := ctrl.SetControllerReference(application, applicationSnapshotEnvironmentBinding, a.client.Scheme())
	if err != nil {
		return nil, err
	}

	err = a.client.Create(a.context, applicationSnapshotEnvironmentBinding)
	if err != nil {
		return nil, err
	}

	return applicationSnapshotEnvironmentBinding, nil
}

// updateExistingApplicationSnapshotEnvironmentBindingWithSnapshot updates and returns applicationSnapshotEnvironmentBinding
// with the given snapshot and components. If it's not possible to patch, an error will be returned.
func (a *Adapter) updateExistingApplicationSnapshotEnvironmentBindingWithSnapshot(applicationSnapshotEnvironmentBinding *applicationapiv1alpha1.ApplicationSnapshotEnvironmentBinding,
	snapshot *applicationapiv1alpha1.ApplicationSnapshot,
	components *[]applicationapiv1alpha1.Component) (*applicationapiv1alpha1.ApplicationSnapshotEnvironmentBinding, error) {

	patch := client.MergeFrom(applicationSnapshotEnvironmentBinding.DeepCopy())

	applicationSnapshotEnvironmentBinding.Spec.Snapshot = snapshot.Name
	applicationSnapshotComponents := gitops.CreateBindingComponents(*components)
	applicationSnapshotEnvironmentBinding.Spec.Components = *applicationSnapshotComponents

	err := a.client.Patch(a.context, applicationSnapshotEnvironmentBinding, patch)
	if err != nil {
		return nil, err
	}

	return applicationSnapshotEnvironmentBinding, nil
}
