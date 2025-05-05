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
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/git/github"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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

// SetLastUpdateTime updates the last udpate time of the given scenario and snapshot to the given time
func (srs *SnapshotReportStatus) SetLastUpdateTime(scenarioName string, snapshotName string, t time.Time) {
	srs.dirty = true
	//use scenarioName and snapshotName as the key to support group snapshot status report
	keyName := scenarioName + "-" + snapshotName
	if scenario, ok := srs.Scenarios[keyName]; ok {
		scenario.LastUpdateTime = &t
		return
	}

	srs.Scenarios[keyName] = &ScenarioReportStatus{
		LastUpdateTime: &t,
	}
}

// IsNewer returns true if given scenario and snapshot has newer time than the last updated
func (srs *SnapshotReportStatus) IsNewer(scenarioName string, snapshotName string, t time.Time) bool {
	key := scenarioName + "-" + snapshotName
	if scenario, ok := srs.Scenarios[key]; ok {
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
		srs.SetLastUpdateTime(testStatDetail.ScenarioName, s.Name, oldLastUpdateTime)
	}

	annotations[gitops.SnapshotStatusReportAnnotation], _ = srs.ToAnnotationString()
}

type StatusInterface interface {
	GetReporter(*applicationapiv1alpha1.Snapshot) ReporterInterface
	// Check if PR/MR is opened
	IsPRMRInSnapshotOpened(context.Context, *applicationapiv1alpha1.Snapshot) (bool, error)
	// Check if github PR is open
	IsPRInSnapshotOpened(context.Context, ReporterInterface, *applicationapiv1alpha1.Snapshot) (bool, error)
	// Check if gitlab MR is open
	IsMRInSnapshotOpened(context.Context, ReporterInterface, *applicationapiv1alpha1.Snapshot) (bool, error)
	// find snapshot with opened PR or MR
	FindSnapshotWithOpenedPR(context.Context, *[]applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Snapshot, error)
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

// GenerateTestReport generates TestReport to be used by all reporters
func GenerateTestReport(ctx context.Context, client client.Client, detail intgteststat.IntegrationTestStatusDetail, testedSnapshot *applicationapiv1alpha1.Snapshot, componentName string) (*TestReport, error) {
	var err error

	text, err := generateText(ctx, client, detail, testedSnapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to generate text message: %w", err)
	}

	summary, err := GenerateSummary(detail.Status, testedSnapshot.Name, detail.ScenarioName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary message: %w", err)
	}

	consoleName := getConsoleName()

	fullName := fmt.Sprintf("%s / %s", consoleName, detail.ScenarioName)
	if componentName != "" {
		fullName = fmt.Sprintf("%s / %s", fullName, componentName)
	}

	report := TestReport{
		Text:                text,
		FullName:            fullName,
		ScenarioName:        detail.ScenarioName,
		SnapshotName:        testedSnapshot.Name,
		ComponentName:       componentName,
		Status:              detail.Status,
		Summary:             summary,
		StartTime:           detail.StartTime,
		CompletionTime:      detail.CompletionTime,
		TestPipelineRunName: detail.TestPipelineRunName,
	}
	return &report, nil
}

// generateText generates a text with details for the given state
func generateText(ctx context.Context, client client.Client, integrationTestStatusDetail intgteststat.IntegrationTestStatusDetail, snapshot *applicationapiv1alpha1.Snapshot) (string, error) {
	log := log.FromContext(ctx)

	var componentSnapshotInfos []*gitops.ComponentSnapshotInfo
	var err error
	if componentSnapshotInfoString, ok := snapshot.Annotations[gitops.GroupSnapshotInfoAnnotation]; ok {
		componentSnapshotInfos, err = gitops.UnmarshalJSON([]byte(componentSnapshotInfoString))
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal JSON string: %w", err)
		}
	}

	pr_group := snapshot.GetAnnotations()[gitops.PRGroupAnnotation]

	if integrationTestStatusDetail.Status == intgteststat.IntegrationTestStatusTestPassed || integrationTestStatusDetail.Status == intgteststat.IntegrationTestStatusTestFail {
		pipelineRunName := integrationTestStatusDetail.TestPipelineRunName
		pipelineRun := &tektonv1.PipelineRun{}
		err := client.Get(ctx, types.NamespacedName{
			Namespace: snapshot.Namespace,
			Name:      pipelineRunName,
		}, pipelineRun)

		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Error(err, "Failed to fetch pipelineRun", "pipelineRun.Name", pipelineRunName)
				text := fmt.Sprintf("%s\n\n\n(Failed to fetch test result details because pipelineRun %s/%s can not be found.)", integrationTestStatusDetail.Details, snapshot.Namespace, pipelineRunName)
				return text, nil
			}

			return "", fmt.Errorf("error while getting the pipelineRun %s: %w", pipelineRunName, err)
		}

		taskRuns, err := helpers.GetAllChildTaskRunsForPipelineRun(ctx, client, pipelineRun)
		if err != nil {
			return "", fmt.Errorf("error while getting all child taskRuns from pipelineRun %s: %w", pipelineRunName, err)
		}
		text, err := FormatTestsSummary(taskRuns, pipelineRunName, snapshot.Namespace, componentSnapshotInfos, pr_group, log)
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
	case intgteststat.BuildPLRInProgress:
		statusDesc = "is pending because build pipelinerun is still running and snapshot has not been created"
	case intgteststat.SnapshotCreationFailed:
		statusDesc = "has not run and is considered as failed because the snapshot was not created"
	case intgteststat.BuildPLRFailed:
		statusDesc = "has not run and is considered as failed because the build pipelinerun failed and snapshot was not created"
	case intgteststat.GroupSnapshotCreationFailed:
		statusDesc = "has not run and is considered as failed because group snapshot was not created"
	default:
		return summary, fmt.Errorf("unknown status")
	}

	if state == intgteststat.BuildPLRInProgress || state == intgteststat.SnapshotCreationFailed || state == intgteststat.BuildPLRFailed || state == intgteststat.GroupSnapshotCreationFailed {
		summary = fmt.Sprintf("Integration test for scenario %s %s", scenarioName, statusDesc)
	} else {
		summary = fmt.Sprintf("Integration test for snapshot %s and scenario %s %s", snapshotName, scenarioName, statusDesc)
	}

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

	return false, fmt.Errorf("invalid git provider, valid git provider must be one of github and gitlab")
}

