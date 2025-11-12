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

package gitops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/pkg/metrics"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	"github.com/konflux-ci/operator-toolkit/metadata"
	"github.com/santhosh-tekuri/jsonschema/v5"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// PipelinesAsCodePrefix contains the prefix applied to labels and annotations copied from Pipelines as Code resources.
	PipelinesAsCodePrefix = "pac.test.appstudio.openshift.io"

	// TestLabelPrefix contains the prefix applied to labels and annotations related to testing.
	TestLabelPrefix = "test.appstudio.openshift.io"

	// ReleaseLabelPrefix contains the prefix applied to to labels and annotations related to release process.
	ReleaseLabelPrefix = "release.appstudio.openshift.io"

	// AutoReleaseLabel contains the label that allows users to overwrite the release behaviour of Snapshots
	AutoReleaseLabel = ReleaseLabelPrefix + "/auto-release"

	// CustomLabelPrefix contains the prefix applied to custom user-defined labels and annotations.
	CustomLabelPrefix = "custom.appstudio.openshift.io"

	// SnapshotTypeLabel contains the type of the Snapshot.
	SnapshotTypeLabel = "test.appstudio.openshift.io/type"

	// SnapshotIntegrationTestRun contains name of test we want to trigger run
	SnapshotIntegrationTestRun = "test.appstudio.openshift.io/run"

	// AppstudioLabelPrefix contains application, component, build-pipelinerun etc.
	AppstudioLabelPrefix = "appstudio.openshift.io"

	// SnapshotLabel contains the name of the Snapshot within appstudio
	SnapshotLabel = "appstudio.openshift.io/snapshot"

	// SnapshotTestScenarioLabel contains the name of the Snapshot test scenario.
	SnapshotTestScenarioLabel = "test.appstudio.openshift.io/scenario"

	// SnapshotTestScenarioLabel contains json data with test results of the particular snapshot
	SnapshotTestsStatusAnnotation = "test.appstudio.openshift.io/status"

	// (Deprecated) SnapshotPRLastUpdate contains timestamp of last time PR was updated
	SnapshotPRLastUpdate = "test.appstudio.openshift.io/pr-last-update"

	// SnapshotGitSourceRepoURLAnnotation contains URL of the git source repository (usually needed for forks)
	SnapshotGitSourceRepoURLAnnotation = "test.appstudio.openshift.io/source-repo-url"

	// PipelineAsCodeGitSourceURLAnnotation contains the source repository url information
	PipelineAsCodeGitSourceURLAnnotation = PipelinesAsCodePrefix + "/source-repo-url"

	// PipelineAsCodeSourceBranchAnnotation contains the source repository branch information
	PipelineAsCodeSourceBranchAnnotation = PipelinesAsCodePrefix + "/source-branch"

	// SnapshotStatusReportAnnotation contains metadata of tests related to status reporting to git provider
	SnapshotStatusReportAnnotation = "test.appstudio.openshift.io/git-reporter-status"

	// PRGroupAnnotation contains the pr group name
	PRGroupAnnotation = "test.appstudio.openshift.io/pr-group"

	// PRGroupHashLabel contains the pr group name in sha format
	PRGroupHashLabel = "test.appstudio.openshift.io/pr-group-sha"

	// PRGroupCreationAnnotation contains the info of groupsnapshot creation
	PRGroupCreationAnnotation = "test.appstudio.openshift.io/create-groupsnapshot-status"

	// GitReportingFailureAnnotation contains information about git reporting failures
	GitReportingFailureAnnotation = "test.appstudio.openshift.io/git-reporting-failure"

	// BuildPipelineRunStartTime contains the start time of build pipelineRun
	BuildPipelineRunStartTime = "test.appstudio.openshift.io/pipelinerunstarttime"

	// BuildPipelineLastBuiltTime contains the time of the last built pipelineRun
	BuildPipelineLastBuiltTime = "test.appstudio.openshift.io/lastbuilttime"

	// IntegrationWorkflowAnnotation contains the workflow type that triggered the snapshot (push or pull-request)
	IntegrationWorkflowAnnotation = "test.appstudio.openshift.io/integration-workflow"

	// BuildPipelineRunPrefix contains the build pipeline run related labels and annotations
	BuildPipelineRunPrefix = "build.appstudio"

	// BuildPipelineRunFinishTimeLabel contains the build PipelineRun finish time of the Snapshot.
	BuildPipelineRunFinishTimeLabel = "test.appstudio.openshift.io/pipelinerunfinishtime"

	// GroupSnapshotInfoAnnotation contains the component snapshot info included in group snapshot
	GroupSnapshotInfoAnnotation = "test.appstudio.openshift.io/group-test-info"

	// BuildPipelineRunNameLabel contains the build PipelineRun name
	BuildPipelineRunNameLabel = AppstudioLabelPrefix + "/build-pipelinerun"

	// ApplicationNameLabel contains the name of the application
	ApplicationNameLabel = AppstudioLabelPrefix + "/application"

	// SnapshotComponentType is the type of Snapshot which was created for a single component build.
	SnapshotComponentType = "component"

	// SnapshotOverrideType is the type of Snapshot which was created for override Global Candidate List.
	SnapshotOverrideType = "override"

	// SnapshotGroupType is the type of Snapshot which was created for pull request groups.
	SnapshotGroupType = "group"

	// PipelineAsCodeEventTypeLabel is the type of event which triggered the pipelinerun in build service
	PipelineAsCodeEventTypeLabel = PipelinesAsCodePrefix + "/event-type"

	// PipelineAsCodeGitProviderLabel is the git provider which triggered the pipelinerun in build service.
	PipelineAsCodeGitProviderLabel = PipelinesAsCodePrefix + "/git-provider"

	// PipelineAsCodeGitProviderAnnotation is the git provider which triggered the pipelinerun in build service.
	PipelineAsCodeGitProviderAnnotation = PipelinesAsCodePrefix + "/git-provider"

	// PipelineAsCodeSHALabel is the commit which triggered the pipelinerun in build service.
	PipelineAsCodeSHALabel = PipelinesAsCodePrefix + "/sha"

	// PipelineAsCodeURLOrgLabel is the organization for the git repo which triggered the pipelinerun in build service.
	PipelineAsCodeURLOrgLabel = PipelinesAsCodePrefix + "/url-org"

	// PipelineAsCodeURLRepositoryLabel is the git repository which triggered the pipelinerun in build service.
	PipelineAsCodeURLRepositoryLabel = PipelinesAsCodePrefix + "/url-repository"

	// PipelineAsCodeRepoURLAnnotation is the URL to the git repository which triggered the pipelinerun in build service.
	PipelineAsCodeRepoURLAnnotation = PipelinesAsCodePrefix + "/repo-url"

	// PipelineAsCodeTargetBranchAnnotation is the SHA of the git revision which triggered the pipelinerun in build service.
	PipelineAsCodeTargetBranchAnnotation = PipelinesAsCodePrefix + "/branch"

	// PipelineAsCodeInstallationIDAnnotation is the GitHub App installation ID for the git repo which triggered the pipelinerun in build service.
	PipelineAsCodeInstallationIDAnnotation = PipelinesAsCodePrefix + "/installation-id"

	// PipelineAsCodePullRequestAnnotation is the git repository's pull request identifier
	PipelineAsCodePullRequestAnnotation = PipelinesAsCodePrefix + "/pull-request"

	// PipelineAsCodeSourceProjectIDAnnotation is the source project ID for gitlab
	PipelineAsCodeSourceProjectIDAnnotation = PipelinesAsCodePrefix + "/source-project-id"

	// PipelineAsCodeTargetProjectIDAnnotation is the target project ID for gitlab
	PipelineAsCodeTargetProjectIDAnnotation = PipelinesAsCodePrefix + "/target-project-id"

	// PipelineAsCodeRepoUrlAnnotation is the target project Repo Url
	PipelineAsCodeRepoUrlAnnotation = PipelinesAsCodePrefix + "/repo-url"

	// PipelineAsCodeSHAAnnotation is the commit which triggered the pipelinerun in build service.
	PipelineAsCodeSHAAnnotation = PipelinesAsCodePrefix + "/sha"

	// PipelineAsCodePushType is the type of push event which triggered the pipelinerun in build service
	PipelineAsCodePushType = "push"

	// PipelineAsCodeGLPushType is the type of gitlab push event which triggered the pipelinerun in build service
	PipelineAsCodeGLPushType = "Push"

	// PipelineAsCodePullRequestType is the type of pull_request event which triggered the pipelinerun in build service
	PipelineAsCodePullRequestType = "pull_request"

	// PipelineAsCodeMergeRequestType is the type of merge request event which triggered the pipelinerun in build service
	PipelineAsCodeMergeRequestType = "merge request"

	// IntegrationWorkflowPushValue is the value for push workflow snapshots
	IntegrationWorkflowPushValue = "push"

	// IntegrationWorkflowPullRequestValue is the value for pull request workflow snapshots
	IntegrationWorkflowPullRequestValue = "pull-request"

	// PipelineAsCodeGitHubProviderType is the git provider type for a GitHub event which triggered the pipelinerun in build service.
	PipelineAsCodeGitHubProviderType = "github"

	// PipelineAsCodeGitLabProviderType is the git provider type for a GitLab event which triggered the pipelinerun in build service.
	PipelineAsCodeGitLabProviderType = "gitlab"

	// PipelineAsCodeGitHubMergeQueueBranchPrefix is the prefix added to temporary branches which are created for merge queues
	PipelineAsCodeGitHubMergeQueueBranchPrefix = "gh-readonly-queue/"

	//AppStudioTestSucceededCondition is the condition for marking if the AppStudio Tests succeeded for the Snapshot.
	AppStudioTestSucceededCondition = "AppStudioTestSucceeded"

	//LegacyTestSucceededCondition is the condition for marking if the AppStudio Tests succeeded for the Snapshot.
	LegacyTestSucceededCondition = "HACBSStudioTestSucceeded"

	// AppStudioIntegrationStatusCondition is the condition for marking the AppStudio integration status of the Snapshot.
	AppStudioIntegrationStatusCondition = "AppStudioIntegrationStatus"

	// LegacyIntegrationStatusCondition is the condition for marking the AppStudio integration status of the Snapshot.
	LegacyIntegrationStatusCondition = "HACBSIntegrationStatus"

	// SnapshotAutoReleasedCondition is the condition for marking if Snapshot was auto-released released with AppStudio.
	SnapshotAutoReleasedCondition = "AutoReleased"

	// SnapshotAddedToGlobalCandidateListCondition is the condition for marking if Snapshot's component was added to
	// the global candidate list.
	// Checking statusCondition has been replaced by checking annotation test.appstudio.openshift.io/added-to-global-candidate-list now but this statusCondition is still kept
	SnapshotAddedToGlobalCandidateListCondition = "AddedToGlobalCandidateList"

	// AppStudioTestSucceededConditionSatisfied is the reason that's set when the AppStudio tests succeed.
	AppStudioTestSucceededConditionSatisfied = "Passed"

	// AppStudioTestSucceededConditionFailed is the reason that's set when the AppStudio tests fail.
	AppStudioTestSucceededConditionFailed = "Failed"

	// AppStudioIntegrationStatusCanceled is the reason that's set when the AppStudio tests cancel because of being superseded by newer Snapshot.
	AppStudioIntegrationStatusCanceled = "Canceled"

	// AppStudioIntegrationStatusInvalid is the reason that's set when the AppStudio integration gets into an invalid state.
	AppStudioIntegrationStatusInvalid = "Invalid"

	// AppStudioIntegrationStatusErrorOccured is the reason that's set when the AppStudio integration gets into an error state.
	AppStudioIntegrationStatusErrorOccured = "ErrorOccured"

	//AppStudioIntegrationStatusInProgress is the reason that's set when the AppStudio tests gets into an in progress state.
	AppStudioIntegrationStatusInProgress = "InProgress"

	//AppStudioIntegrationStatusFinished is the reason that's set when the AppStudio tests finish.
	AppStudioIntegrationStatusFinished = "Finished"

	//AppStudioIntegrationStatusCancelled is the reason that's set when the AppStudio tests pipelinerun gets cancelled.
	AppStudioIntegrationStatusCancelled = "CancelledRunFinally"

	// This annotation helps track PipelineRuns that were stopped before completion, enabling better auditability and observability.
	PRGroupCancelledAnnotation = PipelinesAsCodePrefix + "/cancelled"

	// the statuses needed to report to GiHub when creating check run or commit status, see doc
	// https://docs.github.com/en/rest/guides/using-the-rest-api-to-interact-with-checks?apiVersion=2022-11-28
	// https://docs.github.com/en/free-pro-team@latest/rest/checks/runs?apiVersion=2022-11-28#create-a-check-run
	//IntegrationTestStatusPendingGithub is the status reported to github when integration test is in a queue
	IntegrationTestStatusPendingGithub = "pending"

	//IntegrationTestStatusSuccessGithub is the status reported to github when integration test succeed
	IntegrationTestStatusSuccessGithub = "success"

	//IntegrationTestStatusFailureGithub is the status reported to github when integration test fail
	IntegrationTestStatusFailureGithub = "failure"

	//IntegrationTestStatusErrorGithub is the status reported to github when integration test experience error
	IntegrationTestStatusErrorGithub = "error"

	//IntegrationTestStatusInProgressGithub is the status reported to github when integration test is in progress
	IntegrationTestStatusInProgressGithub = "in_progress"

	//IntegrationTestStatusCancelledGithub is the status reported to github when integration test is cancelled
	IntegrationTestStatusCancelledGithub = "cancelled"

	//IntegrationTestStatusNeutralGithub is the status reported to github when integration test is neutral
	IntegrationTestStatusNeutralGithub = "neutral"

	ComponentNameForGroupSnapshot = "pr group"

	FailedToCreateGroupSnapshotMsg = "Failed to create group snapshot for pr group"

	GroupSnapshotCreationFailureReported = "group snapshot creation failure is reported to git provider"

	Success = "Success"
	// AddedToGlobalCandidateListAnnotation is the annotation for marking if Snapshot/build PLR's component was added to
	// the global candidate list.
	AddedToGlobalCandidateListAnnotation = "test.appstudio.openshift.io/added-to-global-candidate-list"
)

