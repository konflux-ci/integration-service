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
	"net/url"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/konflux-ci/operator-toolkit/metadata"
	gitlab "github.com/xanzy/go-gitlab"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	object          metav1.Object
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

// Detect if object has label/annotation for gitlab provider
func (r *GitLabReporter) Detect(obj metav1.Object) bool {
	return metadata.HasAnnotationWithValue(obj, gitops.PipelineAsCodeGitProviderLabel, gitops.PipelineAsCodeGitLabProviderType) ||
		metadata.HasLabelWithValue(obj, gitops.PipelineAsCodeGitProviderAnnotation, gitops.PipelineAsCodeGitLabProviderType) ||
		metadata.HasAnnotationWithValue(obj, gitops.BuildPipelineAsCodeGitProviderLabel, gitops.PipelineAsCodeGitLabProviderType) ||
		metadata.HasLabelWithValue(obj, gitops.BuildPipelineAsCodeGitProviderAnnotation, gitops.PipelineAsCodeGitLabProviderType)
}

// GetReporterName returns the reporter name
func (r *GitLabReporter) GetReporterName() string {
	return "GitlabReporter"
}

// Initialize initializes gitlab reporter for snapshot or build pipelinerun
func (r *GitLabReporter) Initialize(ctx context.Context, object metav1.Object) error {
	var unRecoverableError error
	objectKind := GetObjectKind(object)
	token, err := GetPACGitProviderToken(ctx, r.k8sClient, object)
	if err != nil {
		r.logger.Error(err, "failed to get PAC token from object", "object.Kind", objectKind,
			"object.NameSpace", object.GetNamespace(), "object.Name", object.GetName())
		return err
	}

	annotations := object.GetAnnotations()
	repoUrl, ok := GetPACAnnotation(object, annotations, gitops.RepoURLAnnotationSuffix)
	if !ok {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to get value of PipelineAsCodeRepoURLAnnotation annotation from the %s %s", objectKind, object.GetName()))
		r.logger.Error(unRecoverableError, "object.Kind", objectKind, "object.Namespace", object.GetNamespace(), "object.Name", object.GetName())
		return unRecoverableError
	}

	burl, err := url.Parse(repoUrl)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to parse repo-url %s: %s", repoUrl, err.Error()))
		r.logger.Error(unRecoverableError, "object.Kind", objectKind, "object.NameSpace", object.GetNamespace(), "object.Name", object.GetName())
		return unRecoverableError
	}
	apiURL := fmt.Sprintf("%s://%s", burl.Scheme, burl.Host)

	r.client, err = gitlab.NewClient(token, gitlab.WithBaseURL(apiURL))
	if err != nil {
		r.logger.Error(err, "failed to create gitlab client", "apiURL", apiURL, "object.Kind", objectKind, "object.NameSpace", object.GetNamespace(), "object.Name", object.GetName())
		return err
	}

	labels := object.GetLabels()
	sha, found := GetPACLabel(object, labels, gitops.SHALabelSuffix)
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("pipelines as code sha label not found %s", gitops.SHALabelSuffix))
		r.logger.Error(unRecoverableError, "object.Kind", objectKind, "object.NameSpace", object.GetNamespace(), "object.Name", object.GetName())
		return unRecoverableError
	}
	r.sha = sha

	targetProjectIDstr, found := GetPACAnnotation(object, annotations, gitops.TargetProjectIDAnnotationSuffix)
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("pipelines as code target project ID annotation not found %s", object.GetAnnotations()))
		r.logger.Error(unRecoverableError, "object.Kind", objectKind, "object.NameSpace", object.GetNamespace(), "object.Name", object.GetName())
		return unRecoverableError
	}

	r.targetProjectID, err = strconv.Atoi(targetProjectIDstr)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert project ID '%s' to integer: %s", targetProjectIDstr, err.Error()))
		r.logger.Error(unRecoverableError, "object.Kind", objectKind, "object.NameSpace", object.GetNamespace(), "object.Name", object.GetName())
		return unRecoverableError
	}

	sourceProjectIDstr, found := GetPACAnnotation(object, annotations, gitops.SourceProjectIDAnnotationSuffix)
	if !found {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("pipelines as code source project ID annotation not found %q", gitops.SourceProjectIDAnnotationSuffix))
		r.logger.Error(unRecoverableError, "object.Kind", objectKind, "object.NameSpace", object.GetNamespace(), "object.Name", object.GetName())
		return unRecoverableError
	}

	r.sourceProjectID, err = strconv.Atoi(sourceProjectIDstr)
	if err != nil {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert project ID '%s' to integer: %s", sourceProjectIDstr, err.Error()))
		r.logger.Error(unRecoverableError, "object.Kind", objectKind, "object.NameSpace", object.GetNamespace(), "object.Name", object.GetName())
		return unRecoverableError
	}

	mergeRequestStr, found := GetPACAnnotation(object, annotations, gitops.PullRequestAnnotationSuffix)
	if !found && !gitops.IsObjectCreatedByPACPushEvent(object) {
		unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("pipelines as code pull-request annotation not found %s", gitops.PullRequestAnnotationSuffix))
		r.logger.Error(unRecoverableError, "object.Kind", objectKind, "object.NameSpace", object.GetNamespace(), "object.Name", object.GetName())
		return unRecoverableError
	}

	if found {
		r.mergeRequest, err = strconv.Atoi(mergeRequestStr)
		if err != nil && !gitops.IsObjectCreatedByPACPushEvent(object) {
			unRecoverableError = helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to convert merge request number '%s' to integer: %s", mergeRequestStr, err.Error()))
			r.logger.Error(unRecoverableError, "object.Kind", objectKind, "object.NameSpace", object.GetNamespace(), "object.Name", object.GetName())
			return unRecoverableError
		}
	}

	r.object = object
	return nil
}

