/*
Copyright 2023.
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

package binding

import (
	"context"
	"fmt"
	"time"

	"github.com/redhat-appstudio/operator-toolkit/controller"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/gitops"
	h "github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/loader"
	"github.com/redhat-appstudio/integration-service/tekton"
	pipeline "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile a SnapshotEnvironmentBinding.
type Adapter struct {
	snapshotEnvironmentBinding *applicationapiv1alpha1.SnapshotEnvironmentBinding
	snapshot                   *applicationapiv1alpha1.Snapshot
	application                *applicationapiv1alpha1.Application
	component                  *applicationapiv1alpha1.Component
	environment                *applicationapiv1alpha1.Environment
	integrationTestScenario    *v1beta1.IntegrationTestScenario
	logger                     h.IntegrationLogger
	client                     client.Client
	context                    context.Context
	loader                     loader.ObjectLoader
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(snapshotEnvironmentBinding *applicationapiv1alpha1.SnapshotEnvironmentBinding, snapshot *applicationapiv1alpha1.Snapshot, environment *applicationapiv1alpha1.Environment, application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, integrationTestScenario *v1beta1.IntegrationTestScenario, logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		snapshotEnvironmentBinding: snapshotEnvironmentBinding,
		snapshot:                   snapshot,
		environment:                environment,
		application:                application,
		component:                  component,
		integrationTestScenario:    integrationTestScenario,
		logger:                     logger,
		loader:                     loader,
		client:                     client,
		context:                    context,
	}
}

// EnsureIntegrationTestPipelineForScenarioExists is an operation that will ensure that the Integration test pipeline
// associated with the Snapshot and the SnapshotEnvironmentBinding's IntegrationTestScenarios exist.
func (a *Adapter) EnsureIntegrationTestPipelineForScenarioExists() (controller.OperationResult, error) {
	if gitops.HaveAppStudioTestsFinished(a.snapshot) {
		a.logger.Info("The Snapshot has finished testing.")
		return controller.ContinueProcessing()
	}

	if gitops.HaveBindingsFailed(a.snapshotEnvironmentBinding) {
		a.logger.Info("The SnapshotEnvironmentBinding has failed to deploy on ephemeral environment and will be deleted later.", "snapshotEnvironmentBinding.Name", a.snapshotEnvironmentBinding.Name)
		return controller.ContinueProcessing()
	}

	if !gitops.IsBindingDeployed(a.snapshotEnvironmentBinding) {
		a.logger.Info("The SnapshotEnvironmentBinding hasn't yet deployed to the ephemeral environment.", "snapshotEnvironmentBinding.Name", a.snapshotEnvironmentBinding.Name)
		return controller.ContinueProcessing()
	}
	a.logger.Info("The SnapshotEnvironmentBinding's deployment succeeded", "snapshotEnvironmentBinding.Name", a.snapshotEnvironmentBinding.Name)

	if a.integrationTestScenario != nil {
		integrationPipelineRun, err := loader.GetLatestPipelineRunForSnapshotAndScenario(a.client, a.context, a.loader, a.snapshot, a.integrationTestScenario)
		if err != nil {
			a.logger.Error(err, "Failed to get latest pipelineRun for snapshot and scenario",
				"snapshot", a.snapshot,
				"integrationTestScenario", a.integrationTestScenario)
			return controller.RequeueWithError(err)
		}
		if integrationPipelineRun != nil {
			a.logger.Info("Found existing integrationPipelineRun",
				"integrationTestScenario.Name", a.integrationTestScenario.Name,
				"integrationPipelineRun.Name", integrationPipelineRun.Name)
		} else {
			a.logger.Info("Creating new pipelinerun for integrationTestscenario",
				"integrationTestScenario.Name", a.integrationTestScenario.Name,
				"app name", a.application.Name,
				"namespace", a.application.Namespace)
			pipelineRun, err := a.createIntegrationPipelineRunWithEnvironment(a.application, a.integrationTestScenario, a.snapshot, a.environment)
			if err != nil {
				a.logger.Error(err, "Failed to create pipelineRun for snapshot, environment and scenario")
				return controller.RequeueWithError(err)
			}
			a.logger.LogAuditEvent("PipelineRun for snapshot created", pipelineRun, h.LogActionAdd,
				"snapshot.Name", a.snapshot.Name)
		}
	}

	return controller.ContinueProcessing()
}

// EnsureEphemeralEnvironmentsCleanedUp will ensure that ephemeral environment(s) associated with the
// SnapshotEnvironmentBinding are cleaned up.
func (a *Adapter) EnsureEphemeralEnvironmentsCleanedUp() (controller.OperationResult, error) {
	if !gitops.HaveBindingsFailed(a.snapshotEnvironmentBinding) {
		return controller.ContinueProcessing()
	}

	// The default value for ErrorOccured is 'True'.  We know that the state
	// of the condition is still 'True' due to the earlier call to
	// HaveBindingsFailed().  If this condition is still true after a
	// reasonable time then we assume that the SEB is stuck in an unrecoverable
	// state and clean it up.  Otherwise we requeue and wait until the timeout
	// has passed
	const snapshotEnvironmentBindingErrorTimeoutSeconds float64 = 300
	var lastTransitionTime time.Time
	bindingStatus := meta.FindStatusCondition(a.snapshotEnvironmentBinding.Status.BindingConditions, gitops.BindingErrorOccurredStatusConditionType)
	if bindingStatus != nil {
		lastTransitionTime = bindingStatus.LastTransitionTime.Time
	}
	sinceLastTransition := time.Since(lastTransitionTime).Seconds()
	if sinceLastTransition < snapshotEnvironmentBindingErrorTimeoutSeconds {
		a.logger.Info(fmt.Sprintf("SnapshotEnvironmentBinding has been in error state for %f "+
			"seconds,  which is less than threshold time of %f. Requeueing cleanup after delay.",
			sinceLastTransition, snapshotEnvironmentBindingErrorTimeoutSeconds))
		return controller.RequeueAfter(time.Duration(snapshotEnvironmentBindingErrorTimeoutSeconds*float64(time.Second)), nil)
	} else {
		a.logger.Info(fmt.Sprintf("SEB has been in the error state for more than the threshold time of %f seconds", snapshotEnvironmentBindingErrorTimeoutSeconds))
	}

	// mark snapshot as failed
	snapshotErrorMessage := "Encountered issue deploying snapshot on ephemeral environments: " +
		meta.FindStatusCondition(a.snapshotEnvironmentBinding.Status.BindingConditions, "ErrorOccurred").Message
	a.logger.Info("The SnapshotEnvironmentBinding encountered an issue deploying snapshot on ephemeral environments",
		"snapshotEnvironmentBinding.Name", a.snapshotEnvironmentBinding.Name,
		"message", snapshotErrorMessage)
	_, err := gitops.MarkSnapshotAsFailed(a.client, a.context, a.snapshot, snapshotErrorMessage)
	if err != nil {
		a.logger.Error(err, "Failed to Update Snapshot status")
		return controller.RequeueWithError(err)
	}

	deploymentTargetClaim, err := a.loader.GetDeploymentTargetClaimForEnvironment(a.client, a.context, a.environment)
	if err != nil {
		a.logger.Error(err, "failed to find deploymentTargetClaim for environment",
			"environment", a.environment.Name)
		return controller.RequeueWithError(err)
	}

	err = h.CleanUpEphemeralEnvironments(a.client, &a.logger, a.context, a.environment, deploymentTargetClaim)
	if err != nil {
		a.logger.Error(err, "Failed to delete the Ephemeral Environment")
		return controller.RequeueWithError(err)
	}

	return controller.ContinueProcessing()

}

// createIntegrationPipelineRunWithEnvironment creates new integration PipelineRun. The Pipeline information and the parameters to it
// will be extracted from the given integrationScenario. The integration's Snapshot will also be passed to the integration PipelineRun.
// If the creation of the PipelineRun is unsuccessful, an error will be returned.
func (a *Adapter) createIntegrationPipelineRunWithEnvironment(application *applicationapiv1alpha1.Application, integrationTestScenario *v1beta1.IntegrationTestScenario, snapshot *applicationapiv1alpha1.Snapshot, environment *applicationapiv1alpha1.Environment) (*pipeline.PipelineRun, error) {
	deploymentTarget, err := a.getDeploymentTargetForEnvironment(environment)
	if err != nil || deploymentTarget == nil {
		return nil, err
	}

	pipelineRun := tekton.NewIntegrationPipelineRun(snapshot.Name, application.Namespace, *integrationTestScenario).
		WithSnapshot(snapshot).
		WithApplicationAndComponent(a.application, a.component).
		WithIntegrationLabels(integrationTestScenario).
		WithEnvironmentAndDeploymentTarget(deploymentTarget, environment.Name).
		AsPipelineRun()
	// copy PipelineRun PAC annotations/labels from snapshot to integration test PipelineRuns
	h.CopyAnnotationsByPrefix(&snapshot.ObjectMeta, &pipelineRun.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
	h.CopyLabelsByPrefix(&snapshot.ObjectMeta, &pipelineRun.ObjectMeta, gitops.PipelinesAsCodePrefix, gitops.PipelinesAsCodePrefix)
	err = ctrl.SetControllerReference(snapshot, pipelineRun, a.client.Scheme())
	if err != nil {
		return nil, err
	}
	err = a.client.Create(a.context, pipelineRun)
	if err != nil {
		return nil, err
	}

	return pipelineRun, nil

}

// getDeploymentTargetForEnvironment gets the DeploymentTarget associated with Environment, if the DeploymentTarget is not found, an error will be returned
func (a *Adapter) getDeploymentTargetForEnvironment(environment *applicationapiv1alpha1.Environment) (*applicationapiv1alpha1.DeploymentTarget, error) {
	deploymentTargetClaim, err := a.loader.GetDeploymentTargetClaimForEnvironment(a.client, a.context, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to find deploymentTargetClaim defined in environment %s: %w", environment.Name, err)
	}

	deploymentTarget, err := a.loader.GetDeploymentTargetForDeploymentTargetClaim(a.client, a.context, deploymentTargetClaim)
	if err != nil {
		return nil, fmt.Errorf("failed to find deploymentTarget defined in deploymentTargetClaim %s: %w", deploymentTargetClaim.Name, err)
	}

	return deploymentTarget, nil
}