var (
	// SnapshotComponentLabel contains the name of the updated Snapshot component - it should match the pipeline label.
	SnapshotComponentLabel = tektonconsts.ComponentNameLabel
)

// ComponentSnapshotInfo contains data about the component snapshots' info in group snapshot
type ComponentSnapshotInfo struct {
	// Namespace
	Namespace string `json:"namespace"`
	// Component name
	Component string `json:"component"`
	// The build PLR name building the container image triggered by pull request
	BuildPipelineRun string `json:"buildPipelineRun"`
	// The built component snapshot from build PLR
	Snapshot string `json:"snapshot"`
	// The repo url for each component
	RepoUrl string `json:"repoUrl"`
	// Pull/Merge request number for updated component
	PullRequestNumber string `json:"pullRequestNumber"`
}

// AddedToGlobalCandidateListStatus contains the information which will be added to build PLR or override snapshot about updating GCL
type AddedToGlobalCandidateListStatus struct {
	// Result for AddedToGlobalCandidateList
	Result bool `json:"result"`
	// Reason for AddedToGlobalCandidateList result
	Reason string `json:"reason"`
	// LastUpdatedTime for AddedToGlobalCandidateList
	LastUpdatedTime string `json:"lastupdatedtime"`
}

const componentSnapshotInfosSchema = `{
	"$schema": "http://json-schema.org/draft/2020-12/schema#",
	"type":  "array",
	"items": {
	  "type": "object",
      "properties": {
        "namespace": {
          "type": "string"
        },
        "component": {
          "type": "string"
        },
        "buildPipelineRun": {
          "type": "string"
        },
        "snapshot": {
          "type": "string"
        },
        "repoUrl": {
          "type": "string"
        },
        "pullRequestNumber": {
          "type": "string"
        }
      },
	  "required": ["namespace", "component", "buildPipelineRun", "snapshot"]
	}
  }`

// IsSnapshotMarkedAsPassed returns true if snapshot is marked as passed
func IsSnapshotMarkedAsPassed(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, AppStudioTestSucceededCondition, metav1.ConditionTrue, "")
}

// MarkSnapshotAsPassed updates the AppStudio Test succeeded condition for the Snapshot to passed.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsPassed(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, message string) error {
	patch := client.MergeFrom(snapshot.DeepCopy())
	condition := metav1.Condition{
		Type:    AppStudioTestSucceededCondition,
		Status:  metav1.ConditionTrue,
		Reason:  AppStudioTestSucceededConditionSatisfied,
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)

	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return err
	}

	snapshotCompletionTime := &metav1.Time{Time: time.Now()}
	go metrics.RegisterCompletedSnapshot(condition.Type, condition.Reason, snapshot.GetCreationTimestamp(), snapshotCompletionTime)
	return nil
}

