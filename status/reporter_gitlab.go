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

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/operator-toolkit/metadata"
	gitlab "github.com/xanzy/go-gitlab"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/redhat-appstudio/integration-service/gitops"
	intgteststat "github.com/redhat-appstudio/integration-service/pkg/integrationteststatus"
)

type GitLabReporter struct {
	logger    *logr.Logger
	k8sClient client.Client
	client    *gitlab.Client
	sha       string
	projectID int
}

func NewGitLabReporter(logger logr.Logger, k8sClient client.Client) *GitLabReporter {
	return &GitLabReporter{
		logger:    &logger,
		k8sClient: k8sClient,
	}
}

// check if interface has been correctly implemented
var _ ReporterInterface = (*GitLabReporter)(nil)

// Detect if snapshot has been created from gitlab provider
func (r *GitLabReporter) Detect(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return metadata.HasLabelWithValue(snapshot, gitops.PipelineAsCodeGitProviderLabel, gitops.PipelineAsCodeGitLabProviderType)
}

// GetReporterName returns the reporter name
func (r *GitLabReporter) GetReporterName() string {
	return "GitlabReporter"
}

// Initialize initializes gitlab reporter
func (r *GitLabReporter) Initialize(ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot) error {
	token, err := GetPACGitProviderToken(ctx, r.k8sClient, snapshot)
	if err != nil {
		r.logger.Error(err, "failed to get token from snapshot",
			"snapshot.NameSpace", snapshot.Namespace, "snapshot.Name", snapshot.Name)
		return fmt.Errorf("failed to get PAC token for gitlab provider: %w", err)
	}

	annotations := snapshot.GetAnnotations()
	repoUrl, ok := annotations[gitops.PipelineAsCodeRepoURLAnnotation]
	if !ok {
		return fmt.Errorf("failed to get value of %s annotation from the snapshot %s", gitops.PipelineAsCodeRepoURLAnnotation, snapshot.Name)
	}

	burl, err := url.Parse(repoUrl)
	if err != nil {
		return fmt.Errorf("failed to parse repo-url: %w", err)
	}
	apiURL := fmt.Sprintf("%s://%s", burl.Scheme, burl.Host)

	r.client, err = gitlab.NewClient(token, gitlab.WithBaseURL(apiURL))
	if err != nil {
		return fmt.Errorf("failed to create gitlab client: %w", err)
	}

	labels := snapshot.GetLabels()
	sha, found := labels[gitops.PipelineAsCodeSHALabel]
	if !found {
		return fmt.Errorf("sha label not found %q", gitops.PipelineAsCodeSHALabel)
	}
	r.sha = sha

	projectIDstr, found := annotations[gitops.PipelineAsCodeSourceProjectIDAnnotation]
	if !found {
		return fmt.Errorf("project ID label not found %q", gitops.PipelineAsCodeSourceProjectIDAnnotation)
	}

	r.projectID, err = strconv.Atoi(projectIDstr)
	if err != nil {
		return fmt.Errorf("failed to convert project ID '%s' to integer: %w", projectIDstr, err)
	}

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

	r.logger.Info("creating commit status for scenario test status of snapshot",
		"scenarioName", report.ScenarioName)

	commitStatus, _, err := r.client.Commits.SetCommitStatus(r.projectID, r.sha, &opt)
	if err != nil {
		return fmt.Errorf("failed to set commit status: %w", err)
	}

	r.logger.Info("Created gitlab commit status", "scenario.name", report.ScenarioName, "commitStatus.ID", commitStatus.ID)
	return nil
}

// ReportStatus reports test result to gitlab
func (r *GitLabReporter) ReportStatus(ctx context.Context, report TestReport) error {
	if r.client == nil {
		return fmt.Errorf("gitlab reporter is not initialized")
	}

	if err := r.setCommitStatus(report); err != nil {
		return fmt.Errorf("failed to set gitlab commit status: %w", err)
	}

	return nil
}

// GenerateGitlabCommitState transforms internal integration test state into Gitlab state
func GenerateGitlabCommitState(state intgteststat.IntegrationTestStatus) (gitlab.BuildStateValue, error) {
	glState := gitlab.Failed

	switch state {
	case intgteststat.IntegrationTestStatusPending:
		glState = gitlab.Pending
	case intgteststat.IntegrationTestStatusInProgress:
		glState = gitlab.Running
	case intgteststat.IntegrationTestStatusEnvironmentProvisionError_Deprecated,
		intgteststat.IntegrationTestStatusDeploymentError_Deprecated,
		intgteststat.IntegrationTestStatusTestInvalid:
		glState = gitlab.Failed
	case intgteststat.IntegrationTestStatusDeleted:
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
