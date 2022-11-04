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
	// ApplicationSnapshotTypeLabel contains the type of the ApplicationSnapshot.
	ApplicationSnapshotTypeLabel = "test.appstudio.openshift.io/type"

	// ApplicationSnapshotComponentLabel contains the name of the updated ApplicationSnapshot component.
	ApplicationSnapshotComponentLabel = "test.appstudio.openshift.io/component"

	// ApplicationSnapshotTestScenarioLabel contains the name of the ApplicationSnapshot test scenario.
	ApplicationSnapshotTestScenarioLabel = "test.appstudio.openshift.io/scenario"

	// ApplicationSnapshotComponentType is the type of ApplicationSnapshot which was created for a single component build.
	ApplicationSnapshotComponentType = "component"

	// ApplicationSnapshotCompositeType is the type of ApplicationSnapshot which was created for multiple components.
	ApplicationSnapshotCompositeType = "composite"

	// PipelineAscodeEventType is the type of event which triggered the pipelinerun in build service
	PipelineAsCodeEventTypeLabel = "pipelinesascode.tekton.dev/event-type"

	// PipelineAsCodeGitProviderLabel is the git provider which triggered the pipelinerun in build service.
	PipelineAsCodeGitProviderLabel = "pipelinesascode.tekton.dev/git-provider"

	// PipelineAsCodeSHALabel is the commit which triggered the pipelinerun in build service.
	PipelineAsCodeSHALabel = "pipelinesascode.tekton.dev/sha"

	// PipelineAsCodeURLOrgLabel is the organization for the git repo which triggered the pipelinerun in build service.
	PipelineAsCodeURLOrgLabel = "pipelinesascode.tekton.dev/url-org"

	// PipelineAsCodeURLRepositoryLabel is the git repository which triggered the pipelinerun in build service.
	PipelineAsCodeURLRepositoryLabel = "pipelinesascode.tekton.dev/url-repository"

	// PipelineAsCodeRepoURLAnnotation is the URL to the git repository which triggered the pipelinerun in build service.
	PipelineAsCodeRepoURLAnnotation = "pipelinesascode.tekton.dev/repo-url"

	// PipelineAsCodeInstallationIDAnnotation is the GitHub App installation ID for the git repo which triggered the pipelinerun in build service.
	PipelineAsCodeInstallationIDAnnotation = "pipelinesascode.tekton.dev/installation-id"

	// PipelineAsCodePullRequestAnnotation is the git repository's pull request identifier
	PipelineAsCodePullRequestAnnotation = "pipelinesascode.tekton.dev/pull-request"

	// PipelineAscodePushType is the type of push event which triggered the pipelinerun in build service
	PipelineAsCodePushType = "push"

	// PipelineAscodePushType is the type of pull_request event which triggered the pipelinerun in build service
	PipelineAsCodePullRequestType = "pull_request"

	// PipelineAsCodeGitHubProviderType is the git provider type for a GitHub event which triggered the pipelinerun in build service.
	PipelineAsCodeGitHubProviderType = "github"

	//HACBSTestSuceededCondition is the condition for marking if the HACBS Tests succeeded for the ApplicationSnapshot.
	HACBSTestSuceededCondition = "HACBSTestSucceeded"

	// HACBSIntegrationStatusCondition is the condition for marking the HACBS integration status of the ApplicationSnapshot.
	HACBSIntegrationStatusCondition = "HACBSIntegrationStatus"

	// HACBSTestSuceededConditionPassed is the reason that's set when the HACBS tests succeed.
	HACBSTestSuceededConditionPassed = "Passed"

	// HACBSTestSuceededConditionFailed is the reason that's set when the HACBS tests fail.
	HACBSTestSuceededConditionFailed = "Failed"

	// HACBSIntegrationStatusInvalid is the reason that's set when the HACBS integration gets into an invalid state.
	HACBSIntegrationStatusInvalid = "Invalid"
)

// MarkSnapshotAsPassed updates the HACBS Test succeeded condition for the ApplicationSnapshot to passed.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsPassed(adapterClient client.Client, ctx context.Context, applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot, message string) (*applicationapiv1alpha1.ApplicationSnapshot, error) {
	patch := client.MergeFrom(applicationSnapshot.DeepCopy())
	meta.SetStatusCondition(&applicationSnapshot.Status.Conditions, metav1.Condition{
		Type:    HACBSTestSuceededCondition,
		Status:  metav1.ConditionTrue,
		Reason:  HACBSTestSuceededConditionPassed,
		Message: message,
	})
	err := adapterClient.Status().Patch(ctx, applicationSnapshot, patch)
	if err != nil {
		return nil, err
	}
	return applicationSnapshot, nil
}

// MarkSnapshotAsFailed updates the HACBS Test succeeded condition for the ApplicationSnapshot to failed.
// If the patch command fails, an error will be returned.
func MarkSnapshotAsFailed(adapterClient client.Client, ctx context.Context, applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot, message string) (*applicationapiv1alpha1.ApplicationSnapshot, error) {
	patch := client.MergeFrom(applicationSnapshot.DeepCopy())
	meta.SetStatusCondition(&applicationSnapshot.Status.Conditions, metav1.Condition{
		Type:    HACBSTestSuceededCondition,
		Status:  metav1.ConditionFalse,
		Reason:  HACBSTestSuceededConditionFailed,
		Message: message,
	})
	err := adapterClient.Status().Patch(ctx, applicationSnapshot, patch)
	if err != nil {
		return nil, err
	}
	return applicationSnapshot, nil
}

