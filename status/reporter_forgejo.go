/*
Copyright 2026 Red Hat Inc.

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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v2"
	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/operator-toolkit/metadata"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	"github.com/konflux-ci/integration-service/pkg/common"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
)

type ForgejoReporter struct {
	logger      *logr.Logger
	k8sClient   client.Client
	client      *forgejo.Client
	sha         string
	owner       string
	repo        string
	pullRequest int
	snapshot    *applicationapiv1alpha1.Snapshot
}

func NewForgejoReporter(logger logr.Logger, k8sClient client.Client) *ForgejoReporter {
	return &ForgejoReporter{
		logger:    &logger,
		k8sClient: k8sClient,
	}
}

var ForgejoProvider = "ForgejoReporter"

// check if interface has been correctly implemented
var _ ReporterInterface = (*ForgejoReporter)(nil)

// Detect if snapshot has been created from forgejo or gitea provider (PaC uses gitea for Forgejo until PaC adds full Forgejo support).
func (r *ForgejoReporter) Detect(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return metadata.HasAnnotationWithValue(snapshot, gitops.PipelineAsCodeGitProviderLabel, gitops.PipelineAsCodeForgejoProviderType) ||
		metadata.HasLabelWithValue(snapshot, gitops.PipelineAsCodeGitProviderAnnotation, gitops.PipelineAsCodeForgejoProviderType) ||
		metadata.HasAnnotationWithValue(snapshot, gitops.PipelineAsCodeGitProviderLabel, gitops.PipelineAsCodeGiteaProviderType) ||
		metadata.HasLabelWithValue(snapshot, gitops.PipelineAsCodeGitProviderAnnotation, gitops.PipelineAsCodeGiteaProviderType)
}

// GetReporterName returns the reporter name
func (r *ForgejoReporter) GetReporterName() string {
	return ForgejoProvider
}

// Initialize initializes forgejo reporter
func (r *ForgejoReporter) Initialize(ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot) (int, error) {
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

	// Extract owner and repo from URL path
	// URL format: https://codeberg.org/owner/repo or https://forgejo.example.com/owner/repo
	pathParts := strings.Split(strings.TrimPrefix(burl.Path, "/"), "/")
	if len(pathParts) < 2 {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to extract owner/repo from URL path %s", burl.Path))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}
	r.owner = pathParts[0]
	r.repo = pathParts[1]

	// Construct API base URL (forgejo client automatically adds /api/v1 to all paths)
	apiURL := fmt.Sprintf("%s://%s", burl.Scheme, burl.Host)

	r.client, err = forgejo.NewClient(apiURL, forgejo.SetToken(token), forgejo.SetUserAgent(common.IntegrationServiceUserAgent))
	if err != nil {
		r.logger.Error(err, "failed to create forgejo client", "apiURL", apiURL, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
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

	pullRequestStr, found := annotations[gitops.PipelineAsCodePullRequestAnnotation]
	if !found && !gitops.IsSnapshotCreatedByPACPushEvent(snapshot) {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("pull-request annotation not found %q", gitops.PipelineAsCodePullRequestAnnotation))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}

	if found {
		r.pullRequest, err = strconv.Atoi(pullRequestStr)
		if err != nil && !gitops.IsSnapshotCreatedByPACPushEvent(snapshot) {
			unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert pull request number '%s' to integer: %s", pullRequestStr, err.Error()))
			r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
			return 0, unRecoverableError
		}
	}

	r.snapshot = snapshot
	return 0, nil
}

// IsPullRequestOpen returns whether the snapshot's pull request is still open.
// Used by status.IsPRMRInSnapshotOpened. For push snapshots (no PR) returns false.
func (r *ForgejoReporter) IsPullRequestOpen(ctx context.Context) (bool, int, error) {
	var statusCode int
	if r.client == nil {
		return false, 0, fmt.Errorf("forgejo reporter not initialized")
	}
	if r.pullRequest == 0 {
		return false, 0, nil
	}
	pr, resp, err := r.client.GetPullRequest(r.owner, r.repo, int64(r.pullRequest))
	if resp != nil {
		statusCode = resp.StatusCode
	}
	if err != nil && strings.Contains(err.Error(), "Not Found") {
		r.logger.Info("pull request not found, it may have been deleted",
			"owner", r.owner, "repo", r.repo, "pullRequest", r.pullRequest)
		return false, statusCode, nil
	}
	if err != nil {
		return false, statusCode, err
	}
	if pr == nil {
		return false, statusCode, nil
	}
	return pr.State == forgejo.StateOpen, statusCode, nil
}

// setCommitStatus sets commit status to be shown as pipeline run in forgejo view
func (r *ForgejoReporter) setCommitStatus(report TestReport) (int, error) {
	var statusCode = 0
	l := loader.NewLoader()
	scenario, err := l.GetScenario(context.Background(), r.k8sClient, report.ScenarioName, r.snapshot.Namespace)

	if err != nil {
		r.logger.Error(err, fmt.Sprintf("could not determine whether scenario %s was optional", report.ScenarioName))
		return statusCode, fmt.Errorf("could not determine whether scenario %s is optional: %w", report.ScenarioName, err)
	}
	optional := helpers.IsIntegrationTestScenarioOptional(scenario)
	fjState, err := GenerateForgejoCommitState(report.Status, optional)
	if err != nil {
		return statusCode, fmt.Errorf("failed to generate forgejo state: %w", err)
	}

	statusOpt := forgejo.CreateStatusOption{
		State:       forgejo.StatusState(fjState),
		TargetURL:   "",
		Description: report.Summary,
		Context:     report.FullName,
	}

	if report.TestPipelineRunName != "" {
		statusOpt.TargetURL = FormatPipelineURL(report.TestPipelineRunName, r.snapshot.Namespace, *r.logger)
	}

	// Fetch existing commit statuses only if necessary
	if fjState == "pending" {
		statuses, resp, err := r.client.ListStatuses(r.owner, r.repo, r.sha, forgejo.ListStatusesOption{})
		if resp != nil {
			statusCode = resp.StatusCode
		}
		if err != nil {
			return statusCode, fmt.Errorf("error while getting all commit statuses for sha %s: %w", r.sha, err)
		}
		existingStatus := r.GetExistingCommitStatus(statuses, report.FullName)

		// special case, we want to skip updating commit status if the status from pending to pending
		if existingStatus != nil && string(existingStatus.State) == fjState {
			r.logger.Info("Skipping commit status update",
				"scenario.name", report.ScenarioName,
				"current_status", existingStatus.State,
				"new_status", fjState)
			return statusCode, nil
		}
	}

	r.logger.Info("creating commit status for scenario test status of snapshot",
		"scenarioName", report.ScenarioName)

	_, resp, err := r.client.CreateStatus(r.owner, r.repo, r.sha, statusOpt)
	if resp != nil {
		statusCode = resp.StatusCode
	}
	if err != nil {
		return statusCode, fmt.Errorf("failed to set commit status to %s: %w", fjState, err)
	}

	r.logger.Info("Created forgejo commit status", "scenario.name", report.ScenarioName, "TargetURL", statusOpt.TargetURL)
	return statusCode, nil
}

// UpdateStatusInComment searches and updates existing comments or creates a new comment in the PR which creates snapshot
func (r *ForgejoReporter) UpdateStatusInComment(commentPrefix, comment string) (int, error) {
	var statusCode = 0

	// get all existing integration test comments according to commentPrefix
	// In Forgejo, PRs are issues, so we use the Issues service
	allComments, resp, err := r.client.ListIssueComments(r.owner, r.repo, int64(r.pullRequest), forgejo.ListIssueCommentOptions{})
	if resp != nil {
		statusCode = resp.StatusCode
	}
	if err != nil {
		r.logger.Error(err, "error while getting all comments for pull-request", "pullRequest", r.pullRequest, "report.SnapshotName", r.snapshot.Name)
		return statusCode, fmt.Errorf("error while getting all comments for pull-request %d: %w", r.pullRequest, err)
	}

	commentIDs := r.GetExistingCommentIDs(allComments, commentPrefix)

	if len(commentIDs) > 0 {
		// update the first existing comment but delete others because sometimes there might be multiple existing comments for the same component due to previous intermittent errors
		r.logger.Info("found multiple existing comments for the same component, updating the first one but delete others", "commentPrefix", commentPrefix, "count", len(allComments))
		commentIdToBeUpdated := commentIDs[0]

		if len(commentIDs) > 1 {
			r.logger.Info("deleting other existing comments since we will update the first one", "commentIDsToBeDeleted", commentIDs[1:])
			statusCode, err = r.DeleteExistingComments(commentIDs[1:])
			if err != nil {
				return statusCode, fmt.Errorf("error while deleting existing comments for pull-request %d: %w", r.pullRequest, err)
			}
		}

		// update the first existing comment
		editOpt := forgejo.EditIssueCommentOption{
			Body: comment,
		}
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			_, resp, err = r.client.EditIssueComment(r.owner, r.repo, commentIdToBeUpdated, editOpt)
			if resp != nil {
				statusCode = resp.StatusCode
			}
			return err
		})
		if err != nil {
			return statusCode, fmt.Errorf("error while updating comment %d for pull-request %d: %w", commentIdToBeUpdated, r.pullRequest, err)
		}
		r.logger.Info("updated existing comment with matching commentPrefix", "commentID", commentIdToBeUpdated, "commentPrefix", commentPrefix)
	} else {
		// create a new comment
		r.logger.Info("no existing comments found with matching commentPrefix, creating a new comment", "commentPrefix", commentPrefix)
		createOpt := forgejo.CreateIssueCommentOption{
			Body: comment,
		}
		_, resp, err = r.client.CreateIssueComment(r.owner, r.repo, int64(r.pullRequest), createOpt)
		if resp != nil {
			statusCode = resp.StatusCode
		}
		if err != nil {
			return statusCode, fmt.Errorf("error while creating comment for pull-request %d: %w", r.pullRequest, err)
		}
	}

	return statusCode, nil
}

// GetExistingCommitStatus returns existing Forgejo commit status that matches the status name
func (r *ForgejoReporter) GetExistingCommitStatus(statuses []*forgejo.Status, statusName string) *forgejo.Status {
	for _, status := range statuses {
		if status.Context == statusName {
			r.logger.Info("found matching existing commit status",
				"status.Context", status.Context)
			return status
		}
	}
	r.logger.Info("found no matching existing commit status", "statusName", statusName)
	return nil
}

// GetExistingCommentIDs returns IDs of comments that contain the commentPrefix
func (r *ForgejoReporter) GetExistingCommentIDs(comments []*forgejo.Comment, commentPrefix string) []int64 {
	var commentIDs []int64
	for _, comment := range comments {
		// get existing comment by searching commentPrefix in comment body
		if strings.Contains(comment.Body, commentPrefix) {
			r.logger.Info("found comment ID with a matching commentPrefix", "commentPrefix", commentPrefix, "commentID", comment.ID)
			commentIDs = append(commentIDs, comment.ID)
		}
	}

	if len(commentIDs) == 0 {
		r.logger.Info("found no comment with a matching commentPrefix", "commentPrefix", commentPrefix)
	}
	return commentIDs
}

// DeleteExistingComments deletes existing Forgejo comments with given commentIDs
func (r *ForgejoReporter) DeleteExistingComments(commentIDs []int64) (int, error) {
	var lastStatusCode = 0
	var errs []error // collect errors during deletion

	for _, commentID := range commentIDs {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			resp, err := r.client.DeleteIssueComment(r.owner, r.repo, commentID)
			if resp != nil {
				lastStatusCode = resp.StatusCode
			}
			return err
		})

		if err != nil {
			r.logger.Error(err, "failed to delete comment", "commentID", commentID)
			errs = append(errs, fmt.Errorf("commentID %d: %w", commentID, err))
			continue // continue to delete next comment
		}
		r.logger.Info("existing comment deleted", "commentID", commentID)
	}

	if len(errs) > 0 {
		return lastStatusCode, fmt.Errorf("errors occurred during deletion existing comments on pull request %d: %v", r.pullRequest, errs)
	}

	return lastStatusCode, nil
}

// ReportStatus reports test result to forgejo
func (r *ForgejoReporter) ReportStatus(ctx context.Context, report TestReport) (int, error) {
	var statusCode = 0
	if r.client == nil {
		return statusCode, fmt.Errorf("forgejo reporter is not initialized")
	}

	var err error
	err = retry.OnError(reporterRetryBackoff, func(err error) bool {
		// statusCode 0 means no HTTP response was received (network/timeout error), always retry
		retryable := statusCode == 0 || !r.ReturnCodeIsUnrecoverable(statusCode)
		if retryable {
			r.logger.Info("retrying to set forgejo commit status after transient error",
				"scenario.name", report.ScenarioName, "statusCode", statusCode, "error", err.Error())
		}
		return retryable
	}, func() error {
		statusCode = 0 // reset before each attempt to avoid stale values
		statusCode, err = r.setCommitStatus(report)
		return err
	})

	if err != nil {
		r.logger.Error(err, "failed to set forgejo commit status after all retries, please refer to the comment created on the PR",
			"scenario.name", report.ScenarioName, "statusCode", statusCode)
		return statusCode, err
	}
	return statusCode, nil
}

func (r *ForgejoReporter) ReturnCodeIsUnrecoverable(statusCode int) bool {
	return statusCode == http.StatusForbidden || statusCode == http.StatusUnauthorized || statusCode == http.StatusBadRequest
}

// GenerateForgejoCommitState transforms internal integration test state into Forgejo state
func GenerateForgejoCommitState(state intgteststat.IntegrationTestStatus, optional bool) (string, error) {
	fjState := "error"

	switch state {
	case intgteststat.IntegrationTestStatusPending, intgteststat.BuildPLRInProgress:
		fjState = "pending"
	case intgteststat.IntegrationTestStatusInProgress:
		fjState = "pending" // Forgejo uses "pending" for in-progress statuses
	case intgteststat.IntegrationTestStatusEnvironmentProvisionError_Deprecated,
		intgteststat.IntegrationTestStatusDeploymentError_Deprecated,
		intgteststat.IntegrationTestStatusTestInvalid:
		if optional {
			fjState = "success" // Forgejo doesn't have "skipped"; optional/skipped tests are not a failure
			break
		}
		fjState = "error"
	case intgteststat.IntegrationTestStatusDeleted,
		intgteststat.BuildPLRFailed, intgteststat.SnapshotCreationFailed, intgteststat.GroupSnapshotCreationFailed:
		fjState = "error"
	case intgteststat.IntegrationTestStatusTestPassed:
		fjState = "success"
	case intgteststat.IntegrationTestStatusTestFail:
		if optional {
			fjState = "warning" // optional ITS failed: non-blocking but visible
			break
		}
		fjState = "error"
	default:
		return fjState, fmt.Errorf("unknown status %s", state)
	}

	return fjState, nil
}
