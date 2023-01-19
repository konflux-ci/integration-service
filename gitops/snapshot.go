package gitops

import (
	"context"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/tekton"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// PipelinesAsCodePrefix contains the prefix applied to labels and annotations copied from Pipelines as Code resources.
	PipelinesAsCodePrefix = "pac.test.appstudio.openshift.io"

	// SnapshotTypeLabel contains the type of the Snapshot.
	SnapshotTypeLabel = "test.appstudio.openshift.io/type"

	// SnapshotTestScenarioLabel contains the name of the Snapshot test scenario.
	SnapshotTestScenarioLabel = "test.appstudio.openshift.io/scenario"

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

	//StonesoupTestSuceededCondition is the condition for marking if the Stonesoup Tests succeeded for the Snapshot.
	StonesoupTestSuceededCondition = "StonesoupTestSucceeded"

	// StonesoupIntegrationStatusCondition is the condition for marking the Stonesoup integration status of the Snapshot.
	StonesoupIntegrationStatusCondition = "StonesoupIntegrationStatus"

	// StonesoupTestSuceededConditionPassed is the reason that's set when the Stonesoup tests succeed.
	StonesoupTestSuceededConditionPassed = "Passed"

	// StonesoupTestSuceededConditionFailed is the reason that's set when the Stonesoup tests fail.
	StonesoupTestSuceededConditionFailed = "Failed"

	// StonesoupIntegrationStatusInvalid is the reason that's set when the Stonesoup integration gets into an invalid state.
	StonesoupIntegrationStatusInvalid = "Invalid"

	//StonesoupIntegrationStatusInProgress is the reason that's set when the Stonesoup tests gets into an in progress state.
	StonesoupIntegrationStatusInProgress = "InProgress"

	//StonesoupIntegrationStatusFinished is the reason that's set when the Stonesoup tests finish.
	StonesoupIntegrationStatusFinished = "Finished"
)

var (
	// SnapshotComponentLabel contains the name of the updated Snapshot component - it should match the pipeline label.
	SnapshotComponentLabel = tekton.ComponentNameLabel
)

// MarkSnapshotAsPassed updates the Stonesoup Test succeeded condition for the Snapshot to passed.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsPassed(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
	patch := client.MergeFrom(snapshot.DeepCopy())
	meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
		Type:    StonesoupTestSuceededCondition,
		Status:  metav1.ConditionTrue,
		Reason:  StonesoupTestSuceededConditionPassed,
		Message: message,
	})
	SetSnapshotIntegrationStatusAsFinished(snapshot, "Marking snapshot integration status condition as finished since the testing is passed")
	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// MarkSnapshotAsFailed updates the Stonesoup Test succeeded condition for the Snapshot to failed.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsFailed(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
	patch := client.MergeFrom(snapshot.DeepCopy())
	meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
		Type:    StonesoupTestSuceededCondition,
		Status:  metav1.ConditionFalse,
		Reason:  StonesoupTestSuceededConditionFailed,
		Message: message,
	})
	SetSnapshotIntegrationStatusAsFinished(snapshot, "Marking snapshot integration status condition as finished since the testing fails")
	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// SetSnapshotIntegrationStatusAsInvalid sets the Stonesoup integration status condition for the Snapshot to invalid.
func SetSnapshotIntegrationStatusAsInvalid(snapshot *applicationapiv1alpha1.Snapshot, message string) {
	meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
		Type:    StonesoupIntegrationStatusCondition,
		Status:  metav1.ConditionFalse,
		Reason:  StonesoupIntegrationStatusInvalid,
		Message: message,
	})
}

// MarkSnapshotIntegrationStatusAsInProgress sets the Stonesoup integration status condition for the Snapshot to In Progress.
func MarkSnapshotIntegrationStatusAsInProgress(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
	patch := client.MergeFrom(snapshot.DeepCopy())
	meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
		Type:    StonesoupIntegrationStatusCondition,
		Status:  metav1.ConditionUnknown,
		Reason:  StonesoupIntegrationStatusInProgress,
		Message: message,
	})
	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// SetSnapshotIntegrationStatusAsFinished sets the Stonesoup integration status condition for the Snapshot to Finished.
func SetSnapshotIntegrationStatusAsFinished(snapshot *applicationapiv1alpha1.Snapshot, message string) {
	meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
		Type:    StonesoupIntegrationStatusCondition,
		Status:  metav1.ConditionTrue,
		Reason:  StonesoupIntegrationStatusFinished,
		Message: message,
	})
}

// HaveStonesoupTestsFinished checks if the Stonesoup tests have finished by checking if the Stonesoup Test Succeeded condition is set.
func HaveStonesoupTestsFinished(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return meta.FindStatusCondition(snapshot.Status.Conditions, StonesoupTestSuceededCondition) != nil
}

// HaveStonesoupTestsSucceeded checks if the Stonesoup tests have finished by checking if the Stonesoup Test Succeeded condition is set.
func HaveStonesoupTestsSucceeded(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return meta.IsStatusConditionTrue(snapshot.Status.Conditions, StonesoupTestSuceededCondition)
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

// FindMatchingSnapshot tries to find the expected Snapshot with the same set of images.
func FindMatchingSnapshot(adapterClient client.Client, ctx context.Context, application *applicationapiv1alpha1.Application, expectedSnapshot *applicationapiv1alpha1.Snapshot) (*applicationapiv1alpha1.Snapshot, error) {
	allSnapshots, err := GetAllSnapshots(adapterClient, ctx, application)
	if err != nil {
		return nil, err
	}

	for _, foundSnapshot := range *allSnapshots {
		foundSnapshot := foundSnapshot
		if CompareSnapshots(expectedSnapshot, &foundSnapshot) {
			return &foundSnapshot, nil
		}
	}
	return nil, nil
}

// GetAllSnapshots returns all Snapshots in the Application's namespace nil if it's not found.
// In the case the List operation fails, an error will be returned.
func GetAllSnapshots(adapterClient client.Client, ctx context.Context, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.Snapshot, error) {
	snapshots := &applicationapiv1alpha1.SnapshotList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := adapterClient.List(ctx, snapshots, opts...)
	if err != nil {
		return nil, err
	}

	return &snapshots.Items, nil
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
			if expectedSnapshotComponent == foundSnapshotComponent {
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
	return helpers.HasLabelWithValue(snapshot, PipelineAsCodeEventTypeLabel, PipelineAsCodePullRequestType)
}
