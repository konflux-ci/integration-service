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
	"strings"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	h "github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/status"

	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	"github.com/konflux-ci/operator-toolkit/controller"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Adapter holds the objects needed to reconcile an integration PipelineRun.
type Adapter struct {
	pipelineRun *tektonv1.PipelineRun
	application *applicationapiv1alpha1.Application
	snapshot    *applicationapiv1alpha1.Snapshot
	loader      loader.ObjectLoader
	logger      h.IntegrationLogger
	client      client.Client
	context     context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(context context.Context, pipelineRun *tektonv1.PipelineRun, application *applicationapiv1alpha1.Application,
	snapshot *applicationapiv1alpha1.Snapshot, logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
) *Adapter {
	return &Adapter{
		pipelineRun: pipelineRun,
		application: application,
		snapshot:    snapshot,
		logger:      logger,
		loader:      loader,
		client:      client,
		context:     context,
	}
}

// EnsureStatusReportedInSnapshot will ensure that status of the integration test pipelines is reported to snapshot
// to be consumed by user
func (a *Adapter) EnsureStatusReportedInSnapshot() (controller.OperationResult, error) {
	// Check if the finalizer has been removed - if so, the status has already been reported
	// and we should skip processing to avoid the TaskRun mismatch error
	if !controllerutil.ContainsFinalizer(a.pipelineRun, h.IntegrationPipelineRunFinalizer) {
		a.logger.Info("Integration PipelineRun finalizer has been removed, status already reported, skipping processing",
			"pipelineRun.Name", a.pipelineRun.Name)
		return controller.ContinueProcessing()
	}

	var pipelinerunStatus intgteststat.IntegrationTestStatus
	var detail string
	var err error

	// pipelines run in parallel and have great potential to cause conflict on update
	// thus `RetryOnConflict` is easy solution here, given the snapshot must be loaded specifically here
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {

		a.snapshot, err = a.loader.GetSnapshotFromPipelineRun(a.context, a.client, a.pipelineRun)
		if err != nil {
			return err
		}

		statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(a.snapshot)
		if err != nil {
			return err
		}

		pipelinerunStatus, detail, err = a.GetIntegrationPipelineRunStatus(a.context, a.client, a.pipelineRun)
		if err != nil {
			return err
		}
		statuses.UpdateTestStatusIfChanged(a.pipelineRun.Labels[tektonconsts.ScenarioNameLabel], pipelinerunStatus, detail)
		if err = statuses.UpdateTestPipelineRunName(a.pipelineRun.Labels[tektonconsts.ScenarioNameLabel], a.pipelineRun.Name); err != nil {
			return err
		}

		// don't return wrapped err for retries
		err = gitops.WriteIntegrationTestStatusesIntoSnapshot(a.context, a.snapshot, statuses, a.client)
		return err
	})
	if err != nil {
		a.logger.Error(err, "Failed to update pipeline status in snapshot")
		return controller.RequeueWithError(fmt.Errorf("failed to update test status in snapshot: %w", err))
	}

	// Remove the finalizer from Integration PLRs if the snapshot is not group or component type and its PLR has finished
	if (!gitops.IsGroupSnapshot(a.snapshot) && !gitops.IsComponentSnapshot(a.snapshot)) && (h.HasPipelineRunFinished(a.pipelineRun) ||
		pipelinerunStatus == intgteststat.IntegrationTestStatusDeleted) {
		err = h.RemoveFinalizerFromPipelineRun(a.context, a.client, a.logger, a.pipelineRun, h.IntegrationPipelineRunFinalizer)
		if err != nil {
			return controller.RequeueWithError(fmt.Errorf("failed to remove the finalizer: %w", err))
		}
	}

	return controller.ContinueProcessing()
}

// EnsureIntegrationPipelineRunLogURL ensures that the integration pipeline run log URL is annotated if available.
func (a *Adapter) EnsureIntegrationPipelineRunLogURL() (controller.OperationResult, error) {
	// var err error
	integrationPipelineRunURL := status.FormatPipelineURL(a.pipelineRun.Name, a.pipelineRun.Namespace, a.logger.Logger)
	if integrationPipelineRunURL != "" {
		err := a.annotateIntegrationPipelineRunLogURL(
			a.context,
			a.client,
			a.pipelineRun,
			tektonconsts.PipelinesAsCodePrefix+"/log-url",
			integrationPipelineRunURL,
		)
		if err != nil {
			return controller.RequeueWithError(err)
		}
	}
	return controller.ContinueProcessing()
}