// IsMRInSnapshotOpened check if the gitlab merge request triggering snapshot is opened
func (s Status) IsMRInSnapshotOpened(ctx context.Context, reporter ReporterInterface, snapshot *applicationapiv1alpha1.Snapshot) (bool, error) {
	var unRecoverableError error
	log := log.FromContext(ctx)
	token, err := GetPACGitProviderToken(ctx, s.client, snapshot)
	if err != nil {
		log.Error(err, "failed to get PAC token from snapshot",
			"snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return false, err
	}

	annotations := snapshot.GetAnnotations()
	repoUrl, ok := annotations[gitops.PipelineAsCodeRepoURLAnnotation]
	if !ok {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to get value of %s annotation from the snapshot %s", gitops.PipelineAsCodeRepoURLAnnotation, snapshot.Name))
		log.Error(unRecoverableError, fmt.Sprintf("failed to get value of %s annotation from the snapshot %s", gitops.PipelineAsCodeRepoURLAnnotation, snapshot.Name))
		return false, unRecoverableError
	}

	burl, err := url.Parse(repoUrl)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to parse repo-url: %s", err.Error()))
		log.Error(unRecoverableError, fmt.Sprintf("failed to parse repo-url: %s", err.Error()))
		return false, unRecoverableError
	}
	apiURL := fmt.Sprintf("%s://%s", burl.Scheme, burl.Host)

	gitLabClient, err := gitlab.NewClient(token, gitlab.WithBaseURL(apiURL))
	if err != nil {
		log.Error(err, "failed to create gitLabClient")
		return false, fmt.Errorf("failed to create gitlab client: %w", err)
	}

	targetProjectIDstr, found := annotations[gitops.PipelineAsCodeTargetProjectIDAnnotation]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("target project ID annotation not found %q", gitops.PipelineAsCodeTargetProjectIDAnnotation))
		log.Error(unRecoverableError, fmt.Sprintf("target project ID annotation not found %q", gitops.PipelineAsCodeTargetProjectIDAnnotation))
		return false, unRecoverableError
	}
	targetProjectID, err := strconv.Atoi(targetProjectIDstr)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert project ID '%s' to integer: %s", targetProjectIDstr, err.Error()))
		log.Error(unRecoverableError, "failed to convert project ID '%s' to integer", targetProjectIDstr)
		return false, unRecoverableError
	}

	mergeRequestStr, found := annotations[gitops.PipelineAsCodePullRequestAnnotation]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("pull-request annotation not found %q", gitops.PipelineAsCodePullRequestAnnotation))
		log.Error(unRecoverableError, fmt.Sprintf("pull-request annotation not found %q", gitops.PipelineAsCodePullRequestAnnotation))
		return false, unRecoverableError
	}
	mergeRequest, err := strconv.Atoi(mergeRequestStr)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert merge request number '%s' to integer: %s", mergeRequestStr, err.Error()))
		log.Error(unRecoverableError, fmt.Sprintf("failed to convert merge request number '%s' to integer", mergeRequestStr))
		return false, unRecoverableError
	}

	log.Info(fmt.Sprintf("try to find the status of merge request projectID/pulls %d/%d", targetProjectID, mergeRequest))
	getOpts := gitlab.GetMergeRequestsOptions{}
	mr, _, err := gitLabClient.MergeRequests.GetMergeRequest(targetProjectID, mergeRequest, &getOpts)

	if err != nil && strings.Contains(err.Error(), "Not Found") {
		log.Info(fmt.Sprintf("not found merge request projectID/pulls %d/%d, it might be deleted", targetProjectID, mergeRequest))
		return false, nil
	}

	if mr != nil && err == nil {
		log.Info(fmt.Sprintf("found merge request projectID/pulls %d/%d", targetProjectID, mergeRequest))
		return mr.State == "opened", nil
	}

	log.Error(err, fmt.Sprintf("can not find merge request projectID/pulls %d/%d", targetProjectID, mergeRequest))
	return false, err
}