// IsSnapshotMarkedAsFailed returns true if snapshot is marked as failed
func IsSnapshotMarkedAsFailed(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, AppStudioTestSucceededCondition, metav1.ConditionFalse, "")
}

// MarkSnapshotAsFailed updates the AppStudio Test succeeded condition for the Snapshot to failed.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsFailed(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, message string) error {
	patch := client.MergeFrom(snapshot.DeepCopy())
	condition := metav1.Condition{
		Type:    AppStudioTestSucceededCondition,
		Status:  metav1.ConditionFalse,
		Reason:  AppStudioTestSucceededConditionFailed,
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)

	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return err
	}

	snapshotCompletionTime := &metav1.Time{Time: time.Now()}
	go metrics.RegisterCompletedSnapshot(condition.Type, condition.Reason, snapshot.GetCreationTimestamp(), snapshotCompletionTime)
	return nil
}

// MarkSnapshotAsCanceled updates the AppStudio Test canceled condition for the Snapshot to 'Canceled'.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsCanceled(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, message string) error {
	patch := client.MergeFrom(snapshot.DeepCopy())
	condition := metav1.Condition{
		Type:    AppStudioIntegrationStatusCondition,
		Status:  metav1.ConditionTrue,
		Reason:  AppStudioIntegrationStatusCanceled,
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)

	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return err
	}

	snapshotCompletionTime := &metav1.Time{Time: time.Now()}
	go metrics.RegisterCompletedSnapshot(condition.Type, condition.Reason, snapshot.GetCreationTimestamp(), snapshotCompletionTime)
	return nil
}

// IsSnapshotMarkedAsCanceled returns true if snapshot is marked as AppStudioIntegrationStatusCanceled
func IsSnapshotMarkedAsCanceled(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, AppStudioIntegrationStatusCondition, metav1.ConditionTrue, AppStudioIntegrationStatusCanceled)
}

// MarkSnapshotAsInvalid updates the AppStudio integration status condition for the Snapshot to invalid.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsInvalid(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, message string) error {
	patch := client.MergeFrom(snapshot.DeepCopy())
	SetSnapshotIntegrationStatusAsInvalid(snapshot, message)
	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return err
	}

	return nil
}

// IsSnapshotMarkedAsInvalid returns true if snapshot is marked as failed
func IsSnapshotMarkedAsInvalid(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, AppStudioIntegrationStatusCondition, metav1.ConditionFalse, AppStudioIntegrationStatusInvalid)
}

// SetSnapshotIntegrationStatusAsInvalid sets the AppStudio integration status condition for the Snapshot to invalid.
func SetSnapshotIntegrationStatusAsInvalid(snapshot *applicationapiv1alpha1.Snapshot, message string) {
	condition := metav1.Condition{
		Type:    AppStudioIntegrationStatusCondition,
		Status:  metav1.ConditionFalse,
		Reason:  AppStudioIntegrationStatusInvalid,
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)
	go metrics.RegisterInvalidSnapshot(AppStudioIntegrationStatusCondition, AppStudioIntegrationStatusInvalid)
}

// SetSnapshotIntegrationStatusAsError sets the AppStudio integration status condition for the Snapshot to error.
func SetSnapshotIntegrationStatusAsError(snapshot *applicationapiv1alpha1.Snapshot, message string) {
	condition := metav1.Condition{
		Type:    AppStudioIntegrationStatusCondition,
		Status:  metav1.ConditionFalse,
		Reason:  AppStudioIntegrationStatusErrorOccured,
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)
}

// MarkSnapshotIntegrationStatusAsInProgress sets the AppStudio integration status condition for the Snapshot to In Progress.
func MarkSnapshotIntegrationStatusAsInProgress(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, message string) error {
	log := log.FromContext(ctx)
	patch := client.MergeFrom(snapshot.DeepCopy())
	meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
		Type:    AppStudioIntegrationStatusCondition,
		Status:  metav1.ConditionUnknown,
		Reason:  AppStudioIntegrationStatusInProgress,
		Message: message,
	})
	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return err
	}

	snapshotInProgressTime := &metav1.Time{Time: time.Now()}
	if metadata.HasLabel(snapshot, BuildPipelineRunFinishTimeLabel) {
		buildPipelineRunFinishTimeStr := snapshot.Labels[BuildPipelineRunFinishTimeLabel]
		buildPipelineRunFinishTimeInt, _ := strconv.ParseInt(buildPipelineRunFinishTimeStr, 10, 64)
		buildPipelineRunFinishTime := time.Unix(buildPipelineRunFinishTimeInt, 0)
		buildPipelineRunFinishTimeMeta := &metav1.Time{Time: buildPipelineRunFinishTime}

		duration := snapshotInProgressTime.Sub(buildPipelineRunFinishTimeMeta.Time)
		log.Info("Integration Service Response time (integration_svc_response_seconds)",
			"snapshot.name", snapshot.Name,
			"pipelinerun.name", snapshot.Labels[BuildPipelineRunNameLabel],
			"duration", duration,
		)
		go metrics.RegisterIntegrationResponse(duration)
	}
	return nil
}

// PrepareToRegisterIntegrationPipelineRunStarted is to do preparation before calling RegisterPipelineRunStarted
// Don't use this function for PLR re-runs
func PrepareToRegisterIntegrationPipelineRunStarted(snapshot *applicationapiv1alpha1.Snapshot) {
	pipelineRunStartTime := &metav1.Time{Time: time.Now()}
	go metrics.RegisterPipelineRunStarted(snapshot.GetCreationTimestamp(), pipelineRunStartTime)
}

// MarkSnapshotIntegrationStatusAsFinished sets the AppStudio integration status condition for the Snapshot to Finished.
func MarkSnapshotIntegrationStatusAsFinished(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, message string) error {
	patch := client.MergeFrom(snapshot.DeepCopy())
	condition := metav1.Condition{
		Type:    AppStudioIntegrationStatusCondition,
		Status:  metav1.ConditionTrue,
		Reason:  AppStudioIntegrationStatusFinished,
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)

	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return err
	}

	return nil
}

// IsSnapshotNotStarted checks if the AppStudio Integration Status condition is not in progress status.
func IsSnapshotNotStarted(snapshot *applicationapiv1alpha1.Snapshot) bool {
	condition := meta.FindStatusCondition(snapshot.Status.Conditions, AppStudioIntegrationStatusCondition)
	if condition == nil {
		condition = meta.FindStatusCondition(snapshot.Status.Conditions, LegacyIntegrationStatusCondition)
	}
	if condition == nil || condition.Reason != AppStudioIntegrationStatusInProgress {
		return true
	}
	return false
}

// IsSnapshotError if the AppStudio Integration Status condition is in ErrorOcurred status.
func IsSnapshotError(snapshot *applicationapiv1alpha1.Snapshot) bool {
	condition := meta.FindStatusCondition(snapshot.Status.Conditions, AppStudioIntegrationStatusCondition)
	if condition == nil {
		condition = meta.FindStatusCondition(snapshot.Status.Conditions, LegacyIntegrationStatusCondition)
	}
	if condition.Reason == AppStudioIntegrationStatusErrorOccured {
		return true
	}
	return false
}

// IsSnapshotValid checks if the AppStudio Integration Status condition is not invalid.
func IsSnapshotValid(snapshot *applicationapiv1alpha1.Snapshot) bool {
	condition := meta.FindStatusCondition(snapshot.Status.Conditions, AppStudioIntegrationStatusCondition)
	if condition == nil {
		condition = meta.FindStatusCondition(snapshot.Status.Conditions, LegacyIntegrationStatusCondition)
	}
	if condition == nil || condition.Reason != AppStudioIntegrationStatusInvalid {
		return true
	}
	return false
}

// IsSnapshotIntegrationStatusMarkedAsFinished returns true if snapshot is marked as finished or canceled
func IsSnapshotIntegrationStatusMarkedAsFinished(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, AppStudioIntegrationStatusCondition, metav1.ConditionTrue, AppStudioIntegrationStatusFinished) || IsSnapshotStatusConditionSet(snapshot, AppStudioIntegrationStatusCondition, metav1.ConditionTrue, AppStudioIntegrationStatusCanceled)
}

// IsSnapshotStatusConditionSet checks if the condition with the conditionType in the status of Snapshot has been marked as the conditionStatus and reason.
func IsSnapshotStatusConditionSet(snapshot *applicationapiv1alpha1.Snapshot, conditionType string, conditionStatus metav1.ConditionStatus, reason string) bool {
	condition := meta.FindStatusCondition(snapshot.Status.Conditions, conditionType)
	if condition == nil && conditionType == AppStudioTestSucceededCondition {
		condition = meta.FindStatusCondition(snapshot.Status.Conditions, LegacyTestSucceededCondition)
	}
	if condition == nil && conditionType == AppStudioIntegrationStatusCondition {
		condition = meta.FindStatusCondition(snapshot.Status.Conditions, LegacyIntegrationStatusCondition)
	}
	if condition == nil || condition.Status != conditionStatus {
		return false
	}
	if reason != "" && reason != condition.Reason {
		return false
	}
	return true
}

