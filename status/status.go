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

package status

//go:generate mockgen -destination mock_status.go -package status github.com/redhat-appstudio/integration-service/status StatusInterface

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	intgteststat "github.com/redhat-appstudio/integration-service/pkg/integrationteststatus"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NamePrefix is a common name prefix for this service.
const NamePrefix = "Red Hat Trusted App Test"

// generate TestReport to be used by all reporters
func generateTestReport(ctx context.Context, client client.Client, detail intgteststat.IntegrationTestStatusDetail, snapshot *applicationapiv1alpha1.Snapshot) (*TestReport, error) {
	text, err := generateText(client, ctx, detail, snapshot.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to generate text message: %w", err)
	}

	summary, err := generateSummary(detail.Status, snapshot.Name, detail.ScenarioName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary message: %w", err)
	}

	report := TestReport{
		Text:           text,
		FullName:       NamePrefix + " / " + snapshot.Name + " / " + detail.ScenarioName,
		ScenarioName:   detail.ScenarioName,
		Status:         detail.Status,
		Summary:        summary,
		StartTime:      detail.StartTime,
		CompletionTime: detail.CompletionTime,
	}
	return &report, nil
}

type StatusInterface interface {
	GetReporter(*applicationapiv1alpha1.Snapshot) ReporterInterface
	ReportSnapshotStatus(context.Context, ReporterInterface, *applicationapiv1alpha1.Snapshot) error
}

type Status struct {
	logger logr.Logger
	client client.Client
}

// check if interface has been implemented correctly
var _ StatusInterface = (*Status)(nil)

func NewStatus(logger logr.Logger, client client.Client) *Status {
	return &Status{
		logger: logger,
		client: client,
	}
}

// GetReporter returns reporter to process snapshot using the right git provider, nil means no suitable reporter found
func (s *Status) GetReporter(snapshot *applicationapiv1alpha1.Snapshot) ReporterInterface {
	githubReporter := NewGitHubReporter(s.logger, s.client)
	if githubReporter.Detect(snapshot) {
		return githubReporter
	}

	return nil
}

// ReportSnapshotStatus reports status of all integration tests into Pull Request
func (s *Status) ReportSnapshotStatus(ctx context.Context, reporter ReporterInterface, snapshot *applicationapiv1alpha1.Snapshot) error {

	statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
	if err != nil {
		s.logger.Error(err, "failed to get test status annotations from snapshot",
			"snapshot.Namespace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return err
	}

	integrationTestStatusDetails := statuses.GetStatuses()
	if len(integrationTestStatusDetails) == 0 {
		// no tests to report, skip
		s.logger.Info("No test result to report to GitHub, skipping",
			"snapshot.Namespace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return nil
	}

	if err := reporter.Initialize(ctx, snapshot); err != nil {
		s.logger.Error(err, "Failed to initialize reporter", "reporter", reporter.GetReporterName())
		return fmt.Errorf("failed to initialize reporter: %w", err)
	}
	s.logger.Info("Reporter initialized", "reporter", reporter.GetReporterName())

	latestUpdateTime, err := gitops.GetLatestUpdateTime(snapshot)
	if err != nil {
		s.logger.Error(err, "failed to get latest update annotation for snapshot",
			"snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		latestUpdateTime = time.Time{}
	}
	newLatestUpdateTime := latestUpdateTime

	for _, integrationTestStatusDetail := range integrationTestStatusDetails {
		if latestUpdateTime.Before(integrationTestStatusDetail.LastUpdateTime) {
			s.logger.Info("Integration Test contains new status updates", "scenario.Name", integrationTestStatusDetail.ScenarioName)
			if newLatestUpdateTime.Before(integrationTestStatusDetail.LastUpdateTime) {
				newLatestUpdateTime = integrationTestStatusDetail.LastUpdateTime
			}
		} else {
			//integration test contains no changes
			continue
		}
		testReport, err := generateTestReport(ctx, s.client, *integrationTestStatusDetail, snapshot)
		if err != nil {
			return fmt.Errorf("failed to generate test report: %w", err)
		}
		if err := reporter.ReportStatus(ctx, *testReport); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}

	}
	_ = gitops.SetLatestUpdateTime(snapshot, newLatestUpdateTime)
	return nil
}

// generateText generates a text with details for the given state
func generateText(k8sClient client.Client, ctx context.Context, integrationTestStatusDetail intgteststat.IntegrationTestStatusDetail, namespace string) (string, error) {
	if integrationTestStatusDetail.Status == intgteststat.IntegrationTestStatusTestPassed || integrationTestStatusDetail.Status == intgteststat.IntegrationTestStatusTestFail {
		pipelineRunName := integrationTestStatusDetail.TestPipelineRunName
		pipelineRun := &tektonv1.PipelineRun{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      pipelineRunName,
		}, pipelineRun)
		if err != nil {
			return "", fmt.Errorf("error while getting the pipelineRun %s: %w", pipelineRunName, err)
		}

		taskRuns, err := helpers.GetAllChildTaskRunsForPipelineRun(k8sClient, ctx, pipelineRun)
		if err != nil {
			return "", fmt.Errorf("error while getting all child taskRuns from pipelineRun %s: %w", pipelineRunName, err)
		}
		text, err := FormatTestsSummary(taskRuns)
		if err != nil {
			return "", err
		}
		return text, nil
	} else {
		text := integrationTestStatusDetail.Details
		return text, nil
	}
}

// generateSummary returns summary for the given state, snapshotName and scenarioName
func generateSummary(state intgteststat.IntegrationTestStatus, snapshotName, scenarioName string) (string, error) {
	var summary string
	var statusDesc string

	switch state {
	case intgteststat.IntegrationTestStatusPending:
		statusDesc = "is pending"
	case intgteststat.IntegrationTestStatusInProgress:
		statusDesc = "is in progress"
	case intgteststat.IntegrationTestStatusEnvironmentProvisionError:
		statusDesc = "experienced an error when provisioning environment"
	case intgteststat.IntegrationTestStatusDeploymentError:
		statusDesc = "experienced an error when deploying snapshotEnvironmentBinding"
	case intgteststat.IntegrationTestStatusDeleted:
		statusDesc = "was deleted before the pipelineRun could finish"
	case intgteststat.IntegrationTestStatusTestPassed:
		statusDesc = "has passed"
	case intgteststat.IntegrationTestStatusTestFail:
		statusDesc = "has failed"
	case intgteststat.IntegrationTestStatusTestInvalid:
		statusDesc = "is invalid"
	default:
		return summary, fmt.Errorf("unknown status")
	}

	summary = fmt.Sprintf("Integration test for snapshot %s and scenario %s %s", snapshotName, scenarioName, statusDesc)

	return summary, nil
}