// GetIntegrationPipelineRunStatus checks the Tekton results for a given PipelineRun and returns status of test.
func (a *Adapter) GetIntegrationPipelineRunStatus(ctx context.Context, adapterClient client.Client, pipelineRun *tektonv1.PipelineRun) (intgteststat.IntegrationTestStatus, string, error) {
	integrationPipelineRunURL := status.FormatPipelineURL(pipelineRun.Name, pipelineRun.Namespace, a.logger.Logger)
	// Check if the pipelineRun finished from the condition of status
	if !h.HasPipelineRunFinished(pipelineRun) {
		// Mark the pipelineRun's status as "Deleted" if its not finished yet and is marked for deletion (with a non-nil deletionTimestamp)
		if pipelineRun.GetDeletionTimestamp() != nil {
			return intgteststat.IntegrationTestStatusDeleted, fmt.Sprintf("Integration test which is running as pipeline run '%s', has been deleted", pipelineRun.Name), nil
		} else {
			return intgteststat.IntegrationTestStatusInProgress, fmt.Sprintf("Integration test is running as pipeline run '%s'", integrationPipelineRunURL), nil
		}
	}

	taskRuns, err := a.loader.GetAllTaskRunsWithMatchingPipelineRunLabel(ctx, adapterClient, pipelineRun)
	if err != nil {
		return intgteststat.IntegrationTestStatusTestInvalid, fmt.Sprintf("Unable to get all the TaskRun(s) related to the pipelineRun '%s'", pipelineRun.Name), err
	}

	taskRunsInClusterCount := len(*taskRuns)
	taskRunsInChildRefCount := len(pipelineRun.Status.ChildReferences)

	if taskRunsInClusterCount != taskRunsInChildRefCount {
		return intgteststat.IntegrationTestStatusTestInvalid, fmt.Sprintf("Failed to determine status of pipelinerun '%s'"+
			", due to mismatch in TaskRuns present in cluster (%v) and those referenced within childReferences (%v)",
			pipelineRun.Name, taskRunsInClusterCount, taskRunsInChildRefCount), nil
	}

	outcome, err := h.GetIntegrationPipelineRunOutcome(ctx, adapterClient, pipelineRun)
	if err != nil {
		return intgteststat.IntegrationTestStatusTestFail, "", fmt.Errorf("failed to evaluate integration test results: %w", err)
	}

	if !outcome.HasPipelineRunPassedTesting() {
		if !outcome.HasPipelineRunValidTestOutputs() {
			return intgteststat.IntegrationTestStatusTestFail, strings.Join(outcome.GetValidationErrorsList(), "; "), nil
		}
		return intgteststat.IntegrationTestStatusTestFail, "Integration test failed", nil
	}

	return intgteststat.IntegrationTestStatusTestPassed, "Integration test passed", nil
}

// annotateIntegrationPipelineRunLogURL adds to ITS PLR the PaC annotation with the log URL in console
func (a *Adapter) annotateIntegrationPipelineRunLogURL(ctx context.Context, adapterClient client.Client, pipelineRun *tektonv1.PipelineRun, annotation, value string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var err error
		a.pipelineRun, err = a.loader.GetPipelineRun(a.context, a.client, a.pipelineRun.Name, a.pipelineRun.Namespace)
		if err != nil {
			return err
		}
		patch := client.MergeFrom(a.pipelineRun.DeepCopy())
		if a.pipelineRun.Annotations == nil {
			a.pipelineRun.Annotations = make(map[string]string)
		}
		a.pipelineRun.Annotations[annotation] = value

		err = adapterClient.Patch(ctx, a.pipelineRun, patch)
		if err == nil {
			a.logger.LogAuditEvent("Updated integration pipelineRun", pipelineRun, h.LogActionUpdate,
				"log-url", value)
		}
		return err
	})
}