// IsPRInSnapshotOpened check if the github pull request triggering snapshot is opened
func (s Status) IsPRInSnapshotOpened(ctx context.Context, reporter ReporterInterface, snapshot *applicationapiv1alpha1.Snapshot) (bool, error) {
	var unRecoverableError error
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
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("org label not found %q", gitops.PipelineAsCodeURLOrgLabel))
		log.Error(unRecoverableError, fmt.Sprintf("org label not found %q", gitops.PipelineAsCodeURLOrgLabel))
		return false, unRecoverableError
	}

	repo, found := labels[gitops.PipelineAsCodeURLRepositoryLabel]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("repository label not found %q", gitops.PipelineAsCodeURLRepositoryLabel))
		log.Error(unRecoverableError, fmt.Sprintf("repository label not found %q", gitops.PipelineAsCodeURLRepositoryLabel))
		return false, unRecoverableError
	}

	pullRequestStr, found := labels[gitops.PipelineAsCodePullRequestAnnotation]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("pull request label not found %q", gitops.PipelineAsCodePullRequestAnnotation))
		log.Error(unRecoverableError, fmt.Sprintf("pull request label not found %q", gitops.PipelineAsCodePullRequestAnnotation))
		return false, unRecoverableError
	}

	pullRequest, err := strconv.Atoi(pullRequestStr)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert pull request number '%s' to integer: %s", pullRequestStr, err.Error()))
		log.Error(unRecoverableError, fmt.Sprintf("failed to convert pull request number '%s' to integer", pullRequestStr))
		return false, unRecoverableError
	}

	log.Info(fmt.Sprintf("try to find the status of pull request owner/repo/pulls %s/%s/%d", owner, repo, pullRequest))
	pr, err := ghClient.GetPullRequest(ctx, owner, repo, pullRequest)

	if err != nil && strings.Contains(err.Error(), "Not Found") {
		log.Error(err, fmt.Sprintf("not found pull request owner/repo/pulls %s/%s/%d, it might be deleted", owner, repo, pullRequest))
		return false, nil
	}

	if pr != nil {
		return *pr.State == "open", nil
	}

	log.Error(err, fmt.Sprintf("cannot find pull request owner/repo/pulls %s/%s/%d", owner, repo, pullRequest))
	return false, err
}

