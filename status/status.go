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

//go:generate mockgen -destination mock_status.go -package status github.com/konflux-ci/integration-service/status StatusInterface

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/konflux-ci/integration-service/git/github"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/operator-toolkit/metadata"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	gitlab "github.com/xanzy/go-gitlab"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ScenarioReportStatus keep report status of git provider for the particular scenario
type ScenarioReportStatus struct {
	LastUpdateTime *time.Time `json:"lastUpdateTime"`
}

// SnapshotReportStatus keep report status of git provider for the snapshot
type SnapshotReportStatus struct {
	Scenarios map[string]*ScenarioReportStatus `json:"scenarios"`
	dirty     bool
}

// SetLastUpdateTime updates the last udpate time of the given scenario to the given time
func (srs *SnapshotReportStatus) SetLastUpdateTime(scenarioName string, t time.Time) {
	srs.dirty = true
	if scenario, ok := srs.Scenarios[scenarioName]; ok {
		scenario.LastUpdateTime = &t
		return
	}

	srs.Scenarios[scenarioName] = &ScenarioReportStatus{
		LastUpdateTime: &t,
	}
}

// IsNewer returns true if given scenario has newer time than the last updated
func (srs *SnapshotReportStatus) IsNewer(scenarioName string, t time.Time) bool {
	if scenario, ok := srs.Scenarios[scenarioName]; ok {
		return scenario.LastUpdateTime.Before(t)
	}

	// no record, it's new
	return true
}

// ToAnnotationString exports data in format for annotation
func (srs *SnapshotReportStatus) ToAnnotationString() (string, error) {
	byteVar, err := json.Marshal(srs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal report status metadata: %w", err)
	}
	return string(byteVar), nil
}

// IsDirty returns true if there are new changes to be written
func (srs *SnapshotReportStatus) IsDirty() bool {
	return srs.dirty
}

// ResetDirty marks changes as synced to snapshot
func (srs *SnapshotReportStatus) ResetDirty() {
	srs.dirty = false
}

// NewSnapshotReportStatus creates new object
func NewSnapshotReportStatus(jsondata string) (*SnapshotReportStatus, error) {
	srs := SnapshotReportStatus{
		Scenarios: map[string]*ScenarioReportStatus{},
	}
	if jsondata != "" {
		err := json.Unmarshal([]byte(jsondata), &srs)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal json '%s': %w", jsondata, err)
		}
	}
	return &srs, nil
}

// NewSnapshotReportStatusFromSnapshot creates new SnapshotTestStatus struct from snapshot annotation
func NewSnapshotReportStatusFromSnapshot(s *applicationapiv1alpha1.Snapshot) (*SnapshotReportStatus, error) {
	var (
		statusAnnotation string
		ok               bool
	)

	if statusAnnotation, ok = s.ObjectMeta.GetAnnotations()[gitops.SnapshotStatusReportAnnotation]; !ok {
		statusAnnotation = ""
	}

	srs, err := NewSnapshotReportStatus(statusAnnotation)
	if err != nil {
		return nil, fmt.Errorf("failed to get integration tests report status metadata from snapshot %s (annotation: %s): %w", s.Name, gitops.SnapshotStatusReportAnnotation, err)
	}

	return srs, nil
}

// WriteSnapshotReportStatus writes report status
func WriteSnapshotReportStatus(ctx context.Context, c client.Client, s *applicationapiv1alpha1.Snapshot, srs *SnapshotReportStatus) error {
	if !srs.IsDirty() {
		return nil // nothing to update
	}
	patch := client.MergeFrom(s.DeepCopy())

	value, err := srs.ToAnnotationString()
	if err != nil {
		return fmt.Errorf("failed to marshal test results into JSON: %w", err)
	}

	if err := metadata.SetAnnotation(&s.ObjectMeta, gitops.SnapshotStatusReportAnnotation, value); err != nil {
		return fmt.Errorf("failed to add annotations: %w", err)
	}

	err = c.Patch(ctx, s, patch)
	if err != nil {
		// don't return wrapped err, so we can use RetryOnConflict
		return err
	}

	srs.ResetDirty()
	return nil
}

