/*
Copyright 2022 Red Hat Inc.

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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	clienterrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	h "github.com/konflux-ci/integration-service/helpers"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/pkg/metrics"
	"github.com/konflux-ci/integration-service/release"
	"github.com/konflux-ci/integration-service/status"
	"github.com/konflux-ci/integration-service/tekton"

	"github.com/konflux-ci/integration-service/loader"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	"github.com/konflux-ci/operator-toolkit/controller"
	"github.com/konflux-ci/operator-toolkit/metadata"
	releasev1alpha1 "github.com/konflux-ci/release-service/api/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const SnapshotRetryTimeout = time.Duration(3 * time.Hour)

// configuration options for scenario
type ScenarioOptions struct {
	IsReRun bool
}

// Adapter holds the objects needed to reconcile a Release.
type Adapter struct {
	snapshot    *applicationapiv1alpha1.Snapshot
	application *applicationapiv1alpha1.Application
	logger      h.IntegrationLogger
	loader      loader.ObjectLoader
	client      client.Client
	context     context.Context
	status      status.StatusInterface
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(context context.Context, snapshot *applicationapiv1alpha1.Snapshot, application *applicationapiv1alpha1.Application, logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
) *Adapter {
	return &Adapter{
		snapshot:    snapshot,
		application: application,
		logger:      logger,
		loader:      loader,
		client:      client,
		context:     context,
		status:      status.NewStatus(logger.Logger, client),
	}
}

func scenariosNamesToList(integrationTestScenarios *[]v1beta2.IntegrationTestScenario) *[]string {
	// transform list of structs into list of strings
	result := make([]string, 0, len(*integrationTestScenarios))
	for _, v := range *integrationTestScenarios {
		result = append(result, v.Name)
	}
	return &result
}

// EnsureRerunPipelineRunsExist is responsible for recreating integration test pipelineruns triggered by users
func (a *Adapter) EnsureRerunPipelineRunsExist() (controller.OperationResult, error) {
	runLabelValue, ok := gitops.GetIntegrationTestRunLabelValue(a.snapshot)
	if !ok {
		return controller.ContinueProcessing()
	}

	integrationTestScenarios, opResult, err := a.getScenariosToRerun(runLabelValue)
	if integrationTestScenarios == nil {
		return opResult, err
	}

	testStatuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(a.snapshot)
	if err != nil {
		return controller.RequeueWithError(err)
	}

	skipScenarioRerunCount, opResult, err := a.handleScenarioReruns(integrationTestScenarios, testStatuses)
	if opResult.CancelRequest || err != nil {
		return opResult, err
	}

	if err = a.cleanupRerunLabelAndUpdateStatus(skipScenarioRerunCount, len(*integrationTestScenarios), runLabelValue, testStatuses); err != nil {
		return controller.RequeueWithError(err)
	}

	return controller.ContinueProcessing()
}

// getScenariosToRerun fetches and filters the IntegrationTestScenarios based on runLabelValue.
func (a *Adapter) getScenariosToRerun(runLabelValue string) (*[]v1beta2.IntegrationTestScenario, controller.OperationResult, error) {
	if runLabelValue == "all" {
		scenarios, err := a.loader.GetAllIntegrationTestScenariosForSnapshot(a.context, a.client, a.application, a.snapshot)
		if err != nil {
			a.logger.Error(err, "Failed to get IntegrationTestScenarios", "Application.Namespace", a.application.Namespace)
			opResult, err := controller.RequeueWithError(err)
			return nil, opResult, err
		}
		if scenarios == nil {
			a.logger.Info("None of the Scenarios' context are applicable to the Snapshot, nothing to re-run")
		}

		return scenarios, controller.OperationResult{}, nil
	}

	scenario, err := a.loader.GetScenario(a.context, a.client, runLabelValue, a.application.Namespace)
	if err != nil {
		if clienterrors.IsNotFound(err) {
			a.logger.Error(err, "IntegrationTestScenario not found", "Scenario", runLabelValue)
			if err = gitops.RemoveIntegrationTestRerunLabel(a.context, a.client, a.snapshot); err != nil {
				opResult, err := controller.RequeueWithError(err)
				return nil, opResult, err
			}
			opResult, _ := controller.ContinueProcessing()
			return nil, opResult, nil
		}
		opResult, err := controller.RequeueWithError(fmt.Errorf("failed to fetch scenario %s: %w", runLabelValue, err))
		return nil, opResult, err
	}
	return &[]v1beta2.IntegrationTestScenario{*scenario}, controller.OperationResult{}, nil
}

// handleScenarioReruns iterates through scenarios, rerunning tests as needed and updating test statuses.
func (a *Adapter) handleScenarioReruns(scenarios *[]v1beta2.IntegrationTestScenario, testStatuses *intgteststat.SnapshotIntegrationTestStatuses) (int, controller.OperationResult, error) {
	skipScenarioRerunCount := 0
	for _, scenario := range *scenarios {
		scenario := scenario
		status, found := testStatuses.GetScenarioStatus(scenario.Name)
		if found && (status.Status == intgteststat.IntegrationTestStatusInProgress || status.Status == intgteststat.IntegrationTestStatusPending) {
			a.logger.Info("Skipping re-run for IntegrationTestScenario since it's in 'InProgress' or 'Pending' state", "Scenario", scenario.Name)
			skipScenarioRerunCount++
			continue
		}

		testStatuses.ResetStatus(scenario.Name)
		if opResult, err := a.rerunIntegrationPipelinerunForScenario(&scenario, testStatuses); opResult.CancelRequest || err != nil {
			a.logger.Error(err, "Failed to create rerun pipelinerun for IntegrationTestScenario", "Scenario", scenario.Name)
			return -1, opResult, err
		}
	}
	return skipScenarioRerunCount, controller.OperationResult{}, nil
}

// rerunIntegrationPipelinerunForScenario creates a pipelinerun for the given scenario and updates its status.
func (a *Adapter) rerunIntegrationPipelinerunForScenario(scenario *v1beta2.IntegrationTestScenario, testStatuses *intgteststat.SnapshotIntegrationTestStatuses) (controller.OperationResult, error) {
	pipelineRun, err := a.createIntegrationPipelineRun(a.application, scenario, a.snapshot)
	if err != nil {
		return a.HandlePipelineCreationError(err, scenario, testStatuses)
	}
	if !gitops.IsSnapshotCreatedByPACPushEvent(a.snapshot) {
		err = a.checkAndCancelOldSnapshotsPipelineRun(a.application, a.snapshot)
		if err != nil {
			a.logger.Error(err, "Failed to check and cancel old snapshot's pipelineruns",
				"snapshot.Name:", a.snapshot.Name)
		}
	}

	testStatuses.UpdateTestStatusIfChanged(scenario.Name, intgteststat.IntegrationTestStatusInProgress, fmt.Sprintf("PipelineRun '%s' created", pipelineRun.Name))
	if err := testStatuses.UpdateTestPipelineRunName(scenario.Name, pipelineRun.Name); err != nil {
		// it doesn't make sense to restart reconciliation here, it will be eventually updated by integrationpipeline adapter
		a.logger.Error(err, "Failed to update pipeline run name in test status")
	}
	return controller.OperationResult{}, nil
}

// cleanupRerunLabelAndUpdateStatus removes rerun labels and updates snapshot status.
func (a *Adapter) cleanupRerunLabelAndUpdateStatus(skipCount, totalScenarios int, runLabelValue string, testStatuses *intgteststat.SnapshotIntegrationTestStatuses) error {
	if err := gitops.RemoveIntegrationTestRerunLabel(a.context, a.client, a.snapshot); err != nil {
		return err
	}

	if skipCount == totalScenarios {
		a.logger.Info(fmt.Sprintf("%[1]d out of %[1]d requested IntegrationTestScenario(s) are either in 'InProgress' or 'Pending' state, skipping their re-runs", totalScenarios), "Label", runLabelValue)
		return nil
	}

	if err := gitops.WriteIntegrationTestStatusesIntoSnapshot(a.context, a.snapshot, testStatuses, a.client); err != nil {
		return err
	}

	if err := gitops.ResetSnapshotStatusConditions(a.context, a.client, a.snapshot, "Integration test re-run initiated for Snapshot"); err != nil {
		a.logger.Error(err, "Failed to reset snapshot status conditions")
		return err
	}

	return nil
}

// loadAndFilterIntegrationTestScenarios loads all test scenarios and filters them based on context
// Returns nil if no scenarios are found, but logs errors and continues processing
func (a *Adapter) loadAndFilterIntegrationTestScenarios() *[]v1beta2.IntegrationTestScenario {
	allIntegrationTestScenarios, err := a.loader.GetAllIntegrationTestScenariosForApplication(a.context, a.client, a.application)
	if err != nil {
		a.logger.Error(err, "Failed to get integration test scenarios for the following application",
			"Application.Namespace", a.application.Namespace)
		// Continue processing like original code - don't return nil on error
	}

	if allIntegrationTestScenarios == nil {
		return nil
	}

	integrationTestScenarios := gitops.FilterIntegrationTestScenariosWithContext(allIntegrationTestScenarios, a.snapshot)
	a.logger.Info(
		fmt.Sprintf("Found %d IntegrationTestScenarios for application", len(*integrationTestScenarios)),
		"Application.Name", a.application.Name,
		"IntegrationTestScenarios", len(*integrationTestScenarios))

	return integrationTestScenarios
}

// initializeTestStatusesWithDefer creates and initializes test status tracking with deferred update setup
// Returns the testStatuses and a cleanup function that should be deferred
func (a *Adapter) initializeTestStatusesWithDefer(integrationTestScenarios *[]v1beta2.IntegrationTestScenario) (*intgteststat.SnapshotIntegrationTestStatuses, func(), error) {
	testStatuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(a.snapshot)
	if err != nil {
		return nil, nil, err
	}

	testStatuses.InitStatuses(scenariosNamesToList(integrationTestScenarios))

	err = gitops.WriteIntegrationTestStatusesIntoSnapshot(a.context, a.snapshot, testStatuses, a.client)
	if err != nil {
		return nil, nil, err
	}

	// Create the deferred function
	deferFunc := func() {
		// Try to update status of test if something failed, because test loop can stop prematurely on error,
		// we should record current status.
		// This is only best effort update
		//
		// When update of statuses worked fine at the end of function, this is just a no-op
		err := gitops.WriteIntegrationTestStatusesIntoSnapshot(a.context, a.snapshot, testStatuses, a.client)
		if err != nil {
			a.logger.Error(err, "Defer: Updating statuses of tests in snapshot failed")
		}
	}

	return testStatuses, deferFunc, nil
}

// processSingleScenario handles pipeline creation for a single test scenario
func (a *Adapter) processSingleScenario(
	integrationTestScenario *v1beta2.IntegrationTestScenario,
	testStatuses *intgteststat.SnapshotIntegrationTestStatuses,
) error {
	// Check if an existing integration pipelineRun is registered in the Snapshot's status
	// We rely on this because the actual pipelineRun CR may have been pruned by this point
	integrationTestScenarioStatus, ok := testStatuses.GetScenarioStatus(integrationTestScenario.Name)
	if ok && integrationTestScenarioStatus.TestPipelineRunName != "" {
		a.logger.Info("Found existing integrationPipelineRun",
			"integrationTestScenario.Name", integrationTestScenario.Name,
			"pipelineRun.Name", integrationTestScenarioStatus.TestPipelineRunName)
		return nil
	}

	// Create new pipeline run
	pipelineRun, err := a.createIntegrationPipelineRun(a.application, integrationTestScenario, a.snapshot)
	if err != nil {
		a.logger.Error(err, "Failed to create pipelineRun for snapshot and scenario",
			"integrationScenario.Name", integrationTestScenario.Name)

		testStatuses.UpdateTestStatusIfChanged(
			integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestInvalid,
			fmt.Sprintf("Creation of pipelineRun failed during creation due to: %s.", err))

		// Only return error if it's not a validation error
		if !clienterrors.IsInvalid(err) {
			return err
		}
		return nil
	}

	// Update status for successful creation
	gitops.PrepareToRegisterIntegrationPipelineRunStarted(a.snapshot) // don't count re-runs
	testStatuses.UpdateTestStatusIfChanged(
		integrationTestScenario.Name, intgteststat.IntegrationTestStatusInProgress,
		fmt.Sprintf("IntegrationTestScenario pipeline '%s' has been created", pipelineRun.Name))

	if err = testStatuses.UpdateTestPipelineRunName(integrationTestScenario.Name, pipelineRun.Name); err != nil {
		// it doesn't make sense to restart reconciliation here, it will be eventually updated by integrationpipeline adapter
		a.logger.Error(err, "Failed to update pipelinerun name in test status")
	}

	return nil
}

// processAllScenarios iterates through all scenarios and creates pipelines
func (a *Adapter) processAllScenarios(
	integrationTestScenarios *[]v1beta2.IntegrationTestScenario,
	testStatuses *intgteststat.SnapshotIntegrationTestStatuses,
) error {
	var errsForPLRCreation error

	for _, integrationTestScenario := range *integrationTestScenarios {
		integrationTestScenario := integrationTestScenario //G601
		err := a.processSingleScenario(&integrationTestScenario, testStatuses)
		if err != nil {
			errsForPLRCreation = errors.Join(errsForPLRCreation, err)
		}
	}

	return errsForPLRCreation
}

// cancelOldPipelinesIfNeeded cancels old pipelines for non-push events
func (a *Adapter) cancelOldPipelinesIfNeeded() {
	if !gitops.IsSnapshotCreatedByPACPushEvent(a.snapshot) {
		err := a.checkAndCancelOldSnapshotsPipelineRun(a.application, a.snapshot)
		if err != nil {
			a.logger.Error(err, "Failed to check and cancel old snapshot's pipelineruns",
				"snapshot.Name:", a.snapshot.Name)
		}
	}
}

// markSnapshotPassedIfNoRequiredScenarios checks if required scenarios exist and marks snapshot accordingly.
// Returns an error if fetching scenarios or updating the snapshot fails.
func (a *Adapter) markSnapshotPassedIfNoRequiredScenarios() (controller.OperationResult, error) {
	requiredIntegrationTestScenarios, err := a.loader.GetRequiredIntegrationTestScenariosForSnapshot(
		a.context, a.client, a.application, a.snapshot)
	if err != nil {
		a.logger.Error(err, "Failed to get all required IntegrationTestScenarios")
		patch := client.MergeFrom(a.snapshot.DeepCopy())
		gitops.SetSnapshotIntegrationStatusAsError(a.snapshot,
			"Failed to get all required IntegrationTestScenarios: "+err.Error())
		a.logger.LogAuditEvent("Snapshot integration status marked as Invalid. Failed to get all required IntegrationTestScenarios",
			a.snapshot, h.LogActionUpdate)
		return controller.RequeueOnErrorOrStop(a.client.Status().Patch(a.context, a.snapshot, patch))
	}

	if len(*requiredIntegrationTestScenarios) == 0 && !gitops.IsSnapshotMarkedAsPassed(a.snapshot) {
		err := gitops.MarkSnapshotAsPassed(a.context, a.client, a.snapshot,
			"No required IntegrationTestScenarios found, skipped testing")
		if err != nil {
			a.logger.Error(err, "Failed to update Snapshot status")
			return controller.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("Snapshot marked as successful. No required IntegrationTestScenarios found, skipped testing",
			a.snapshot, h.LogActionUpdate,
			"snapshot.Status", a.snapshot.Status)
	}

	return controller.ContinueProcessing()
}

// EnsureIntegrationPipelineRunsExist is an operation that will ensure that all Integration pipeline runs
// associated with the Snapshot and the Application's IntegrationTestScenarios exist.
func (a *Adapter) EnsureIntegrationPipelineRunsExist() (controller.OperationResult, error) {
	if gitops.HaveAppStudioTestsFinished(a.snapshot) {
		a.logger.Info("The Snapshot has finished testing.")
		return controller.ContinueProcessing()
	}

	integrationTestScenarios := a.loadAndFilterIntegrationTestScenarios()

	var errsForPLRCreation error

	// Process scenarios if they exist
	if integrationTestScenarios != nil {
		testStatuses, deferFunc, err := a.initializeTestStatusesWithDefer(integrationTestScenarios)
		if err != nil {
			return controller.RequeueWithError(err)
		}
		defer deferFunc()

		// Process all scenarios
		errsForPLRCreation = a.processAllScenarios(integrationTestScenarios, testStatuses)

		// Cancel old pipelines if needed
		a.cancelOldPipelinesIfNeeded()

		// Persist final status
		err = gitops.WriteIntegrationTestStatusesIntoSnapshot(a.context, a.snapshot, testStatuses, a.client)
		if err != nil {
			a.logger.Error(err, "Failed to update test status in snapshot annotation")
			errsForPLRCreation = errors.Join(errsForPLRCreation, err)
		}
	}

	// Always validate required scenarios, even if scenario loading failed
	result, err := a.markSnapshotPassedIfNoRequiredScenarios()
	if err != nil || result.CancelRequest {
		return result, err
	}

	// Return any pipeline creation errors after required scenario validation
	if errsForPLRCreation != nil {
		return controller.RequeueWithError(errsForPLRCreation)
	}

	return controller.ContinueProcessing()
}

// EnsureGlobalCandidateImageUpdated is an operation that ensure the ContainerImage in the Global Candidate List
// being updated when the Snapshot is created
func (a *Adapter) EnsureGlobalCandidateImageUpdated() (controller.OperationResult, error) {
	if !a.shouldUpdateGlobalCandidateList() {
		return controller.ContinueProcessing()
	}

	var err error

	if gitops.IsComponentSnapshot(a.snapshot) {
		if a.isSnapshotOlderThanLastBuild(a.snapshot) {
			a.logger.Info("The Glocal Candidate list was updated in newer build pipelinerun or snapshot")
		} else {
			err = a.updateGCLForComponentSnapshot()
		}
	} else if gitops.IsOverrideSnapshot(a.snapshot) {
		err = a.updateGCLForOverrideSnapshot()
	}

	if err != nil {
		return controller.RequeueWithError(err)
	}

	addedToGlobalCandidateListStatus := gitops.AddedToGlobalCandidateListStatus{
		Result:          true,
		Reason:          "The Snapshot's component(s) was/were added to the global candidate list",
		LastUpdatedTime: time.Now().Format(time.RFC3339),
	}

	annotationJson, err := json.Marshal(addedToGlobalCandidateListStatus)
	if err != nil {
		return controller.RequeueWithError(err)
	}

	// Mark the Snapshot as already added to global candidate list to prevent it from getting added again when the Snapshot
	// gets reconciled at a later time
	err = gitops.MarkSnapshotAsAddedToGlobalCandidateList(a.context, a.client, a.snapshot, string(annotationJson))
	if err != nil {
		a.logger.Error(err, "Failed to update the Snapshot's status to AddedToGlobalCandidateList")
		return controller.RequeueWithError(err)
	}

	return controller.ContinueProcessing()
}

// shouldUpdateGlobalCandidateList checks if the snapshot should update the global candidate list
func (a *Adapter) shouldUpdateGlobalCandidateList() bool {
	if !gitops.IsComponentSnapshotCreatedByPACPushEvent(a.snapshot) && !gitops.IsOverrideSnapshot(a.snapshot) {
		a.logger.Info("The Snapshot was neither created for a single component push event nor override type, not updating the global candidate list.")
		return false
	}

	// check both of the new GCL update check and old GCL update check
	if gitops.IsSnapshotMarkedAsAddedToGlobalCandidateList(a.snapshot) || gitops.IsSnapshotMarkedAsAddedToGlobalCandidateList_Legacy(a.snapshot) {
		a.logger.Info("The Snapshot's component was previously added to the global candidate list, skipping adding it.")
		return false
	}

	return true
}

// updateGCLForComponentSnapshot updates global candidate list for component snapshots
func (a *Adapter) updateGCLForComponentSnapshot() error {
	var componentToUpdate *applicationapiv1alpha1.Component
	var err error

	err = retry.OnError(retry.DefaultRetry, func(_ error) bool { return true }, func() error {
		componentToUpdate, err = a.loader.GetComponentFromSnapshot(a.context, a.client, a.snapshot)
		return err
	})
	if err != nil {
		_, loaderError := h.HandleLoaderError(a.logger, err, fmt.Sprintf("Component or '%s' label", tektonconsts.ComponentNameLabel), "Snapshot")
		if loaderError != nil {
			return loaderError
		}
		return nil // Continue processing if no loader error
	}

	// Find and update the matching snapshot component
	for _, snapshotComponent := range a.snapshot.Spec.Components {
		snapshotComponent := snapshotComponent //G601
		if snapshotComponent.Name == componentToUpdate.Name {
			return gitops.UpdateComponentImageAndSource(a.context, a.client, a.snapshot, componentToUpdate, snapshotComponent.Source, snapshotComponent.ContainerImage)
		}
	}

	return nil
}

// updateGCLForOverrideSnapshot handles updating global candidate list for override snapshots
func (a *Adapter) updateGCLForOverrideSnapshot() error {
	for _, snapshotComponent := range a.snapshot.Spec.Components {
		snapshotComponent := snapshotComponent //G601

		componentToUpdate, err := a.loader.GetComponent(a.context, a.client, snapshotComponent.Name, a.snapshot.Namespace)
		if err != nil {
			a.logger.Error(err, "Failed to get component from application, won't update global candidate list for this component", "application.Name", a.application.Name, "component.Name", snapshotComponent.Name)
			_, loaderError := h.HandleLoaderError(a.logger, err, snapshotComponent.Name, a.application.Name)
			if loaderError != nil {
				return loaderError
			}
			continue
		}

		// Validate image digest before updating
		if err := gitops.ValidateImageDigest(snapshotComponent.ContainerImage); err != nil {
			a.logger.Error(err, "containerImage cannot be updated to component Global Candidate List due to invalid digest in containerImage", "component.Name", snapshotComponent.Name, "snapshotComponent.ContainerImage", snapshotComponent.ContainerImage)
			continue
		}

		if err := gitops.UpdateComponentImageAndSource(a.context, a.client, a.snapshot, componentToUpdate, snapshotComponent.Source, snapshotComponent.ContainerImage); err != nil {
			return err
		}
	}

	return nil
}

// EnsureAllReleasesExist is an operation that will ensure that all pipeline Releases associated
// to the Snapshot and the Application's ReleasePlans exist.
// Otherwise, it will create new Releases for each ReleasePlan.
func (a *Adapter) EnsureAllReleasesExist() (controller.OperationResult, error) {
	if !a.shouldProcessReleases() {
		return controller.ContinueProcessing()
	}

	releasePlans, err := a.getAutoReleasePlans()
	if err != nil {
		return a.handleReleaseError(err, "Failed to get all ReleasePlans")
	}

	autoReleaseMessage := ""
	if len(*releasePlans) > 0 {
		if a.isSnapshotOlderThanLastBuild(a.snapshot) {
			autoReleaseMessage = "Released in newer Snapshot"
		} else {
			if err := a.createMissingReleasesForReleasePlans(releasePlans, a.snapshot); err != nil {
				return a.handleReleaseError(err, "Failed to create new release")
			}
			autoReleaseMessage = "The Snapshot was auto-released"
		}
	} else {
		autoReleaseMessage = "Skipping auto-release of the Snapshot because no ReleasePlans have the 'auto-release' label set to 'true'"
	}

	err = gitops.MarkSnapshotAsAutoReleased(a.context, a.client, a.snapshot, autoReleaseMessage)
	if err != nil {
		a.logger.Error(err, "Failed to update the Snapshot's status to auto-released")
		return controller.RequeueWithError(err)
	}

	return controller.ContinueProcessing()
}

// shouldProcessReleases checks if snapshot is ready for release
func (a *Adapter) shouldProcessReleases() bool {
	canSnapshotBePromoted, reasons := gitops.CanSnapshotBePromoted(a.snapshot)
	if !canSnapshotBePromoted {
		a.logger.Info("The Snapshot won't be released.", "reasons", strings.Join(reasons, ","))
		return false
	}

	if gitops.IsSnapshotMarkedAsAutoReleased(a.snapshot) {
		a.logger.Info("The Snapshot was previously auto-released, skipping auto-release.")
		return false
	}

	return true
}

// getAutoReleasePlans fetch release plans
func (a *Adapter) getAutoReleasePlans() (*[]releasev1alpha1.ReleasePlan, error) {
	releasePlans, err := a.loader.GetAutoReleasePlansForApplication(a.context, a.client, a.application, a.snapshot)
	if err != nil {
		a.logger.Error(err, "Failed to get all ReleasePlans")
		return nil, err
	}
	return releasePlans, nil
}

// handleReleaseError updates snapshot status and determine controller op for failure
func (a *Adapter) handleReleaseError(err error, message string) (controller.OperationResult, error) {
	a.logger.Error(err, message)

	patch := client.MergeFrom(a.snapshot.DeepCopy())
	gitops.SetSnapshotIntegrationStatusAsError(a.snapshot, message+": "+err.Error())

	if patchErr := a.client.Status().Patch(a.context, a.snapshot, patch); patchErr != nil {
		a.logger.Error(patchErr, "Failed to mark snapshot integration status as invalid", "snapshot.Name", a.snapshot.Name)
		return a.RequeueIfYoungerThanThreshold(errors.Join(err, patchErr))
	}

	a.logger.LogAuditEvent("Snapshot integration status marked as Invalid. "+message, a.snapshot, h.LogActionUpdate)
	return a.RequeueIfYoungerThanThreshold(err)
}

// EnsureOverrideSnapshotValid is an operation that ensure the manually created override snapshot have valid
// digest and git source in snapshotComponents, mark it as invalid otherwise
func (a *Adapter) EnsureOverrideSnapshotValid() (controller.OperationResult, error) {
	if !gitops.IsOverrideSnapshot(a.snapshot) {
		a.logger.Info("The snapshot was not override snapshot, skipping")
		return controller.ContinueProcessing()
	}

	if gitops.IsSnapshotMarkedAsInvalid(a.snapshot) {
		a.logger.Info("The override snapshot has been marked as invalid, skipping")
		return controller.ContinueProcessing()
	}

	// validate all snapshotComponents' containerImages/source in snapshot, make all errors joined
	var err, errsForSnapshot error
	for _, snapshotComponent := range a.snapshot.Spec.Components {
		snapshotComponent := snapshotComponent //G601
		_, err := a.loader.GetComponent(a.context, a.client, snapshotComponent.Name, a.snapshot.Namespace)
		if err != nil {
			a.logger.Error(err, "Failed to get component from application", "application.Name", a.application.Name, "component.Name", snapshotComponent.Name)
			_, loaderError := h.HandleLoaderError(a.logger, err, snapshotComponent.Name, a.application.Name)
			if loaderError != nil {
				return controller.RequeueWithError(loaderError)
			} else {
				errsForSnapshot = errors.Join(errsForSnapshot, fmt.Errorf("snapshotComponent %s defined in snapshot %s doesn't exist under application %s/%s", snapshotComponent.Name, a.snapshot.Name, a.application.Namespace, a.application.Name))
			}
		}

		err = gitops.ValidateImageDigest(snapshotComponent.ContainerImage)
		if err != nil {
			a.logger.Error(err, "containerImage in snapshotComponent has invalid digest", "snapshotComponent.Name", snapshotComponent.Name, "snapshotComponent.ContainerImage", snapshotComponent.ContainerImage)
			errsForSnapshot = errors.Join(errsForSnapshot, err)
		}

		if !gitops.HaveGitSource(snapshotComponent) {
			a.logger.Error(err, "snapshotComponent has no git url/revision fields defined", "snapshotComponent.Name", snapshotComponent.Name, "snapshotComponent.ContainerImage", snapshotComponent.ContainerImage)
			errsForSnapshot = errors.Join(errsForSnapshot, err)
		}
	}

	if errsForSnapshot != nil {
		a.logger.Error(errsForSnapshot, "mark the override snapshot as invalid due to invalid snapshotComponent")
		err = gitops.MarkSnapshotAsInvalid(a.context, a.client, a.snapshot, errsForSnapshot.Error())
		if err != nil {
			a.logger.Error(err, "Failed to update snapshot to Invalid",
				"snapshot.Namespace", a.snapshot.Namespace, "snapshot.Name", a.snapshot.Name)
			return controller.RequeueWithError(err)
		}
		a.logger.Info("Snapshot has been marked as invalid due to invalid snapshotComponent",
			"snapshot.Namespace", a.snapshot.Namespace, "snapshot.Name", a.snapshot.Name)
	}
	return controller.ContinueProcessing()
}

// EnsureGroupSnapshotExist is an operation that ensure the group snapshot is created for component snapshots
// once a new component snapshot is created for an pull request and there are multiple existing PRs belonging to the same PR group
func (a *Adapter) EnsureGroupSnapshotExist() (controller.OperationResult, error) {
	if gitops.IsSnapshotCreatedByPACPushEvent(a.snapshot) {
		a.logger.Info("The snapshot is not created by PAC pull request, no need to create group snapshot")
		return controller.ContinueProcessing()
	}

	if !metadata.HasLabelWithValue(a.snapshot, gitops.SnapshotTypeLabel, gitops.SnapshotComponentType) {
		a.logger.Info("The snapshot is not a component snapshot, no need to create group snapshot for it")
		return controller.ContinueProcessing()
	}

	if gitops.HasPRGroupProcessed(a.snapshot) {
		a.logger.Info("The PR group info has been processed for this component snapshot, no need to process it again")
		return controller.ContinueProcessing()
	}

	prGroupHash, prGroup := gitops.GetPRGroup(a.snapshot)
	if prGroupHash == "" || prGroup == "" {
		a.logger.Error(fmt.Errorf("NotFound"), fmt.Sprintf("Failed to get PR group label/annotation from snapshot %s/%s", a.snapshot.Namespace, a.snapshot.Name))
		err := gitops.AnnotateSnapshot(a.context, a.snapshot, gitops.PRGroupCreationAnnotation, fmt.Sprintf("Failed to get PR group label/annotation from snapshot %s/%s", a.snapshot.Namespace, a.snapshot.Name), a.client)
		if err != nil {
			return controller.RequeueWithError(err)
		}
		return controller.ContinueProcessing()
	}

	// check if all build plr have been processed for the given pr group
	haveAllPipelineRunProcessedForPrGroup, err := a.haveAllPipelineRunProcessedForPrGroup(prGroup, prGroupHash)
	if err != nil {
		return controller.RequeueWithError(err)
	}
	// don't need to create group snapshot if there is any unready pipelinerun for pr group
	if !haveAllPipelineRunProcessedForPrGroup {
		return controller.ContinueProcessing()
	}

	groupSnapshot, componentSnapshotInfos, err := a.prepareGroupSnapshot(a.application, prGroup, prGroupHash)
	if err != nil {
		a.logger.Error(err, "failed to prepare group snapshot")
		if h.IsUnrecoverableMetadataError(err) || clienterrors.IsNotFound(err) {
			err = gitops.AnnotateSnapshot(a.context, a.snapshot, gitops.PRGroupCreationAnnotation, fmt.Sprintf("failed to prepare group snapshot for pr group %s due to error %s, skipping group snapshot creation", prGroup, err.Error()), a.client)
			if err != nil {
				return controller.RequeueWithError(err)
			}
			return controller.ContinueProcessing()
		}
		return controller.RequeueWithError(err)

	}

	if groupSnapshot == nil {
		a.logger.Info(fmt.Sprintf("The number %d of component snapshots belonging to this pr group hash %s is less than 2, skipping group snapshot creation", len(componentSnapshotInfos), prGroupHash))
		err = gitops.AnnotateSnapshot(a.context, a.snapshot, gitops.PRGroupCreationAnnotation, fmt.Sprintf("The number %d of component snapshots belonging to this pr group hash %s is less than 2, skipping group snapshot creation", len(componentSnapshotInfos), prGroupHash), a.client)
		if err != nil {
			return controller.RequeueWithError(err)
		}
		return controller.ContinueProcessing()
	}

	err = a.client.Create(a.context, groupSnapshot)
	if err != nil {
		a.logger.Error(err, "Failed to create group snapshot")
		if clienterrors.IsForbidden(err) {
			// notify all component snapshots that group snapshot is not created for them due to
			err = gitops.NotifyComponentSnapshotsInGroupSnapshot(a.context, a.client, componentSnapshotInfos, fmt.Sprintf(gitops.FailedToCreateGroupSnapshotMsg+" %s due to error %s", prGroup, err.Error()))
			if err != nil {
				a.logger.Error(err, fmt.Sprintf("Failed to annotate the component snapshots of pr group %s", prGroup))
				return controller.RequeueWithError(err)
			}
			return controller.StopProcessing()
		}
		return controller.RequeueWithError(err)
	}

	// notify all component snapshots that group snapshot is created for them
	err = gitops.NotifyComponentSnapshotsInGroupSnapshot(a.context, a.client, componentSnapshotInfos, fmt.Sprintf("Group snapshot %s/%s is created for pr group %s", groupSnapshot.Namespace, groupSnapshot.Name, prGroup))
	if err != nil {
		a.logger.Error(err, fmt.Sprintf("Failed to annotate the component snapshots for group snapshot %s/%s", a.snapshot.Namespace, a.snapshot.Name))
		return controller.RequeueWithError(err)
	}
	return controller.ContinueProcessing()
}

// createMissingReleasesForReleasePlans checks if there's existing Releases for a given list of ReleasePlans and creates
// new ones if they are missing. In case the Releases can't be created, an error will be returned.
func (a *Adapter) createMissingReleasesForReleasePlans(releasePlans *[]releasev1alpha1.ReleasePlan, snapshot *applicationapiv1alpha1.Snapshot) error {
	releases, err := a.loader.GetReleasesWithSnapshot(a.context, a.client, a.snapshot)
	if err != nil {
		return err
	}

	firstRelease := true
	for _, releasePlan := range *releasePlans {
		releasePlan := releasePlan // G601
		existingRelease := release.FindMatchingReleaseWithReleasePlan(releases, releasePlan)
		if existingRelease != nil {
			a.logger.Info("Found existing Release",
				"snapshot.Name", snapshot.Name,
				"releasePlan.Name", releasePlan.Name,
				"release.Name", existingRelease.Name)
		} else {
			err = a.createAutomatedRelease(&releasePlan, snapshot)
			if err != nil {
				return err
			}
		}
		// Register the first release time for metrics calculation
		if firstRelease {
			startTime, ok := gitops.GetAppStudioTestsFinishedTime(a.snapshot)
			if ok {
				metrics.RegisterReleaseLatency(startTime)
				firstRelease = false
			}
		}
	}
	return nil
}

// createAutomatedRelease  creates a new release for a given releasePlan and snapshot and marks it as automated.
// In case the Releases can't be created or their status can't be updated, an error will be returned.
func (a *Adapter) createAutomatedRelease(releasePlan *releasev1alpha1.ReleasePlan, snapshot *applicationapiv1alpha1.Snapshot) error {
	newRelease := release.NewReleaseForReleasePlan(a.context, releasePlan, snapshot)
	err := retry.OnError(retry.DefaultRetry, func(_ error) bool { return true }, func() error {
		err := a.client.Create(a.context, newRelease)
		if err != nil {
			return err
		}
		a.logger.LogAuditEvent("Created a new Release", newRelease, h.LogActionAdd,
			"releasePlan.Name", releasePlan.Name)
		return nil
	})
	if err != nil {
		return err
	}

	patch := client.MergeFrom(newRelease.DeepCopy())
	newRelease.SetAutomated()
	err = retry.OnError(retry.DefaultRetry, func(_ error) bool { return true }, func() error {
		err = a.client.Status().Patch(a.context, newRelease, patch)
		if err != nil {
			return err
		}
		a.logger.Info("Marked Release status automated", "release.Name", newRelease.Name)
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// shouldUpdateIntegrationGitResolver checks if the integration test resolver should be updated based on the source repo
func shouldUpdateIntegrationGitResolver(integrationTestScenario *v1beta2.IntegrationTestScenario, snapshot *applicationapiv1alpha1.Snapshot) bool {
	// only "pull-requests" are applicable
	if gitops.IsSnapshotCreatedByPACPushEvent(snapshot) {
		return false
	}

	// only integrationTestScenarios with the pipelineRun resourceKind can update their task references
	if integrationTestScenario.Spec.ResolverRef.ResourceKind != tektonconsts.ResourceKindPipelineRun {
		return false
	}
	return true
}

// createIntegrationPipelineRun creates and returns a new integration PipelineRun. The Pipeline information and the parameters to it
// will be extracted from the given integrationScenario. The integration's Snapshot will also be passed to the integration PipelineRun.
func (a *Adapter) createIntegrationPipelineRun(application *applicationapiv1alpha1.Application, integrationTestScenario *v1beta2.IntegrationTestScenario, snapshot *applicationapiv1alpha1.Snapshot) (*tektonv1.PipelineRun, error) {
	a.logger.Info("Creating new pipelinerun for integrationTestscenario",
		"integrationTestScenario.Name", integrationTestScenario.Name)

	pipelineRunBuilder, err := tekton.NewIntegrationPipelineRun(a.client, a.context, a.loader, a.logger, integrationTestScenario.Name, application.Namespace, integrationTestScenario, a.snapshot)
	if err != nil {
		return nil, err
	}

	pipelineRunBuilder = pipelineRunBuilder.WithSnapshot(snapshot, integrationTestScenario).
		WithIntegrationLabels(integrationTestScenario).
		WithIntegrationAnnotations(integrationTestScenario).
		WithApplication(a.application).
		WithDefaultServiceAccount(tektonconsts.DefaultIntegrationPipelineServiceAccount).
		WithExtraParams(integrationTestScenario.Spec.Params).
		WithFinalizer(h.IntegrationPipelineRunFinalizer).
		WithIntegrationTimeouts(integrationTestScenario, a.logger.Logger)

	if shouldUpdateIntegrationGitResolver(integrationTestScenario, snapshot) {
		a.logger.Info("use the integration test task/taskrun from the code in the pr of snapshot", "integrationTestScneario.Name", integrationTestScenario.Name)
		pipelineRunBuilder.WithUpdatedTasksGitResolver(snapshot)
		pipelineRunBuilder.WithUpdatedPipelineGitResolver(snapshot)
	}

	pipelineRun := pipelineRunBuilder.AsPipelineRun()

	err = ctrl.SetControllerReference(snapshot, pipelineRun, a.client.Scheme())
	if err != nil {
		return nil, fmt.Errorf("failed to set snapshot %s as ControllerReference of pipelineRun: %w", snapshot.Name, err)
	}
	err = a.client.Create(a.context, pipelineRun)
	if err != nil {
		return nil, fmt.Errorf("failed to call client.Create to create pipelineRun for snapshot %s: %w", snapshot.Name, err)
	}

	go metrics.RegisterNewIntegrationPipelineRun()

	a.logger.LogAuditEvent("IntegrationTestscenario pipeline has been created", pipelineRun, h.LogActionAdd,
		"integrationTestScenario.Name", integrationTestScenario.Name)
	if gitops.IsSnapshotNotStarted(a.snapshot) {
		err := gitops.MarkSnapshotIntegrationStatusAsInProgress(a.context, a.client, a.snapshot, "Snapshot starts being tested by the integrationPipelineRun")
		if err != nil {
			a.logger.Error(err, "Failed to update integration status condition to in progress for snapshot")
		} else {
			a.logger.LogAuditEvent("Snapshot integration status marked as In Progress. Snapshot starts being tested by the integrationPipelineRun",
				a.snapshot, h.LogActionUpdate)
		}
	}
	return pipelineRun, nil
}

// RequeueIfYoungerThanThreshold checks if the adapter's snapshot is younger than the threshold defined
// in the function.  If it is, the function returns an operation result instructing the reconciler
// to requeue the object and the error message passed to the function.  If not, the function returns
// an operation result instructing the reconciler NOT to requeue the object.
func (a *Adapter) RequeueIfYoungerThanThreshold(retErr error) (controller.OperationResult, error) {
	if h.IsObjectYoungerThanThreshold(a.snapshot, SnapshotRetryTimeout) {
		return controller.RequeueWithError(retErr)
	}
	return controller.ContinueProcessing()
}

func (a *Adapter) HandlePipelineCreationError(err error, integrationTestScenario *v1beta2.IntegrationTestScenario, testStatuses *intgteststat.SnapshotIntegrationTestStatuses) (controller.OperationResult, error) {
	a.logger.Error(err, "Failed to create pipelineRun for snapshot and scenario",
		"integrationScenario.Name", integrationTestScenario.Name)
	testStatuses.UpdateTestStatusIfChanged(
		integrationTestScenario.Name, intgteststat.IntegrationTestStatusTestInvalid,
		fmt.Sprintf("Creation of pipelineRun failed during creation due to: %s.", err))
	itsErr := gitops.WriteIntegrationTestStatusesIntoSnapshot(a.context, a.snapshot, testStatuses, a.client)
	if itsErr != nil {
		a.logger.Error(err, "Failed to write Test Status into Snapshot")
		return controller.RequeueWithError(itsErr)
	}

	if strings.Contains(err.Error(), "admission webhook") && strings.Contains(err.Error(), "denied the request") {
		//Stop processing in case the error runs in admission webhook validation error:
		//failed to call client.Create to create pipelineRun for snapshot
		//<snapshot-name>: admission webhook \"validation.webhook.pipeline.tekton.dev\" denied the request: validation failed: <reason>
		return controller.StopProcessing()
	}
	if clienterrors.IsInvalid(err) {
		return controller.StopProcessing()
	}
	return controller.RequeueWithError(err)
}

func (a *Adapter) prepareGroupSnapshot(application *applicationapiv1alpha1.Application, prGroup, prGroupHash string) (*applicationapiv1alpha1.Snapshot, []gitops.ComponentSnapshotInfo, error) {
	componentsToCheck, err := a.loader.GetComponentsFromSnapshotForPRGroup(a.context, a.client, application.Namespace, prGroup, prGroupHash, application.Name)
	if err != nil {
		return nil, nil, err
	}
	if len(componentsToCheck) < 2 {
		a.logger.Info(fmt.Sprintf("The number %d of components affected by this PR group %s is less than 2, skipping group snapshot creation", len(componentsToCheck), prGroup))
		return nil, nil, err
	}

	applicationComponents, err := a.loader.GetAllApplicationComponents(a.context, a.client, application)
	if err != nil {
		return nil, nil, err
	}

	snapshotComponents := make([]applicationapiv1alpha1.SnapshotComponent, 0)
	componentSnapshotInfos := make([]gitops.ComponentSnapshotInfo, 0)
	for _, applicationComponent := range *applicationComponents {
		applicationComponent := applicationComponent // G601

		var foundSnapshotWithOpenedPR *applicationapiv1alpha1.Snapshot
		var statusCode int
		if slices.Contains(componentsToCheck, applicationComponent.Name) {
			snapshots, err := a.loader.GetMatchingComponentSnapshotsForComponentAndPRGroupHash(a.context, a.client, application.Namespace, applicationComponent.Name, prGroupHash, application.Name)
			if err != nil {
				a.logger.Error(err, "Failed to fetch Snapshots for component", "component.Name", applicationComponent.Name)
				return nil, nil, err
			}
			foundSnapshotWithOpenedPR, statusCode, err = a.status.FindSnapshotWithOpenedPR(a.context, snapshots)
			a.logger.Error(err, "failed to find snapshot with open PR or MR", "statusCode", statusCode)
			if err != nil {
				return nil, nil, err
			}
			if foundSnapshotWithOpenedPR != nil {
				a.logger.Info("PR/MR in snapshot is opened, will find snapshotComponent and add to groupSnapshot")
				snapshotComponent := gitops.FindMatchingSnapshotComponent(foundSnapshotWithOpenedPR, &applicationComponent)
				componentSnapshotInfos = append(componentSnapshotInfos, gitops.ComponentSnapshotInfo{
					Component:         applicationComponent.Name,
					BuildPipelineRun:  foundSnapshotWithOpenedPR.Labels[gitops.BuildPipelineRunNameLabel],
					Snapshot:          foundSnapshotWithOpenedPR.Name,
					Namespace:         a.snapshot.Namespace,
					RepoUrl:           foundSnapshotWithOpenedPR.Annotations[gitops.PipelineAsCodeRepoUrlAnnotation],
					PullRequestNumber: foundSnapshotWithOpenedPR.Annotations[gitops.PipelineAsCodePullRequestAnnotation],
				})
				snapshotComponents = append(snapshotComponents, snapshotComponent)
				continue
			}
		}

		a.logger.Info("can't find snapshot with open pull/merge request for component, try to find snapshotComponent from Global Candidate List", "component", applicationComponent.Name)
		// if there is no component snapshot found for open PR/MR, we get snapshotComponent from gcl
		componentSource, err := gitops.GetComponentSourceFromComponent(&applicationComponent)
		if err != nil {
			a.logger.Error(err, "component cannot be added to snapshot for application due to missing git source", "component.Name", applicationComponent.Name)
			continue
		}
		containerImage := applicationComponent.Status.LastPromotedImage
		if containerImage == "" {
			a.logger.Info("component cannot be added to snapshot for application due to missing containerImage", "component.Name", applicationComponent.Name)
			continue
		} else {
			// if the containerImage doesn't have a valid digest, the component
			// will not be added to snapshot
			err := gitops.ValidateImageDigest(containerImage)
			if err != nil {
				a.logger.Error(err, "component cannot be added to snapshot for application due to invalid digest in containerImage", "component.Name", applicationComponent.Name)
				continue
			}
			snapshotComponent := applicationapiv1alpha1.SnapshotComponent{
				Name:           applicationComponent.Name,
				ContainerImage: containerImage,
				Source:         *componentSource,
			}
			a.logger.Info("component with containerImage from Global Candidate List will be added to group snapshot", "component.Name", snapshotComponent.Name)
			snapshotComponents = append(snapshotComponents, snapshotComponent)
		}
	}

	// if the valid component snapshot from open MR/PR is less than 2, won't create group snapshot
	if len(componentSnapshotInfos) < 2 {
		return nil, componentSnapshotInfos, nil
	}

	groupSnapshot := gitops.NewSnapshot(application, &snapshotComponents)
	err = ctrl.SetControllerReference(application, groupSnapshot, a.client.Scheme())
	if err != nil {
		a.logger.Error(err, "failed to set owner reference to group snapshot")
		return nil, nil, err
	}

	groupSnapshot, err = gitops.SetAnnotationAndLabelForGroupSnapshot(groupSnapshot, a.snapshot, componentSnapshotInfos)
	if err != nil {
		a.logger.Error(err, "failed to annotate group snapshot")
		return nil, nil, err
	}

	return groupSnapshot, componentSnapshotInfos, nil
}

// haveAllPipelineRunProcessedForPrGroup checks if all build plr has been processed for the given pr group
func (a *Adapter) haveAllPipelineRunProcessedForPrGroup(prGroup, prGroupHash string) (bool, error) {
	pipelineRuns, err := a.loader.GetPipelineRunsWithPRGroupHash(a.context, a.client, a.snapshot.Namespace, prGroupHash, a.application.Name)
	if err != nil {
		a.logger.Error(err, fmt.Sprintf("Failed to get build pipelineRuns for given pr group hash %s", prGroupHash))
		return false, err
	}

	for _, pipelineRun := range *pipelineRuns {
		pipelineRun := pipelineRun //G601
		// check if the build PLR is the latest existing one
		if !tekton.IsLatestBuildPipelineRunInComponent(&pipelineRun, pipelineRuns) {
			a.logger.Info(fmt.Sprintf("The build pipelineRun %s/%s with pr group %s is not the latest for its component, skipped", pipelineRun.Namespace, pipelineRun.Name, prGroup))
			continue
		}

		// check if build PLR finishes
		if !h.HasPipelineRunFinished(&pipelineRun) {
			a.logger.Info(fmt.Sprintf("The build pipelineRun %s/%s with pr group %s is still running, won't create group snapshot", pipelineRun.Namespace, pipelineRun.Name, prGroup))
			err := gitops.AnnotateSnapshot(a.context, a.snapshot, gitops.PRGroupCreationAnnotation, fmt.Sprintf("The build pipelineRun %s/%s with pr group %s is still running, won't create group snapshot", pipelineRun.Namespace, pipelineRun.Name, prGroup), a.client)
			if err != nil {
				return false, err
			}
			return false, nil
		}

		// check if build PLR succeeds
		if !h.HasPipelineRunSucceeded(&pipelineRun) {
			a.logger.Info(fmt.Sprintf("The build pipelineRun %s/%s with pr group %s failed, won't create group snapshot", pipelineRun.Namespace, pipelineRun.Name, prGroup))
			err := gitops.AnnotateSnapshot(a.context, a.snapshot, gitops.PRGroupCreationAnnotation, fmt.Sprintf("The build pipelineRun %s/%s with pr group %s failed, won't create group snapshot", pipelineRun.Namespace, pipelineRun.Name, prGroup), a.client)
			if err != nil {
				return false, err
			}
			return false, nil
		}

		// check if build PLR has component snapshot created except the build that snapshot is created from because the build plr has not been labeled with snapshot name
		if !metadata.HasAnnotation(&pipelineRun, tektonconsts.SnapshotNameLabel) && !metadata.HasLabelWithValue(a.snapshot, gitops.BuildPipelineRunNameLabel, pipelineRun.Name) {
			a.logger.Info(fmt.Sprintf("The build pipelineRun %s/%s with pr group %s has succeeded but component snapshot has not been created now", pipelineRun.Namespace, pipelineRun.Name, prGroup))
			return false, nil
		}
	}
	return true, nil
}

// checkAndCancelOldSnapshotsPipelineRun sorts all snapshots for application and cancels all running integrationTest pipelineruns within application
// removes finalizer before the pipelinerun is set as CancelledRunFinally to be gracefully cancelled
func (a *Adapter) checkAndCancelOldSnapshotsPipelineRun(application *applicationapiv1alpha1.Application, snapshot *applicationapiv1alpha1.Snapshot) error {
	var err error
	snapshots := &[]applicationapiv1alpha1.Snapshot{}
	if gitops.IsComponentSnapshot(snapshot) {
		snapshots, err = a.loader.GetAllSnapshotsForPR(a.context, a.client, application, snapshot.GetLabels()[gitops.SnapshotComponentLabel], snapshot.GetLabels()[gitops.PipelineAsCodePullRequestAnnotation])
		if err != nil {
			a.logger.Error(err, "Failed to fetch Snapshots for the application",
				"application.Name:", application.Name)
			return err
		}
	}

	if gitops.IsGroupSnapshot(snapshot) {
		prGroupHash, prGroup := gitops.GetPRGroup(snapshot)
		if prGroupHash == "" || prGroup == "" {
			a.logger.Error(fmt.Errorf("pr group info can't be found in group snapshot"), "snapshot.Namespace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
			return fmt.Errorf("pr group info can't be found in group snapshot %s/%s", snapshot.Namespace, snapshot.Name)
		}
		snapshots, err = a.loader.GetMatchingGroupSnapshotsForPRGroupHash(a.context, a.client, application.Namespace, prGroupHash, application.Name)
		if err != nil {
			a.logger.Error(fmt.Errorf("failed to get group snapshot for pr group from group snapshot"), "snapshot.Namespace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
			return err
		}

	}

	sortedSnapshots := gitops.SortSnapshots(*snapshots)
	// no snapshots found that fulfill the condition
	if len(sortedSnapshots) < 2 {
		return nil
	}
	for i := 1; i < len(sortedSnapshots); i++ {
		if gitops.IsSnapshotMarkedAsCanceled(&sortedSnapshots[i]) {
			a.logger.Info("Snapshot has been marked as cancelled previously, skipping marking it", "snapshot.Name", &sortedSnapshots[i].Name)
			continue
		}
		a.logger.Info("integration test pipelineruns have been cancelled for older snapshot")
		err = gitops.MarkSnapshotAsCanceled(a.context, a.client, &sortedSnapshots[i], "Snapshot canceled/superseded")
		if err != nil {
			a.logger.Error(err, "Failed to mark snapshot as canceled", "snapshot.Name", &sortedSnapshots[i].Name)
			return err
		}
		err = a.cancelAllPipelineRunsForSnapshot(&sortedSnapshots[i])
		if err != nil {
			a.logger.Error(err, "failed to cancel all integration pipelinerun for older snapshot", "snapshot.Name", &sortedSnapshots[i].Name)
			return err
		}
		a.logger.Info(fmt.Sprintf("older snapshot %s and its integration pipelinerun have been marked as cancelled", sortedSnapshots[i].Name))
	}
	return err
}

// cancelAllPipelineRunsForSnapshot gets all integration test pipelieruns for a given snapshot
func (a *Adapter) cancelAllPipelineRunsForSnapshot(snapshot *applicationapiv1alpha1.Snapshot) error {
	// get all integration pipelineruns for a snapshot
	integrationTestPipelineRuns, err := a.loader.GetAllIntegrationPipelineRunsForSnapshot(a.context, a.client, snapshot)
	if err != nil {
		a.logger.Error(err, "Failed to get all integration pipelineruns for snapshot", "snapshot.Name", snapshot.Name)
		return err
	}
	if len(integrationTestPipelineRuns) < 1 {
		a.logger.Info("No integrationTest pipelineruns were found for snapshot", "snapshot.Name", snapshot.Name)
		return nil
	}
	return gitops.CancelPipelineRuns(a.client, a.context, a.logger, integrationTestPipelineRuns)
}

func (a *Adapter) isSnapshotOlderThanLastBuild(snapshot *applicationapiv1alpha1.Snapshot) bool {
	componentName := snapshot.Labels[gitops.SnapshotComponentLabel]
	if componentName == "" {
		return false
	}
	snapshotBuildStartTime := snapshot.Annotations[gitops.BuildPipelineRunStartTime]
	if snapshotBuildStartTime == "" {
		return false
	}
	component, err := a.loader.GetComponent(a.context, a.client, componentName, a.snapshot.Namespace)
	if err != nil {
		a.logger.Error(err, "Failed to get component from snapshot", "snapshot.Name", snapshot.Name)
		return false
	}
	componentlastBuiltTime := component.Annotations[gitops.BuildPipelineLastBuiltTime]
	if componentlastBuiltTime == "" {
		return false
	}
	snapshotBuildStartTimeInt, snapshotBuildStartTimeIntErr := strconv.ParseInt(snapshotBuildStartTime, 10, 64)
	if snapshotBuildStartTimeIntErr != nil {
		return false
	}
	componentlastBuiltTimeInt, componentlastBuiltTimeIntErr := strconv.ParseInt(componentlastBuiltTime, 10, 64)
	if componentlastBuiltTimeIntErr != nil {
		return false
	}

	// Normalize BuildPipelineRunStartTime to seconds if it's in milliseconds
	// Millisecond timestamps are > 1000000000000 (year 2001 in milliseconds)
	// Second timestamps are < 10000000000 (year 2286 in seconds)
	if snapshotBuildStartTimeInt > 10000000000 {
		// It's in milliseconds, convert to seconds for comparison
		snapshotBuildStartTimeInt = snapshotBuildStartTimeInt / 1000
	}

	if snapshotBuildStartTimeInt < componentlastBuiltTimeInt {
		return true
	}
	return false
}
