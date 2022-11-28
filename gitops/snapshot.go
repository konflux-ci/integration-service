package gitops

import (
	"context"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/helpers"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// PipelinesAsCodePrefix contains the prefix applied to labels and annotations copied from Pipelines as Code resources.
	PipelinesAsCodePrefix = "pac.test.appstudio.openshift.io"

	// SnapshotTypeLabel contains the type of the Snapshot.
	SnapshotTypeLabel = "test.appstudio.openshift.io/type"

	// SnapshotComponentLabel contains the name of the updated Snapshot component.
	SnapshotComponentLabel = "test.appstudio.openshift.io/component"

	// SnapshotTestScenarioLabel contains the name of the Snapshot test scenario.
	SnapshotTestScenarioLabel = "test.appstudio.openshift.io/scenario"

	// SnapshotComponentType is the type of Snapshot which was created for a single component build.
	SnapshotComponentType = "component"

	// SnapshotCompositeType is the type of Snapshot which was created for multiple components.
	SnapshotCompositeType = "composite"

	// PipelineAsCodeEventType is the type of event which triggered the pipelinerun in build service
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

	// PipelineAscodePushType is the type of push event which triggered the pipelinerun in build service
	PipelineAsCodePushType = "push"

	// PipelineAscodePushType is the type of pull_request event which triggered the pipelinerun in build service
	PipelineAsCodePullRequestType = "pull_request"

	// PipelineAsCodeGitHubProviderType is the git provider type for a GitHub event which triggered the pipelinerun in build service.
	PipelineAsCodeGitHubProviderType = "github"

	//HACBSTestSuceededCondition is the condition for marking if the HACBS Tests succeeded for the Snapshot.
	HACBSTestSuceededCondition = "HACBSTestSucceeded"

	// HACBSIntegrationStatusCondition is the condition for marking the HACBS integration status of the Snapshot.
	HACBSIntegrationStatusCondition = "HACBSIntegrationStatus"

	// HACBSTestSuceededConditionPassed is the reason that's set when the HACBS tests succeed.
	HACBSTestSuceededConditionPassed = "Passed"

	// HACBSTestSuceededConditionFailed is the reason that's set when the HACBS tests fail.
	HACBSTestSuceededConditionFailed = "Failed"

	// HACBSIntegrationStatusInvalid is the reason that's set when the HACBS integration gets into an invalid state.
	HACBSIntegrationStatusInvalid = "Invalid"
)

// MarkSnapshotAsPassed updates the HACBS Test succeeded condition for the Snapshot to passed.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsPassed(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
	patch := client.MergeFrom(snapshot.DeepCopy())
	meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
		Type:    HACBSTestSuceededCondition,
		Status:  metav1.ConditionTrue,
		Reason:  HACBSTestSuceededConditionPassed,
		Message: message,
	})
	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// MarkSnapshotAsFailed updates the HACBS Test succeeded condition for the Snapshot to failed.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsFailed(adapterClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, message string) (*applicationapiv1alpha1.Snapshot, error) {
	patch := client.MergeFrom(snapshot.DeepCopy())
	meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
		Type:    HACBSTestSuceededCondition,
		Status:  metav1.ConditionFalse,
		Reason:  HACBSTestSuceededConditionFailed,
		Message: message,
	})
	err := adapterClient.Status().Patch(ctx, snapshot, patch)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// SetSnapshotIntegrationStatusAsInvalid sets the HACBS integration status condition for the Snapshot to invalid.
func SetSnapshotIntegrationStatusAsInvalid(snapshot *applicationapiv1alpha1.Snapshot, message string) {
	meta.SetStatusCondition(&snapshot.Status.Conditions, metav1.Condition{
		Type:    HACBSIntegrationStatusCondition,
		Status:  metav1.ConditionFalse,
		Reason:  HACBSIntegrationStatusInvalid,
		Message: message,
	})
}

// HaveHACBSTestsFinished checks if the HACBS tests have finished by checking if the HACBS Test Succeeded condition is set.
func HaveHACBSTestsFinished(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return meta.FindStatusCondition(snapshot.Status.Conditions, HACBSTestSuceededCondition) != nil
}

// HaveHACBSTestsSucceeded checks if the HACBS tests have finished by checking if the HACBS Test Succeeded condition is set.
func HaveHACBSTestsSucceeded(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return meta.IsStatusConditionTrue(snapshot.Status.Conditions, HACBSTestSuceededCondition)
}

// CreateSnapshot creates a new snapshot based on the supplied application and components
func CreateSnapshot(application *applicationapiv1alpha1.Application, snapshotComponents *[]applicationapiv1alpha1.SnapshotComponent) *applicationapiv1alpha1.Snapshot {
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
	if IsSnapshotCreatedByPushEvent(expectedSnapshot) != IsSnapshotCreatedByPushEvent(foundSnapshot) {
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

// IsSnapshotCreatedByPushEvent checks if a snapshot has label PipelineAsCodeEventTypeLabel and with push value
func IsSnapshotCreatedByPushEvent(snapshot *applicationapiv1alpha1.Snapshot) bool {
	return helpers.HasLabelWithValue(snapshot, PipelineAsCodeEventTypeLabel, PipelineAsCodePushType)
}