// MigrateSnapshotToReportStatus migrates old way of keeping updates sync to the new way by updating annotations in snapshot
func MigrateSnapshotToReportStatus(s *applicationapiv1alpha1.Snapshot, testStatuses []*intgteststat.IntegrationTestStatusDetail) {
	if s.ObjectMeta.GetAnnotations() == nil {
		return // nothing to migrate
	}
	annotations := s.ObjectMeta.GetAnnotations()
	_, ok := annotations[gitops.SnapshotPRLastUpdate]
	if !ok {
		return // nothing to migrate
	}

	oldLastUpdateTime, err := gitops.GetLatestUpdateTime(s)
	delete(annotations, gitops.SnapshotPRLastUpdate) // we don't care at this point
	if err != nil {
		return // something happen, cancel migration and start with fresh data
	}

	srs, err := NewSnapshotReportStatus("")
	if err != nil {
		return // something happen, cancel migration and start with fresh data
	}

	for _, testStatDetail := range testStatuses {
		srs.SetLastUpdateTime(testStatDetail.ScenarioName, oldLastUpdateTime)
	}

	annotations[gitops.SnapshotStatusReportAnnotation], _ = srs.ToAnnotationString()
}

type StatusInterface interface {
	GetReporter(*applicationapiv1alpha1.Snapshot) ReporterInterface
	ReportSnapshotStatus(context.Context, ReporterInterface, *applicationapiv1alpha1.Snapshot) error
	// Check if PR/MR is opened
	IsPRMRInSnapshotOpened(context.Context, *applicationapiv1alpha1.Snapshot) (bool, error)
	// Check if github PR is open
	IsPRInSnapshotOpened(context.Context, ReporterInterface, *applicationapiv1alpha1.Snapshot) (bool, error)
	// Check if gitlab MR is open
	IsMRInSnapshotOpened(context.Context, ReporterInterface, *applicationapiv1alpha1.Snapshot) (bool, error)
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

	gitlabReporter := NewGitLabReporter(s.logger, s.client)
	if gitlabReporter.Detect(snapshot) {
		return gitlabReporter
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

	MigrateSnapshotToReportStatus(snapshot, integrationTestStatusDetails)

	srs, err := NewSnapshotReportStatusFromSnapshot(snapshot)
	if err != nil {
		s.logger.Error(err, "failed to get latest snapshot write metadata annotation for snapshot",
			"snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		srs, _ = NewSnapshotReportStatus("")
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		for _, integrationTestStatusDetail := range integrationTestStatusDetails {
			if srs.IsNewer(integrationTestStatusDetail.ScenarioName, integrationTestStatusDetail.LastUpdateTime) {
				s.logger.Info("Integration Test contains new status updates", "scenario.Name", integrationTestStatusDetail.ScenarioName)
			} else {
				//integration test contains no changes
				continue
			}
			testReport, reportErr := s.generateTestReport(ctx, *integrationTestStatusDetail, snapshot)
			if reportErr != nil {
				if writeErr := WriteSnapshotReportStatus(ctx, s.client, snapshot, srs); writeErr != nil { // try to write what was already written
					return fmt.Errorf("failed to generate test report AND write snapshot report status metadata: %w", errors.Join(reportErr, writeErr))
				}
				return fmt.Errorf("failed to generate test report: %w", reportErr)
			}
			if reportStatusErr := reporter.ReportStatus(ctx, *testReport); reportStatusErr != nil {
				if writeErr := WriteSnapshotReportStatus(ctx, s.client, snapshot, srs); writeErr != nil { // try to write what was already written
					return fmt.Errorf("failed to report status AND write snapshot report status metadata: %w", errors.Join(reportStatusErr, writeErr))
				}
				return fmt.Errorf("failed to update status: %w", reportStatusErr)
			}
			srs.SetLastUpdateTime(integrationTestStatusDetail.ScenarioName, integrationTestStatusDetail.LastUpdateTime)
		}
		if err := WriteSnapshotReportStatus(ctx, s.client, snapshot, srs); err != nil {
			return fmt.Errorf("failed to write snapshot report status metadata: %w", err)
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("issue occured during generating or updating report status: %w", err)
	}

	s.logger.Info(fmt.Sprintf("Successfully updated the %s annotation", gitops.SnapshotStatusReportAnnotation), "snapshotReporterStatus.value", srs)

	return nil
}

// generateTestReport generates TestReport to be used by all reporters
func (s *Status) generateTestReport(ctx context.Context, detail intgteststat.IntegrationTestStatusDetail, snapshot *applicationapiv1alpha1.Snapshot) (*TestReport, error) {
	text, err := s.generateText(ctx, detail, snapshot.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to generate text message: %w", err)
	}

	summary, err := GenerateSummary(detail.Status, snapshot.Name, detail.ScenarioName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary message: %w", err)
	}

	consoleName := getConsoleName()

	fullName := fmt.Sprintf("%s / %s", consoleName, detail.ScenarioName)
	if snapshot.Labels[gitops.SnapshotComponentLabel] != "" {
		fullName = fmt.Sprintf("%s / %s", fullName, snapshot.Labels[gitops.SnapshotComponentLabel])
	}

	report := TestReport{
		Text:                text,
		FullName:            fullName,
		ScenarioName:        detail.ScenarioName,
		SnapshotName:        snapshot.Name,
		ComponentName:       snapshot.Labels[gitops.SnapshotComponentLabel],
		Status:              detail.Status,
		Summary:             summary,
		StartTime:           detail.StartTime,
		CompletionTime:      detail.CompletionTime,
		TestPipelineRunName: detail.TestPipelineRunName,
	}
	return &report, nil
}

// generateText generates a text with details for the given state
func (s *Status) generateText(ctx context.Context, integrationTestStatusDetail intgteststat.IntegrationTestStatusDetail, namespace string) (string, error) {
	if integrationTestStatusDetail.Status == intgteststat.IntegrationTestStatusTestPassed || integrationTestStatusDetail.Status == intgteststat.IntegrationTestStatusTestFail {
		pipelineRunName := integrationTestStatusDetail.TestPipelineRunName
		pipelineRun := &tektonv1.PipelineRun{}
		err := s.client.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      pipelineRunName,
		}, pipelineRun)

		if err != nil {
			if apierrors.IsNotFound(err) {
				s.logger.Error(err, "Failed to fetch pipelineRun", "pipelineRun.Name", pipelineRunName)
				text := fmt.Sprintf("%s\n\n\n(Failed to fetch test result details.)", integrationTestStatusDetail.Details)
				return text, nil
			}

			return "", fmt.Errorf("error while getting the pipelineRun %s: %w", pipelineRunName, err)
		}

		taskRuns, err := helpers.GetAllChildTaskRunsForPipelineRun(ctx, s.client, pipelineRun)
		if err != nil {
			return "", fmt.Errorf("error while getting all child taskRuns from pipelineRun %s: %w", pipelineRunName, err)
		}
		text, err := FormatTestsSummary(taskRuns, pipelineRunName, namespace, s.logger)
		if err != nil {
			return "", err
		}
		return text, nil
	} else {
		text := integrationTestStatusDetail.Details
		return text, nil
	}
}

// GenerateSummary returns summary for the given state, snapshotName and scenarioName
func GenerateSummary(state intgteststat.IntegrationTestStatus, snapshotName, scenarioName string) (string, error) {
	var summary string
	var statusDesc string

	switch state {
	case intgteststat.IntegrationTestStatusPending:
		statusDesc = "is pending"
	case intgteststat.IntegrationTestStatusInProgress:
		statusDesc = "is in progress"
	case intgteststat.IntegrationTestStatusEnvironmentProvisionError_Deprecated:
		statusDesc = "experienced an error when provisioning environment"
	case intgteststat.IntegrationTestStatusDeploymentError_Deprecated:
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

func getConsoleName() string {
	consoleName := os.Getenv("CONSOLE_NAME")
	if consoleName == "" {
		return "Integration Service"
	}
	return consoleName
}

func (s Status) IsPRMRInSnapshotOpened(ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot) (bool, error) {
	// need to rework reporter.Detect() function and reuse it here
	githubReporter := NewGitHubReporter(s.logger, s.client)
	if githubReporter.Detect(snapshot) {
		err := githubReporter.Initialize(ctx, snapshot)
		if err != nil {
			return false, err
		}
		return s.IsPRInSnapshotOpened(ctx, githubReporter, snapshot)
	}

	gitlabReporter := NewGitLabReporter(s.logger, s.client)
	if gitlabReporter.Detect(snapshot) {
		err := gitlabReporter.Initialize(ctx, snapshot)
		if err != nil {
			return false, err
		}
		return s.IsMRInSnapshotOpened(ctx, gitlabReporter, snapshot)
	}

	return false, fmt.Errorf("invalid git provier, valid git provider must be one of github and gitlab")
}

// IsMRInSnapshotOpened check if the gitlab merge request triggering snapshot is opened
func (s Status) IsMRInSnapshotOpened(ctx context.Context, reporter ReporterInterface, snapshot *applicationapiv1alpha1.Snapshot) (bool, error) {
	log := log.FromContext(ctx)
	token, err := GetPACGitProviderToken(ctx, s.client, snapshot)
	if err != nil {
		log.Error(err, "failed to get token from snapshot",
			"snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return false, fmt.Errorf("failed to get PAC token for gitlab provider: %w", err)
	}

	annotations := snapshot.GetAnnotations()
	repoUrl, ok := annotations[gitops.PipelineAsCodeRepoURLAnnotation]
	if !ok {
		return false, fmt.Errorf("failed to get value of %s annotation from the snapshot %s", gitops.PipelineAsCodeRepoURLAnnotation, snapshot.Name)
	}

	burl, err := url.Parse(repoUrl)
	if err != nil {
		return false, fmt.Errorf("failed to parse repo-url: %w", err)
	}
	apiURL := fmt.Sprintf("%s://%s", burl.Scheme, burl.Host)

	gitLabClient, err := gitlab.NewClient(token, gitlab.WithBaseURL(apiURL))
	if err != nil {
		return false, fmt.Errorf("failed to create gitlab client: %w", err)
	}

	targetProjectIDstr, found := annotations[gitops.PipelineAsCodeTargetProjectIDAnnotation]
	if !found {
		return false, fmt.Errorf("target project ID annotation not found %q", gitops.PipelineAsCodeTargetProjectIDAnnotation)
	}
	targetProjectID, err := strconv.Atoi(targetProjectIDstr)
	if err != nil {
		return false, fmt.Errorf("failed to convert project ID '%s' to integer: %w", targetProjectIDstr, err)
	}

	mergeRequestStr, found := annotations[gitops.PipelineAsCodePullRequestAnnotation]
	if !found {
		return false, fmt.Errorf("pull-request annotation not found %q", gitops.PipelineAsCodePullRequestAnnotation)
	}
	mergeRequest, err := strconv.Atoi(mergeRequestStr)
	if err != nil {
		return false, fmt.Errorf("failed to convert merge request number '%s' to integer: %w", mergeRequestStr, err)
	}

	log.Info(fmt.Sprintf("try to find the status of merge request projectID/pulls %d/%d", targetProjectID, mergeRequest))
	getOpts := gitlab.GetMergeRequestsOptions{}
	mr, _, err := gitLabClient.MergeRequests.GetMergeRequest(targetProjectID, mergeRequest, &getOpts)
	if mr != nil {
		log.Info(fmt.Sprintf("found merge request projectID/pulls %d/%d", targetProjectID, mergeRequest))
		return mr.State == "opened", err
	}
	log.Info(fmt.Sprintf("can not find merge request projectID/pulls %d/%d", targetProjectID, mergeRequest))
	return false, err
}

// IsPRInSnapshotOpened check if the github pull request triggering snapshot is opened
func (s Status) IsPRInSnapshotOpened(ctx context.Context, reporter ReporterInterface, snapshot *applicationapiv1alpha1.Snapshot) (bool, error) {
	log := log.FromContext(ctx)
	ghClient := github.NewClient(s.logger)
	githubAppCreds, err := GetAppCredentials(ctx, s.client, snapshot)

	if err != nil {
		log.Error(err, "failed to get app credentials from Snapshot",
			"snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return false, err
	}

	token, err := ghClient.CreateAppInstallationToken(ctx, githubAppCreds.AppID, githubAppCreds.InstallationID, githubAppCreds.PrivateKey)
	if err != nil {
		log.Error(err, "failed to create app installation token",
			"githubAppCreds.AppID", githubAppCreds.AppID, "githubAppCreds.InstallationID", githubAppCreds.InstallationID)
		return false, err
	}

	ghClient.SetOAuthToken(ctx, token)

	labels := snapshot.GetLabels()

	owner, found := labels[gitops.PipelineAsCodeURLOrgLabel]
	if !found {
		return false, fmt.Errorf("org label not found %q", gitops.PipelineAsCodeURLOrgLabel)
	}

	repo, found := labels[gitops.PipelineAsCodeURLRepositoryLabel]
	if !found {
		return false, fmt.Errorf("repository label not found %q", gitops.PipelineAsCodeURLRepositoryLabel)
	}

	pullRequestStr, found := labels[gitops.PipelineAsCodePullRequestAnnotation]
	if !found {
		return false, fmt.Errorf("pull request label not found %q", gitops.PipelineAsCodePullRequestAnnotation)
	}

	pullRequest, err := strconv.Atoi(pullRequestStr)
	if err != nil {
		return false, fmt.Errorf("failed to convert pull request number '%s' to integer: %w", pullRequestStr, err)
	}

	log.Info(fmt.Sprintf("try to find the status of pull request owner/repo/pulls %s/%s/%d", owner, repo, pullRequest))
	pr, err := ghClient.GetPullRequest(ctx, owner, repo, pullRequest)
	if pr != nil {
		return *pr.State == "open", nil
	}
	return false, err
}
