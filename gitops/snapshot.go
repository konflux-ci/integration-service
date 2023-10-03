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
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/metrics"
	"github.com/redhat-appstudio/integration-service/tekton"
	"github.com/redhat-appstudio/operator-toolkit/metadata"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// PipelinesAsCodePrefix contains the prefix applied to labels and annotations copied from Pipelines as Code resources.
	PipelinesAsCodePrefix = "pac.test.appstudio.openshift.io"

	// SnapshotTypeLabel contains the type of the Snapshot.
	SnapshotTypeLabel = "test.appstudio.openshift.io/type"

	// SnapshotLabel contains the name of the Snapshot within appstudio
	SnapshotLabel = "appstudio.openshift.io/snapshot"

	// SnapshotTestScenarioLabel contains the name of the Snapshot test scenario.
	SnapshotTestScenarioLabel = "test.appstudio.openshift.io/scenario"

	// SnapshotTestScenarioLabel contains json data with test results of the particular snapshot
	SnapshotTestsStatusAnnotation = "test.appstudio.openshift.io/status"

	// BuildPipelineRunPrefix contains the build pipeline run related labels and annotations
	BuildPipelineRunPrefix = "build.appstudio"

	// BuildPipelineRunFinishTimeLabel contains the build PipelineRun finish time of the Snapshot.
	BuildPipelineRunFinishTimeLabel = "test.appstudio.openshift.io/pipelinerunfinishtime"

	// BuildPipelineRunNameLabel contains the build PipelineRun name
	BuildPipelineRunNameLabel = "appstudio.openshift.io/build-pipelinerun"

	// SnapshotComponentType is the type of Snapshot which was created for a single component build.
	SnapshotComponentType = "component"

	// SnapshotCompositeType is the type of Snapshot which was created for multiple components.
	SnapshotCompositeType = "composite"

	// PipelineAsCodeEventTypeLabel is the type of event which triggered the pipelinerun in build service
	PipelineAsCodeEventTypeLabel = PipelinesAsCodePrefix + "/event-type"

	// PipelineAsCodeGitProviderLabel is the git provider which triggered the pipelinerun in build service.
	PipelineAsCodeGitProviderLabel = PipelinesAsCodePrefix + "/git-provider"

	// PipelineAsCodeSHALabel is the commit which triggered the pipelinerun in build service.
	PipelineAsCodeSHALabel = PipelinesAsCodePrefix + "/sha"

	// PipelineAsCodeURLOrgLabel is the organization for the git repo which triggered the pipelinerun in build service.
	PipelineAsCodeURLOrgLabel = PipelinesAsCodePrefix + "/url-org"

	// PipelineAsCodeURLRepositoryLabel is the git repository which triggered the pipelinerun in build service.
	PipelineAsCodeURLRepositoryLabel = PipelinesAsCodePrefix + "/url-repository"

	// PipelineAsCodeRepoURLAnnotation is the URL to the git repository which triggered the pipelinerun in build service.
	PipelineAsCodeRepoURLAnnotation = PipelinesAsCodePrefix + "/repo-url"

	// PipelineAsCodeInstallationIDAnnotation is the GitHub App installation ID for the git repo which triggered the pipelinerun in build service.
	PipelineAsCodeInstallationIDAnnotation = PipelinesAsCodePrefix + "/installation-id"

	// PipelineAsCodePullRequestAnnotation is the git repository's pull request identifier
	PipelineAsCodePullRequestAnnotation = PipelinesAsCodePrefix + "/pull-request"

	// PipelineAsCodePushType is the type of push event which triggered the pipelinerun in build service
	PipelineAsCodePushType = "push"

	// PipelineAsCodePullRequestType is the type of pull_request event which triggered the pipelinerun in build service
	PipelineAsCodePullRequestType = "pull_request"

	// PipelineAsCodeGitHubProviderType is the git provider type for a GitHub event which triggered the pipelinerun in build service.
	PipelineAsCodeGitHubProviderType = "github"

	//AppStudioTestSucceededCondition is the condition for marking if the AppStudio Tests succeeded for the Snapshot.
	AppStudioTestSucceededCondition = "AppStudioTestSucceeded"

	//LegacyTestSucceededCondition is the condition for marking if the AppStudio Tests succeeded for the Snapshot.
	LegacyTestSucceededCondition = "HACBSStudioTestSucceeded"

	// AppStudioIntegrationStatusCondition is the condition for marking the AppStudio integration status of the Snapshot.
	AppStudioIntegrationStatusCondition = "AppStudioIntegrationStatus"

	// LegacyIntegrationStatusCondition is the condition for marking the AppStudio integration status of the Snapshot.
	LegacyIntegrationStatusCondition = "HACBSIntegrationStatus"

	// IntegrationTestScenarioValid is the condition for marking the AppStudio integration status of the Scenario.
	IntegrationTestScenarioValid = "IntegrationTestScenarioValid"

	// SnapshotDeployedToRootEnvironmentsCondition is the condition for marking if Snapshot was deployed to root environments
	// within the user's workspace.
	SnapshotDeployedToRootEnvironmentsCondition = "DeployedToRootEnvironments"

	// SnapshotAutoReleasedCondition is the condition for marking if Snapshot was auto-released released with AppStudio.
	SnapshotAutoReleasedCondition = "AutoReleased"

	// SnapshotAddedToGlobalCandidateListCondition is the condition for marking if Snapshot's component was added to
	// the global candidate list.
	SnapshotAddedToGlobalCandidateListCondition = "AddedToGlobalCandidateList"

	// AppStudioTestSucceededConditionPassed is the reason that's set when the AppStudio tests succeed.
	AppStudioTestSucceededConditionPassed = "Passed"

	// AppStudioTestSucceededConditionFailed is the reason that's set when the AppStudio tests fail.
	AppStudioTestSucceededConditionFailed = "Failed"

	// AppStudioIntegrationStatusInvalid is the reason that's set when the AppStudio integration gets into an invalid state.
	AppStudioIntegrationStatusInvalid = "Invalid"

	// AppStudioIntegrationStatusErrorOccured is the reason that's set when the AppStudio integration gets into an error state.
	AppStudioIntegrationStatusErrorOccured = "ErrorOccured"

	// AppStudioIntegrationStatusValid is the reason that's set when the AppStudio integration gets into an valid state.
	AppStudioIntegrationStatusValid = "Valid"

	//AppStudioIntegrationStatusInProgress is the reason that's set when the AppStudio tests gets into an in progress state.
	AppStudioIntegrationStatusInProgress = "InProgress"

	//AppStudioIntegrationStatusFinished is the reason that's set when the AppStudio tests finish.
	AppStudioIntegrationStatusFinished = "Finished"
)

