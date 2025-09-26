/*
Copyright 2025 Red Hat Inc.

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

	"code.gitea.io/sdk/gitea"
	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/operator-toolkit/metadata"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ForgejoReporter struct {
	logger        *logr.Logger
	k8sClient     client.Client
	client        *gitea.Client
	sha           string
	owner         string
	repo          string
	pullRequest   int64
	snapshot      *applicationapiv1alpha1.Snapshot
	isPullRequest bool
}

func NewForgejoReporter(logger logr.Logger, k8sClient client.Client) *ForgejoReporter {
	return &ForgejoReporter{
		logger:    &logger,
		k8sClient: k8sClient,
	}
}

// check if interface has been correctly implemented
var _ ReporterInterface = (*ForgejoReporter)(nil)

// Detect if snapshot has been created from forgejo provider
func (r *ForgejoReporter) Detect(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return metadata.HasAnnotationWithValue(snapshot, gitops.PipelineAsCodeGitProviderAnnotation, gitops.PipelineAsCodeForgejoProviderType) ||
		metadata.HasLabelWithValue(snapshot, gitops.PipelineAsCodeGitProviderLabel, gitops.PipelineAsCodeForgejoProviderType)
}

// GetReporterName returns the reporter name
func (r *ForgejoReporter) GetReporterName() string {
	return "ForgejoReporter"
}

// Initialize initializes forgejo reporter
func (r *ForgejoReporter) Initialize(ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot) (int, error) {
	var unRecoverableError error

	// Validate all required metadata first before making any API calls
	annotations := snapshot.GetAnnotations()
	repoUrl, ok := annotations[gitops.PipelineAsCodeRepoURLAnnotation]
	if !ok {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to get value of %s annotation from the snapshot %s", gitops.PipelineAsCodeRepoURLAnnotation, snapshot.Name))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}

	labels := snapshot.GetLabels()
	sha, found := labels[gitops.PipelineAsCodeSHALabel]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("sha label not found %q", gitops.PipelineAsCodeSHALabel))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}
	r.sha = sha

	owner, found := labels[gitops.PipelineAsCodeURLOrgLabel]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("org label not found %q", gitops.PipelineAsCodeURLOrgLabel))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}
	r.owner = owner

	repo, found := labels[gitops.PipelineAsCodeURLRepositoryLabel]
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("repository label not found %q", gitops.PipelineAsCodeURLRepositoryLabel))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}
	r.repo = repo

	pullRequestStr, found := annotations[gitops.PipelineAsCodePullRequestAnnotation]
	if !found && !gitops.IsSnapshotCreatedByPACPushEvent(snapshot) {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("pull-request annotation not found %q", gitops.PipelineAsCodePullRequestAnnotation))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}

	if found {
		r.isPullRequest = true
		pullRequestInt, err := strconv.Atoi(pullRequestStr)
		if err != nil && !gitops.IsSnapshotCreatedByPACPushEvent(snapshot) {
			unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert pull request number '%s' to integer: %s", pullRequestStr, err.Error()))
			r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
			return 0, unRecoverableError
		}
		r.pullRequest = int64(pullRequestInt)
	}

	// Now get the token and create the client
	token, err := GetPACGitProviderToken(ctx, r.k8sClient, snapshot)
	if err != nil {
		r.logger.Error(err, "failed to get PAC token from snapshot",
			"snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, err
	}

	burl, err := url.Parse(repoUrl)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to parse repo-url %s: %s", repoUrl, err.Error()))
		r.logger.Error(unRecoverableError, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, unRecoverableError
	}
	apiURL := fmt.Sprintf("%s://%s", burl.Scheme, burl.Host)

	r.client, err = gitea.NewClient(apiURL, gitea.SetToken(token))
	if err != nil {
		r.logger.Error(err, "failed to create forgejo client", "apiURL", apiURL, "snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return 0, err
	}

	r.snapshot = snapshot
	return 0, nil
}

// setCommitStatus sets commit status to be shown as pipeline run in forgejo view
func (r *ForgejoReporter) setCommitStatus(report TestReport) (int, error) {
	var statusCode = 0
	forgejoState, err := GenerateForgejoCommitState(report.Status)
	if err != nil {
		return statusCode, fmt.Errorf("failed to generate forgejo state: %w", err)
	}

	opt := gitea.CreateStatusOption{
		State:       forgejoState,
		Context:     report.FullName,
		Description: report.Summary,
	}

	if report.TestPipelineRunName == "" {
		r.logger.Info("TestPipelineRunName is not set, cannot add URL to message")
	} else {
		url := FormatPipelineURL(report.TestPipelineRunName, r.snapshot.Namespace, *r.logger)
		opt.TargetURL = url
	}

	// Check for existing commit status to avoid duplicate pending/running states
	if forgejoState == gitea.StatusPending {
		allCommitStatuses, response, err := r.client.ListStatuses(r.owner, r.repo, r.sha, gitea.ListStatusesOption{})
		if err != nil {
			if response != nil {
				statusCode = response.StatusCode
			}
			return statusCode, fmt.Errorf("error while getting all commitStatuses for sha %s: %w", r.sha, err)
		}

		existingCommitStatus := r.GetExistingCommitStatus(allCommitStatuses, report.FullName)
		// Skip updating commit status if transitioning from pending to pending
		if existingCommitStatus != nil && string(existingCommitStatus.State) == string(forgejoState) {
			r.logger.Info("Skipping commit status update",
				"scenario.name", report.ScenarioName,
				"commitStatus.ID", existingCommitStatus.ID,
				"current_status", existingCommitStatus.State,
				"new_status", forgejoState)
			return statusCode, nil
		}
	}

	r.logger.Info("creating commit status for scenario test status of snapshot",
		"scenarioName", report.ScenarioName)

	commitStatus, response, err := r.client.CreateStatus(r.owner, r.repo, r.sha, opt)
	if response != nil {
		statusCode = response.StatusCode
	}
	if err != nil {
		return statusCode, fmt.Errorf("failed to set commit status to %s: %w", string(forgejoState), err)
	}

	r.logger.Info("Created forgejo commit status", "scenario.name", report.ScenarioName, "commitStatus.ID", commitStatus.ID, "TargetURL", opt.TargetURL)
	return statusCode, nil
}

// updateStatusInComment will create/update a comment in the PR which creates snapshot
func (r *ForgejoReporter) updateStatusInComment(report TestReport) (int, error) {
	var statusCode = 0
	if !r.isPullRequest {
		return statusCode, nil
	}

	comment, err := FormatComment(report.Summary, report.Text)
	if err != nil {
		unRecoverableError := helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to generate comment for pull-request %d: %s", r.pullRequest, err.Error()))
		r.logger.Error(unRecoverableError, "report.SnapshotName", report.SnapshotName)
		return statusCode, unRecoverableError
	}

	allComments, response, err := r.client.ListIssueComments(r.owner, r.repo, r.pullRequest, gitea.ListIssueCommentOptions{})
	if response != nil {
		statusCode = response.StatusCode
	}
	if err != nil {
		r.logger.Error(err, "error while getting all comments for pull-request", "pullRequest", r.pullRequest, "report.SnapshotName", report.SnapshotName)
		return statusCode, fmt.Errorf("error while getting all comments for pull-request %d: %w", r.pullRequest, err)
	}

	existingCommentId := r.GetExistingCommentID(allComments, report.ScenarioName, report.SnapshotName)
	if existingCommentId == nil {
		_, response, err := r.client.CreateIssueComment(r.owner, r.repo, r.pullRequest, gitea.CreateIssueCommentOption{Body: comment})
		if response != nil {
			statusCode = response.StatusCode
		}
		if err != nil {
			return statusCode, fmt.Errorf("error while creating comment for pull-request %d: %w", r.pullRequest, err)
		}
	} else {
		_, response, err := r.client.EditIssueComment(r.owner, r.repo, *existingCommentId, gitea.EditIssueCommentOption{Body: comment})
		if response != nil {
			statusCode = response.StatusCode
		}
		if err != nil {
			return statusCode, fmt.Errorf("error while updating comment for pull-request %d: %w", r.pullRequest, err)
		}
	}

	return statusCode, nil
}

// GetExistingCommitStatus returns existing Forgejo commit status that matches.
func (r *ForgejoReporter) GetExistingCommitStatus(commitStatuses []*gitea.Status, statusName string) *gitea.Status {
	for _, commitStatus := range commitStatuses {
		if commitStatus.Context == statusName {
			r.logger.Info("found matching existing commitStatus",
				"commitStatus.Context", commitStatus.Context, "commitStatus.ID", commitStatus.ID)
			return commitStatus
		}
	}
	r.logger.Info("found no matching existing commitStatus", "statusName", statusName)
	return nil
}

// GetExistingCommentID returns existing Forgejo comment for the scenario of ref.
func (r *ForgejoReporter) GetExistingCommentID(comments []*gitea.Comment, scenarioName, snapshotName string) *int64 {
	for _, comment := range comments {
		if strings.Contains(comment.Body, snapshotName) && strings.Contains(comment.Body, scenarioName) {
			r.logger.Info("found comment ID with a matching scenarioName", "scenarioName", scenarioName, "commentID", &comment.ID)
			return &comment.ID
		}
	}
	r.logger.Info("found no comment with a matching scenarioName", "scenarioName", scenarioName)
	return nil
}

// ReportStatus reports test result to forgejo
func (r *ForgejoReporter) ReportStatus(ctx context.Context, report TestReport) (int, error) {
	var statusCode = 0
	if r.client == nil {
		return statusCode, fmt.Errorf("forgejo reporter is not initialized")
	}

	// Set commit status for all scenarios
	var err error
	if statusCode, err = r.setCommitStatus(report); err != nil {
		return statusCode, fmt.Errorf("failed to set forgejo commit status: %w", err)
	}

	// Create a comment when integration test is neither pending nor inprogress since comment for pending/inprogress is less meaningful
	if report.Status != intgteststat.IntegrationTestStatusPending && report.Status != intgteststat.IntegrationTestStatusInProgress && report.Status != intgteststat.SnapshotCreationFailed && r.isPullRequest {
		statusCode, err = r.updateStatusInComment(report)
		if err != nil {
			return statusCode, err
		}
	}

	return statusCode, nil
}

func (r *ForgejoReporter) ReturnCodeIsUnrecoverable(statusCode int) bool {
	return statusCode == http.StatusForbidden || statusCode == http.StatusUnauthorized || statusCode == http.StatusBadRequest
}

// GenerateForgejoCommitState transforms internal integration test state into Forgejo state
func GenerateForgejoCommitState(state intgteststat.IntegrationTestStatus) (gitea.StatusState, error) {
	var forgejoState gitea.StatusState

	switch state {
	case intgteststat.IntegrationTestStatusPending, intgteststat.BuildPLRInProgress:
		forgejoState = gitea.StatusPending
	case intgteststat.IntegrationTestStatusInProgress:
		forgejoState = gitea.StatusPending // Forgejo doesn't have a "running" state, use pending
	case intgteststat.IntegrationTestStatusEnvironmentProvisionError_Deprecated,
		intgteststat.IntegrationTestStatusDeploymentError_Deprecated,
		intgteststat.IntegrationTestStatusTestInvalid:
		forgejoState = gitea.StatusError
	case intgteststat.IntegrationTestStatusDeleted,
		intgteststat.BuildPLRFailed, intgteststat.SnapshotCreationFailed, intgteststat.GroupSnapshotCreationFailed:
		forgejoState = gitea.StatusFailure
	case intgteststat.IntegrationTestStatusTestPassed:
		forgejoState = gitea.StatusSuccess
	case intgteststat.IntegrationTestStatusTestFail:
		forgejoState = gitea.StatusFailure
	default:
		return forgejoState, fmt.Errorf("unknown status %s", state)
	}

	return forgejoState, nil
}
