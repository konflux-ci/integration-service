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

package statusreport

import (
	"context"
	e "errors"
	"fmt"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/operator-toolkit/controller"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/status"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const SnapshotRetryTimeout = time.Duration(3 * time.Hour)

// Adapter holds the objects needed to reconcile a snapshot's test status report.
type Adapter struct {
	snapshot    *applicationapiv1alpha1.Snapshot
	application *applicationapiv1alpha1.Application
	logger      helpers.IntegrationLogger
	loader      loader.ObjectLoader
	client      client.Client
	context     context.Context
	status      status.StatusInterface
}

// NewAdapter creates and returns an Adapter instance.
func NewAdapter(context context.Context, snapshot *applicationapiv1alpha1.Snapshot, application *applicationapiv1alpha1.Application,
	logger helpers.IntegrationLogger, loader loader.ObjectLoader, client client.Client,
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

// EnsureSnapshotTestStatusReportedToGitProvider will ensure that integration test status is reported to the git provider
// which (indirectly) triggered its execution.
// The status is reported to git provider if it is a component snapshot
// Or reported to git providers which trigger component snapshots included in group snapshot if it is a group snapshot
func (a *Adapter) EnsureSnapshotTestStatusReportedToGitProvider() (controller.OperationResult, error) {
	// Only report status for component or group Snapshots
	if !gitops.IsGroupSnapshot(a.snapshot) && !gitops.IsComponentSnapshot(a.snapshot) {
		return controller.ContinueProcessing()
	}
	err := a.ReportSnapshotStatus(a.snapshot)
	if err != nil {
		a.logger.Error(err, "failed to report test status to git provider for snapshot",
			"snapshot.Namespace", a.snapshot.Namespace, "snapshot.Name", a.snapshot.Name)
		if helpers.IsObjectYoungerThanThreshold(a.snapshot, SnapshotRetryTimeout) {
			return controller.RequeueWithError(err)
		}
	}
	testStatuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(a.snapshot)
	if err != nil {
		return controller.RequeueWithError(err)
	}
	for _, testDetails := range testStatuses.GetStatuses() {
		if testDetails.Status.IsFinal() && testDetails.TestPipelineRunName != "" {
			pipelineRunName := testDetails.TestPipelineRunName
			pipelineRun := &tektonv1.PipelineRun{}
			err := a.client.Get(a.context, types.NamespacedName{
				Namespace: a.snapshot.Namespace,
				Name:      pipelineRunName,
			}, pipelineRun)

			// if the PLR doesn't exist on cluster we continue the loop
			if err != nil {
				if !errors.IsNotFound(err) {
					return controller.RequeueWithError(err)
				}
				continue
			}

			err = helpers.RemoveFinalizerFromPipelineRun(a.context, a.client, a.logger, pipelineRun, helpers.IntegrationPipelineRunFinalizer)
			if err != nil {
				return controller.RequeueWithError(err)
			}
		}
	}
	return controller.ContinueProcessing()
}

// EnsureSnapshotFinishedAllTests is an operation that will ensure that a pipeline Snapshot
// to the PipelineRun being processed finished and passed all tests for all defined required IntegrationTestScenarios.
func (a *Adapter) EnsureSnapshotFinishedAllTests() (controller.OperationResult, error) {
	// Get all required integrationTestScenarios for the Snapshot and then use the Snapshot status annotation
	// to check if all Integration tests were finished for that Snapshot
	integrationTestScenarios, err := a.loader.GetRequiredIntegrationTestScenariosForSnapshot(a.context, a.client, a.application, a.snapshot)
	if err != nil {
		return controller.RequeueWithError(err)
	}
	a.logger.Info(fmt.Sprintf("Found %d required integration test scenarios", len(*integrationTestScenarios)))

	testStatuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(a.snapshot)
	if err != nil {
		return controller.RequeueWithError(err)
	}

	allIntegrationTestsFinished, allIntegrationTestsPassed := a.determineIfAllRequiredIntegrationTestsFinishedAndPassed(integrationTestScenarios, testStatuses)

	// Skip doing anything if not all Integration tests were finished for all integrationTestScenarios
	if !allIntegrationTestsFinished {
		a.logger.Info("Not all required Integration PipelineRuns finished",
			"snapshot.Name", a.snapshot.Name)

		// If for the snapshot there are IntegrationTestScenarios that are not triggered, it will add run labebl to snapshot to trigger them
		err = a.labelSnapshotToTriggerUntriggeredTest(integrationTestScenarios, testStatuses)
		if err != nil {
			return controller.RequeueWithError(err)
		}

		return controller.ContinueProcessing()
	}

	finishedStatusMessage := "Snapshot integration status condition is finished since all testing pipelines completed"
	if len(*integrationTestScenarios) == 0 {
		finishedStatusMessage = "Snapshot integration status condition is finished since there are no required testing pipelines defined for its application"
	}

	if !gitops.IsSnapshotIntegrationStatusMarkedAsFinished(a.snapshot) {
		err = gitops.MarkSnapshotIntegrationStatusAsFinished(a.context, a.client, a.snapshot, finishedStatusMessage)
		if err != nil {
			a.logger.Error(err, "Failed to Update Snapshot AppStudioIntegrationStatus status")
			return controller.RequeueWithError(err)
		}
		a.logger.LogAuditEvent(finishedStatusMessage, a.snapshot, helpers.LogActionUpdate)
	}

	// If all Integration Pipeline runs passed, mark the snapshot as succeeded, otherwise mark it as failed
	// This updates the Snapshot resource on the cluster
	if allIntegrationTestsPassed {
		if !gitops.IsSnapshotMarkedAsPassed(a.snapshot) {
			err = gitops.MarkSnapshotAsPassed(a.context, a.client, a.snapshot, "All Integration Pipeline tests passed")
			if err != nil {
				a.logger.Error(err, "Failed to Update Snapshot AppStudioTestSucceeded status")
				return controller.RequeueWithError(err)
			}
			a.logger.LogAuditEvent(fmt.Sprintf("Snapshot integration status condition marked as passed, all of %d required Integration PipelineRuns succeeded", len(*integrationTestScenarios)),
				a.snapshot, helpers.LogActionUpdate)
		}
	} else if !gitops.IsSnapshotMarkedAsFailed(a.snapshot) {
		err = gitops.MarkSnapshotAsFailed(a.context, a.client, a.snapshot, "Some Integration pipeline tests failed")
		if err != nil {
			a.logger.Error(err, "Failed to Update Snapshot AppStudioTestSucceeded status")
			return controller.RequeueWithError(err)
		}
		a.logger.LogAuditEvent("Snapshot integration status condition marked as failed, some tests within Integration PipelineRuns failed",
			a.snapshot, helpers.LogActionUpdate)
	}

	return controller.ContinueProcessing()
}

// determineIfAllRequiredIntegrationTestsFinishedAndPassed checks if all Integration tests finished and passed for the given
// list of integrationTestScenarios.
func (a *Adapter) determineIfAllRequiredIntegrationTestsFinishedAndPassed(integrationTestScenarios *[]v1beta2.IntegrationTestScenario, testStatuses *intgteststat.SnapshotIntegrationTestStatuses) (bool, bool) {
	allIntegrationTestsFinished, allIntegrationTestsPassed := true, true
	integrationTestsFinished := 0
	integrationTestsPassed := 0

	for _, integrationTestScenario := range *integrationTestScenarios {
		integrationTestScenario := integrationTestScenario // G601
		testDetails, ok := testStatuses.GetScenarioStatus(integrationTestScenario.Name)
		if !ok || !testDetails.Status.IsFinal() {
			allIntegrationTestsFinished = false
		} else {
			integrationTestsFinished++
		}
		if ok && testDetails.Status != intgteststat.IntegrationTestStatusTestPassed {
			allIntegrationTestsPassed = false
		} else {
			integrationTestsPassed++
		}

	}
	a.logger.Info(fmt.Sprintf("%[1]d out of %[3]d required integration tests finished, %[2]d out of %[3]d required integration tests passed", integrationTestsFinished, integrationTestsPassed, len(*integrationTestScenarios)))
	return allIntegrationTestsFinished, allIntegrationTestsPassed
}

// findUntriggeredIntegrationTestFromStatus returns name of integrationTestScenario that is not triggered yet.
func (a *Adapter) findUntriggeredIntegrationTestFromStatus(integrationTestScenarios *[]v1beta2.IntegrationTestScenario, testStatuses *intgteststat.SnapshotIntegrationTestStatuses) string {
	for _, integrationTestScenario := range *integrationTestScenarios {
		integrationTestScenario := integrationTestScenario // G601
		_, ok := testStatuses.GetScenarioStatus(integrationTestScenario.Name)
		if !ok {
			return integrationTestScenario.Name
		}

	}
	return ""
}

// ReportSnapshotStatus reports status of all integration tests into Pull Requests from component snapshot or group snapshot
func (a *Adapter) ReportSnapshotStatus(testedSnapshot *applicationapiv1alpha1.Snapshot) error {

	statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(testedSnapshot)
	if err != nil {
		a.logger.Error(err, "failed to get test status annotations from snapshot",
			"snapshot.Namespace", testedSnapshot.Namespace, "snapshot.Name", testedSnapshot.Name)
		return err
	}

	integrationTestStatusDetails := statuses.GetStatuses()
	if len(integrationTestStatusDetails) == 0 {
		// no tests to report, skip
		a.logger.Info("No test result to report to GitHub, skipping",
			"snapshot.Namespace", testedSnapshot.Namespace, "snapshot.Name", testedSnapshot.Name)
		return nil
	}

	// get the component snapshot list that include the git provider info the report will be reported to
	destinationSnapshots, err := a.getDestinationSnapshots(testedSnapshot)
	if err != nil {
		a.logger.Error(err, "failed to get component snapshots from group snapshot",
			"snapshot.NameSpace", testedSnapshot.Namespace, "snapshot.Name", testedSnapshot.Name)
		return fmt.Errorf("failed to get component snapshots from snapshot %s/%s", testedSnapshot.Namespace, testedSnapshot.Name)
	}

	status.MigrateSnapshotToReportStatus(testedSnapshot, integrationTestStatusDetails)

	srs, err := status.NewSnapshotReportStatusFromSnapshot(testedSnapshot)
	if err != nil {
		a.logger.Error(err, "failed to get latest snapshot write metadata annotation for snapshot",
			"snapshot.NameSpace", testedSnapshot.Namespace, "snapshot.Name", testedSnapshot.Name)
		srs, _ = status.NewSnapshotReportStatus("")
	}

	// Report the integration test status to pr/commit included in the tested component snapshot
	// or the component snapshot included in group snapshot
	for _, destinationComponentSnapshot := range destinationSnapshots {
		reporter := a.status.GetReporter(destinationComponentSnapshot)
		if reporter == nil {
			a.logger.Info("No suitable reporter found, skipping report")
			continue
		}
		a.logger.Info(fmt.Sprintf("Detected reporter: %s", reporter.GetReporterName()), "destinationComponentSnapshot.Name", destinationComponentSnapshot.Name, "testedSnapshot", testedSnapshot.Name)

		if err := reporter.Initialize(a.context, destinationComponentSnapshot); err != nil {
			a.logger.Error(err, "Failed to initialize reporter", "reporter", reporter.GetReporterName())
			return fmt.Errorf("failed to initialize reporter: %w", err)
		}
		a.logger.Info("Reporter initialized", "reporter", reporter.GetReporterName())

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := a.iterateIntegrationTestStatusDetailsInStatusReport(reporter, integrationTestStatusDetails, testedSnapshot, destinationComponentSnapshot, srs)
			if err != nil {
				a.logger.Error(err, fmt.Sprintf("failed to report integration test status for snapshot %s/%s",
					destinationComponentSnapshot.Namespace, destinationComponentSnapshot.Name))
				return fmt.Errorf("failed to report integration test status for snapshot %s/%s: %w",
					destinationComponentSnapshot.Namespace, destinationComponentSnapshot.Name, err)
			}
			if err := status.WriteSnapshotReportStatus(a.context, a.client, testedSnapshot, srs); err != nil {
				a.logger.Error(err, "failed to write snapshot report status metadata")
				return fmt.Errorf("failed to write snapshot report status metadata: %w", err)
			}
			return err
		})

	}

	if err != nil {
		return fmt.Errorf("issue occurred during generating or updating report status: %w", err)
	}

	a.logger.Info(fmt.Sprintf("Successfully updated the %s annotation", gitops.SnapshotStatusReportAnnotation), "snapshot.Name", testedSnapshot.Name)

	return nil
}