// IsSnapshotMarkedAsAutoReleased returns true if snapshot is marked as deployed to root environments
func IsSnapshotMarkedAsAutoReleased(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, SnapshotAutoReleasedCondition, metav1.ConditionTrue, "")
}

// MarkSnapshotAsAutoReleased updates the SnapshotAutoReleasedCondition for the Snapshot to 'AutoReleased'.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsAutoReleased(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, message string) error {
	patch := client.MergeFrom(snapshot.DeepCopy())
	condition := metav1.Condition{
		Type:    SnapshotAutoReleasedCondition,
		Status:  metav1.ConditionTrue,
		Reason:  "AutoReleased",
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)

	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return err
	}

	return nil
}

// IsSnapshotMarkedAsAddedToGlobalCandidateList returns true if snapshot's AddedToGlobalCandidateListAnnotation result is marked as true to global candidate list
func IsSnapshotMarkedAsAddedToGlobalCandidateList(snapshot *applicationapiv1alpha1.Snapshot) bool {
	annotationValue, ok := snapshot.GetAnnotations()[AddedToGlobalCandidateListAnnotation]
	if !ok || annotationValue == "" {
		return false
	}

	var addedToGlobalCandidateListStatus AddedToGlobalCandidateListStatus
	if err := json.Unmarshal([]byte(annotationValue), &addedToGlobalCandidateListStatus); err != nil {
		return false
	}
	return addedToGlobalCandidateListStatus.Result
}

// IsSnapshotMarkedAsAddedToGlobalCandidateList_Legacy is old way to track GCL update status by checking statusCondition and returns true if snapshot's component is marked as added to global candidate list
func IsSnapshotMarkedAsAddedToGlobalCandidateList_Legacy(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, SnapshotAddedToGlobalCandidateListCondition, metav1.ConditionTrue, "")
}

// MarkSnapshotAsAddedToGlobalCandidateList updates the AddedToGlobalCandidateListAnnotation for the Snapshot.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsAddedToGlobalCandidateList(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, message string) error {
	return AnnotateSnapshot(ctx, snapshot, AddedToGlobalCandidateListAnnotation, message, adapterClient)
}

// ValidateImageDigest checks if image url contains valid digest, return error if check fails
func ValidateImageDigest(imageUrl string) error {
	_, err := name.NewDigest(imageUrl)
	return err
}

// HaveGitSource checks if snapshotComponent contains non-empty source.git field
// and have both url and revision fields defined
func HaveGitSource(snapshotComponent applicationapiv1alpha1.SnapshotComponent) bool {
	return reflect.ValueOf(snapshotComponent.Source).IsValid() && snapshotComponent.Source.GitSource != nil &&
		snapshotComponent.Source.GitSource.Revision != "" && snapshotComponent.Source.GitSource.URL != ""
}

// HaveGitSourceInComponent checks if component contains non-empty source.git field
// and have both url and revision fields defined
func HaveGitSourceInComponent(component applicationapiv1alpha1.Component) bool {
	return reflect.ValueOf(component.Spec.Source).IsValid() && component.Spec.Source.GitSource != nil &&
		component.Spec.Source.GitSource.Revision != "" && component.Spec.Source.GitSource.URL != ""
}

// HaveAppStudioTestsFinished checks if the AppStudio tests have finished by checking if the AppStudio Test Succeeded condition is set.
func HaveAppStudioTestsFinished(snapshot *applicationapiv1alpha1.Snapshot) bool {
	statusCondition := meta.FindStatusCondition(snapshot.Status.Conditions, AppStudioTestSucceededCondition)
	if statusCondition == nil {
		statusCondition = meta.FindStatusCondition(snapshot.Status.Conditions, LegacyTestSucceededCondition)
		return statusCondition != nil && statusCondition.Status != metav1.ConditionUnknown
	}
	return statusCondition != nil && statusCondition.Status != metav1.ConditionUnknown
}

// HaveAppStudioTestsSucceeded checks if the AppStudio tests have finished by checking if the AppStudio Test Succeeded condition is set.
func HaveAppStudioTestsSucceeded(snapshot *applicationapiv1alpha1.Snapshot) bool {
	if meta.FindStatusCondition(snapshot.Status.Conditions, AppStudioTestSucceededCondition) == nil {
		return meta.IsStatusConditionTrue(snapshot.Status.Conditions, LegacyTestSucceededCondition)
	}

	return meta.IsStatusConditionTrue(snapshot.Status.Conditions, AppStudioTestSucceededCondition)
}

// GetTestSucceededCondition checks status of tests on the snapshot
func GetTestSucceededCondition(snapshot *applicationapiv1alpha1.Snapshot) (condition *metav1.Condition, ok bool) {

	condition = meta.FindStatusCondition(snapshot.Status.Conditions, AppStudioTestSucceededCondition)
	if condition == nil {
		condition = meta.FindStatusCondition(snapshot.Status.Conditions, LegacyTestSucceededCondition)
	}

	ok = (condition != nil && condition.Status != metav1.ConditionUnknown)
	return
}

// GetAppStudioTestsFinishedTime finds the timestamp of tests succeeded condition
func GetAppStudioTestsFinishedTime(snapshot *applicationapiv1alpha1.Snapshot) (metav1.Time, bool) {
	condition, ok := GetTestSucceededCondition(snapshot)
	if ok {
		return condition.LastTransitionTime, true
	}
	return metav1.Time{}, false
}

// CanSnapshotBePromoted checks if the Snapshot in question can be promoted for deployment and release.
func CanSnapshotBePromoted(snapshot *applicationapiv1alpha1.Snapshot) (bool, []string) {
	canBePromoted := true
	reasons := make([]string, 0)
	if !HaveAppStudioTestsFinished(snapshot) {
		canBePromoted = false
		reasons = append(reasons, "the Snapshot has not yet finished testing")
	} else {
		if !HaveAppStudioTestsSucceeded(snapshot) {
			canBePromoted = false
			reasons = append(reasons, "the Snapshot hasn't passed all required integration tests")
		}
		if !IsSnapshotValid(snapshot) {
			canBePromoted = false
			reasons = append(reasons, "the Snapshot is invalid")
		}
		if !IsSnapshotCreatedByPACPushEvent(snapshot) {
			canBePromoted = false
			reasons = append(reasons, "the Snapshot was created for a PaC pull request event")
		}
		if IsSnapshotAutoReleaseDisabled(snapshot) {
			reasons = append(reasons, fmt.Sprintf("the Snapshot '%s' label is 'false'", AutoReleaseLabel))
		}
		if IsGroupSnapshot(snapshot) {
			canBePromoted = false
			reasons = append(reasons, "the Snapshot is group snapshot")
		}
	}
	return canBePromoted, reasons
}

// NewSnapshot creates a new snapshot based on the supplied application and components
func NewSnapshot(application *applicationapiv1alpha1.Application, snapshotComponents *[]applicationapiv1alpha1.SnapshotComponent) *applicationapiv1alpha1.Snapshot {
	// truncate the application name so the GenerateName function can accommodate Kubernetes 63-character limit
	const maxPrefixLength = 57
	prefix := application.Name
	if len(prefix) > maxPrefixLength {
		prefix = prefix[:maxPrefixLength]
	}

	snapshot := &applicationapiv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: prefix + "-",
			Namespace:    application.Namespace,
		},
		Spec: applicationapiv1alpha1.SnapshotSpec{
			Application: application.Name,
			Components:  *snapshotComponents,
		},
	}
	return snapshot
}

// CompareSnapshots compares two Snapshots and returns boolean true if their images match exactly.
func CompareSnapshots(expectedSnapshot *applicationapiv1alpha1.Snapshot, foundSnapshot *applicationapiv1alpha1.Snapshot) bool {
	// Check if the snapshots are created by the same event type
	if !IsSnapshotCreatedBySamePACEvent(expectedSnapshot, foundSnapshot) {
		return false
	}
	// If the number of components doesn't match, we immediately know that the snapshots are not equal.
	if len(expectedSnapshot.Spec.Components) != len(foundSnapshot.Spec.Components) {
		return false
	}

	// Check if all Component information matches, including the containerImage status field
	for _, expectedSnapshotComponent := range expectedSnapshot.Spec.Components {
		foundImage := false
		for _, foundSnapshotComponent := range foundSnapshot.Spec.Components {
			if reflect.DeepEqual(expectedSnapshotComponent, foundSnapshotComponent) {
				foundImage = true
				break
			}
		}
		if !foundImage {
			return false
		}
	}

	return true
}

