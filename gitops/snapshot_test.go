package gitops_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Gitops functions for managing Snapshots", Ordered, func() {

	var (
		hasApp      *applicationapiv1alpha1.Application
		hasComp     *applicationapiv1alpha1.Component
		hasSnapshot *applicationapiv1alpha1.Snapshot
		sampleImage string
	)

	const (
		SampleRepoLink  = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		namespace       = "default"
		applicationName = "application-sample"
		componentName   = "component-sample"
		snapshotName    = "snapshot-sample"
	)

	BeforeAll(func() {
		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
				Namespace: namespace,
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())
		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  componentName,
				Application:    applicationName,
				ContainerImage: "",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: SampleRepoLink,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())
	})

	BeforeEach(func() {
		sampleImage = "quay.io/redhat-appstudio/sample-image:latest"

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotName,
				Namespace: namespace,
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:               gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel:          componentName,
					gitops.BuildPipelineRunFinishTimeLabel: "1675992257",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           componentName,
						ContainerImage: sampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasSnapshot.Name,
				Namespace: namespace,
			}, hasSnapshot)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("ensures the Snapshots status can be marked as passed", func() {
		updatedSnapshot, err := gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "Test message")
		Expect(err == nil).To(BeTrue())
		Expect(updatedSnapshot != nil).To(BeTrue())
		Expect(updatedSnapshot.Status.Conditions != nil).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(updatedSnapshot.Status.Conditions, gitops.HACBSTestSuceededCondition)).To(BeTrue())
	})

	It("ensures the Snapshots status can be marked as failed", func() {
		updatedSnapshot, err := gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "Test message")
		Expect(err == nil).To(BeTrue())
		Expect(updatedSnapshot != nil).To(BeTrue())
		Expect(updatedSnapshot.Status.Conditions != nil).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(updatedSnapshot.Status.Conditions, gitops.HACBSTestSuceededCondition)).To(BeFalse())
	})

	It("ensures the Snapshots status can be marked as invalid", func() {
		gitops.SetSnapshotIntegrationStatusAsInvalid(hasSnapshot, "Test message")
		Expect(hasSnapshot != nil).To(BeTrue())
		Expect(hasSnapshot.Status.Conditions != nil).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.HACBSIntegrationStatusCondition)).To(BeFalse())
	})

	It("ensures the Snapshots status can be marked as finished", func() {
		gitops.SetSnapshotIntegrationStatusAsFinished(hasSnapshot, "Test message")
		Expect(hasSnapshot.Status.Conditions != nil).To(BeTrue())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.HACBSIntegrationStatusCondition)).To(BeTrue())
		foundStatusCondition := meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.HACBSIntegrationStatusCondition)
		Expect(foundStatusCondition.Reason == gitops.HACBSIntegrationStatusFinished).To(BeTrue())
	})

	It("ensures the Snapshots status can be marked as in progress", func() {
		updatedSnapshot, err := gitops.MarkSnapshotIntegrationStatusAsInProgress(k8sClient, ctx, hasSnapshot, "Test message")
		Expect(err == nil).To(BeTrue())
		Expect(updatedSnapshot != nil).To(BeTrue())
		Expect(updatedSnapshot.Status.Conditions != nil).To(BeTrue())
		foundStatusCondition := meta.FindStatusCondition(updatedSnapshot.Status.Conditions, gitops.HACBSIntegrationStatusCondition)
		Expect(foundStatusCondition.Reason == gitops.HACBSIntegrationStatusInProgress).To(BeTrue())
	})

	It("ensures the Snapshots can be checked for the HACBSTestSuceededCondition", func() {
		checkResult := gitops.HaveHACBSTestsFinished(hasSnapshot)
		Expect(checkResult).To(BeFalse())
	})

	It("ensures the Snapshots can be checked for the HACBSTestSuceededCondition", func() {
		checkResult := gitops.HaveHACBSTestsSucceeded(hasSnapshot)
		Expect(checkResult).To(BeFalse())
	})

	It("ensures that a new Snapshots can be successfully created", func() {
		snapshotComponents := []applicationapiv1alpha1.SnapshotComponent{}
		createdSnapshot := gitops.NewSnapshot(hasApp, &snapshotComponents)
		Expect(createdSnapshot != nil).To(BeTrue())
	})

	It("ensures a matching Snapshot can be found", func() {
		expectedSnapshot := hasSnapshot.DeepCopy()
		foundSnapshot, err := gitops.FindMatchingSnapshot(k8sClient, ctx, hasApp, expectedSnapshot)
		Expect(err == nil).To(BeTrue())
		Expect(foundSnapshot != nil).To(BeTrue())
		Expect(foundSnapshot.Name == hasSnapshot.Name).To(BeTrue())
	})

	It("ensures that all Snapshots for a given application can be found", func() {
		snapshots, err := gitops.GetAllSnapshots(k8sClient, ctx, hasApp)
		Expect(err == nil).To(BeTrue())
		Expect(snapshots != nil).To(BeTrue())
	})

	It("ensures the same Snapshots can be successfully compared", func() {
		expectedSnapshot := hasSnapshot.DeepCopy()
		comparisonResult := gitops.CompareSnapshots(hasSnapshot, expectedSnapshot)
		Expect(comparisonResult).To(BeTrue())
	})

	It("ensures the different Snapshots can be compared and the difference is detected", func() {
		expectedSnapshot := hasSnapshot.DeepCopy()
		newSnapshotComponent := applicationapiv1alpha1.SnapshotComponent{
			Name:           "temporaryComponent",
			ContainerImage: sampleImage,
		}
		expectedSnapshot.Spec.Components = append(expectedSnapshot.Spec.Components, newSnapshotComponent)
		comparisonResult := gitops.CompareSnapshots(hasSnapshot, expectedSnapshot)
		Expect(comparisonResult).To(BeFalse())
	})

})