// iterates iterateIntegrationTestStatusDetails to report to destination snapshot for them
func (a *Adapter) iterateIntegrationTestStatusDetailsInStatusReport(reporter status.ReporterInterface,
	integrationTestStatusDetails []*intgteststat.IntegrationTestStatusDetail,
	testedSnapshot *applicationapiv1alpha1.Snapshot,
	destinationSnapshot *applicationapiv1alpha1.Snapshot,
	srs *status.SnapshotReportStatus) error {
	// set componentName to component name of component snapshot or pr group name of group snapshot when reporting status to git provider
	componentName := ""
	if gitops.IsGroupSnapshot(testedSnapshot) {
		componentName = "pr group " + testedSnapshot.Annotations[gitops.PRGroupAnnotation]
	} else if gitops.IsComponentSnapshot(testedSnapshot) {
		componentName = testedSnapshot.Labels[gitops.SnapshotComponentLabel]
	} else {
		return fmt.Errorf("unsupported snapshot type: %s", testedSnapshot.Annotations[gitops.SnapshotTypeLabel])
	}

	for _, integrationTestStatusDetail := range integrationTestStatusDetails {
		if srs.IsNewer(integrationTestStatusDetail.ScenarioName, destinationSnapshot.Name, integrationTestStatusDetail.LastUpdateTime) {
			a.logger.Info("Integration Test contains new status updates", "scenario.Name", integrationTestStatusDetail.ScenarioName, "destinationSnapshot.Name", destinationSnapshot.Name, "testedSnapshot", testedSnapshot.Name)
		} else {
			//integration test contains no changes
			a.logger.Info("Integration Test doen't contain new status updates", "scenario.Name", integrationTestStatusDetail.ScenarioName)
			continue
		}
		testReport, reportErr := status.GenerateTestReport(a.context, a.client, *integrationTestStatusDetail, testedSnapshot, componentName)
		if reportErr != nil {
			if writeErr := status.WriteSnapshotReportStatus(a.context, a.client, testedSnapshot, srs); writeErr != nil { // try to write what was already written
				return fmt.Errorf("failed to generate test report AND write snapshot report status metadata: %w", e.Join(reportErr, writeErr))
			}
			return fmt.Errorf("failed to generate test report: %w", reportErr)
		}
		if reportStatusErr := reporter.ReportStatus(a.context, *testReport); reportStatusErr != nil {
			if writeErr := status.WriteSnapshotReportStatus(a.context, a.client, testedSnapshot, srs); writeErr != nil { // try to write what was already written
				return fmt.Errorf("failed to report status AND write snapshot report status metadata: %w", e.Join(reportStatusErr, writeErr))
			}
			return fmt.Errorf("failed to update status: %w", reportStatusErr)
		}
		a.logger.Info("Successfully report integration test status for snapshot",
			"testedSnapshot.Name", testedSnapshot.Name,
			"destinationSnapshot.Name", destinationSnapshot.Name,
			"testStatus", integrationTestStatusDetail.Status)
		srs.SetLastUpdateTime(integrationTestStatusDetail.ScenarioName, destinationSnapshot.Name, integrationTestStatusDetail.LastUpdateTime)
	}
	return nil
}