func IsSnapshotCreatedByPACMergeQueueEvent(snapshot *applicationapiv1alpha1.Snapshot) bool {
	if branch, found := snapshot.Annotations[PipelineAsCodeSourceBranchAnnotation]; found {
		if strings.HasPrefix(branch, PipelineAsCodeGitHubMergeQueueBranchPrefix) {
			return true
		}
	}
	return false
}

// IsSnapshotCreatedByPACPushEvent checks if a snapshot has label PipelineAsCodeEventTypeLabel and with push value
// it the label doesn't exist for some manual snapshot
func IsSnapshotCreatedByPACPushEvent(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return !IsSnapshotCreatedByPACMergeQueueEvent(snapshot) && !IsGroupSnapshot(snapshot) &&
		(metadata.HasLabelWithValue(snapshot, PipelineAsCodeEventTypeLabel, PipelineAsCodePushType) ||
			metadata.HasLabelWithValue(snapshot, PipelineAsCodeEventTypeLabel, PipelineAsCodeGLPushType) ||
			!metadata.HasLabel(snapshot, PipelineAsCodeEventTypeLabel) ||
			!metadata.HasLabel(snapshot, PipelineAsCodePullRequestAnnotation))
}

// IsSnapshotAutoReleaseDisabled checks if a snapshot has a AutoReleaseLabel label and if its value is "false"
func IsSnapshotAutoReleaseDisabled(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return metadata.HasLabelWithValue(snapshot, AutoReleaseLabel, "false")
}

// IsSnapshotCreatedBySamePACEvent checks if the two snapshot are created by the same PAC event
// or they don't have event type
func IsSnapshotCreatedBySamePACEvent(snapshot1, snapshot2 *applicationapiv1alpha1.Snapshot) bool {
	value1, ok1 := snapshot1.GetLabels()[PipelineAsCodeEventTypeLabel]
	value2, ok2 := snapshot2.GetLabels()[PipelineAsCodeEventTypeLabel]
	// if label exists and two snapshots have the same value
	if ok1 && ok2 && value1 == value2 {
		return true
	}
	// if label doesn't exist in two snapshot
	if !ok1 && !ok2 {
		return true
	}
	return false
}

// HasSnapshotTestingChangedToFinished returns a boolean indicating whether the Snapshot testing status has
// changed to finished. If the objects passed to this function are not Snapshots, the function will return false.
func HasSnapshotTestingChangedToFinished(objectOld, objectNew client.Object) bool {
	if oldSnapshot, ok := objectOld.(*applicationapiv1alpha1.Snapshot); ok {
		if newSnapshot, ok := objectNew.(*applicationapiv1alpha1.Snapshot); ok {
			return !HaveAppStudioTestsFinished(oldSnapshot) && HaveAppStudioTestsFinished(newSnapshot)
		}
	}
	return false
}

// HasSnapshotTestAnnotationChanged returns a boolean indicating whether the Snapshot annotation has
// changed. If the objects passed to this function are not Snapshots, the function will return false.
func HasSnapshotTestAnnotationChanged(objectOld, objectNew client.Object) bool {
	if oldSnapshot, ok := objectOld.(*applicationapiv1alpha1.Snapshot); ok {
		if newSnapshot, ok := objectNew.(*applicationapiv1alpha1.Snapshot); ok {
			// update for integration test status change
			if !metadata.HasAnnotation(oldSnapshot, SnapshotTestsStatusAnnotation) && metadata.HasAnnotation(newSnapshot, SnapshotTestsStatusAnnotation) {
				return true
			}
			if old_value, ok := oldSnapshot.GetAnnotations()[SnapshotTestsStatusAnnotation]; ok {
				if new_value, ok := newSnapshot.GetAnnotations()[SnapshotTestsStatusAnnotation]; ok {
					if old_value != new_value {
						return true
					}
				}
			}
			// update for component snapshot's group snapshot creation status
			if !metadata.HasAnnotation(oldSnapshot, PRGroupCreationAnnotation) && metadata.HasAnnotation(newSnapshot, PRGroupCreationAnnotation) {
				return true
			}
			if old_value, ok := oldSnapshot.GetAnnotations()[PRGroupCreationAnnotation]; ok {
				if new_value, ok := newSnapshot.GetAnnotations()[PRGroupCreationAnnotation]; ok {
					if old_value != new_value {
						return true
					}
				}
			}

		}
	}
	return false
}

// HasSnapshotRerunLabelChanged returns a boolean indicating whether the Snapshot label for re-running
// integration test has changed. If the objects passed to this function are not Snapshots, the function will return false.
func HasSnapshotRerunLabelChanged(objectOld, objectNew client.Object) bool {
	if oldSnapshot, ok := objectOld.(*applicationapiv1alpha1.Snapshot); ok {
		if newSnapshot, ok := objectNew.(*applicationapiv1alpha1.Snapshot); ok {
			if !metadata.HasLabel(oldSnapshot, SnapshotIntegrationTestRun) && metadata.HasLabel(newSnapshot, SnapshotIntegrationTestRun) {
				return true
			}
			if old_value, ok := oldSnapshot.GetLabels()[SnapshotIntegrationTestRun]; ok {
				if new_value, ok := newSnapshot.GetLabels()[SnapshotIntegrationTestRun]; ok {
					if old_value != new_value {
						return true
					}
				}
			}
		}
	}
	return false
}

// ExtractPullRequestNumberFromMergeQueueSnapshot attempts to extract the pull request number for the Snapshot
// If the pull request annotation is present, it returns it, otherwise it extracts it from the source branch name
func ExtractPullRequestNumberFromMergeQueueSnapshot(snapshot *applicationapiv1alpha1.Snapshot) string {
	// Attempt to find the PaC pull request label or annotation first
	if snapshot.Labels != nil && snapshot.Labels[PipelineAsCodePullRequestAnnotation] != "" {
		return snapshot.Labels[PipelineAsCodePullRequestAnnotation]
	}
	if snapshot.Annotations != nil && snapshot.Annotations[PipelineAsCodePullRequestAnnotation] != "" {
		return snapshot.Annotations[PipelineAsCodePullRequestAnnotation]
	}

	// If the PR number is not found above, attempt to extract it from the source branch name of the merge queue
	// The branch should be in the format of 'gh-readonly-queue/{original_branch_name}/pr-{pull_request_number}-{sha}'
	if snapshot.Annotations != nil && snapshot.Annotations[PipelineAsCodeSourceBranchAnnotation] != "" {
		branchWithoutPrefix := strings.Split(snapshot.Annotations[PipelineAsCodeSourceBranchAnnotation], "/")
		if len(branchWithoutPrefix) > 1 {
			branchSections := strings.Split(branchWithoutPrefix[len(branchWithoutPrefix)-1], "-")
			if len(branchSections) > 1 && branchSections[0] == "pr" && branchSections[1] != "" {
				return branchSections[1]
			}
		}
	}

	return ""
}

