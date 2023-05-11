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
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	h "github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/release"
	"github.com/redhat-appstudio/integration-service/tekton"

	"github.com/redhat-appstudio/integration-service/loader"
	"github.com/redhat-appstudio/operator-goodies/reconciler"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	snapshot    *applicationapiv1alpha1.Snapshot
	application *applicationapiv1alpha1.Application
	component   *applicationapiv1alpha1.Component
	logger      h.IntegrationLogger
	client      client.Client
	context     context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(snapshot *applicationapiv1alpha1.Snapshot, application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, logger h.IntegrationLogger, client client.Client,
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
	if gitops.HaveAppStudioTestsFinished(a.snapshot) {
		a.logger.Info("The Snapshot has finished testing.")
		return reconciler.ContinueProcessing()
	}

	integrationTestScenarios, err := h.GetAllIntegrationTestScenariosForApplication(a.client, a.context, a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get Integration test scenarios for the following application",
			"Application.Name", a.application.Name,
			"Application.Namespace", a.application.Namespace)
	}

	if integrationTestScenarios != nil {
		a.logger.Info("Found IntegrationTestScenarios for application",
			"Application.Name", a.application.Name,
			"IntegrationTestScenarios", len(*integrationTestScenarios))
		for _, integrationTestScenario := range *integrationTestScenarios {
			integrationTestScenario := integrationTestScenario //G601
			if !reflect.ValueOf(integrationTestScenario.Spec.Environment).IsZero() {
				// the test pipeline for scenario needing an ephemeral environment will be handled in STONEINTG-333
				a.logger.Info("IntegrationTestScenario has environment defined, skipping creation of pipelinerun.", "IntegrationTestScenario", integrationTestScenario)
				continue
			}
			integrationPipelineRuns, err := h.GetAllPipelineRunsForSnapshotAndScenario(a.client, a.context, a.snapshot, &integrationTestScenario)
			if err != nil {
				a.logger.Error(err, "Failed to get pipelineRuns for snapshot and scenario",
					"integrationTestScenario.Name", integrationTestScenario.Name)
				return reconciler.RequeueOnErrorOrStop(err)
			}
			if integrationPipelineRuns != nil && len(*integrationPipelineRuns) > 0 {
				a.logger.Info("Found existing integrationPipelineRuns",
					"integrationTestScenario.Name", integrationTestScenario.Name,
					"len(integrationPipelineRuns)", len(*integrationPipelineRuns))
			} else {
				a.logger.Info("Creating new pipelinerun for integrationTestscenario",
					"integrationTestScenario.Name", integrationTestScenario.Name)
				pipelineRun, err := a.createIntegrationPipelineRun(a.application, &integrationTestScenario, a.snapshot)
				if err != nil {
					a.logger.Error(err, "Failed to create pipelineRun for snapshot and scenario")
					return reconciler.RequeueOnErrorOrStop(err)
				}
				a.logger.LogAuditEvent("IntegrationTestscenario pipeline has been created", pipelineRun, h.LogActionAdd,
					"integrationTestScenario.Name", integrationTestScenario.Name)
				gitops.PrepareToRegisterIntegrationPipelineRun(a.snapshot)
				if gitops.IsSnapshotNotStarted(a.snapshot) {
					_, err := gitops.MarkSnapshotIntegrationStatusAsInProgress(a.client, a.context, a.snapshot, "Snapshot starts being tested by the integrationPipelineRun")
					if err != nil {
						a.logger.Error(err, "Failed to update integration status condition to in progress for snapshot")
					} else {
						a.logger.LogAuditEvent("Snapshot integration status marked as In Progress. Snapshot starts being tested by the integrationPipelineRun",
							a.snapshot, h.LogActionUpdate)
					}
				}

			}
		}
	}

	requiredIntegrationTestScenarios, err := h.GetRequiredIntegrationTestScenariosForApplication(a.client, a.context, a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get all required IntegrationTestScenarios")
		patch := client.MergeFrom(a.snapshot.DeepCopy())
		gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to get all required IntegrationTestScenarios")
		a.logger.LogAuditEvent("Snapshot integration status marked as Invalid. Failed to get all required IntegrationTestScenarios",
			a.snapshot, h.LogActionUpdate)
		return reconciler.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
	}
	if len(*requiredIntegrationTestScenarios) == 0 {
		updatedSnapshot, err := gitops.MarkSnapshotAsPassed(a.client, a.context, a.snapshot, "No required IntegrationTestScenarios found, skipped testing")
		if err != nil {
			a.logger.Error(err, "Failed to update Snapshot status")
			return reconciler.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("Snapshot marked as successful. No required IntegrationTestScenarios found, skipped testing",
			updatedSnapshot, h.LogActionUpdate,
			"snapshot.Status", updatedSnapshot.Status)
	}

	return reconciler.ContinueProcessing()
}