// setCommitStatus sets commit status to be shown as pipeline run in gitlab view
func (r *GitLabReporter) setCommitStatus(report TestReport) error {
	glState, err := GenerateGitlabCommitState(report.Status)
	if err != nil {
		return fmt.Errorf("failed to generate gitlab state: %w", err)
	}

	opt := gitlab.SetCommitStatusOptions{
		State:       gitlab.BuildStateValue(glState),
		Name:        gitlab.Ptr(report.FullName),
		Description: gitlab.Ptr(report.Summary),
	}

	if report.TestPipelineRunName == "" {
		r.logger.Info("TestPipelineRunName is not set, cannot add URL to message")
	} else {
		url := FormatPipelineURL(report.TestPipelineRunName, r.object.GetNamespace(), *r.logger)
		opt.TargetURL = gitlab.Ptr(url)
	}

	// Fetch commit statuses only if necessary
	if glState == gitlab.Running || glState == gitlab.Pending {
		allCommitStatuses, _, err := r.client.Commits.GetCommitStatuses(r.sourceProjectID, r.sha, nil)
		if err != nil {
			return fmt.Errorf("error while getting all commitStatuses for sha %s: %w", r.sha, err)
		}
		existingCommitStatus = r.GetExistingCommitStatus(allCommitStatuses, report.FullName)

		// special case, we want to skip updating commit status if the status from running to running, from enque to pending
		if existingCommitStatus != nil && (existingCommitStatus.Status == "enqueue" ||
			existingCommitStatus.Status == string(gitlab.Running)) {
			r.logger.Info("Skipping commit status update",
				"scenario.name", report.ScenarioName,
				"commitStatus.ID", existingCommitStatus.ID,
				"current_status", existingCommitStatus.Status,
				"new_status", glState)
			return nil
		}
	}

	r.logger.Info("creating commit status for scenario test status of object",
		"scenarioName", report.ScenarioName)

	commitStatus, _, err := r.client.Commits.SetCommitStatus(r.sourceProjectID, r.sha, &opt)
	if err != nil {
		return fmt.Errorf("failed to set commit status: %w", err)
	}

	r.logger.Info("Created gitlab commit status", "scenario.name", report.ScenarioName, "commitStatus.ID", commitStatus.ID, "TargetURL", opt.TargetURL)
	return nil
}