// PrepareSnapshot prepares the Snapshot for a given application, components and the updated component (if any).
// In case the Snapshot can't be created, an error will be returned.
func PrepareSnapshot(ctx context.Context, adapterClient client.Client, application *applicationapiv1alpha1.Application, applicationComponents *[]applicationapiv1alpha1.Component, component *applicationapiv1alpha1.Component, newContainerImage string, newComponentSource *applicationapiv1alpha1.ComponentSource) (*applicationapiv1alpha1.Snapshot, error) {
	log := log.FromContext(ctx)
	var snapshotComponents []applicationapiv1alpha1.SnapshotComponent
	var invalidComponents []string
	for _, applicationComponent := range *applicationComponents {
		applicationComponent := applicationComponent // G601
		containerImage := applicationComponent.Status.LastPromotedImage

		var componentSource *applicationapiv1alpha1.ComponentSource
		var err error
		if applicationComponent.Name == component.Name {
			// if the containerImage doesn't have a valid digest, we cannot construct a Snapshot
			// for the given component
			err = ValidateImageDigest(newContainerImage)
			if err != nil {
				log.Error(err, "component cannot be added to snapshot for application due to invalid digest in containerImage",
					"component.Name", applicationComponent.Name,
					"newContainerImage", newContainerImage)
				return nil, errors.Join(helpers.NewInvalidImageDigestError(component.Name, newContainerImage), err)
			}
			containerImage = newContainerImage
			componentSource = newComponentSource
		} else {
			// Get ComponentSource for the component which is not built in this pipeline
			componentSource, err = GetComponentSourceFromComponent(&applicationComponent)
			if err != nil {
				log.Error(err, "component cannot be added to snapshot for application due to missing git source", "component.Name", applicationComponent.Name)
				invalidComponents = append(invalidComponents, applicationComponent.Name)
				continue
			}
		}

		// If containerImage is empty, we have run into a race condition in
		// which multiple components are being built in close succession.
		// We omit this not-yet-built component from the snapshot rather than
		// including a component that is incomplete.
		if containerImage == "" {
			log.Info("component cannot be added to snapshot for application due to missing containerImage", "component.Name", applicationComponent.Name)
			invalidComponents = append(invalidComponents, applicationComponent.Name)
			continue
		} else {
			// if the containerImage doesn't have a valid digest, the component
			// will not be added to snapshot
			err = ValidateImageDigest(containerImage)
			if err != nil {
				log.Error(err, "component cannot be added to snapshot for application due to invalid digest in containerImage", "component.Name", applicationComponent.Name)
				invalidComponents = append(invalidComponents, applicationComponent.Name)
				continue
			}
			snapshotComponents = append(snapshotComponents, applicationapiv1alpha1.SnapshotComponent{
				Name:           applicationComponent.Name,
				ContainerImage: containerImage,
				Source:         *componentSource,
			})
		}
	}

	if len(snapshotComponents) == 0 {
		return nil, helpers.NewMissingValidComponentError(strings.Join(invalidComponents, ", "))
	}
	snapshot := NewSnapshot(application, &snapshotComponents)

	// expose the source repo URL and SHA in the snapshot as annotation do we don't have to do lookup in integration tests
	if newComponentSource.GitSource != nil {
		if err := metadata.SetAnnotation(snapshot, SnapshotGitSourceRepoURLAnnotation, newComponentSource.GitSource.URL); err != nil {
			return nil, fmt.Errorf("failed to set annotation %s: %w", SnapshotGitSourceRepoURLAnnotation, err)
		}
	}

	if len(invalidComponents) > 0 {
		if err := metadata.SetAnnotation(snapshot, helpers.CreateSnapshotAnnotationName, fmt.Sprintf("Component(s) '%s' is(are) not included in snapshot due to missing valid containerImage or git source", strings.Join(invalidComponents, ", "))); err != nil {
			return nil, fmt.Errorf("failed to set annotation %s: %w", SnapshotGitSourceRepoURLAnnotation, err)
		}
	}

	err := ctrl.SetControllerReference(application, snapshot, adapterClient.Scheme())
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

// FindMatchingSnapshot tries to finds the expected Snapshot with the same set of images.
func FindMatchingSnapshot(application *applicationapiv1alpha1.Application, allSnapshots *[]applicationapiv1alpha1.Snapshot, expectedSnapshot *applicationapiv1alpha1.Snapshot) *applicationapiv1alpha1.Snapshot {
	for _, foundSnapshot := range *allSnapshots {
		foundSnapshot := foundSnapshot
		if CompareSnapshots(expectedSnapshot, &foundSnapshot) {
			return &foundSnapshot
		}
	}
	return nil
}

// GetComponentSourceFromComponent gets the component source from the given Component as Revision
// and set Component.Status.LastBuiltCommit as Component.Source.GitSource.Revision if it is defined.
func GetComponentSourceFromComponent(component *applicationapiv1alpha1.Component) (*applicationapiv1alpha1.ComponentSource, error) {
	if !HaveGitSourceInComponent(*component) {
		return nil, fmt.Errorf("git source is not defined in component %s/%s", component.Namespace, component.Name)
	}
	componentSource := component.Spec.Source.DeepCopy()
	if component.Status.LastBuiltCommit != "" {
		componentSource.GitSource.Revision = component.Status.LastBuiltCommit
	}
	return componentSource, nil
}

// GetIntegrationTestRunLabelValue returns value of the label responsible for re-running tests
func GetIntegrationTestRunLabelValue(obj metav1.Object) (string, bool) {
	labels := obj.GetLabels()
	labelVal, ok := labels[SnapshotIntegrationTestRun]
	return labelVal, ok
}

// RemoveIntegrationTestRerunLabel removes re-run label from snapshot
func RemoveIntegrationTestRerunLabel(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot) error {
	patch := client.MergeFrom(snapshot.DeepCopy())
	err := metadata.DeleteLabel(snapshot, SnapshotIntegrationTestRun)
	if err != nil {
		return fmt.Errorf("failed to delete label %s: %w", SnapshotIntegrationTestRun, err)
	}
	err = adapterClient.Patch(ctx, snapshot, patch)
	if err != nil {
		return fmt.Errorf("failed to patch snapshot: %w", err)
	}

	return nil
}

// AddIntegrationTestRerunLabel adding re-run label to snapshot
func AddIntegrationTestRerunLabel(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, integrationTestScenarioName string) error {
	patch := client.MergeFrom(snapshot.DeepCopy())
	newLabel := map[string]string{}
	newLabel[SnapshotIntegrationTestRun] = integrationTestScenarioName
	err := metadata.AddLabels(snapshot, newLabel)
	if err != nil {
		return fmt.Errorf("failed to add label %s: %w", SnapshotIntegrationTestRun, err)
	}
	err = adapterClient.Patch(ctx, snapshot, patch)
	if err != nil {
		return fmt.Errorf("failed to patch snapshot: %w", err)
	}

	return nil
}

// Deprecated
func GetLatestUpdateTime(snapshot *applicationapiv1alpha1.Snapshot) (time.Time, error) {
	latestUpdateTime := snapshot.GetAnnotations()[SnapshotPRLastUpdate]
	if latestUpdateTime == "" {
		return time.Time{}, nil
	}
	t := time.Time{}
	err := t.UnmarshalText([]byte(latestUpdateTime))
	if err != nil {
		return t, fmt.Errorf("failed to unmarshal time annotation: %w", err)
	}
	return t, nil
}

func ResetSnapshotStatusConditions(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, message string) error {
	if HaveAppStudioTestsFinished(snapshot) {
		patch := client.MergeFrom(snapshot.DeepCopy())
		meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
			Type:    AppStudioIntegrationStatusCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  AppStudioIntegrationStatusInProgress,
			Message: message,
		})
		meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
			Type:    AppStudioTestSucceededCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  AppStudioIntegrationStatusInProgress,
			Message: message,
		})

		err := adapterClient.Status().Patch(ctx, snapshot, patch)
		return err
	}

	return nil
}

// CopySnapshotLabelsAndAnnotations coppies labels and annotations from build pipelineRun or tested snapshot
// into regular snapshot
func CopySnapshotLabelsAndAnnotations(application *applicationapiv1alpha1.Application, snapshot *applicationapiv1alpha1.Snapshot, componentName string, source *metav1.ObjectMeta, prefixes []string) {

	if snapshot.Labels == nil {
		snapshot.Labels = map[string]string{}
	}

	if snapshot.Annotations == nil {
		snapshot.Annotations = map[string]string{}
	}
	snapshot.Labels[SnapshotTypeLabel] = SnapshotComponentType

	snapshot.Labels[SnapshotComponentLabel] = componentName
	snapshot.Labels[ApplicationNameLabel] = application.Name

	// Copy PAC annotations/labels from source(tested snapshot or pipelinerun) to snapshot.
	_ = metadata.CopyLabelsWithPrefixReplacement(source, &snapshot.ObjectMeta, "pipelinesascode.tekton.dev", PipelinesAsCodePrefix)
	_ = metadata.CopyAnnotationsWithPrefixReplacement(source, &snapshot.ObjectMeta, "pipelinesascode.tekton.dev", PipelinesAsCodePrefix)

	for _, prefix := range prefixes {
		// Copy labels and annotations prefixed with defined prefix
		_ = metadata.CopyLabelsByPrefix(source, &snapshot.ObjectMeta, prefix)
		_ = metadata.CopyAnnotationsByPrefix(source, &snapshot.ObjectMeta, prefix)
	}

}