// EnsureCreationOfEnvironment makes sure that all envrionments that were requested via
// IntegrationTestScenarios get created, in case that environment is already created, provides
// a message about this fact
func (a *Adapter) EnsureCreationOfEnvironment() (reconciler.OperationResult, error) {
	if gitops.HaveAppStudioTestsFinished(a.snapshot) {
		a.logger.Info("The Snapshot has finished testing.")
		return reconciler.ContinueProcessing()
	}

	integrationTestScenarios, err := h.GetAllIntegrationTestScenariosForApplication(a.client, a.context, a.application)

	if err != nil {
		a.logger.Error(err, "Failed to get Integration test scenarios for the following application")
		return reconciler.RequeueOnErrorOrStop(err)
	}
	if integrationTestScenarios == nil {
		a.logger.Info("No integration test scenario found for Application")
		return reconciler.ContinueProcessing()
	}

	allEnvironments, err := loader.GetAllEnvironments(a.client, a.context, a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get all environments.")
		return reconciler.RequeueOnErrorOrStop(err)
	}

TestScenarioLoop:
	for _, integrationTestScenario := range *integrationTestScenarios {
		integrationTestScenario := integrationTestScenario //G601
		if reflect.ValueOf(integrationTestScenario.Spec.Environment).IsZero() {
			continue
		}
		for _, environment := range *allEnvironments {
			environment := environment //G601
			//prevent creating already existing environments
			if h.HasLabelWithValue(&environment, gitops.SnapshotLabel, a.snapshot.Name) && h.HasLabelWithValue(&environment, gitops.SnapshotTestScenarioLabel, integrationTestScenario.Name) {
				a.logger.Info("Environment already exists and contains snapshot and scenario:",
					"environment.Name", environment.Name,
					"integrationScenario.Name", integrationTestScenario.Name)
				continue TestScenarioLoop
			}
		}

		//get the existing environment according to environment name from integrationTestScenario
		existingEnv, err := a.getEnvironmentFromIntegrationTestScenario(&integrationTestScenario)
		if err != nil {
			a.logger.Error(err, "Failed to find the env defined in integrationTestScenario.",
				"integrationTestScenario.Namespace", integrationTestScenario.Namespace,
				"integrationTestScenario.Name", integrationTestScenario.Name)
			return reconciler.RequeueOnErrorOrStop(err)
		}

		//create an ephemeral copy env of existing environment
		copyEnv, err := a.createCopyOfExistingEnvironment(existingEnv, a.snapshot.Namespace, &integrationTestScenario, a.snapshot, a.application)

		if err != nil {
			a.logger.Error(err, "Copying of environment failed.")
			return reconciler.RequeueOnErrorOrStop(err)
		}
		a.logger.LogAuditEvent("An ephemeral Environment is created for integrationTestScenario",
			copyEnv, h.LogActionAdd,
			"integrationTestScenario.Name", integrationTestScenario.Name)

		components, err := loader.GetAllApplicationComponents(a.client, a.context, a.application)
		if err != nil {
			return reconciler.RequeueWithError(err)
		}

		binding, err := a.createSnapshotEnvironmentBindingForSnapshot(a.application, copyEnv, a.snapshot, components)
		if err != nil {
			a.logger.Error(err, "Failed to create snapshotEnvironmentbinding for snapshot.",
				"binding", binding,
				"snapshot.Spec.Components", a.snapshot.Spec.Components)
		}
		a.logger.LogAuditEvent("A snapshotEnvironmentbinding is created", binding, h.LogActionAdd,
			"environment.Name", copyEnv.Name,
			"integrationTestScenario.Name", integrationTestScenario.Name)
	}
	return reconciler.ContinueProcessing()
}