// GetComponentSnapshotsFromGroupSnapshot return the component snapshot list which component snapshot is created from
func GetComponentSnapshotsFromGroupSnapshot(ctx context.Context, c client.Client, groupSnapshot *applicationapiv1alpha1.Snapshot) ([]*applicationapiv1alpha1.Snapshot, error) {
	log := log.FromContext(ctx)
	var componentSnapshotInfos []*gitops.ComponentSnapshotInfo
	var componentSnapshots []*applicationapiv1alpha1.Snapshot
	var err error
	if componentSnapshotInfoString, ok := groupSnapshot.Annotations[gitops.GroupSnapshotInfoAnnotation]; ok {
		componentSnapshotInfos, err = gitops.UnmarshalJSON([]byte(componentSnapshotInfoString))
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON string: %w", err)
		}
	}

	for _, componentSnapshotInfo := range componentSnapshotInfos {
		componentSnapshot := &applicationapiv1alpha1.Snapshot{}
		err = c.Get(ctx, types.NamespacedName{
			Namespace: groupSnapshot.Namespace,
			Name:      componentSnapshotInfo.Snapshot,
		}, componentSnapshot)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to find component snapshot %s included in group snapshot %s/%s", componentSnapshotInfo.Snapshot, groupSnapshot.Namespace, groupSnapshot.Name))
			continue
		}
		componentSnapshots = append(componentSnapshots, componentSnapshot)
	}
	return componentSnapshots, nil

}

// FindSnapshotWithOpenedPR find the latest snapshot with opened PR/MR for the given sorted snapshots
func (s Status) FindSnapshotWithOpenedPR(ctx context.Context, snapshots *[]applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Snapshot, error) {
	log := log.FromContext(ctx)
	sortedSnapshots := gitops.SortSnapshots(*snapshots)
	// find the latest component snapshot created for open PR/MR
	for _, snapshot := range sortedSnapshots {
		snapshot := snapshot
		// find the built image for pull/merge request build PLR from the latest opened pull request component snapshot
		isPRMROpened, err := s.IsPRMRInSnapshotOpened(ctx, &snapshot)
		if err != nil {
			log.Error(err, "Failed to fetch PR/MR status for component snapshot", "snapshot.Name", snapshot.Name)
			return nil, err
		}
		if isPRMROpened {
			log.Info("PR/MR in snapshot is opened, will find snapshotComponent and add to groupSnapshot")
			return &snapshot, nil
		}
	}
	return nil, nil
}

// iterates integrationTestScenarios to set integration test status in PR/MR
func IterateIntegrationTestInStatusReport(ctx context.Context, client client.Client, reporter ReporterInterface,
	snapshot *applicationapiv1alpha1.Snapshot,
	integrationTestScenarios *[]v1beta2.IntegrationTestScenario,
	integrationTestStatusDetail intgteststat.IntegrationTestStatusDetail,
	componentName string) error {

	for _, integrationTestScenario := range *integrationTestScenarios {
		integrationTestScenario := integrationTestScenario //G601
		integrationTestStatusDetail.ScenarioName = integrationTestScenario.Name

		testReport, reportErr := GenerateTestReport(ctx, client, integrationTestStatusDetail, snapshot, componentName)
		if reportErr != nil {
			return fmt.Errorf("failed to generate test report: %w", reportErr)
		}
		if reportStatusErr := reporter.ReportStatus(ctx, *testReport); reportStatusErr != nil {
			return fmt.Errorf("failed to report status to git provider: %w", reportStatusErr)
		}
	}
	return nil
}
