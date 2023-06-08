package release_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	integrationservicerelease "github.com/redhat-appstudio/integration-service/release"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	releasemetadata "github.com/redhat-appstudio/release-service/metadata"
	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Release functions for managing Releases", Ordered, func() {

	var (
		hasSnapshot *applicationapiv1alpha1.Snapshot
		hasApp      *applicationapiv1alpha1.Application
		releasePlan *releasev1alpha1.ReleasePlan
	)

	const (
		namespace = "default"
	)

	BeforeAll(func() {
		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "application-sample",
				Namespace: namespace,
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "snapshot-sample-",
				Namespace:    namespace,
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "application-sample",
				Components:  []applicationapiv1alpha1.SnapshotComponent{},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		releasePlan = &releasev1alpha1.ReleasePlan{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "releaseplan-sample-",
				Namespace:    namespace,
				Labels: map[string]string{
					releasemetadata.AutoReleaseLabel: "true",
				},
			},
			Spec: releasev1alpha1.ReleasePlanSpec{
				Application: "application-sample",
				Target:      "default",
			},
		}
		Expect(k8sClient.Create(ctx, releasePlan)).Should(Succeed())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, releasePlan)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("ensures the Release can be created for ReleasePlan and is labelled as automated", func() {
		createdRelease := integrationservicerelease.NewReleaseForReleasePlan(releasePlan, hasSnapshot)
		Expect(createdRelease.Spec.ReleasePlan).To(Equal(releasePlan.Name))
		Expect(createdRelease.GetLabels()[releasemetadata.AutomatedLabel]).To(Equal("true"))
	})

	It("ensures the matching Release can be found for ReleasePlan", func() {
		releases := []releasev1alpha1.Release{
			{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "release-sample-",
					Namespace:    namespace,
				},
				Spec: releasev1alpha1.ReleaseSpec{
					Snapshot:    hasSnapshot.GetName(),
					ReleasePlan: releasePlan.GetName(),
				},
			},
		}
		foundMatchingRelease := integrationservicerelease.FindMatchingReleaseWithReleasePlan(&releases, *releasePlan)
		Expect(foundMatchingRelease.Spec.ReleasePlan).To(Equal(releasePlan.Name))
	})

	It("ensures the ReleasePlan can be gotten for Application", func() {
		gottenReleasePlanItems, err := integrationservicerelease.GetAutoReleasePlansForApplication(k8sClient, ctx, hasApp)
		Expect(err).To(BeNil())
		Expect(gottenReleasePlanItems).NotTo(BeNil())
	})
})