// CopyTempGroupSnapshotLabelsAndAnnotations coppies labels and annotations from build pipelineRun or tested snapshot
// into regular snapshot
func CopyTempGroupSnapshotLabelsAndAnnotations(application *applicationapiv1alpha1.Application, snapshot *applicationapiv1alpha1.Snapshot, componentName string, source *metav1.ObjectMeta, prefixes []string) {
	if snapshot.Labels == nil {
		snapshot.Labels = map[string]string{}
	}

	if snapshot.Annotations == nil {
		snapshot.Annotations = map[string]string{}
	}
	snapshot.Labels[SnapshotTypeLabel] = SnapshotGroupType

	snapshot.Labels[ApplicationNameLabel] = application.Name

	// Copy PAC annotations/labels from source(tested snapshot or pipelinerun) to snapshot.
	_ = metadata.CopyLabelsWithPrefixReplacement(source, &snapshot.ObjectMeta, "pipelinesascode.tekton.dev", PipelinesAsCodePrefix)
	_ = metadata.CopyAnnotationsWithPrefixReplacement(source, &snapshot.ObjectMeta, "pipelinesascode.tekton.dev", PipelinesAsCodePrefix)

	for _, prefix := range prefixes {
		// Copy labels and annotations prefixed with defined prefix
		_ = metadata.CopyLabelsByPrefix(source, &snapshot.ObjectMeta, prefix)
		_ = metadata.CopyAnnotationsByPrefix(source, &snapshot.ObjectMeta, prefix)
	}
}

// IsOverrideSnapshot returns true if snapshot label 'test.appstudio.openshift.io/type' is 'override'
func IsOverrideSnapshot(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return metadata.HasLabelWithValue(snapshot, SnapshotTypeLabel, SnapshotOverrideType)
}

func IsComponentSnapshot(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return metadata.HasLabelWithValue(snapshot, SnapshotTypeLabel, SnapshotComponentType)
}

// IsGroupSnapshot returns true if snapshot label 'test.appstudio.openshift.io/type' is 'group'
func IsGroupSnapshot(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return metadata.HasLabelWithValue(snapshot, SnapshotTypeLabel, SnapshotGroupType)
}

func IsComponentSnapshotCreatedByPACPushEvent(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsComponentSnapshot(snapshot) && IsSnapshotCreatedByPACPushEvent(snapshot)
}

func SetOwnerReference(ctx context.Context, adapterClient client.Client, snapshot *applicationapiv1alpha1.Snapshot, owner *applicationapiv1alpha1.Application) (*applicationapiv1alpha1.Snapshot, error) {
	patch := client.MergeFrom(snapshot.DeepCopy())
	err := ctrl.SetControllerReference(owner, snapshot, adapterClient.Scheme())
	if err != nil {
		return snapshot, err
	}
	err = adapterClient.Patch(ctx, snapshot, patch)
	if err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

// IsContextValidForSnapshot checks the context and compares it against the Snapshot to determine if it applies
func IsContextValidForSnapshot(scenarioContextName string, snapshot *applicationapiv1alpha1.Snapshot) bool {
	// `application` context is supported for backwards-compatibility and considered the same as `all`
	if scenarioContextName == "application" || scenarioContextName == "all" {
		return true
	} else if scenarioContextName == "component" && IsComponentSnapshot(snapshot) {
		return true
	} else if strings.HasPrefix(scenarioContextName, "component_") {
		componentName := strings.TrimPrefix(scenarioContextName, "component_")
		if metadata.HasLabelWithValue(snapshot, SnapshotComponentLabel, componentName) {
			return true
		}
	} else if scenarioContextName == "group" && IsGroupSnapshot(snapshot) {
		return true
	} else if scenarioContextName == "override" && IsOverrideSnapshot(snapshot) {
		return true
	} else if scenarioContextName == "push" && IsSnapshotCreatedByPACPushEvent(snapshot) {
		return true
	} else if scenarioContextName == "pull_request" && !IsSnapshotCreatedByPACPushEvent(snapshot) {
		return true
	}
	return false
}

// IsScenarioApplicableToSnapshotsContext checks the contexts list for a given IntegrationTestScenario and
// compares it against the Snapshot to determine if the scenario applies to it
func IsScenarioApplicableToSnapshotsContext(scenario *v1beta2.IntegrationTestScenario, snapshot *applicationapiv1alpha1.Snapshot) bool {
	// If the contexts list is empty, we assume that the scenario applies to all contexts by default
	if len(scenario.Spec.Contexts) == 0 {
		return true
	}
	for _, scenarioContext := range scenario.Spec.Contexts {
		scenarioContext := scenarioContext //G601
		if IsContextValidForSnapshot(scenarioContext.Name, snapshot) {
			return true
		}
	}
	return false
}

// FilterIntegrationTestScenariosWithContext returns a filtered list of IntegrationTestScenario from the given list
// of IntegrationTestScenarios compared against the given Snapshot based on individual IntegrationTestScenario contexts
func FilterIntegrationTestScenariosWithContext(scenarios *[]v1beta2.IntegrationTestScenario, snapshot *applicationapiv1alpha1.Snapshot) *[]v1beta2.IntegrationTestScenario {
	var filteredScenarioList []v1beta2.IntegrationTestScenario
	for _, scenario := range *scenarios {
		scenario := scenario //G601
		if IsScenarioApplicableToSnapshotsContext(&scenario, snapshot) {
			filteredScenarioList = append(filteredScenarioList, scenario)
		}
	}
	return &filteredScenarioList
}

// HasPRGroupProcessed checks if the group creation has been handled for this snapshot by snapshot adapter when
// to avoid duplicate check when reconciling this snapshot, if yes, won't handle the snapshot again
// the annotation updated from other component pipelinerun should not be counted in
func HasPRGroupProcessed(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return metadata.HasAnnotation(snapshot, PRGroupCreationAnnotation) && !strings.Contains(snapshot.GetAnnotations()[PRGroupCreationAnnotation], "waiting for it to create a new group Snapshot for PR group")
}

// GetPRGroup gets the value of label test.appstudio.openshift.io/pr-group-sha and annotation from component snapshot or pipelinerun
func GetPRGroup(object client.Object) (string, string) {
	if metadata.HasLabel(object, PRGroupHashLabel) && metadata.HasAnnotation(object, PRGroupAnnotation) {
		return object.GetLabels()[PRGroupHashLabel], object.GetAnnotations()[PRGroupAnnotation]
	}
	return "", ""
}

// FindMatchingSnapshotComponent find the snapshot component from the given snapshot according to the name of the given component name
func FindMatchingSnapshotComponent(snapshot *applicationapiv1alpha1.Snapshot, component *applicationapiv1alpha1.Component) applicationapiv1alpha1.SnapshotComponent {
	for _, snapshotComponent := range snapshot.Spec.Components {
		if snapshotComponent.Name == component.Name {
			return snapshotComponent
		}
	}
	return applicationapiv1alpha1.SnapshotComponent{}

}

// SortSnapshots sorts the snapshots according to the snapshot annotation BuildPipelineRunStartTime
func SortSnapshots(snapshots []applicationapiv1alpha1.Snapshot) []applicationapiv1alpha1.Snapshot {
	sort.Slice(snapshots, func(i, j int) bool {
		// sorting snapshots according to the annotation BuildPipelineRunStartTime which
		// represents the start time of build PLR
		// when BuildPipelineRunStartTime is not set, we use its creation time
		var time_i, time_j int
		if metadata.HasAnnotation(&snapshots[i], BuildPipelineRunStartTime) && metadata.HasAnnotation(&snapshots[j], BuildPipelineRunStartTime) {
			time_i, _ = strconv.Atoi(snapshots[i].Annotations[BuildPipelineRunStartTime])
			time_j, _ = strconv.Atoi(snapshots[j].Annotations[BuildPipelineRunStartTime])
		} else {
			time_i = int(snapshots[i].CreationTimestamp.Unix())
			time_j = int(snapshots[j].CreationTimestamp.Unix())
		}
		return time_i > time_j
	})
	return snapshots
}

// AnnotateSnapshot sets annotation for a snapshot in defined context, return error if meeting it
func AnnotateSnapshot(ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, key, value string, cl client.Client) error {
	patch := client.MergeFrom(snapshot.DeepCopy())

	_ = metadata.SetAnnotation(&snapshot.ObjectMeta, key, value)

	err := cl.Patch(ctx, snapshot, patch)
	if err != nil {
		return err
	}
	return nil
}

// AnnotateComponent sets annotation for a component in defined context, return error if meeting it
func AnnotateComponent(ctx context.Context, component *applicationapiv1alpha1.Component, key, value string, cl client.Client) error {
	patch := client.MergeFrom(component.DeepCopy())

	_ = metadata.SetAnnotation(&component.ObjectMeta, key, value)

	err := cl.Patch(ctx, component, patch)
	if err != nil {
		return err
	}
	return nil
}

// NotifyComponentSnapshotsInGroupSnapshot annotate the msg to the given component snapshots in componentSnapshotInfos
func NotifyComponentSnapshotsInGroupSnapshot(ctx context.Context, cl client.Client, componentSnapshotInfos []ComponentSnapshotInfo, msg string) error {
	log := log.FromContext(ctx)
	for _, componentSnapshotInfo := range componentSnapshotInfos {
		snapshot := &applicationapiv1alpha1.Snapshot{}
		err := cl.Get(ctx, types.NamespacedName{
			Namespace: componentSnapshotInfo.Namespace,
			Name:      componentSnapshotInfo.Snapshot,
		}, snapshot)
		if err != nil {
			log.Error(err, fmt.Sprintf("error while getting snapshot %s from namespace: %s", componentSnapshotInfo.Snapshot, componentSnapshotInfo.Namespace))
			return err
		}

		err = AnnotateSnapshot(ctx, snapshot, PRGroupCreationAnnotation, msg, cl)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to annotate group snapshot creation status to component snapshot %s/%s", componentSnapshotInfo.Namespace, componentSnapshotInfo.Snapshot))
			return err
		}
	}
	return nil
}