// updateStatusInComment will create/update a comment in the MR which creates snapshot or build pipelineRun
func (r *GitLabReporter) updateStatusInComment(report TestReport) error {
	comment, err := FormatComment(report.Summary, report.Text)
	if err != nil {
		unRecoverableError := helpers.NewUnrecoverableMetadataError(fmt.Sprintf("failed to generate comment for merge-request %d: %s", r.mergeRequest, err.Error()))
		r.logger.Error(unRecoverableError, "report.ObjectName", report.ObjectName)
		return unRecoverableError
	}

	allNotes, _, err := r.client.Notes.ListMergeRequestNotes(r.targetProjectID, r.mergeRequest, nil)
	if err != nil {
		r.logger.Error(err, "error while getting all comments for merge-request", "mergeRequest", r.mergeRequest, "report.ObjectName", report.ObjectName)
		return fmt.Errorf("error while getting all comments for merge-request %d: %w", r.mergeRequest, err)
	}
	existingCommentId := r.GetExistingNoteID(allNotes, report.ScenarioName, report.ObjectName)
	if existingCommentId == nil {
		noteOptions := gitlab.CreateMergeRequestNoteOptions{Body: &comment}
		_, _, err := r.client.Notes.CreateMergeRequestNote(r.targetProjectID, r.mergeRequest, &noteOptions)
		if err != nil {
			return fmt.Errorf("error while creating comment for merge-request %d: %w", r.mergeRequest, err)
		}
	} else {
		noteOptions := gitlab.UpdateMergeRequestNoteOptions{Body: &comment}
		_, _, err := r.client.Notes.UpdateMergeRequestNote(r.targetProjectID, r.mergeRequest, *existingCommentId, &noteOptions)
		if err != nil {
			return fmt.Errorf("error while creating comment for merge-request %d: %w", r.mergeRequest, err)
		}
	}

	return nil
}

// GetExistingCommitStatus returns existing GitLab commit status that matches .
func (r *GitLabReporter) GetExistingCommitStatus(commitStatuses []*gitlab.CommitStatus, statusName string) *gitlab.CommitStatus {
	for _, commitStatus := range commitStatuses {
		if commitStatus.Name == statusName {
			r.logger.Info("found matching existing commitStatus",
				"commitName", commitStatus.Name, "commitID", commitStatus.ID)
			return commitStatus
		}
	}
	r.logger.Info("found no matching existing commitStatus", "statusName", statusName)
	return nil
}

// GetExistingNoteID returns existing GitLab note for the scenario of ref.
func (r *GitLabReporter) GetExistingNoteID(notes []*gitlab.Note, scenarioName, objectName string) *int {
	for _, note := range notes {
		if strings.Contains(note.Body, objectName) && strings.Contains(note.Body, scenarioName) {
			r.logger.Info("found note ID with a matching scenarioName", "scenarioName", scenarioName, "noteID", &note.ID)
			return &note.ID
		}
	}
	r.logger.Info("found no note with a matching scenarioName", "scenarioName", scenarioName)
	return nil
}

// ReportStatus reports test result to gitlab
func (r *GitLabReporter) ReportStatus(ctx context.Context, report TestReport) error {
	if r.client == nil {
		return fmt.Errorf("gitlab reporter is not initialized")
	}

	// We only create/update commitStatus when source project and target project are
	// the same one due to the access limitation for forked repo
	// refer to the same issue in pipelines-as-code https://github.com/openshift-pipelines/pipelines-as-code/blob/2f78eb8fd04d149b266ba93f2bea706b4b026403/pkg/provider/gitlab/gitlab.go#L207
	if r.sourceProjectID == r.targetProjectID {
		if err := r.setCommitStatus(report); err != nil {
			return fmt.Errorf("failed to set gitlab commit status: %w", err)
		}
	} else {
		r.logger.Info("Won't create/update commitStatus due to the access limitation for forked repo", "r.sourceProjectID", r.sourceProjectID, "r.targetProjectID", r.targetProjectID)
	}

	// Create a note when integration test is neither pending nor inprogress since comment for pending/inprogress is less meaningful
	_, isMergeRequest := r.object.GetAnnotations()[gitops.PipelineAsCodePullRequestAnnotation]
	if report.Status != intgteststat.IntegrationTestStatusPending && report.Status != intgteststat.IntegrationTestStatusInProgress && report.Status != intgteststat.SnapshotCreationFailed && isMergeRequest {
		err := r.updateStatusInComment(report)
		if err != nil {
			return err
		}
	}

	return nil
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
		intgteststat.BuildPLRFailed, intgteststat.SnapshotCreationFailed:
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