// IntegrationTestScenario test runs status
type IntegrationTestStatus int

//go:generate enumer -type=IntegrationTestStatus -linecomment -json
const (
	// Nothing is done yet for the ITS list and snapshot
	IntegrationTestStatusPending IntegrationTestStatus = iota + 1 // Pending
	// Starting to handle an integration test scenario for a snapshot
	IntegrationTestStatusInProgress // InProgress
	// The environment provision experienced error for this ITS and snapshot
	IntegrationTestStatusEnvironmentProvisionError // EnvironmentProvisionError
	// The SEB deployment experienced error for this ITS and snapshot
	IntegrationTestStatusDeploymentError // DeploymentError
	// Integration PLR failed for this ITS and snapshot
	IntegrationTestStatusTestFail // TestFail
	// Integration PLR passed for this ITS and snapshot
	IntegrationTestStatusTestPassed // TestPassed
)

var (
	// SnapshotComponentLabel contains the name of the updated Snapshot component - it should match the pipeline label.
	SnapshotComponentLabel = tekton.ComponentNameLabel
)

// IsSnapshotMarkedAsPassed returns true if snapshot is marked as passed
func IsSnapshotMarkedAsPassed(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, AppStudioTestSucceededCondition, metav1.ConditionTrue, "")
}

// MarkSnapshotAsPassed updates the AppStudio Test succeeded condition for the Snapshot to passed.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsPassed(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
	patch := client.MergeFrom(snapshot.DeepCopy())
	condition := metav1.Condition{
		Type:    AppStudioTestSucceededCondition,
		Status:  metav1.ConditionTrue,
		Reason:  AppStudioTestSucceededConditionPassed,
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)

	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return nil, err
	}

	snapshotCompletionTime := &metav1.Time{Time: time.Now()}
	go metrics.RegisterCompletedSnapshot(condition.Type, condition.Reason, snapshot.GetCreationTimestamp(), snapshotCompletionTime)
	return snapshot, nil
}

