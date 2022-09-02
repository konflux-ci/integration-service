package gitops

import (
	"context"
	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ApplicationSnapshotTypeLabel contains the type of the ApplicationSnapshot.
	ApplicationSnapshotTypeLabel = "test.appstudio.openshift.io/type"

	// ApplicationSnapshotComponentLabel contains the name of the updated ApplicationSnapshot component.
	ApplicationSnapshotComponentLabel = "test.appstudio.openshift.io/component"

	// ApplicationSnapshotComponentType is the type of ApplicationSnapshot which was created for a single component build.
	ApplicationSnapshotComponentType = "component"

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
func MarkSnapshotAsPassed(adapterClient client.Client, ctx context.Context, applicationSnapshot *appstudioshared.ApplicationSnapshot, message string) (*appstudioshared.ApplicationSnapshot, error) {
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
func MarkSnapshotAsFailed(adapterClient client.Client, ctx context.Context, applicationSnapshot *appstudioshared.ApplicationSnapshot, message string) (*appstudioshared.ApplicationSnapshot, error) {
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
func SetSnapshotIntegrationStatusAsInvalid(applicationSnapshot *appstudioshared.ApplicationSnapshot, message string) {
	meta.SetStatusCondition(&applicationSnapshot.Status.Conditions, metav1.Condition{
		Type:    HACBSIntegrationStatusCondition,
		Status:  metav1.ConditionFalse,
		Reason:  HACBSIntegrationStatusInvalid,
		Message: message,
	})
}

// HaveHACBSTestsFinished checks if the HACBS tests have finished by checking if the HACBS Test Succeeded condition is set.
func HaveHACBSTestsFinished(applicationSnapshot *appstudioshared.ApplicationSnapshot) bool {
	return meta.FindStatusCondition(applicationSnapshot.Status.Conditions, HACBSTestSuceededCondition) != nil
}

// HaveHACBSTestsSucceeded checks if the HACBS tests have finished by checking if the HACBS Test Succeeded condition is set.
func HaveHACBSTestsSucceeded(applicationSnapshot *appstudioshared.ApplicationSnapshot) bool {
	return meta.IsStatusConditionTrue(applicationSnapshot.Status.Conditions, HACBSTestSuceededCondition)
}

// CreateApplicationSnapshot creates a new applicationSnapshot based on the supplied application and components
func CreateApplicationSnapshot(application *hasv1alpha1.Application, components *[]appstudioshared.ApplicationSnapshotComponent) *appstudioshared.ApplicationSnapshot {
	applicationSnapshot := &appstudioshared.ApplicationSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: application.Name + "-",
			Namespace:    application.Namespace,
		},
		Spec: appstudioshared.ApplicationSnapshotSpec{
			Application: application.Name,
			Components:  *components,
		},
	}
	return applicationSnapshot
}

// FindMatchingApplicationSnapshot tries to find the expected ApplicationSnapshot with the same set of images.
func FindMatchingApplicationSnapshot(adapterClient client.Client, ctx context.Context, application *hasv1alpha1.Application, expectedApplicationSnapshot *appstudioshared.ApplicationSnapshot) (*appstudioshared.ApplicationSnapshot, error) {
	allApplicationSnapshots, err := GetAllApplicationSnapshots(adapterClient, ctx, application)
	if err != nil {
		return nil, err
	}

	for _, foundApplicationSnapshot := range *allApplicationSnapshots {
		foundApplicationSnapshot := foundApplicationSnapshot
		if compareApplicationSnapshots(expectedApplicationSnapshot, &foundApplicationSnapshot) {
			return &foundApplicationSnapshot, nil
		}
	}
	return nil, nil
}

// GetAllApplicationSnapshots returns all ApplicationSnapshots in the Application's namespace nil if it's not found.
// In the case the List operation fails, an error will be returned.
func GetAllApplicationSnapshots(adapterClient client.Client, ctx context.Context, application *hasv1alpha1.Application) (*[]appstudioshared.ApplicationSnapshot, error) {
	applicationSnapshots := &appstudioshared.ApplicationSnapshotList{}
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

// compareApplicationSnapshots compares two ApplicationSnapshots and returns boolean true if their images match exactly.
func compareApplicationSnapshots(expectedApplicationSnapshot *appstudioshared.ApplicationSnapshot, foundApplicationSnapshot *appstudioshared.ApplicationSnapshot) bool {
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