// EnsureGlobalCandidateImageUpdated is an operation that ensure the ContainerImage in the Global Candidate List
// being updated when the Snapshot passed all the integration tests
func (a *Adapter) EnsureGlobalCandidateImageUpdated() (reconciler.OperationResult, error) {
	if (a.component != nil) && gitops.HaveAppStudioTestsSucceeded(a.snapshot) && !gitops.IsSnapshotCreatedByPACPullRequestEvent(a.snapshot) {
		patch := client.MergeFrom(a.component.DeepCopy())
		for _, component := range a.snapshot.Spec.Components {
			if component.Name == a.component.Name {
				a.component.Spec.ContainerImage = component.ContainerImage
				err := a.client.Patch(a.context, a.component, patch)
				if err != nil {
					a.logger.Error(err, "Failed to update .Spec.ContainerImage of Global Candidate for the Component",
						"component.Name", a.component.Name)
					return reconciler.RequeueWithError(err)
				}
				a.logger.LogAuditEvent("Updated .Spec.ContainerImage of Global Candidate for the Component",
					a.component, h.LogActionUpdate,
					"containerImage", component.ContainerImage)
				if reflect.ValueOf(component.Source).IsValid() && component.Source.GitSource != nil && component.Source.GitSource.Revision != "" {
					a.component.Status.LastBuiltCommit = component.Source.GitSource.Revision
					err = a.client.Status().Patch(a.context, a.component, patch)
					if err != nil {
						a.logger.Error(err, "Failed to update .Status.LastBuiltCommit of Global Candidate for the Component",
							"component.Name", a.component.Name)
						return reconciler.RequeueWithError(err)
					}
					a.logger.LogAuditEvent("Updated .Status.LastBuiltCommit of Global Candidate for the Component",
						a.component, h.LogActionUpdate,
						"lastBuildCommit", a.component.Status.LastBuiltCommit)
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
	canSnapshotBePromoted, reasons := gitops.CanSnapshotBePromoted(a.snapshot)
	if !canSnapshotBePromoted {
		a.logger.Info("The Snapshot won't be released.",
			"reasons", strings.Join(reasons, ","))
		return reconciler.ContinueProcessing()
	}

	releasePlans, err := release.GetAutoReleasePlansForApplication(a.client, a.context, a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get all ReleasePlans")
		patch := client.MergeFrom(a.snapshot.DeepCopy())
		gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to get all ReleasePlans")
		a.logger.LogAuditEvent("Snapshot integration status marked as Invalid. Failed to get all ReleasePlans",
			a.snapshot, h.LogActionUpdate)
		return reconciler.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
	}

	err = a.createMissingReleasesForReleasePlans(a.application, releasePlans, a.snapshot)
	if err != nil {
		a.logger.Error(err, "Failed to create new Releases")
		patch := client.MergeFrom(a.snapshot.DeepCopy())
		gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to create new Releases")
		a.logger.LogAuditEvent("Snapshot integration status marked as Invalid. Failed to create new Releases",
			a.snapshot, h.LogActionUpdate)
		return reconciler.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
	}
	return reconciler.ContinueProcessing()
}

// EnsureSnapshotEnvironmentBindingExist is an operation that will ensure that all
// SnapshotEnvironmentBindings for non-ephemeral root environments point to the newly constructed snapshot.
// If the bindings don't already exist, it will create new ones for each of the environments.
func (a *Adapter) EnsureSnapshotEnvironmentBindingExist() (reconciler.OperationResult, error) {
	canSnapshotBePromoted, reasons := gitops.CanSnapshotBePromoted(a.snapshot)
	if !canSnapshotBePromoted {
		a.logger.Info("The Snapshot won't be deployed.",
			"reasons", strings.Join(reasons, ","))
		return reconciler.ContinueProcessing()
	}

	availableEnvironments, err := a.findAvailableEnvironments()
	if err != nil {
		return reconciler.RequeueWithError(err)
	}

	components, err := loader.GetAllApplicationComponents(a.client, a.context, a.application)
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
					"snapshotEnvironmentBinding.Application", snapshotEnvironmentBinding.Spec.Application,
					"snapshotEnvironmentBinding.Environment", snapshotEnvironmentBinding.Spec.Environment,
					"snapshotEnvironmentBinding.Snapshot", snapshotEnvironmentBinding.Spec.Snapshot)
				patch := client.MergeFrom(a.snapshot.DeepCopy())
				gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to update SnapshotEnvironmentBinding")
				a.logger.LogAuditEvent("Snapshot integration status marked as Invalid. Failed to update SnapshotEnvironmentBinding",
					a.snapshot, h.LogActionUpdate)
				return reconciler.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
			}
			a.logger.LogAuditEvent("Existing SnapshotEnvironmentBinding updated with Snapshot",
				snapshotEnvironmentBinding, h.LogActionUpdate,
				"snapshotEnvironmentBinding.Application", snapshotEnvironmentBinding.Spec.Application,
				"snapshotEnvironmentBinding.Environment", snapshotEnvironmentBinding.Spec.Environment,
				"snapshotEnvironmentBinding.Snapshot", snapshotEnvironmentBinding.Spec.Snapshot)

		} else {
			snapshotEnvironmentBinding, err = a.createSnapshotEnvironmentBindingForSnapshot(a.application, &availableEnvironment, a.snapshot, components)
			if err != nil {
				a.logger.Error(err, "Failed to create SnapshotEnvironmentBinding",
					"snapshotEnvironmentBinding.Application", snapshotEnvironmentBinding.Spec.Application,
					"snapshotEnvironmentBinding.Environment", snapshotEnvironmentBinding.Spec.Environment,
					"snapshotEnvironmentBinding.Snapshot", snapshotEnvironmentBinding.Spec.Snapshot)
				patch := client.MergeFrom(a.snapshot.DeepCopy())
				gitops.SetSnapshotIntegrationStatusAsInvalid(a.snapshot, "Failed to create SnapshotEnvironmentBinding")
				a.logger.LogAuditEvent("Snapshot integration status marked as Invalid. Failed to create SnapshotEnvironmentBinding",
					a.snapshot, h.LogActionUpdate)
				return reconciler.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
			}
			a.logger.LogAuditEvent("SnapshotEnvironmentBinding created for Snapshot",
				snapshotEnvironmentBinding, h.LogActionAdd,
				"snapshotEnvironmentBinding.Application", snapshotEnvironmentBinding.Spec.Application,
				"snapshotEnvironmentBinding.Environment", snapshotEnvironmentBinding.Spec.Environment,
				"snapshotEnvironmentBinding.Snapshot", snapshotEnvironmentBinding.Spec.Snapshot)

		}
	}
	return reconciler.ContinueProcessing()
}