// IsSnapshotMarkedAsFailed returns true if snapshot is marked as failed
func IsSnapshotMarkedAsFailed(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, AppStudioTestSucceededCondition, metav1.ConditionFalse, "")
}

// MarkSnapshotAsFailed updates the AppStudio Test succeeded condition for the Snapshot to failed.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsFailed(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
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
		return nil, err
	}

	snapshotCompletionTime := &metav1.Time{Time: time.Now()}
	go metrics.RegisterCompletedSnapshot(condition.Type, condition.Reason, snapshot.GetCreationTimestamp(), snapshotCompletionTime)
	return snapshot, nil
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
func MarkSnapshotIntegrationStatusAsInProgress(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
	patch := client.MergeFrom(snapshot.DeepCopy())
	meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
		Type:    AppStudioIntegrationStatusCondition,
		Status:  metav1.ConditionUnknown,
		Reason:  AppStudioIntegrationStatusInProgress,
		Message: message,
	})
	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return nil, err
	}

	snapshotInProgressTime := &metav1.Time{Time: time.Now()}
	if metadata.HasLabel(snapshot, BuildPipelineRunFinishTimeLabel) {
		buildPipelineRunFinishTimeStr := snapshot.Labels[BuildPipelineRunFinishTimeLabel]
		buildPipelineRunFinishTimeInt, _ := strconv.ParseInt(buildPipelineRunFinishTimeStr, 10, 64)
		buildPipelineRunFinishTime := time.Unix(buildPipelineRunFinishTimeInt, 0)
		buildPipelineRunFinishTimeMeta := &metav1.Time{Time: buildPipelineRunFinishTime}

		go metrics.RegisterIntegrationResponse(*buildPipelineRunFinishTimeMeta, snapshotInProgressTime)
	}
	return snapshot, nil
}

// PrepareToRegisterIntegrationPipelineRun is to do preparation before calling RegisterNewIntegrationPipelineRun
func PrepareToRegisterIntegrationPipelineRun(snapshot *applicationapiv1alpha1.Snapshot) {
	pipelineRunStartTime := &metav1.Time{Time: time.Now()}
	go metrics.RegisterNewIntegrationPipelineRun(snapshot.GetCreationTimestamp(), pipelineRunStartTime)
}

// SetSnapshotIntegrationStatusAsFinished sets the AppStudio integration status condition for the Snapshot to Finished.
func SetSnapshotIntegrationStatusAsFinished(snapshot *applicationapiv1alpha1.Snapshot, message string) {
	condition := metav1.Condition{
		Type:    AppStudioIntegrationStatusCondition,
		Status:  metav1.ConditionTrue,
		Reason:  AppStudioIntegrationStatusFinished,
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)
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

// IsSnapshotMarkedAsDeployedToRootEnvironments returns true if snapshot is marked as deployed to root environments
func IsSnapshotMarkedAsDeployedToRootEnvironments(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, SnapshotDeployedToRootEnvironmentsCondition, metav1.ConditionTrue, "")
}

// MarkSnapshotAsDeployedToRootEnvironments updates the SnapshotDeployedToRootEnvironmentsCondition for the Snapshot to 'Deployed'.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsDeployedToRootEnvironments(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
	patch := client.MergeFrom(snapshot.DeepCopy())
	condition := metav1.Condition{
		Type:    SnapshotDeployedToRootEnvironmentsCondition,
		Status:  metav1.ConditionTrue,
		Reason:  "Deployed",
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)

	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// IsSnapshotMarkedAsAutoReleased returns true if snapshot is marked as deployed to root environments
func IsSnapshotMarkedAsAutoReleased(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, SnapshotAutoReleasedCondition, metav1.ConditionTrue, "")
}

// MarkSnapshotAsAutoReleased updates the SnapshotAutoReleasedCondition for the Snapshot to 'AutoReleased'.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsAutoReleased(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
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
		return nil, err
	}

	return snapshot, nil
}

// IsSnapshotMarkedAsAddedToGlobalCandidateList returns true if snapshot's component is marked as added to global candidate list
func IsSnapshotMarkedAsAddedToGlobalCandidateList(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return IsSnapshotStatusConditionSet(snapshot, SnapshotAddedToGlobalCandidateListCondition, metav1.ConditionTrue, "")
}

