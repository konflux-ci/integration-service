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

package integrationpipeline

import (
	"context"
	"fmt"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/gitops"
	h "github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/loader"
	"github.com/redhat-appstudio/integration-service/metrics"
	"github.com/redhat-appstudio/integration-service/status"
	"github.com/redhat-appstudio/integration-service/tekton"
	"github.com/redhat-appstudio/operator-toolkit/controller"
	"github.com/redhat-appstudio/operator-toolkit/metadata"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Adapter holds the objects needed to reconcile an integration PipelineRun.
type Adapter struct {
	pipelineRun *tektonv1beta1.PipelineRun
	component   *applicationapiv1alpha1.Component
	application *applicationapiv1alpha1.Application
	loader      loader.ObjectLoader
	logger      h.IntegrationLogger
	client      client.Client
	context     context.Context
	status      status.Status
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(pipelineRun *tektonv1beta1.PipelineRun, component *applicationapiv1alpha1.Component, application *applicationapiv1alpha1.Application, logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
	context context.Context) *Adapter {
	return &Adapter{
		pipelineRun: pipelineRun,
		component:   component,
		application: application,
		logger:      logger,
		loader:      loader,
		client:      client,
		context:     context,
		status:      status.NewAdapter(logger.Logger, client),
	}
}

// EnsureSnapshotPassedAllTests is an operation that will ensure that a pipeline Snapshot
// to the PipelineRun being processed passed all tests for all defined non-optional IntegrationTestScenarios.
func (a *Adapter) EnsureSnapshotPassedAllTests() (controller.OperationResult, error) {
	if !h.HasPipelineRunFinished(a.pipelineRun) {
		return controller.ContinueProcessing()
	}
	existingSnapshot, err := a.loader.GetSnapshotFromPipelineRun(a.client, a.context, a.pipelineRun)
	if err != nil {
		return controller.RequeueWithError(err)
	}
	if existingSnapshot != nil {
		a.logger.Info("Found existing Snapshot",
			"snapshot.Name", existingSnapshot.Name,
			"snapshot.Spec.Components", existingSnapshot.Spec.Components)
	}

	// Get all integrationTestScenarios for the Application and then find the latest Succeeded Integration PipelineRuns
	// for the Snapshot
	integrationTestScenarios, err := a.loader.GetRequiredIntegrationTestScenariosForApplication(a.client, a.context, a.application)
	if err != nil {
		return controller.RequeueWithError(err)
	}
	integrationPipelineRuns, err := a.getAllPipelineRunsForSnapshot(existingSnapshot, integrationTestScenarios)
	if err != nil {
		a.logger.Error(err, "Failed to get Integration PipelineRuns",
			"snapshot.Name", existingSnapshot.Name)
		return controller.RequeueWithError(err)
	}

	// Skip doing anything if not all Integration PipelineRuns were found for all integrationTestScenarios
	if len(*integrationTestScenarios) != len(*integrationPipelineRuns) {
		a.logger.Info("Not all required Integration PipelineRuns finished",
			"snapshot.Name", existingSnapshot.Name,
			"snapshot.Spec.Components", existingSnapshot.Spec.Components)
		return controller.ContinueProcessing()
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
			"snapshot.Name", existingSnapshot.Name)
		return controller.RequeueWithError(err)
	}

	// If the snapshot is a component type, check if the global component list changed in the meantime and
	// create a composite snapshot if it did. Does not apply for PAC pull request events.
	if a.component != nil && metadata.HasLabelWithValue(existingSnapshot, gitops.SnapshotTypeLabel, gitops.SnapshotComponentType) && !gitops.IsSnapshotCreatedByPACPullRequestEvent(existingSnapshot) {
		compositeSnapshot, err := a.createCompositeSnapshotsIfConflictExists(a.application, a.component, existingSnapshot)
		if err != nil {
			a.logger.Error(err, "Failed to determine if a composite snapshot needs to be created because of a conflict",
				"snapshot.Name", existingSnapshot.Name)
			return controller.RequeueWithError(err)
		}

		if compositeSnapshot != nil {
			a.logger.Info("The global component list has changed in the meantime, marking snapshot as Invalid",
				"snapshot.Name", existingSnapshot.Name)
			if !gitops.IsSnapshotMarkedAsInvalid(existingSnapshot) {
				patch := client.MergeFrom(existingSnapshot.DeepCopy())
				gitops.SetSnapshotIntegrationStatusAsInvalid(existingSnapshot,
					"The global component list has changed in the meantime, superseding with a composite snapshot")
				err := a.client.Status().Patch(a.context, existingSnapshot, patch)
				if err != nil {
					a.logger.Error(err, "Failed to update the status to Invalid for the snapshot",
						"snapshot.Name", existingSnapshot.Name)
					return controller.RequeueWithError(err)
				}
				a.logger.LogAuditEvent("Snapshot integration status condition marked as invalid, the global component list has changed in the meantime",
					existingSnapshot, h.LogActionUpdate)
			}
			return controller.ContinueProcessing()
		}
	}

	// If all Integration Pipeline runs passed, mark the snapshot as succeeded, otherwise mark it as failed
	// This updates the Snapshot resource on the cluster
	if allIntegrationPipelineRunsPassed {
		if !gitops.IsSnapshotMarkedAsPassed(existingSnapshot) {
			existingSnapshot, err = gitops.MarkSnapshotAsPassed(a.client, a.context, existingSnapshot, "All Integration Pipeline tests passed")
			if err != nil {
				a.logger.Error(err, "Failed to Update Snapshot AppStudioTestSucceeded status")
				return controller.RequeueWithError(err)
			}
			a.logger.LogAuditEvent("Snapshot integration status condition marked as passed, all Integration PipelineRuns succeeded",
				existingSnapshot, h.LogActionUpdate)
		}
	} else {
		if !gitops.IsSnapshotMarkedAsFailed(existingSnapshot) {
			existingSnapshot, err = gitops.MarkSnapshotAsFailed(a.client, a.context, existingSnapshot, "Some Integration pipeline tests failed")
			if err != nil {
				a.logger.Error(err, "Failed to Update Snapshot AppStudioTestSucceeded status")
				return controller.RequeueWithError(err)
			}
			a.logger.LogAuditEvent("Snapshot integration status condition marked as failed, some tests within Integration PipelineRuns failed",
				existingSnapshot, h.LogActionUpdate)
		}
	}

	return controller.ContinueProcessing()
}

// EnsureStatusReported will ensure that integration PipelineRun status is reported to the git provider
// which (indirectly) triggered its execution.
func (a *Adapter) EnsureStatusReported() (controller.OperationResult, error) {
	reporters, err := a.status.GetReporters(a.pipelineRun)

	if err != nil {
		return controller.RequeueWithError(err)
	}

	for _, reporter := range reporters {
		if err := reporter.ReportStatus(a.client, a.context, a.pipelineRun); err != nil {
			return controller.RequeueWithError(err)
		}
	}

	return controller.ContinueProcessing()
}

// EnsureStatusReportedInSnapshot will ensure that status of the integration test pipelines is reported to snapshot
// to be consumed by user
func (a *Adapter) EnsureStatusReportedInSnapshot() (controller.OperationResult, error) {

	// pipelines run in parallel and have great potential to cause conflict on update
	// thus `RetryOnConflict` is easy solution here, given the snaphost must be loaded specifically here
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {

		snapshot, err := a.loader.GetSnapshotFromPipelineRun(a.client, a.context, a.pipelineRun)
		if err != nil {
			return err
		}

		statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
		if err != nil {
			return err
		}

		status, detail, err := GetIntegrationPipelineRunStatus(a.client, a.context, a.pipelineRun)
		if err != nil {
			return err
		}
		statuses.UpdateTestStatusIfChanged(a.pipelineRun.Labels[tekton.ScenarioNameLabel], status, detail)
		if err = statuses.UpdateTestPipelineRunName(a.pipelineRun.Labels[tekton.ScenarioNameLabel], a.pipelineRun.Name); err != nil {
			return err
		}

		// don't return wrapped err for retries
		err = gitops.WriteIntegrationTestStatusesIntoSnapshot(snapshot, statuses, a.client, a.context)
		return err
	})
	if err != nil {
		a.logger.Error(err, "Failed to update pipeline status in snapshot")
		return controller.RequeueWithError(fmt.Errorf("failed to update test status in snapshot: %w", err))
	}
	return controller.ContinueProcessing()
}

