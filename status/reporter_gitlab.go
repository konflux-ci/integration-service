/*
Copyright 2024 Red Hat Inc.

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

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/operator-toolkit/metadata"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	"github.com/konflux-ci/integration-service/pkg/common"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
)

type GitLabReporter struct {
	logger          *logr.Logger
	k8sClient       client.Client
	client          *gitlab.Client
	sha             string
	sourceProjectID int
	targetProjectID int
	mergeRequest    int
	snapshot        *applicationapiv1alpha1.Snapshot
}

func NewGitLabReporter(logger logr.Logger, k8sClient client.Client) *GitLabReporter {
	return &GitLabReporter{
		logger:    &logger,
		k8sClient: k8sClient,
	}
}

var GitLabProvider = "GitlabReporter"

// check if interface has been correctly implemented
var _ ReporterInterface = (*GitLabReporter)(nil)
var existingCommitStatus *gitlab.CommitStatus

// Detect if snapshot has been created from gitlab provider
func (r *GitLabReporter) Detect(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return metadata.HasAnnotationWithValue(snapshot, gitops.PipelineAsCodeGitProviderLabel, gitops.PipelineAsCodeGitLabProviderType) ||
		metadata.HasLabelWithValue(snapshot, gitops.PipelineAsCodeGitProviderAnnotation, gitops.PipelineAsCodeGitLabProviderType)
}

// GetReporterName returns the reporter name
func (r *GitLabReporter) GetReporterName() string {
	return GitLabProvider
}

// Initialize initializes gitlab reporter
func (r *GitLabReporter) Initialize(ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot) (int, error) {
	var unRecoverableError error
	token, err := GetPACGitProviderToken(ctx, r.k8sClient, snapshot)
	if err != nil {
		r.logger.Error(err, "failed to get PAC token from snapshot",
			"snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, err
	}

	annotations := snapshot.GetAnnotations()
	repoUrl, ok := annotations[gitops.PipelineAsCodeRepoURLAnnotation]
	if !ok {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to get value of %s annotation from the snapshot %s", gitops.PipelineAsCodeRepoURLAnnotation, snapshot.Name))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}

	burl, err := url.Parse(repoUrl)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to parse repo-url %s: %s", repoUrl, err.Error()))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}
	apiURL := fmt.Sprintf("%s://%s", burl.Scheme, burl.Host)

	r.client, err = gitlab.NewClient(token, gitlab.WithBaseURL(apiURL), gitlab.WithUserAgent(common.IntegrationServiceUserAgent))
	if err != nil {
		r.logger.Error(err, "failed to create gitlab client", "apiURL", apiURL, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, err
	}

	labels := snapshot.GetLabels()
	sha, found := labels[gitops.PipelineAsCodeSHALabel]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("sha label not found %q", gitops.PipelineAsCodeSHALabel))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}
	r.sha = sha

	targetProjectIDstr, found := annotations[gitops.PipelineAsCodeTargetProjectIDAnnotation]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("target project ID annotation not found %q", gitops.PipelineAsCodeTargetProjectIDAnnotation))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}

	r.targetProjectID, err = strconv.Atoi(targetProjectIDstr)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert project ID '%s' to integer: %s", targetProjectIDstr, err.Error()))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}

	sourceProjectIDstr, found := annotations[gitops.PipelineAsCodeSourceProjectIDAnnotation]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("source project ID annotation not found %q", gitops.PipelineAsCodeSourceProjectIDAnnotation))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}

	r.sourceProjectID, err = strconv.Atoi(sourceProjectIDstr)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert project ID '%s' to integer: %s", sourceProjectIDstr, err.Error()))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}

	mergeRequestStr, found := annotations[gitops.PipelineAsCodePullRequestAnnotation]
	if !found && !gitops.IsSnapshotCreatedByPACPushEvent(snapshot) {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("pull-request annotation not found %q", gitops.PipelineAsCodePullRequestAnnotation))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}

	if found {
		r.mergeRequest, err = strconv.Atoi(mergeRequestStr)
		if err != nil && !gitops.IsSnapshotCreatedByPACPushEvent(snapshot) {
			unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert merge request number '%s' to integer: %s", mergeRequestStr, err.Error()))
			r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
			return 0, unRecoverableError
		}
	}

	r.snapshot = snapshot
	return 0, nil
}

// setCommitStatus sets commit status to be shown as pipeline run in gitlab view
func (r *GitLabReporter) setCommitStatus(report TestReport) (int, error) {
	var statusCode = 0
	l := loader.NewLoader()
	scenario, err := l.GetScenario(context.Background(), r.k8sClient, report.ScenarioName, r.snapshot.Namespace)

	if err != nil {
		r.logger.Error(err, fmt.Sprintf("could not determine whether scenario %s was optional", report.ScenarioName))
		return 0, fmt.Errorf("could not determine whether scenario %s is optional, %w", report.ScenarioName, err)
	}
	optional := helpers.IsIntegrationTestScenarioOptional(scenario)
	glState, err := GenerateGitlabCommitState(report.Status, optional)
	if err != nil {
		return 0, fmt.Errorf("failed to generate gitlab state: %w", err)
	}

	opt := gitlab.SetCommitStatusOptions{
		State:       gitlab.BuildStateValue(glState),
		Name:        gitlab.Ptr(report.FullName),
		Description: gitlab.Ptr(report.Summary),
	}

	if report.TestPipelineRunName == "" {
		r.logger.Info("TestPipelineRunName is not set, cannot add URL to message")
	} else {
		url := FormatPipelineURL(report.TestPipelineRunName, r.snapshot.Namespace, *r.logger)
		opt.TargetURL = gitlab.Ptr(url)
	}

	// Fetch commit statuses only if necessary
	if glState == gitlab.Running || glState == gitlab.Pending {
		allCommitStatuses, response, err := r.client.Commits.GetCommitStatuses(r.sourceProjectID, r.sha, nil)
		if response != nil {
			statusCode = response.StatusCode
		}
		if err != nil {
			return statusCode, fmt.Errorf("error while getting all commitStatuses for sha %s: %w", r.sha, err)
		}
		existingCommitStatus = r.GetExistingCommitStatus(allCommitStatuses, report.FullName)

		// special case, we want to skip updating commit status if the status from running to running, from pending to pending
		if existingCommitStatus != nil && existingCommitStatus.Status == string(glState) {
			r.logger.Info("Skipping redundant status update since the existing commit status has the same state",
				"scenario.name", report.ScenarioName,
				"commitStatus.ID", existingCommitStatus.ID,
				"current_status", existingCommitStatus.Status,
				"new_status", glState)
			return statusCode, nil
		}
	}

	r.logger.Info("creating commit status for scenario test status of snapshot",
		"scenarioName", report.ScenarioName)

	sourceProjectCommitStatus, sourceProjectResponse, sourceProjectErr := r.client.Commits.SetCommitStatus(r.sourceProjectID, r.sha, &opt)
	if sourceProjectResponse != nil {
		statusCode = sourceProjectResponse.StatusCode
	}
	if sourceProjectErr == nil {
		r.logger.Info("Created gitlab commit status", "scenario.name", report.ScenarioName, "commitStatus.ID", sourceProjectCommitStatus.ID, "TargetURL", opt.TargetURL)
		return statusCode, nil
	} else if strings.Contains(sourceProjectErr.Error(), "Cannot transition status via :enqueue from :pending") {
		r.logger.Info("Ignoring the error when transition from pending to pending when the commitStatus might be created/updated in multiple threads at the same time occasionally")
		return statusCode, nil
	} else {
		r.logger.Error(sourceProjectErr, "failed to set commit status to gitlab source project, try to set commit status to gitlab target project",
			"sourceProjectID", r.sourceProjectID, "targetProjectID", r.targetProjectID, "sha", r.sha,
			"scenario.name", report.ScenarioName, "statusCode", statusCode, "targetState", string(glState))
	}

	targetProjectCommitStatus, targetProjectResponse, targetProjectErr := r.client.Commits.SetCommitStatus(r.targetProjectID, r.sha, &opt)
	if targetProjectResponse != nil {
		statusCode = targetProjectResponse.StatusCode
	}
	if targetProjectErr == nil {
		r.logger.Info("Created gitlab commit status", "scenario.name", report.ScenarioName, "commitStatus2.ID", targetProjectCommitStatus.ID, "TargetURL", opt.TargetURL)
		return statusCode, nil
	}
	// this code will only be reached if both source project and target project cannot be updated
	// when commitStatus is created in multiple thread occasionally, we can still see the transition error, so let's ignore it as a workaround
	if strings.Contains(targetProjectErr.Error(), "Cannot transition status via :enqueue from :pending") {
		r.logger.Info("Ignoring the error when transition from pending to pending when the commitStatus might be created/updated in multiple threads at the same time occasionally")
		return statusCode, nil
	}

	r.logger.Error(errors.Join(targetProjectErr, sourceProjectErr), "failed to set commit status to gitlab source project and target project",
		"sourceProjectID", r.sourceProjectID, "targetProjectID", r.targetProjectID, "sha", r.sha,
		"scenario.name", report.ScenarioName, "statusCode", statusCode, "targetState", string(glState))
	return statusCode, fmt.Errorf("failed to set commit status to %s with returned statusCode %d: %w", string(glState), statusCode, errors.Join(targetProjectErr, sourceProjectErr))
}

// UpdateStatusInComment searches and deletes existing comments according to commentPrefix and create a new comment in the MR which creates snapshot
func (r *GitLabReporter) UpdateStatusInComment(commentPrefix, comment string) (int, error) {
	var statusCode = 0

	// get all existing integration test comment according to commentPrefix in integration test summary and delete them
	allNotes, response, err := r.client.Notes.ListMergeRequestNotes(r.targetProjectID, r.mergeRequest, nil)
	if response != nil {
		statusCode = response.StatusCode
	}
	if err != nil {
		r.logger.Error(err, "error while getting all comments for merge-request", "mergeRequest", r.mergeRequest, "report.SnapshotName", r.snapshot.Name)
		return statusCode, fmt.Errorf("error while getting all comments for merge-request %d: %w", r.mergeRequest, err)
	}

	noteIDs := r.GetExistingCommentIDs(allNotes, commentPrefix)

	if len(noteIDs) > 0 {
		// update the first existing comment but delete others because sometimes there might be multiple existing comments for the same component due to previous intermittent errors
		r.logger.Info("found multiple existing notes for the same component, updating the first one but delete others", "commentPrefix", commentPrefix, "count", len(allNotes))
		noteIdToBeUpdated := noteIDs[0]

		if len(noteIDs) > 1 {
			r.logger.Info("deleting other existing notes since we will update the first one", "noteIDsToBeDeleted", noteIDs[1:])
			statusCode, err = r.DeleteExistingNotes(noteIDs[1:])
			if err != nil {
				return statusCode, fmt.Errorf("error while deleting existing comments for merge-request %d from project %d : %w", r.mergeRequest, r.targetProjectID, err)
			}
		}

		// update the first existing comment
		noteOptions := gitlab.UpdateMergeRequestNoteOptions{Body: &comment}
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			_, response, err = r.client.Notes.UpdateMergeRequestNote(r.targetProjectID, r.mergeRequest, noteIdToBeUpdated, &noteOptions)
			return err
		})
		if response != nil {
			statusCode = response.StatusCode
		}
		if err != nil {
			return statusCode, fmt.Errorf("error while updating comment %d for merge-request %d: %w", noteIdToBeUpdated, r.mergeRequest, err)
		}
		r.logger.Info("updated existing note with matching commentPrefix", "noteID", noteIdToBeUpdated, "commentPrefix", commentPrefix)
	} else {
		// create a new comment
		r.logger.Info("no existing notes found with matching commentPrefix, creating a new note", "commentPrefix", commentPrefix)
		noteOptions := gitlab.CreateMergeRequestNoteOptions{Body: &comment}
		_, response, err = r.client.Notes.CreateMergeRequestNote(r.targetProjectID, r.mergeRequest, &noteOptions)
		if response != nil {
			statusCode = response.StatusCode
		}
		if err != nil {
			return statusCode, fmt.Errorf("error while creating comment for merge-request %d: %w", r.mergeRequest, err)
		}
	}

	if err != nil {
		return statusCode, fmt.Errorf("error while creating comment for merge-request %d: %w", r.mergeRequest, err)
	}

	return statusCode, nil
}

// GetExistingCommitStatus returns existing GitLab commit status that matches .
func (r *GitLabReporter) GetExistingCommitStatus(commitStatuses []*gitlab.CommitStatus, statusName string) *gitlab.CommitStatus {
	for _, commitStatus := range commitStatuses {
		if commitStatus.Name == statusName {
			r.logger.Info("found matching existing commitStatus",
				"commitStatus.Name", commitStatus.Name, "commitStatus.ID", commitStatus.ID)
			return commitStatus
		}
	}
	r.logger.Info("found no matching existing commitStatus", "statusName", statusName)
	return nil
}

func (r *GitLabReporter) GetExistingCommentIDs(notes []*gitlab.Note, commentPrefix string) []int {
	var commentIDs []int
	for _, note := range notes {
		// get existing note by search commentTitle in report summary
		// GetExistingCommentIDs for github comment has the similar logic
		if strings.Contains(note.Body, commentPrefix) {
			r.logger.Info("found note ID with a matching commentTitle", "commentTitle", commentPrefix, "noteID", &note.ID)
			commentIDs = append(commentIDs, note.ID)
		}
	}

	r.logger.Info("found no note with a matching commentTitle", "commentTitle", commentPrefix)
	return commentIDs
}

// DeleteExistingNotes deletes existing GitLab note with given noteIDs.
func (r *GitLabReporter) DeleteExistingNotes(noteIDs []int) (int, error) {
	var lastStatusCode = 0
	var errs []error // collect errors during deletion

	for _, noteID := range noteIDs {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			response, err := r.client.Notes.DeleteMergeRequestNote(r.targetProjectID, r.mergeRequest, noteID)
			if response != nil {
				lastStatusCode = response.StatusCode
			}
			return err
		})

		if err != nil {
			r.logger.Error(err, "failed to delete note", "noteID", noteID)
			errs = append(errs, fmt.Errorf("noteID %d: %w", noteID, err))
			continue // continue to delete next note
		}
		r.logger.Info("existing note deleted", "noteID", noteID)
	}

	if len(errs) > 0 {
		return lastStatusCode, fmt.Errorf("errors occurred during deletion existing notes on mergerequest %d of target project %d: %v", r.mergeRequest, r.targetProjectID, errs)
	}

	return lastStatusCode, nil
}

// ReportStatus reports test result to gitlab
func (r *GitLabReporter) ReportStatus(ctx context.Context, report TestReport) (int, error) {
	var statusCode = 0
	if r.client == nil {
		return statusCode, fmt.Errorf("gitlab reporter is not initialized")
	}

	var err error
	err = retry.OnError(reporterRetryBackoff, func(err error) bool {
		// statusCode 0 means no HTTP response was received (network/timeout error), always retry
		retryable := statusCode == 0 || !r.ReturnCodeIsUnrecoverable(statusCode)
		if retryable {
			r.logger.Info("retrying to set gitlab commit status after transient error",
				"scenario.name", report.ScenarioName, "statusCode", statusCode, "error", err.Error())
		}
		return retryable
	}, func() error {
		statusCode = 0 // reset before each attempt to avoid stale values
		statusCode, err = r.setCommitStatus(report)
		return err
	})

	if err != nil {
		r.logger.Error(err, "failed to set gitlab commit status after all retries, please refer to the comment created on the MR",
			"scenario.name", report.ScenarioName, "statusCode", statusCode)
		return statusCode, err
	}
	return statusCode, nil

}

func (r *GitLabReporter) ReturnCodeIsUnrecoverable(statusCode int) bool {
	return statusCode == http.StatusForbidden || statusCode == http.StatusUnauthorized || statusCode == http.StatusBadRequest
}

// GenerateGitlabCommitState transforms internal integration test state into Gitlab state
func GenerateGitlabCommitState(state intgteststat.IntegrationTestStatus, optional bool) (gitlab.BuildStateValue, error) {
	glState := gitlab.Failed

	switch state {
	case intgteststat.IntegrationTestStatusPending, intgteststat.BuildPLRInProgress:
		glState = gitlab.Pending
	case intgteststat.IntegrationTestStatusInProgress:
		glState = gitlab.Running
	case intgteststat.IntegrationTestStatusEnvironmentProvisionError_Deprecated,
		intgteststat.IntegrationTestStatusDeploymentError_Deprecated,
		intgteststat.IntegrationTestStatusTestInvalid:
		if optional {
			glState = gitlab.Skipped
			break
		}
		glState = gitlab.Failed
	case intgteststat.IntegrationTestStatusDeleted,
		intgteststat.BuildPLRFailed, intgteststat.SnapshotCreationFailed, intgteststat.GroupSnapshotCreationFailed:
		glState = gitlab.Canceled
	case intgteststat.IntegrationTestStatusTestPassed:
		glState = gitlab.Success
	case intgteststat.IntegrationTestStatusTestFail:
		if optional {
			glState = gitlab.Skipped
			break
		}
		glState = gitlab.Failed
	default:
		return glState, fmt.Errorf("unknown status %s", state)
	}

	return glState, nil
}
