package gitops_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"time"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		SampleCommit    = "a2ba645d50e471d5f084b"
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
							URL:      SampleRepoLink,
							Revision: SampleCommit,
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

	It("ensures the a decision can be made to NOT promote when the snaphot has not been marked as passed/failed", func() {
		canBePromoted, reasons := gitops.CanSnapshotBePromoted(hasSnapshot)
		Expect(canBePromoted).To(BeFalse())
		Expect(reasons).To(HaveLen(1))
		Expect(reasons[0]).To(Equal("the Snapshot has not yet finished testing"))
	})

	It("ensures the Snapshots status can be marked as passed", func() {
		updatedSnapshot, err := gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "Test message")
		Expect(err).To(BeNil())
		Expect(updatedSnapshot).NotTo(BeNil())
		Expect(updatedSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(updatedSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeTrue())
		Expect(gitops.IsSnapshotStatusConditionSet(hasSnapshot, gitops.AppStudioTestSucceededCondition, metav1.ConditionTrue, "")).To(BeTrue())
	})

	It("ensures the Snapshots LegacyTestSucceededCondition status can be marked as passed", func() {
		patch := client.MergeFrom(hasSnapshot.DeepCopy())
		condition := metav1.Condition{
			Type:    gitops.LegacyTestSucceededCondition,
			Status:  metav1.ConditionTrue,
			Reason:  gitops.AppStudioTestSucceededConditionPassed,
			Message: "Test message",
		}
		meta.SetStatusCondition(&hasSnapshot.Status.Conditions, condition)

		err := k8sClient.Status().Patch(ctx, hasSnapshot, patch)
		Expect(err).To(BeNil())
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.LegacyTestSucceededCondition)).To(BeTrue())
		Expect(gitops.IsSnapshotStatusConditionSet(hasSnapshot, gitops.AppStudioTestSucceededCondition, metav1.ConditionTrue, "")).To(BeTrue())
		Expect(gitops.IsSnapshotStatusConditionSet(hasSnapshot, gitops.AppStudioTestSucceededCondition, metav1.ConditionFalse, "")).To(BeFalse())
	})

	It("ensures the Snapshots LegacyIntegrationStatusCondition status can be marked as invalid", func() {
		patch := client.MergeFrom(hasSnapshot.DeepCopy())
		condition := metav1.Condition{
			Type:    gitops.LegacyIntegrationStatusCondition,
			Status:  metav1.ConditionFalse,
			Reason:  gitops.AppStudioIntegrationStatusInvalid,
			Message: "Test message",
		}
		meta.SetStatusCondition(&hasSnapshot.Status.Conditions, condition)

		err := k8sClient.Status().Patch(ctx, hasSnapshot, patch)
		Expect(err).To(BeNil())
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.LegacyIntegrationStatusCondition)).To(BeFalse())
		Expect(gitops.IsSnapshotStatusConditionSet(hasSnapshot, gitops.AppStudioIntegrationStatusCondition, metav1.ConditionFalse, "Invalid")).To(BeTrue())
		Expect(gitops.IsSnapshotStatusConditionSet(hasSnapshot, gitops.AppStudioIntegrationStatusCondition, metav1.ConditionFalse, "Valid")).To(BeFalse())
	})

	It("ensures the Snapshots status can be marked as failed", func() {
		updatedSnapshot, err := gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "Test message")
		Expect(err).To(BeNil())
		Expect(updatedSnapshot).NotTo(BeNil())
		Expect(updatedSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(updatedSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeFalse())
		Expect(gitops.IsSnapshotStatusConditionSet(hasSnapshot, gitops.AppStudioTestSucceededCondition, metav1.ConditionFalse, "")).To(BeTrue())
	})

	It("ensures the Snapshots status can be marked as error", func() {
		gitops.SetSnapshotIntegrationStatusAsError(hasSnapshot, "Test message")
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)).To(BeFalse())
		Expect(gitops.IsSnapshotError(hasSnapshot)).To(BeTrue())
	})

	It("ensures the Snapshots status can be marked as finished", func() {
		gitops.SetSnapshotIntegrationStatusAsFinished(hasSnapshot, "Test message")
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)).To(BeTrue())
		foundStatusCondition := meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)
		Expect(foundStatusCondition.Reason).To(Equal(gitops.AppStudioIntegrationStatusFinished))
	})

	It("ensures the Snapshots status can be marked as in progress", func() {
		updatedSnapshot, err := gitops.MarkSnapshotIntegrationStatusAsInProgress(k8sClient, ctx, hasSnapshot, "Test message")
		Expect(err).To(BeNil())
		Expect(updatedSnapshot).NotTo(BeNil())
		Expect(updatedSnapshot.Status.Conditions).NotTo(BeNil())
		foundStatusCondition := meta.FindStatusCondition(updatedSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)
		Expect(foundStatusCondition.Reason).To(Equal(gitops.AppStudioIntegrationStatusInProgress))
	})

	It("ensures the Snapshots can be checked for the AppStudioTestSucceededCondition", func() {
		checkResult := gitops.HaveAppStudioTestsFinished(hasSnapshot)
		Expect(checkResult).To(BeFalse())
	})

	It("ensures the Snapshots can be checked for the AppStudioTestSucceededCondition", func() {
		checkResult := gitops.HaveAppStudioTestsSucceeded(hasSnapshot)
		Expect(checkResult).To(BeFalse())
	})

	It("ensures that a new Snapshots can be successfully created", func() {
		snapshotComponents := []applicationapiv1alpha1.SnapshotComponent{}
		createdSnapshot := gitops.NewSnapshot(hasApp, &snapshotComponents)
		Expect(createdSnapshot).NotTo(BeNil())
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

	It("ensures the Snapshots status can be detected to be invalid", func() {
		gitops.SetSnapshotIntegrationStatusAsInvalid(hasSnapshot, "Test message")
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(gitops.IsSnapshotValid(hasSnapshot)).To(BeFalse())
		Expect(gitops.IsSnapshotStatusConditionSet(hasSnapshot, gitops.AppStudioIntegrationStatusCondition,
			metav1.ConditionFalse, gitops.AppStudioIntegrationStatusInvalid)).To(BeTrue())
	})

	It("ensures the Snapshots status can be detected to be valid", func() {
		gitops.SetSnapshotIntegrationStatusAsFinished(hasSnapshot, "Test message")
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(gitops.IsSnapshotValid(hasSnapshot)).To(BeTrue())
	})

	It("ensures the a decision can be made to promote the Snapshot based on its status", func() {
		gitops.SetSnapshotIntegrationStatusAsFinished(hasSnapshot, "Test message")
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())

		updatedSnapshot, err := gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "Test message")
		Expect(err).To(BeNil())
		Expect(updatedSnapshot).NotTo(BeNil())
		Expect(updatedSnapshot.Status.Conditions).NotTo(BeNil())

		canBePromoted, reasons := gitops.CanSnapshotBePromoted(updatedSnapshot)
		Expect(canBePromoted).To(BeTrue())
		Expect(reasons).To(BeEmpty())
	})

	It("ensures the a decision can be made to NOT promote the Snapshot based on its status", func() {
		gitops.SetSnapshotIntegrationStatusAsFinished(hasSnapshot, "Test message")
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())

		updatedSnapshot, err := gitops.MarkSnapshotAsFailed(k8sClient, ctx, hasSnapshot, "Test message")
		Expect(err).To(BeNil())
		Expect(updatedSnapshot).NotTo(BeNil())
		Expect(updatedSnapshot.Status.Conditions).NotTo(BeNil())

		canBePromoted, reasons := gitops.CanSnapshotBePromoted(updatedSnapshot)
		Expect(canBePromoted).To(BeFalse())
		Expect(reasons).To(HaveLen(1))

		updatedSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodePullRequestType
		canBePromoted, reasons = gitops.CanSnapshotBePromoted(updatedSnapshot)
		Expect(canBePromoted).To(BeFalse())
		Expect(reasons).To(HaveLen(2))

		gitops.SetSnapshotIntegrationStatusAsInvalid(updatedSnapshot, "Test message")
		canBePromoted, reasons = gitops.CanSnapshotBePromoted(updatedSnapshot)
		Expect(canBePromoted).To(BeFalse())
		Expect(reasons).To(HaveLen(3))
	})

	It("Return false when the image url contains invalid digest", func() {
		imageUrl := "quay.io/redhat-appstudio/sample-image:latest"
		Expect(gitops.ValidateImageDigest(imageUrl)).NotTo(BeNil())
	})

	It("Return true when the image url contains valid digest", func() {
		// Prepare a valid image with digest
		imageUrl := "quay.io/redhat-appstudio/sample-image@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		Expect(gitops.ValidateImageDigest(imageUrl)).To(BeNil())
	})

	It("ensure snapshot can be prepared for pipelinerun ", func() {
		imagePullSpec := "quay.io/redhat-appstudio/sample-image@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		componentSource := &applicationapiv1alpha1.ComponentSource{
			ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
				GitSource: &applicationapiv1alpha1.GitSource{
					URL:      SampleRepoLink,
					Revision: SampleCommit,
				},
			},
		}
		allApplicationComponents := &[]applicationapiv1alpha1.Component{*hasComp}
		snapshot, err := gitops.PrepareSnapshot(k8sClient, ctx, hasApp, allApplicationComponents, hasComp, imagePullSpec, componentSource)
		Expect(snapshot).NotTo(BeNil())
		Expect(err).To(BeNil())
		Expect(snapshot.Spec.Components).To(HaveLen(1), "One component should have been added to snapshot.  Other component should have been omited due to empty ContainerImage field or missing valid digest")
		Expect(snapshot.Spec.Components[0].Name).To(Equal(hasComp.Name), "The built component should have been added to the snapshot")
	})

	It("Return false when the image url contains invalid digest", func() {
		imageUrl := "quay.io/redhat-appstudio/sample-image:latest"
		Expect(gitops.ValidateImageDigest(imageUrl)).NotTo(BeNil())
	})

	It("ensure ComponentSource can returned when component have Status.LastBuiltCommit defined or not", func() {
		componentSource := gitops.GetComponentSourceFromComponent(hasComp)
		Expect(componentSource.GitSource.Revision).To(Equal("a2ba645d50e471d5f084b"))

		hasComp.Status = applicationapiv1alpha1.ComponentStatus{
			LastBuiltCommit: "lastbuildcommit",
		}
		//Expect(k8sClient.Status().Update(ctx, hasComp)).Should(Succeed())
		componentSource = gitops.GetComponentSourceFromComponent(hasComp)
		Expect(componentSource.GitSource.Revision).To(Equal("lastbuildcommit"))
	})

	It("ensure existing snapshot can be found", func() {
		allSnapshots := &[]applicationapiv1alpha1.Snapshot{*hasSnapshot}
		existingSnapshot := gitops.FindMatchingSnapshot(hasApp, allSnapshots, hasSnapshot)
		Expect(existingSnapshot.Name).To(Equal(hasSnapshot.Name))
	})

	Context("TestStatus type tests", func() {
		DescribeTable("Status to string and vice versa",
			func(st gitops.IntegrationTestStatus, expectedStr string) {
				strRepr := st.String()
				Expect(strRepr).To(Equal(expectedStr))
				controlStatus, err := gitops.IntegrationTestStatusString(strRepr)
				Expect(err).To(BeNil())
				Expect(controlStatus).To(Equal(st))
			},
			Entry("When status is Pending", gitops.IntegrationTestStatusPending, "Pending"),
			Entry("When status is InProgress", gitops.IntegrationTestStatusInProgress, "InProgress"),
			Entry("When status is EnvironmentProvisionError", gitops.IntegrationTestStatusEnvironmentProvisionError, "EnvironmentProvisionError"),
			Entry("When status is DeploymentError", gitops.IntegrationTestStatusDeploymentError, "DeploymentError"),
			Entry("When status is TestFail", gitops.IntegrationTestStatusTestFail, "TestFail"),
			Entry("When status is TestPass", gitops.IntegrationTestStatusTestPassed, "TestPassed"),
		)

		DescribeTable("Status to JSON and vice versa",
			func(st gitops.IntegrationTestStatus, expectedStr string) {
				jsonRepr, err := st.MarshalJSON()
				Expect(err).To(BeNil())
				Expect(jsonRepr).To(Equal([]byte("\"" + expectedStr + "\"")))

				var controlStatus gitops.IntegrationTestStatus
				err = controlStatus.UnmarshalJSON(jsonRepr)
				Expect(err).To(BeNil())
				Expect(controlStatus).To(Equal(st))
			},
			Entry("When status is Pending", gitops.IntegrationTestStatusPending, "Pending"),
			Entry("When status is InProgress", gitops.IntegrationTestStatusInProgress, "InProgress"),
			Entry("When status is EnvironmentProvisionError", gitops.IntegrationTestStatusEnvironmentProvisionError, "EnvironmentProvisionError"),
			Entry("When status is DeploymentError", gitops.IntegrationTestStatusDeploymentError, "DeploymentError"),
			Entry("When status is TestFail", gitops.IntegrationTestStatusTestFail, "TestFail"),
			Entry("When status is TestPass", gitops.IntegrationTestStatusTestPassed, "TestPassed"),
		)

		DescribeTable("Status is final",
			func(st gitops.IntegrationTestStatus, isFinal bool) {
				Expect(st.IsFinal()).To(Equal(isFinal))
			},
			Entry("When status is Pending", gitops.IntegrationTestStatusPending, false),
			Entry("When status is InProgress", gitops.IntegrationTestStatusInProgress, false),
			Entry("When status is EnvironmentProvisionError", gitops.IntegrationTestStatusEnvironmentProvisionError, true),
			Entry("When status is DeploymentError", gitops.IntegrationTestStatusDeploymentError, true),
			Entry("When status is TestFail", gitops.IntegrationTestStatusTestFail, true),
			Entry("When status is TestPass", gitops.IntegrationTestStatusTestPassed, true),
		)

		It("Invalid status to type fails with error", func() {
			_, err := gitops.IntegrationTestStatusString("Unknown")
			Expect(err).NotTo(BeNil())
		})

		It("Invalid JSON status to type fails with error", func() {
			const unknownJson = "\"Unknown\""
			var controlStatus gitops.IntegrationTestStatus
			err := controlStatus.UnmarshalJSON([]byte(unknownJson))
			Expect(err).NotTo(BeNil())
		})
	})
})
