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
	"github.com/redhat-appstudio/integration-service/gitops"
	h "github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/loader"
	intgteststat "github.com/redhat-appstudio/integration-service/pkg/integrationteststatus"
	"github.com/redhat-appstudio/integration-service/status"
	"github.com/redhat-appstudio/integration-service/tekton"

	"github.com/redhat-appstudio/operator-toolkit/controller"
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

// EnsureStatusReportedInSnapshot will ensure that status of the integration test pipelines is reported to snapshot
// to be consumed by user
func (a *Adapter) EnsureStatusReportedInSnapshot() (controller.OperationResult, error) {
	var pipelinerunStatus intgteststat.IntegrationTestStatus
	var snapshot *applicationapiv1alpha1.Snapshot
	var detail string
	var err error

	// pipelines run in parallel and have great potential to cause conflict on update
	// thus `RetryOnConflict` is easy solution here, given the snapshot must be loaded specifically here
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {

		snapshot, err = a.loader.GetSnapshotFromPipelineRun(a.client, a.context, a.pipelineRun)
		if err != nil {
			return err
		}

		statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
		if err != nil {
			return err
		}

		pipelinerunStatus, detail, err = GetIntegrationPipelineRunStatus(a.client, a.context, a.pipelineRun)
		if err != nil {
			return err
		}
		statuses.UpdateTestStatusIfChanged(a.pipelineRun.Labels[tekton.ScenarioNameLabel], pipelinerunStatus, detail)
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

	// Remove the finalizer from Integration PLRs only if they aren't related to Snapshots created by Pull-Request event
	// If they are related, then the statusreport controller removes the finalizers from these PLRs
	if !gitops.IsSnapshotCreatedByPACPullRequestEvent(snapshot) && (h.HasPipelineRunFinished(a.pipelineRun) || pipelinerunStatus == intgteststat.IntegrationTestStatusDeleted) {
		err = h.RemoveFinalizerFromPipelineRun(a.client, a.logger, a.context, a.pipelineRun, h.IntegrationPipelineRunFinalizer)
		if err != nil {
			return controller.RequeueWithError(fmt.Errorf("failed to remove the finalizer: %w", err))
		}
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

// GetIntegrationPipelineRunStatus checks the Tekton results for a given PipelineRun and returns status of test.
func GetIntegrationPipelineRunStatus(adapterClient client.Client, ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) (intgteststat.IntegrationTestStatus, string, error) {
	// Check if the pipelineRun finished from the condition of status
	if !h.HasPipelineRunFinished(pipelineRun) {
		// Mark the pipelineRun's status as "Deleted" if its not finished yet and is marked for deletion (with a non-nil deletionTimestamp)
		if pipelineRun.GetDeletionTimestamp() != nil {
			return intgteststat.IntegrationTestStatusDeleted, fmt.Sprintf("Integration test which is running as pipeline run '%s', has been deleted", pipelineRun.Name), nil
		} else {
			return intgteststat.IntegrationTestStatusInProgress, fmt.Sprintf("Integration test is running as pipeline run '%s'", pipelineRun.Name), nil
		}
	}

	outcome, err := h.GetIntegrationPipelineRunOutcome(adapterClient, ctx, pipelineRun)
	if err != nil {
		return intgteststat.IntegrationTestStatusTestFail, "", fmt.Errorf("failed to evaluate inegration test results: %w", err)
	}

	if !outcome.HasPipelineRunPassedTesting() {
		return intgteststat.IntegrationTestStatusTestFail, "Integration test failed", nil
	}

	return intgteststat.IntegrationTestStatusTestPassed, "Integration test passed", nil
}
