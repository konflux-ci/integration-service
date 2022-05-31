package release

import (
	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateApplicationSnapshot creates an ApplicationSnapshot for the given application components and the build pipelineRun.
func CreateApplicationSnapshot(application *hasv1alpha1.Application, images *[]releasev1alpha1.Image) (*releasev1alpha1.ApplicationSnapshot, error) {
	return &releasev1alpha1.ApplicationSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: application.Name + "-",
			Namespace:    application.Namespace,
		},
		Spec: releasev1alpha1.ApplicationSnapshotSpec{
			Images: *images,
		},
	}, nil
}

// CreateRelease creates a Release for the given ApplicationSnapshot and the ReleaseLink.
func CreateRelease(applicationSnapshot *releasev1alpha1.ApplicationSnapshot, releaseLink *releasev1alpha1.ReleaseLink) (*releasev1alpha1.Release, error) {
	return &releasev1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: applicationSnapshot.Name + "-",
			Namespace:    applicationSnapshot.Namespace,
		},
		Spec: releasev1alpha1.ReleaseSpec{
			ApplicationSnapshot: applicationSnapshot.Name,
			ReleaseLink:         releaseLink.Name,
		},
	}, nil
}