func SetAnnotationAndLabelForGroupSnapshot(groupSnapshot *applicationapiv1alpha1.Snapshot, componentSnapshot *applicationapiv1alpha1.Snapshot, componentSnapshotInfos []ComponentSnapshotInfo) (*applicationapiv1alpha1.Snapshot, error) {
	err := metadata.SetAnnotation(groupSnapshot, PRGroupAnnotation, componentSnapshot.Annotations[PRGroupAnnotation])
	if err != nil {
		return nil, err
	}
	annotationJson, err := json.Marshal(componentSnapshotInfos)
	if err != nil {
		return nil, err
	}
	groupSnapshot.Annotations[GroupSnapshotInfoAnnotation] = string(annotationJson)
	groupSnapshot.Annotations[PRGroupAnnotation] = componentSnapshot.Annotations[PRGroupAnnotation]

	err = metadata.SetLabel(groupSnapshot, PipelineAsCodeEventTypeLabel, componentSnapshot.Labels[PipelineAsCodeEventTypeLabel])
	if err != nil {
		return nil, err
	}
	groupSnapshot.Labels[SnapshotTypeLabel] = SnapshotGroupType
	groupSnapshot.Labels[PRGroupHashLabel] = componentSnapshot.Labels[PRGroupHashLabel]
	groupSnapshot.Labels[ApplicationNameLabel] = componentSnapshot.Spec.Application

	return groupSnapshot, nil
}

// UnmarshalJSON load data from JSON
func UnmarshalJSON(b []byte) ([]*ComponentSnapshotInfo, error) {
	var componentSnapshotInfos []*ComponentSnapshotInfo

	sch, err := jsonschema.CompileString("schema.json", componentSnapshotInfosSchema)
	if err != nil {
		return nil, fmt.Errorf("error while compiling json data for schema validation: %w", err)
	}
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json data raw: %w", err)
	}
	if err = sch.Validate(v); err != nil {
		return nil, fmt.Errorf("error validating snapshot info: %w", err)
	}
	err = json.Unmarshal(b, &componentSnapshotInfos)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal json data: %w", err)
	}

	return componentSnapshotInfos, nil
}

func GetSourceRepoOwnerFromSnapshot(snapshot *applicationapiv1alpha1.Snapshot) string {
	sourceRepoUrlAnnotation, found := snapshot.GetAnnotations()[PipelineAsCodeGitSourceURLAnnotation]
	if found {
		arr := strings.Split(sourceRepoUrlAnnotation, "/")
		return arr[len(arr)-2]
	}
	return ""
}

// GetShaFromSnapshot returns the value of "pac.test.appstudio.openshift.io/sha"
// annotation of length 7 for short SHA from the Snapshot, if it exists.
// If the SHA is shorter than 7 characters, it returns it as is.
// If the SHA is empty, it logs an info message and returns an empty string.
func GetShaFromSnapshot(ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot) string {
	log := log.FromContext(ctx)

	sha, found := snapshot.GetAnnotations()[PipelineAsCodeSHAAnnotation]
	if found {
		if len(sha) == 0 {
			log.Info(fmt.Sprintf("annotation '%s' in Snapshot '%s' is present but empty", PipelineAsCodeSHAAnnotation, snapshot.Name))
			return ""
		}
		if len(sha) >= 7 {
			return sha[:7]
		}
		return sha // Return as is if shorter than 7 characters
	}

	log.Info(fmt.Sprintf("annotation '%s' not found in Snapshot '%s', won't add SHA value to the Release name", PipelineAsCodeSHAAnnotation, snapshot.Name))

	return ""
}

// PrepareTempGroupSnapshot will prepare a temp group snapshot used to check the integration test scenario that should be applied to the group snapshot under that application
func PrepareTempGroupSnapshot(application *applicationapiv1alpha1.Application, snapshot *applicationapiv1alpha1.Snapshot) *applicationapiv1alpha1.Snapshot {
	tempGroupSnapshot := NewSnapshot(application, &[]applicationapiv1alpha1.SnapshotComponent{})
	tempGroupSnapshot, _ = SetAnnotationAndLabelForGroupSnapshot(tempGroupSnapshot, snapshot, []ComponentSnapshotInfo{})
	return tempGroupSnapshot
}

func CancelPipelineRuns(c client.Client, ctx context.Context, logger helpers.IntegrationLogger, integrationTestPipelineRuns []tektonv1.PipelineRun) error {
	// get all integration pipelineruns for a snapshot
	for _, plr := range integrationTestPipelineRuns {
		plr := plr
		if !helpers.HasPipelineRunFinished(&plr) {
			// remove finalizer and cancel pipelinerun
			err := helpers.RemoveFinalizerFromPipelineRun(ctx, c, logger, &plr, helpers.IntegrationPipelineRunFinalizer)
			if err != nil {
				return err
			}

			// set "CancelledRunFinally" to PLR status, should gracefully cancel pipelinerun, this is so raw I hate this
			patch := client.MergeFrom(plr.DeepCopy())
			plr.Spec.Status = tektonv1.PipelineRunSpecStatusCancelledRunFinally
			plr.Annotations[PRGroupCancelledAnnotation] = "true"

			err = c.Patch(ctx, &plr, patch)
			if err != nil {
				return err
			}
			logger.LogAuditEvent("IntegrationTestscenario pipelineRun has been cancelled.", &plr, helpers.LogActionUpdate)
		}
	}
	return nil
}

// UpdateComponentImageAndSource updates both .Status.LastPromotedImage and .Status.LastBuiltCommit for a component resulting in updating the GCL, as well as annotation test.appstudio.openshift.io/lastbuilttime according to build plr's start time or current time for override snapshot
func UpdateComponentImageAndSource(ctx context.Context, adapterClient client.Client, object client.Object, component *applicationapiv1alpha1.Component, componentSource applicationapiv1alpha1.ComponentSource, containerImage string) error {
	log := log.FromContext(ctx)
	patch := client.MergeFrom(component.DeepCopy())

	// update .Status.LastPromotedImage for the component
	component.Status.LastPromotedImage = containerImage

	// update .Status.LastBuiltCommit for the component
	if reflect.ValueOf(componentSource).IsValid() && componentSource.GitSource != nil && componentSource.GitSource.Revision != "" {
		component.Status.LastBuiltCommit = componentSource.GitSource.Revision
	}

	err := adapterClient.Status().Patch(ctx, component, patch)
	if err != nil {
		log.Error(err, "Failed to update .Status.LastBuiltCommit and .Status.LastPromotedImage of Global Candidate for the Component",
			"component.Name", component.Name)
		return err
	}

	log.Info("Updated .Status.LastBuiltCommit and .Status.LastPromotedImage of Global Candidate for the Component",
		"Component.Namespace", component.Namespace, "LastPromotedImage", component.Status.LastPromotedImage, "Component.Name", component.Name,
		"lastBuildCommit", component.Status.LastBuiltCommit)

	// update the component's last build time annotation
	var buildTimeStr string
	if pr, ok := object.(*tektonv1.PipelineRun); ok {
		if pr.Status.StartTime != nil {
			buildTimeStr = strconv.FormatInt(pr.Status.StartTime.Unix(), 10)
		}
	}
	if _, ok := object.(*applicationapiv1alpha1.Snapshot); ok {
		buildTimeStr = strconv.FormatInt(time.Now().Unix(), 10)
	}
	return AnnotateComponent(ctx, component, BuildPipelineLastBuiltTime, buildTimeStr, adapterClient)
}