// EnsureEphemeralEnvironmentsCleanedUp will ensure that ephemeral environment(s) associated with the
// integration PipelineRun are cleaned up.
func (a *Adapter) EnsureEphemeralEnvironmentsCleanedUp() (controller.OperationResult, error) {
	if !h.HasPipelineRunFinished(a.pipelineRun) {
		return controller.ContinueProcessing()
	}

	testEnvironment, err := a.loader.GetEnvironmentFromIntegrationPipelineRun(a.client, a.context, a.pipelineRun)
	if err != nil {
		a.logger.Error(err, "Failed to find the environment for the pipelineRun")
		return controller.RequeueWithError(err)
	} else if testEnvironment == nil {
		a.logger.Info("The pipelineRun does not have any test Environments associated with it, skipping cleanup.")
		return controller.ContinueProcessing()
	}

	isEphemeral := h.IsEnvironmentEphemeral(testEnvironment)

	if isEphemeral {
		dtc, err := a.loader.GetDeploymentTargetClaimForEnvironment(a.client, a.context, testEnvironment)
		if err != nil || dtc == nil {
			a.logger.Error(err, "Failed to find deploymentTargetClaim defined in environment", "environment.Name", testEnvironment.Name)
			return controller.RequeueWithError(err)
		}

		binding, err := a.loader.FindExistingSnapshotEnvironmentBinding(a.client, a.context, a.application, testEnvironment)
		if err != nil || binding == nil {
			a.logger.Error(err, "Failed to find snapshotEnvironmentBinding associated with environment", "environment.Name", testEnvironment.Name)
			return controller.RequeueWithError(err)
		}

		err = h.CleanUpEphemeralEnvironments(a.client, &a.logger, a.context, testEnvironment, dtc)
		if err != nil {
			a.logger.Error(err, "Failed to delete the Ephemeral Environment")
			return controller.RequeueWithError(err)
		}
	}

	return controller.ContinueProcessing()
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

// determineIfAllIntegrationPipelinesPassed checks all Integration pipelines passed all of their test tasks.
// Returns an error if it can't get the PipelineRun outcomes
func (a *Adapter) determineIfAllIntegrationPipelinesPassed(integrationPipelineRuns *[]tektonv1beta1.PipelineRun) (bool, error) {
	allIntegrationPipelineRunsPassed := true
	for _, integrationPipelineRun := range *integrationPipelineRuns {
		integrationPipelineRun := integrationPipelineRun // G601
		if !h.HasPipelineRunFinished(&integrationPipelineRun) {
			a.logger.Info(
				fmt.Sprintf("Integration Pipeline Run %s has not finished yet", integrationPipelineRun.Name),
				"pipelineRun.Name", integrationPipelineRun.Name,
				"pipelineRun.Namespace", integrationPipelineRun.Namespace,
			)
			allIntegrationPipelineRunsPassed = false
			continue
		}
		pipelineRunOutcome, err := h.GetIntegrationPipelineRunOutcome(a.client, a.context, &integrationPipelineRun)
		if err != nil {
			a.logger.Error(err, fmt.Sprintf("Failed to get Integration Pipeline Run %s outcome", integrationPipelineRun.Name),
				"pipelineRun.Name", integrationPipelineRun.Name,
				"pipelineRun.Namespace", integrationPipelineRun.Namespace,
			)
			return false, err
		}
		if !pipelineRunOutcome.HasPipelineRunPassedTesting() {
			if !pipelineRunOutcome.HasPipelineRunSucceeded() {
				a.logger.Info(
					fmt.Sprintf("integration Pipeline Run %s failed without test results of TaskRuns: %s", integrationPipelineRun.Name, h.GetPipelineRunFailedReason(&integrationPipelineRun)),
					"pipelineRun.Name", integrationPipelineRun.Name,
					"pipelineRun.Namespace", integrationPipelineRun.Namespace,
				)

			} else {
				a.logger.Info(
					fmt.Sprintf("Integration Pipeline Run %s did not pass all tests", integrationPipelineRun.Name),
					"pipelineRun.Name", integrationPipelineRun.Name,
					"pipelineRun.Namespace", integrationPipelineRun.Namespace,
				)
			}
			allIntegrationPipelineRunsPassed = false
		}
		if a.pipelineRun.Name == integrationPipelineRun.Name {
			// log only results of current pipeline run, other pipelines will be logged in their reconciliations
			// prevent mess in logs
			pipelineRunOutcome.LogResults(a.logger.Logger)
		}
	}
	return allIntegrationPipelineRunsPassed, nil
}

// getAllPipelineRunsForSnapshot loads from the cluster all Integration PipelineRuns for each IntegrationTestScenario
// associated with the Snapshot. If the Application doesn't have any IntegrationTestScenarios associated with it,
// an error will be returned.
func (a *Adapter) getAllPipelineRunsForSnapshot(snapshot *applicationapiv1alpha1.Snapshot, integrationTestScenarios *[]v1beta1.IntegrationTestScenario) (*[]tektonv1beta1.PipelineRun, error) {
	var integrationPipelineRuns []tektonv1beta1.PipelineRun
	for _, integrationTestScenario := range *integrationTestScenarios {
		integrationTestScenario := integrationTestScenario // G601
		if a.pipelineRun.Labels[tekton.ScenarioNameLabel] != integrationTestScenario.Name {
			integrationPipelineRun, err := loader.GetLatestPipelineRunForSnapshotAndScenario(a.client, a.context, a.loader, snapshot, &integrationTestScenario)
			if err != nil {
				return nil, err
			}
			if integrationPipelineRun != nil {
				a.logger.Info("Found existing integrationPipelineRun",
					"integrationTestScenario.Name", integrationTestScenario.Name,
					"integrationPipelineRun.Name", integrationPipelineRun.Name)
				integrationPipelineRuns = append(integrationPipelineRuns, *integrationPipelineRun)
			}
		} else {
			integrationPipelineRuns = append(integrationPipelineRuns, *a.pipelineRun)
			a.logger.Info("The current integrationPipelineRun matches the integration test scenario",
				"integrationTestScenario.Name", integrationTestScenario.Name,
				"integrationPipelineRun.Name", a.pipelineRun.Name)
		}
	}

	return &integrationPipelineRuns, nil
}

// prepareCompositeSnapshot prepares the Composite Snapshot for a given application,
// componentnew, containerImage and newContainerSource. In case the Snapshot can't be created, an error will be returned.
func (a *Adapter) prepareCompositeSnapshot(application *applicationapiv1alpha1.Application, component *applicationapiv1alpha1.Component, newContainerImage string, newComponentSource *applicationapiv1alpha1.ComponentSource) (*applicationapiv1alpha1.Snapshot, error) {
	applicationComponents, err := a.loader.GetAllApplicationComponents(a.client, a.context, application)
	if err != nil {
		return nil, err
	}

	snapshot, err := gitops.PrepareSnapshot(a.client, a.context, application, applicationComponents, component, newContainerImage, newComponentSource)
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
	_ = metadata.CopyLabelsByPrefix(&testedSnapshot.ObjectMeta, &compositeSnapshot.ObjectMeta, gitops.PipelinesAsCodePrefix)
	_ = metadata.CopyAnnotationsByPrefix(&testedSnapshot.ObjectMeta, &compositeSnapshot.ObjectMeta, gitops.PipelinesAsCodePrefix)

	// Mark tested snapshot as failed and create the new composite snapshot if it doesn't exist already
	if !gitops.CompareSnapshots(compositeSnapshot, testedSnapshot) {
		allSnapshots, err := a.loader.GetAllSnapshots(a.client, a.context, application)
		if err != nil {
			return nil, err
		}
		existingCompositeSnapshot := gitops.FindMatchingSnapshot(a.application, allSnapshots, compositeSnapshot)

		if existingCompositeSnapshot != nil {
			a.logger.Info("Found existing composite Snapshot",
				"snapshot.Name", existingCompositeSnapshot.Name,
				"snapshot.Spec.Components", existingCompositeSnapshot.Spec.Components)
			return existingCompositeSnapshot, nil
		} else {
			err = a.client.Create(a.context, compositeSnapshot)
			if err != nil {
				return nil, err
			}
			go metrics.RegisterNewSnapshot()
			a.logger.LogAuditEvent("CompositeSnapshot created", compositeSnapshot, h.LogActionAdd,
				"snapshot.Spec.Components", compositeSnapshot.Spec.Components)
			return compositeSnapshot, nil
		}
	}

	return nil, nil
}

// GetIntegrationPipelineRunStatus checks the Tekton results for a given PipelineRun and returns status of test.
func GetIntegrationPipelineRunStatus(adapterClient client.Client, ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) (gitops.IntegrationTestStatus, string, error) {
	// Check if the pipelineRun finished from the condition of status
	if !h.HasPipelineRunFinished(pipelineRun) {
		return gitops.IntegrationTestStatusInProgress, fmt.Sprintf("Integration test is running as pipeline run '%s'", pipelineRun.Name), nil
	}

	outcome, err := h.GetIntegrationPipelineRunOutcome(adapterClient, ctx, pipelineRun)
	if err != nil {
		return gitops.IntegrationTestStatusTestFail, "", fmt.Errorf("failed to evaluate inegration test results: %w", err)
	}

	if !outcome.HasPipelineRunPassedTesting() {
		return gitops.IntegrationTestStatusTestFail, "Integration test failed", nil
	}

	return gitops.IntegrationTestStatusTestPassed, "Integration test passed", nil
}