// getDestinationSnapshots gets the component snapshots that include the git provider info the report will be reported to
func (a *Adapter) getDestinationSnapshots(testedSnapshot *applicationapiv1alpha1.Snapshot) ([]*applicationapiv1alpha1.Snapshot, error) {
	destinationSnapshots := make([]*applicationapiv1alpha1.Snapshot, 0)
	if gitops.IsComponentSnapshot(testedSnapshot) {
		destinationSnapshots = append(destinationSnapshots, testedSnapshot)
		return destinationSnapshots, nil
	} else if gitops.IsGroupSnapshot(testedSnapshot) {
		// get component snapshots from group snapshot annotation GroupSnapshotInfoAnnotation
		destinationSnapshots, err := status.GetComponentSnapshotsFromGroupSnapshot(a.context, a.client, testedSnapshot)
		if err != nil {
			a.logger.Error(err, "failed to get component snapshots included in group snapshot",
				"snapshot.NameSpace", testedSnapshot.Namespace, "snapshot.Name", testedSnapshot.Name)
			return nil, fmt.Errorf("failed to get component snapshots included in group snapshot %s/%s", testedSnapshot.Namespace, testedSnapshot.Name)
		}
		return destinationSnapshots, nil
	}
	return nil, fmt.Errorf("unsupported snapshot type in snapshot %s/%s", testedSnapshot.Namespace, testedSnapshot.Name)
}

// labelSnapshotToTriggerUntriggeredTest get the untriggered integration test and add label to snapshot to trigger them
// return error if annotating meet error
func (a *Adapter) labelSnapshotToTriggerUntriggeredTest(integrationTestScenarios *[]v1beta2.IntegrationTestScenario, testStatuses *intgteststat.SnapshotIntegrationTestStatuses) error {
	integrationTestScenarioNotTriggered := a.findUntriggeredIntegrationTestFromStatus(integrationTestScenarios, testStatuses)
	if integrationTestScenarioNotTriggered != "" {
		a.logger.Info("Detected an integrationTestScenario was not triggered, applying snapshot reconcilation",
			"integrationTestScenario.Name", integrationTestScenarioNotTriggered)
		return gitops.AddIntegrationTestRerunLabel(a.context, a.client, a.snapshot, integrationTestScenarioNotTriggered)
	}
	return nil
}