// MarkSnapshotAsAddedToGlobalCandidateList updates the SnapshotAddedToGlobalCandidateListCondition for the Snapshot to true with reason 'Added'.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsAddedToGlobalCandidateList(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
	patch := client.MergeFrom(snapshot.DeepCopy())
	condition := metav1.Condition{
		Type:    SnapshotAddedToGlobalCandidateListCondition,
		Status:  metav1.ConditionTrue,
		Reason:  "Added",
		Message: message,
	}
	meta.SetStatusCondition(&snapshot.Status.Conditions, condition)

	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

// ValidateImageDigest checks if image url contains valid digest, return error if check fails
func ValidateImageDigest(imageUrl string) error {
	_, err := name.NewDigest(imageUrl)
	return err
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
		if IsSnapshotCreatedByPACPullRequestEvent(snapshot) {
			canBePromoted = false
			reasons = append(reasons, "the Snapshot was created for a PaC pull request event")
		}
	}
	return canBePromoted, reasons
}

// NewSnapshot creates a new snapshot based on the supplied application and components
func NewSnapshot(application *applicationapiv1alpha1.Application, snapshotComponents *[]applicationapiv1alpha1.SnapshotComponent) *applicationapiv1alpha1.Snapshot {
	snapshot := &applicationapiv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: application.Name + "-",
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
	if IsSnapshotCreatedByPACPullRequestEvent(expectedSnapshot) != IsSnapshotCreatedByPACPullRequestEvent(foundSnapshot) {
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

// IsSnapshotCreatedByPACPullRequestEvent checks if a snapshot has label PipelineAsCodeEventTypeLabel and with push value
func IsSnapshotCreatedByPACPullRequestEvent(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return metadata.HasLabelWithValue(snapshot, PipelineAsCodeEventTypeLabel, PipelineAsCodePullRequestType)
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

// PrepareSnapshot prepares the Snapshot for a given application, components and the updated component (if any).
// In case the Snapshot can't be created, an error will be returned.
func PrepareSnapshot(adapterClient client.Client, ctx context.Context, application *applicationapiv1alpha1.Application, applicationComponents *[]applicationapiv1alpha1.Component, component *applicationapiv1alpha1.Component, newContainerImage string, newComponentSource *applicationapiv1alpha1.ComponentSource) (*applicationapiv1alpha1.Snapshot, error) {
	log := log.FromContext(ctx)
	var snapshotComponents []applicationapiv1alpha1.SnapshotComponent
	for _, applicationComponent := range *applicationComponents {
		applicationComponent := applicationComponent // G601
		containerImage := applicationComponent.Spec.ContainerImage

		var componentSource *applicationapiv1alpha1.ComponentSource
		if applicationComponent.Name == component.Name {
			containerImage = newContainerImage
			componentSource = newComponentSource
		} else {
			// Get ComponentSource for the component which is not built in this pipeline
			componentSource = GetComponentSourceFromComponent(&applicationComponent)
		}

		// If containerImage is empty, we have run into a race condition in
		// which multiple components are being built in close succession.
		// We omit this not-yet-built component from the snapshot rather than
		// including a component that is incomplete.
		if containerImage == "" {
			log.Error(nil, "component cannot be added to snapshot for application due to missing containerImage", "component.Name", applicationComponent.Name)
			continue
		}
		// if the containerImage doesn't have a valid digest, the component
		// will not be added to snapshot
		err := ValidateImageDigest(containerImage)
		if err != nil {
			log.Error(err, "component cannot added to snapshot for application due to invalid digest in containerImage", "component.Name", applicationComponent.Name)
			continue
		}
		snapshotComponents = append(snapshotComponents, applicationapiv1alpha1.SnapshotComponent{
			Name:           applicationComponent.Name,
			ContainerImage: containerImage,
			Source:         *componentSource,
		})
	}

	if len(snapshotComponents) == 0 {
		return nil, fmt.Errorf("failed to prepare snapshot due to missing valid digest in containerImage for all components of application")
	}
	snapshot := NewSnapshot(application, &snapshotComponents)

	err := ctrl.SetControllerReference(application, snapshot, adapterClient.Scheme())
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

// FindMatchingSnapshot tries to find the expected Snapshot with the same set of images.
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
func GetComponentSourceFromComponent(component *applicationapiv1alpha1.Component) *applicationapiv1alpha1.ComponentSource {
	componentSource := component.Spec.Source.DeepCopy()
	if component.Status.LastBuiltCommit != "" {
		componentSource.GitSource.Revision = component.Status.LastBuiltCommit
	}
	return componentSource
}