// SetSnapshotIntegrationStatusAsInvalid sets the HACBS integration status condition for the ApplicationSnapshot to invalid.
func SetSnapshotIntegrationStatusAsInvalid(applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot, message string) {
	meta.SetStatusCondition(&applicationSnapshot.Status.Conditions, metav1.Condition{
		Type:    HACBSIntegrationStatusCondition,
		Status:  metav1.ConditionFalse,
		Reason:  HACBSIntegrationStatusInvalid,
		Message: message,
	})
}

// HaveHACBSTestsFinished checks if the HACBS tests have finished by checking if the HACBS Test Succeeded condition is set.
func HaveHACBSTestsFinished(applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot) bool {
	return meta.FindStatusCondition(applicationSnapshot.Status.Conditions, HACBSTestSuceededCondition) != nil
}

// HaveHACBSTestsSucceeded checks if the HACBS tests have finished by checking if the HACBS Test Succeeded condition is set.
func HaveHACBSTestsSucceeded(applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot) bool {
	return meta.IsStatusConditionTrue(applicationSnapshot.Status.Conditions, HACBSTestSuceededCondition)
}

// CreateApplicationSnapshot creates a new applicationSnapshot based on the supplied application and components
func CreateApplicationSnapshot(application *applicationapiv1alpha1.Application, snapshotComponents *[]applicationapiv1alpha1.ApplicationSnapshotComponent) *applicationapiv1alpha1.ApplicationSnapshot {
	applicationSnapshot := &applicationapiv1alpha1.ApplicationSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: application.Name + "-",
			Namespace:    application.Namespace,
		},
		Spec: applicationapiv1alpha1.ApplicationSnapshotSpec{
			Application: application.Name,
			Components:  *snapshotComponents,
		},
	}
	return applicationSnapshot
}

// FindMatchingApplicationSnapshot tries to find the expected ApplicationSnapshot with the same set of images.
func FindMatchingApplicationSnapshot(adapterClient client.Client, ctx context.Context, application *applicationapiv1alpha1.Application, expectedApplicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot) (*applicationapiv1alpha1.ApplicationSnapshot, error) {
	allApplicationSnapshots, err := GetAllApplicationSnapshots(adapterClient, ctx, application)
	if err != nil {
		return nil, err
	}

	for _, foundApplicationSnapshot := range *allApplicationSnapshots {
		foundApplicationSnapshot := foundApplicationSnapshot
		if CompareApplicationSnapshots(expectedApplicationSnapshot, &foundApplicationSnapshot) {
			return &foundApplicationSnapshot, nil
		}
	}
	return nil, nil
}

// GetAllApplicationSnapshots returns all ApplicationSnapshots in the Application's namespace nil if it's not found.
// In the case the List operation fails, an error will be returned.
func GetAllApplicationSnapshots(adapterClient client.Client, ctx context.Context, application *applicationapiv1alpha1.Application) (*[]applicationapiv1alpha1.ApplicationSnapshot, error) {
	applicationSnapshots := &applicationapiv1alpha1.ApplicationSnapshotList{}
	opts := []client.ListOption{
		client.InNamespace(application.Namespace),
		client.MatchingFields{"spec.application": application.Name},
	}

	err := adapterClient.List(ctx, applicationSnapshots, opts...)
	if err != nil {
		return nil, err
	}

	return &applicationSnapshots.Items, nil
}

// CompareApplicationSnapshots compares two ApplicationSnapshots and returns boolean true if their images match exactly.
func CompareApplicationSnapshots(expectedApplicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot, foundApplicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot) bool {
	// Check if the snapshots are created by the same event type
	if IsSnapshotCreatedByPushEvent(expectedApplicationSnapshot) != IsSnapshotCreatedByPushEvent(foundApplicationSnapshot) {
		return false
	}
	// If the number of components doesn't match, we immediately know that the snapshots are not equal.
	if len(expectedApplicationSnapshot.Spec.Components) != len(foundApplicationSnapshot.Spec.Components) {
		return false
	}

	// Check if all Component information matches, including the containerImage status field
	for _, expectedSnapshotComponent := range expectedApplicationSnapshot.Spec.Components {
		foundImage := false
		for _, foundSnapshotComponent := range foundApplicationSnapshot.Spec.Components {
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

// IsSnapshotCreatedByPushEvent checks if an applicationSnapshot has label PipelineAsCodeEventTypeLabel and with push value
func IsSnapshotCreatedByPushEvent(applicationSnapshot *applicationapiv1alpha1.ApplicationSnapshot) bool {
	return helpers.HasLabelWithValue(applicationSnapshot, PipelineAsCodeEventTypeLabel, PipelineAsCodePushType)
}
