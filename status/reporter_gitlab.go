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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/operator-toolkit/metadata"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
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
	return "GitlabReporter"
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

	r.client, err = gitlab.NewClient(token, gitlab.WithBaseURL(apiURL))
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
	glState, err := GenerateGitlabCommitState(report.Status)
	if err != nil {
		return statusCode, fmt.Errorf("failed to generate gitlab state: %w", err)
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
			r.logger.Info("Skipping commit status update",
				"scenario.name", report.ScenarioName,
				"commitStatus.ID", existingCommitStatus.ID,
				"current_status", existingCommitStatus.Status,
				"new_status", glState)
			return statusCode, nil
		}
	}

	r.logger.Info("creating commit status for scenario test status of snapshot",
		"scenarioName", report.ScenarioName)

	commitStatus, response, err := r.client.Commits.SetCommitStatus(r.sourceProjectID, r.sha, &opt)
	if response != nil {
		statusCode = response.StatusCode
	}
	if err != nil {
		// when commitStatus is created in multiple thread occasionally, we can still see the transition error, so let's ignore it as a workaround
		if strings.Contains(err.Error(), "Cannot transition status via :enqueue from :pending") {
			r.logger.Info("Ingoring the error when transition from pending to pending when the commitStatus might be created/updated in multiple threads at the same time occasionally")
			return statusCode, nil
		}
		return statusCode, fmt.Errorf("failed to set commit status to %s: %w", string(glState), err)
	}

	r.logger.Info("Created gitlab commit status", "scenario.name", report.ScenarioName, "commitStatus.ID", commitStatus.ID, "TargetURL", opt.TargetURL)
	return statusCode, nil
}

// updateStatusInComment will create/update a comment in the MR which creates snapshot
func (r *GitLabReporter) updateStatusInComment(report TestReport) (int, error) {
	var statusCode = 0
	comment, err := FormatComment(report.Summary, report.Text)
	if err != nil {
		unRecoverableError := helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to generate comment for merge-request %d: %s", r.mergeRequest, err.Error()))
		r.logger.Error(unRecoverableError, "report.SnapshotName", report.SnapshotName)
		return statusCode, unRecoverableError
	}

	allNotes, response, err := r.client.Notes.ListMergeRequestNotes(r.targetProjectID, r.mergeRequest, nil)
	if response != nil {
		statusCode = response.StatusCode
	}
	if err != nil {
		r.logger.Error(err, "error while getting all comments for merge-request", "mergeRequest", r.mergeRequest, "report.SnapshotName", report.SnapshotName)
		return statusCode, fmt.Errorf("error while getting all comments for merge-request %d: %w", r.mergeRequest, err)
	}
	existingCommentId := r.GetExistingNoteID(allNotes, report.ScenarioName, report.SnapshotName)
	if existingCommentId == nil {
		noteOptions := gitlab.CreateMergeRequestNoteOptions{Body: &comment}
		_, response, err := r.client.Notes.CreateMergeRequestNote(r.targetProjectID, r.mergeRequest, &noteOptions)
		if response != nil {
			statusCode = response.StatusCode
		}
		if err != nil {
			return statusCode, fmt.Errorf("error while creating comment for merge-request %d: %w", r.mergeRequest, err)
		}
	} else {
		noteOptions := gitlab.UpdateMergeRequestNoteOptions{Body: &comment}
		_, response, err := r.client.Notes.UpdateMergeRequestNote(r.targetProjectID, r.mergeRequest, *existingCommentId, &noteOptions)
		if response != nil {
			statusCode = response.StatusCode
		}
		if err != nil {
			return statusCode, fmt.Errorf("error while creating comment for merge-request %d: %w", r.mergeRequest, err)
		}
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

// GetExistingNoteID returns existing GitLab note for the scenario of ref.
func (r *GitLabReporter) GetExistingNoteID(notes []*gitlab.Note, scenarioName, snapshotName string) *int {
	for _, note := range notes {
		if strings.Contains(note.Body, snapshotName) && strings.Contains(note.Body, scenarioName) {
			r.logger.Info("found note ID with a matching scenarioName", "scenarioName", scenarioName, "noteID", &note.ID)
			return &note.ID
		}
	}
	r.logger.Info("found no note with a matching scenarioName", "scenarioName", scenarioName)
	return nil
}

// ReportStatus reports test result to gitlab
func (r *GitLabReporter) ReportStatus(ctx context.Context, report TestReport) (int, error) {
	var statusCode = 0
	if r.client == nil {
		return statusCode, fmt.Errorf("gitlab reporter is not initialized")
	}

	// We only create/update commitStatus when source project and target project are
	// the same one due to the access limitation for forked repo
	// refer to the same issue in pipelines-as-code https://github.com/openshift-pipelines/pipelines-as-code/blob/2f78eb8fd04d149b266ba93f2bea706b4b026403/pkg/provider/gitlab/gitlab.go#L207
	if r.sourceProjectID == r.targetProjectID {
		if statusCode, err := r.setCommitStatus(report); err != nil {
			return statusCode, fmt.Errorf("failed to set gitlab commit status: %w", err)
		}
	} else {
		r.logger.Info("Won't create/update commitStatus due to the access limitation for forked repo", "r.sourceProjectID", r.sourceProjectID, "r.targetProjectID", r.targetProjectID)
	}

	// Create a note when integration test is neither pending nor inprogress since comment for pending/inprogress is less meaningful
	_, isMergeRequest := r.snapshot.GetAnnotations()[gitops.PipelineAsCodePullRequestAnnotation]
	if report.Status != intgteststat.IntegrationTestStatusPending && report.Status != intgteststat.IntegrationTestStatusInProgress && report.Status != intgteststat.SnapshotCreationFailed && isMergeRequest {
		statusCode, err := r.updateStatusInComment(report)
		if err != nil {
			return statusCode, err
		}
	}

	return statusCode, nil
}

func (r *GitLabReporter) ReturnCodeIsUnrecoverable(statusCode int) bool {
	return statusCode == http.StatusForbidden || statusCode == http.StatusUnauthorized
}

// GenerateGitlabCommitState transforms internal integration test state into Gitlab state
func GenerateGitlabCommitState(state intgteststat.IntegrationTestStatus) (gitlab.BuildStateValue, error) {
	glState := gitlab.Failed

	switch state {
	case intgteststat.IntegrationTestStatusPending, intgteststat.BuildPLRInProgress:
		glState = gitlab.Pending
	case intgteststat.IntegrationTestStatusInProgress:
		glState = gitlab.Running
	case intgteststat.IntegrationTestStatusEnvironmentProvisionError_Deprecated,
		intgteststat.IntegrationTestStatusDeploymentError_Deprecated,
		intgteststat.IntegrationTestStatusTestInvalid:
		glState = gitlab.Failed
	case intgteststat.IntegrationTestStatusDeleted,
		intgteststat.BuildPLRFailed, intgteststat.SnapshotCreationFailed, intgteststat.GroupSnapshotCreationFailed:
		glState = gitlab.Canceled
	case intgteststat.IntegrationTestStatusTestPassed:
		glState = gitlab.Success
	case intgteststat.IntegrationTestStatusTestFail:
		glState = gitlab.Failed
	default:
		return glState, fmt.Errorf("unknown status %s", state)
	}

	return glState, nil
}