// createMissingReleasesForReleasePlans checks if there's existing Releases for a given list of ReleasePlans and creates
// new ones if they are missing. In case the Releases can't be created, an error will be returned.
func (a *Adapter) createMissingReleasesForReleasePlans(application *applicationapiv1alpha1.Application, releasePlans *[]releasev1alpha1.ReleasePlan, snapshot *applicationapiv1alpha1.Snapshot) error {
	releases, err := loader.GetReleasesWithSnapshot(a.client, a.context, a.snapshot)
	if err != nil {
		return err
	}

	for _, releasePlan := range *releasePlans {
		releasePlan := releasePlan // G601
		existingRelease := release.FindMatchingReleaseWithReleasePlan(releases, releasePlan)
		if existingRelease != nil {
			a.logger.Info("Found existing Release",
				"snapshot.Name", snapshot.Name,
				"releasePlan.Name", releasePlan.Name,
				"release.Name", existingRelease.Name)
		} else {
			newRelease := release.NewReleaseForReleasePlan(&releasePlan, snapshot)
			// Propagate annotations/labels from snapshot to Release
			h.CopyAnnotationsByPrefix(&snapshot.ObjectMeta, &newRelease.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
			h.CopyLabelsByPrefix(&snapshot.ObjectMeta, &newRelease.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)

			err := ctrl.SetControllerReference(application, newRelease, a.client.Scheme())
			if err != nil {
				return err
			}
			err = a.client.Create(a.context, newRelease)
			if err != nil {
				return err
			}
			a.logger.LogAuditEvent("Created a new Release", newRelease, h.LogActionAdd,
				"releasePlan.Name", releasePlan.Name)
		}
	}
	return nil
}

// findAvailableEnvironments gets all environments that don't have a ParentEnvironment and are not tagged as ephemeral.
func (a *Adapter) findAvailableEnvironments() (*[]applicationapiv1alpha1.Environment, error) {
	allEnvironments, err := loader.GetAllEnvironments(a.client, a.context, a.application)
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

// createIntegrationPipelineRun creates and returns a new integration PipelineRun. The Pipeline information and the parameters to it
// will be extracted from the given integrationScenario. The integration's Snapshot will also be passed to the integration PipelineRun.
func (a *Adapter) createIntegrationPipelineRun(application *applicationapiv1alpha1.Application, integrationTestScenario *v1alpha1.IntegrationTestScenario, snapshot *applicationapiv1alpha1.Snapshot) (*v1beta1.PipelineRun, error) {
	pipelineRun := tekton.NewIntegrationPipelineRun(snapshot.Name, application.Namespace, *integrationTestScenario).
		WithSnapshot(snapshot).
		WithIntegrationLabels(integrationTestScenario).
		WithApplicationAndComponent(a.application, a.component).
		WithExtraParams(integrationTestScenario.Spec.Params).
		AsPipelineRun()
	// copy PipelineRun PAC annotations/labels from snapshot to integration test PipelineRuns
	h.CopyAnnotationsByPrefix(&snapshot.ObjectMeta, &pipelineRun.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
	h.CopyLabelsByPrefix(&snapshot.ObjectMeta, &pipelineRun.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
	err := ctrl.SetControllerReference(snapshot, pipelineRun, a.client.Scheme())
	if err != nil {
		return nil, err
	}
	err = a.client.Create(a.context, pipelineRun)
	if err != nil {
		return nil, err
	}

	return pipelineRun, nil
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

// createCopyOfExistingEnvironment uses existing env as input, specifies namespace where the environment is situated,
// integrationTestScenario contains information about existing environment
// snapshot is mainly used for adding labels
// returns copy of already existing environment with updated envVars
func (a *Adapter) createCopyOfExistingEnvironment(existingEnvironment *applicationapiv1alpha1.Environment, namespace string, integrationTestScenario *v1alpha1.IntegrationTestScenario, snapshot *applicationapiv1alpha1.Snapshot, application *applicationapiv1alpha1.Application) (*applicationapiv1alpha1.Environment, error) {
	// Try to find a available DeploymentTargetClass with the right provisioner
	deploymentTargetClass, err := a.FindAvailableDeploymentTargetClass()
	if err != nil || deploymentTargetClass == nil {
		a.logger.Error(err, "Failed to find deploymentTargetClass with right provisioner for copy of existingEnvironment!",
			"existingEnvironment.NameSpace", existingEnvironment.Namespace,
			"existingEnvironment.Name", existingEnvironment.Name,
			"deploymentTargetClass.Provisioner", applicationapiv1alpha1.Provisioner_Devsandbox)
		return nil, err
	}
	a.logger.Info("Found DeploymentTargetClass with Provisioner appstudio.redhat.com/devsandbox, creating new DeploymentTargetClaim for Environment",
		"deploymentTargetClass.Name", deploymentTargetClass.Name)

	dtc, err := a.CreateDeploymentTargetClaimForEnvironment(existingEnvironment.Namespace, deploymentTargetClass.Name)
	if err != nil {
		a.logger.Error(err, "Failed to create deploymentTargetClaim with deploymentTargetClass for copy of environment!",
			"existingEnvironment.NameSpace", existingEnvironment.Namespace,
			"existingEnvironment.Name", existingEnvironment.Name,
			"deploymentTargetClass.Name", deploymentTargetClass.Name)
		return nil, fmt.Errorf("failed to create deploymentTargetClaim with deploymentTargetClass %s: %w", deploymentTargetClass.Name, err)
	}
	a.logger.LogAuditEvent("DeploymentTargetClaim is created for environment", dtc, h.LogActionAdd,
		"integrationTestScenario.Name", integrationTestScenario.Name)

	environment := gitops.NewCopyOfExistingEnvironment(existingEnvironment, namespace, integrationTestScenario, dtc.Name).
		WithIntegrationLabels(integrationTestScenario).
		WithSnapshot(snapshot).
		AsEnvironment()
	ref := ctrl.SetControllerReference(application, environment, a.client.Scheme())
	if ref != nil {
		a.logger.Error(ref, "Failed to set controller reference for Environment!",
			"application.Name", application.Name,
			"environment.Name", environment.Name)
	}

	err = a.client.Create(a.context, environment)
	if err != nil {
		// We don't want to leave the new DTC on the cluster without the matching environment
		dtcErr := a.client.Delete(a.context, dtc)
		if dtcErr != nil {
			return nil, fmt.Errorf("failed to delete DTC %s: %v; failed to create ephemeral environment %s: %w", dtc.Name, dtcErr, environment.Name, err)
		}
		a.logger.LogAuditEvent("Deleted DTC after creation of environment failed", dtc, h.LogActionDelete,
			"integrationTestScenario.Name", integrationTestScenario.Name)
		return nil, fmt.Errorf("failed to create ephemeral environment %s: %w", environment.Name, err)
	}
	a.logger.LogAuditEvent("Ephemeral environment is created for integrationTestScenario", environment, h.LogActionAdd,
		"integrationTestScenario.Name", integrationTestScenario.Name)
	return environment, nil
}

// CreateDeploymentTargetClaimForEnvironment creates a new DeploymentTargetClaim based on the supplied Namespace and DeploymentTargetClassName
func (a *Adapter) CreateDeploymentTargetClaimForEnvironment(namespace string, deploymentTargetClassName string) (*applicationapiv1alpha1.DeploymentTargetClaim, error) {
	deploymentTargetClaim := gitops.NewDeploymentTargetClaim(namespace, deploymentTargetClassName)
	err := a.client.Create(a.context, deploymentTargetClaim)
	if err != nil {
		return nil, fmt.Errorf("failed to create DeploymentTargetClaim %s: %w", deploymentTargetClaim.Name, err)
	}

	return deploymentTargetClaim, nil
}

// FindAvailableDeploymentTargetClass attempts to find a DeploymentTargetClass with applicationapiv1alpha1.Provisioner_Devsandbox as provisioner.
func (a *Adapter) FindAvailableDeploymentTargetClass() (*applicationapiv1alpha1.DeploymentTargetClass, error) {
	deploymentTargetClassList := &applicationapiv1alpha1.DeploymentTargetClassList{}
	err := a.client.List(a.context, deploymentTargetClassList)
	if err != nil {
		return nil, fmt.Errorf("cannot find the avaiable DeploymentTargetClass with provisioner %s: %w", applicationapiv1alpha1.Provisioner_Devsandbox, err)
	}

	for _, dtcls := range deploymentTargetClassList.Items {
		if dtcls.Spec.Provisioner == applicationapiv1alpha1.Provisioner_Devsandbox {
			return &dtcls, nil
		}
	}

	return nil, fmt.Errorf("cannot find the avaiable DeploymentTargetClass with provisioner %s", applicationapiv1alpha1.Provisioner_Devsandbox)
}

// getEnvironmentFromIntegrationTestScenario looks for already existing environment, if it exists it is returned, if not, nil is returned then together with
// information about what went wrong
func (a *Adapter) getEnvironmentFromIntegrationTestScenario(integrationTestScenario *v1alpha1.IntegrationTestScenario) (*applicationapiv1alpha1.Environment, error) {
	existingEnv := &applicationapiv1alpha1.Environment{}

	err := a.client.Get(a.context, types.NamespacedName{
		Namespace: a.application.Namespace,
		Name:      integrationTestScenario.Spec.Environment.Name,
	}, existingEnv)

	if err != nil {
		a.logger.Info("Environment doesn't exist in same namespace as IntegrationTestScenario at all.",
			"integrationTestScenario:", integrationTestScenario.Name,
			"environment:", integrationTestScenario.Spec.Environment.Name)
		return nil, fmt.Errorf("environment %s doesn't exist in same namespace as IntegrationTestScenario at all: %w", integrationTestScenario.Spec.Environment.Name, err)
	}
	return existingEnv, nil
}
