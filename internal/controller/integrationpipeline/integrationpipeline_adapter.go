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
	"errors"
	"fmt"
	"strings"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	h "github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/pkg/tracing"
	"github.com/konflux-ci/integration-service/status"

	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	"github.com/konflux-ci/operator-toolkit/controller"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// taskRunResolutionGracePeriod is how long a finished PipelineRun may be retried
// while TaskRuns named by its childReferences are not all visible. The TaskRun
// informer can lag the PipelineRun informer when the API server is under load.
const taskRunResolutionGracePeriod = 5 * time.Minute

// missingTaskRuns describes TaskRuns named by status.childReferences that could not be found.
type missingTaskRuns struct {
	pipelineRunName string
	missing         []string
}

func (m missingTaskRuns) detail() string {
	return fmt.Sprintf("failed to determine status of pipelinerun '%s', missing TaskRuns referenced by childReferences: %s",
		m.pipelineRunName, strings.Join(m.missing, ", "))
}

// retryableTaskRunsError is returned when referenced TaskRuns are not yet visible
// and the PipelineRun is still inside taskRunResolutionGracePeriod.
type retryableTaskRunsError struct {
	missingTaskRuns
}

func (e retryableTaskRunsError) Error() string {
	return e.detail() + "; retrying within the resolution grace period"
}

// expiredTaskRunsError is returned when referenced TaskRuns are still missing
// after taskRunResolutionGracePeriod.
type expiredTaskRunsError struct {
	missingTaskRuns
}

func (e expiredTaskRunsError) Error() string {
	return e.detail() + "; resolution grace period has elapsed"
}

// Adapter holds the objects needed to reconcile an integration PipelineRun.
type Adapter struct {
	pipelineRun *tektonv1.PipelineRun
	snapshot    *applicationapiv1alpha1.Snapshot
	loader      loader.ObjectLoader
	logger      h.IntegrationLogger
	client      client.Client
	context     context.Context
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(context context.Context, pipelineRun *tektonv1.PipelineRun,
	snapshot *applicationapiv1alpha1.Snapshot, logger h.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
) *Adapter {
	return &Adapter{
		pipelineRun: pipelineRun,
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

		keptExistingStatus := false
		pipelinerunStatus, detail, err = a.GetIntegrationPipelineRunStatus(a.context, a.client, a.pipelineRun)
		if err != nil {
			var retryable *retryableTaskRunsError
			if errors.As(err, &retryable) {
				return err
			}
			var expired *expiredTaskRunsError
			if !errors.As(err, &expired) {
				return err
			}
			scenarioName := a.pipelineRun.Labels[tektonconsts.ScenarioNameLabel]
			if scenarioName == "" {
				return fmt.Errorf("pipelinerun %q is missing the %q label", a.pipelineRun.Name, tektonconsts.ScenarioNameLabel)
			}
			existing, ok := statuses.GetScenarioStatus(scenarioName)
			switch {
			case ok && existing.Status.IsFinal() && existing.TestPipelineRunName == a.pipelineRun.Name:
				pipelinerunStatus = existing.Status
				detail = existing.Details
				a.logger.Info("Referenced TaskRuns are still missing after the resolution grace period, keeping the existing final scenario status",
					"pipelineRun.Name", a.pipelineRun.Name,
					"scenario.Name", scenarioName,
					"status", existing.Status)
				keptExistingStatus = true
			case ok && existing.Status.IsFinal():
				// This run did not produce the recorded result. Writing TestInvalid
				// would replace a final status from another PipelineRun.
				a.logger.Info("Referenced TaskRuns are still missing after the resolution grace period, keeping the final scenario status from a different PipelineRun",
					"pipelineRun.Name", a.pipelineRun.Name,
					"scenario.Name", scenarioName,
					"existingPipelineRun.Name", existing.TestPipelineRunName,
					"status", existing.Status)
				keptExistingStatus = true
			default:
				pipelinerunStatus = intgteststat.IntegrationTestStatusTestInvalid
				detail = expired.Error()
			}
		}
		if !keptExistingStatus {
			statuses.UpdateTestStatusIfChanged(a.pipelineRun.Labels[tektonconsts.ScenarioNameLabel], pipelinerunStatus, detail)
			if err = statuses.UpdateTestPipelineRunName(a.pipelineRun.Labels[tektonconsts.ScenarioNameLabel], a.pipelineRun.Name); err != nil {
				return err
			}

			// don't return wrapped err for retries
			err = gitops.WriteIntegrationTestStatusesIntoSnapshot(a.context, a.snapshot, statuses, a.client)
			return err
		}
		return nil
	})
	if err != nil {
		var retryable *retryableTaskRunsError
		if errors.As(err, &retryable) {
			a.logger.Info("Referenced TaskRuns are not all visible yet, requeueing status report",
				"pipelineRun.Name", a.pipelineRun.Name,
				"missingTaskRuns", retryable.missing)
			return controller.RequeueWithError(err)
		}
		a.logger.Error(err, "Failed to update pipeline status in snapshot")
		return controller.RequeueWithError(fmt.Errorf("failed to update test status in snapshot: %w", err))
	}

	if h.HasPipelineRunFinished(a.pipelineRun) {
		var failureMsg string
		if pipelinerunStatus == intgteststat.IntegrationTestStatusTestFail {
			failureMsg = detail
		}
		a.emitTestTimingSpans(failureMsg)
	}

	// Remove the finalizer from Integration PLRs if the snapshot is not group or component type and its PLR has finished
	if (!gitops.IsGroupSnapshot(a.snapshot) && !gitops.IsComponentSnapshot(a.snapshot)) && (h.HasPipelineRunFinished(a.pipelineRun) ||
		pipelinerunStatus == intgteststat.IntegrationTestStatusDeleted) {
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			latest, fetchErr := a.loader.GetPipelineRun(a.context, a.client, a.pipelineRun.Name, a.pipelineRun.Namespace)
			if fetchErr != nil {
				return fetchErr
			}
			return h.RemoveFinalizerFromPipelineRun(a.context, a.client, a.logger, latest, h.IntegrationPipelineRunFinalizer)
		})
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

	// A failed PipelineRun does not need its TaskRuns. TEST_OUTPUT is only read
	// after the PipelineRun itself has succeeded.
	if !h.HasPipelineRunSucceeded(pipelineRun) {
		outcome, err := h.GetIntegrationPipelineRunOutcome(ctx, adapterClient, pipelineRun)
		if err != nil {
			return intgteststat.IntegrationTestStatusTestFail, "", fmt.Errorf("failed to evaluate integration test results: %w", err)
		}
		return integrationTestStatusFromOutcome(outcome)
	}

	resolved, missing, err := a.loader.GetTaskRunsReferencedByPipelineRun(ctx, adapterClient, pipelineRun)
	if err != nil {
		return intgteststat.IntegrationTestStatusTestInvalid, fmt.Sprintf("Unable to get all the TaskRun(s) related to the pipelineRun '%s'", pipelineRun.Name), err
	}
	if len(missing) > 0 {
		unavailable := missingTaskRuns{
			pipelineRunName: pipelineRun.Name,
			missing:         missing,
		}
		if h.TaskRunsResolutionWithinGrace(pipelineRun, taskRunResolutionGracePeriod) {
			return 0, "", &retryableTaskRunsError{missingTaskRuns: unavailable}
		}
		return 0, "", &expiredTaskRunsError{missingTaskRuns: unavailable}
	}

	outcome, err := h.GetIntegrationPipelineRunOutcomeFromTaskRuns(pipelineRun, resolved)
	if err != nil {
		return intgteststat.IntegrationTestStatusTestFail, "", fmt.Errorf("failed to evaluate integration test results: %w", err)
	}
	return integrationTestStatusFromOutcome(outcome)
}

// integrationTestStatusFromOutcome maps an integration PipelineRun outcome to a
// snapshot test status and detail. Invalid TEST_OUTPUT and a failed run are
// TestFail, a warning result is TestWarning, and any other passing run is TestPassed.
func integrationTestStatusFromOutcome(outcome *h.IntegrationPipelineRunOutcome) (intgteststat.IntegrationTestStatus, string, error) {
	if !outcome.HasPipelineRunPassedTesting() {
		if !outcome.HasPipelineRunValidTestOutputs() {
			return intgteststat.IntegrationTestStatusTestFail, strings.Join(outcome.GetValidationErrorsList(), "; "), nil
		}
		return intgteststat.IntegrationTestStatusTestFail, "Integration test failed", nil
	}

	if outcome.HasPipelineRunWarningTesting() {
		return intgteststat.IntegrationTestStatusTestWarning, "Integration test passed with warnings", nil
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

// emitTestTimingSpans emits timing spans for the integration test PipelineRun if not already emitted.
func (a *Adapter) emitTestTimingSpans(failureMsg string) {
	spanContext := ""
	if a.snapshot != nil && a.snapshot.Annotations != nil {
		spanContext = a.snapshot.Annotations[tracing.SpanContextAnnotation]
	}
	patched, err := tracing.EmitAndMarkTimingSpans(a.context, a.pipelineRun, spanContext, failureMsg, a.client, func() (*tektonv1.PipelineRun, error) {
		return a.loader.GetPipelineRun(a.context, a.client, a.pipelineRun.Name, a.pipelineRun.Namespace)
	})
	if err != nil {
		a.logger.Error(err, "Failed to emit and mark test timing spans")
		return
	}
	if patched != nil {
		a.pipelineRun = patched
	}
}
