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

package gitops_test

import (
	"github.com/konflux-ci/integration-service/api/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"strconv"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Gitops functions for managing Snapshots", Ordered, func() {

	var (
		hasApp          *applicationapiv1alpha1.Application
		hasComp         *applicationapiv1alpha1.Component
		badComp         *applicationapiv1alpha1.Component
		hasSnapshot     *applicationapiv1alpha1.Snapshot
		hasComSnapshot1 *applicationapiv1alpha1.Snapshot
		hasComSnapshot2 *applicationapiv1alpha1.Snapshot
		hasComSnapshot3 *applicationapiv1alpha1.Snapshot
		sampleImage     string
	)

	const (
		SampleRepoLink      = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		namespace           = "default"
		applicationName     = "application-sample"
		componentName       = "component-sample"
		snapshotName        = "snapshot-sample"
		hasComSnapshot1Name = "hascomsnapshot1-sample"
		hasComSnapshot2Name = "hascomsnapshot2-sample"
		hasComSnapshot3Name = "hascomsnapshot3-sample"
		SampleCommit        = "a2ba645d50e471d5f084b"
		plrstarttime        = 1775992257
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

		badComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bad-component",
				Namespace: namespace,
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "bad-component",
				Application:    applicationName,
				ContainerImage: sampleImage,
			},
		}
		Expect(k8sClient.Create(ctx, badComp)).Should(Succeed())
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
					gitops.PipelineAsCodeEventTypeLabel:    gitops.PipelineAsCodePushType,
				},
				Annotations: map[string]string{
					"test.appstudio.openshift.io/pr-last-update": "2023-08-26T17:57:50+02:00",
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

		hasComSnapshot1 = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hasComSnapshot1Name,
				Namespace: namespace,
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                   gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel:              hasComSnapshot1Name,
					gitops.PipelineAsCodeEventTypeLabel:        gitops.PipelineAsCodePullRequestType,
					gitops.PipelineAsCodePullRequestAnnotation: "1",
				},
				Annotations: map[string]string{
					"test.appstudio.openshift.io/pr-last-update": "2023-08-26T17:57:50+02:00",
					gitops.BuildPipelineRunStartTime:             strconv.Itoa(plrstarttime),
				},
				// this CreationTimestamp don't take effect when snapshot is created
				// CreationTimestamp: metav1.NewTime(time.Now().Add(time.Hour * 2)),
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           "component1",
						ContainerImage: "test-image",
					},
					{
						Name:           componentName,
						ContainerImage: sampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComSnapshot1)).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasComSnapshot1.Name,
				Namespace: namespace,
			}, hasComSnapshot1)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		hasComSnapshot2 = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hasComSnapshot2Name,
				Namespace: namespace,
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                   gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel:              hasComSnapshot2Name,
					gitops.PipelineAsCodeEventTypeLabel:        gitops.PipelineAsCodePullRequestType,
					gitops.PipelineAsCodePullRequestAnnotation: "1",
				},
				Annotations: map[string]string{
					"test.appstudio.openshift.io/pr-last-update": "2023-08-26T17:57:50+02:00",
					gitops.BuildPipelineRunStartTime:             strconv.Itoa(plrstarttime + 100),
				},
				// this CreationTimestamp don't take effect when snapshot is created
				// CreationTimestamp: metav1.NewTime(time.Now().Add(time.Hour * 1)),
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           "component1",
						ContainerImage: "test-image",
					},
					{
						Name:           componentName,
						ContainerImage: sampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComSnapshot2)).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasComSnapshot2.Name,
				Namespace: namespace,
			}, hasComSnapshot2)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		hasComSnapshot3 = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hasComSnapshot3Name,
				Namespace: namespace,
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                   gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel:              hasComSnapshot3Name,
					gitops.PipelineAsCodeEventTypeLabel:        gitops.PipelineAsCodePullRequestType,
					gitops.PipelineAsCodePullRequestAnnotation: "1",
				},
				Annotations: map[string]string{
					"test.appstudio.openshift.io/pr-last-update": "2023-08-26T17:57:50+02:00",
					gitops.BuildPipelineRunStartTime:             strconv.Itoa(plrstarttime + 200),
				},
				// this CreationTimestamp don't take effect when snapshot is created
				// CreationTimestamp: metav1.NewTime(time.Now()),
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           "component1",
						ContainerImage: "test-image",
					},
					{
						Name:           componentName,
						ContainerImage: sampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComSnapshot3)).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      hasComSnapshot3.Name,
				Namespace: namespace,
			}, hasComSnapshot3)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComSnapshot1)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComSnapshot2)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComSnapshot3)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, badComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("ensures the a decision can be made to NOT promote when the snaphot has not been marked as passed/failed", func() {
		canBePromoted, reasons := gitops.CanSnapshotBePromoted(hasSnapshot)
		Expect(canBePromoted).To(BeFalse())
		Expect(reasons).To(HaveLen(1))
		Expect(reasons[0]).To(Equal("the Snapshot has not yet finished testing"))
	})

	It("ensures the Snapshots status can be marked as passed", func() {
		err := gitops.MarkSnapshotAsPassed(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeTrue())
		Expect(gitops.IsSnapshotMarkedAsPassed(hasSnapshot)).To(BeTrue())
	})

	It("ensures the Snapshots LegacyTestSucceededCondition status can be marked as passed", func() {
		patch := client.MergeFrom(hasSnapshot.DeepCopy())
		condition := metav1.Condition{
			Type:    gitops.LegacyTestSucceededCondition,
			Status:  metav1.ConditionTrue,
			Reason:  gitops.AppStudioTestSucceededConditionSatisfied,
			Message: "Test message",
		}
		meta.SetStatusCondition(&hasSnapshot.Status.Conditions, condition)

		err := k8sClient.Status().Patch(ctx, hasSnapshot, patch)
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.LegacyTestSucceededCondition)).To(BeTrue())
		Expect(gitops.IsSnapshotMarkedAsPassed(hasSnapshot)).To(BeTrue())
		Expect(gitops.IsSnapshotMarkedAsFailed(hasSnapshot)).To(BeFalse())
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
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.LegacyIntegrationStatusCondition)).To(BeFalse())
		Expect(gitops.IsSnapshotMarkedAsInvalid(hasSnapshot)).To(BeTrue())
		Expect(gitops.IsSnapshotStatusConditionSet(hasSnapshot, gitops.AppStudioIntegrationStatusCondition, metav1.ConditionFalse, "Valid")).To(BeFalse())
	})

	It("ensures the Snapshots status can be marked as failed", func() {
		err := gitops.MarkSnapshotAsFailed(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioTestSucceededCondition)).To(BeFalse())
		Expect(gitops.IsSnapshotMarkedAsFailed(hasSnapshot)).To(BeTrue())
	})

	It("ensures the Snapshots status can be marked as error", func() {
		gitops.SetSnapshotIntegrationStatusAsError(hasSnapshot, "Test message")
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)).To(BeFalse())
		Expect(gitops.IsSnapshotError(hasSnapshot)).To(BeTrue())
	})

	It("ensures the Snapshots status can be marked as finished", func() {
		err := gitops.MarkSnapshotIntegrationStatusAsFinished(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(gitops.IsSnapshotIntegrationStatusMarkedAsFinished(hasSnapshot)).To(BeTrue())
	})

	It("ensures the Snapshots status won't be marked as finished if it is canceled", func() {
		err := gitops.MarkSnapshotAsCanceled(ctx, k8sClient, hasSnapshot, "canceled")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		if !gitops.IsSnapshotIntegrationStatusMarkedAsFinished(hasSnapshot) {
			err := gitops.MarkSnapshotIntegrationStatusAsFinished(ctx, k8sClient, hasSnapshot, "Test message")
			Expect(err).ToNot(HaveOccurred())
		}
		Expect(gitops.IsSnapshotMarkedAsCanceled(hasSnapshot)).To(BeTrue())
	})

	It("ensures the Snapshots status can be marked as in progress", func() {
		err := gitops.MarkSnapshotIntegrationStatusAsInProgress(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		foundStatusCondition := meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)
		Expect(foundStatusCondition.Reason).To(Equal(gitops.AppStudioIntegrationStatusInProgress))
	})

	It("ensures the Snapshots status can be marked as invalid", func() {
		err := gitops.MarkSnapshotAsInvalid(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(hasSnapshot.Status.Conditions, gitops.AppStudioIntegrationStatusCondition)).To(BeFalse())
		Expect(gitops.IsSnapshotMarkedAsPassed(hasSnapshot)).To(BeFalse())
	})

	It("ensures the Snapshots status can be marked as auto released", func() {
		Expect(gitops.IsSnapshotMarkedAsAutoReleased(hasSnapshot)).To(BeFalse())

		err := gitops.MarkSnapshotAsAutoReleased(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		foundStatusCondition := meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.SnapshotAutoReleasedCondition)
		Expect(foundStatusCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(foundStatusCondition.Message).To(Equal("Test message"))

		Expect(gitops.IsSnapshotMarkedAsAutoReleased(hasSnapshot)).To(BeTrue())
	})

	It("ensures the Snapshots status can be marked as component added to global candidate list", func() {
		Expect(gitops.IsSnapshotMarkedAsAddedToGlobalCandidateList(hasSnapshot)).To(BeFalse())
		Expect(gitops.IsComponentSnapshotCreatedByPACPushEvent(hasSnapshot)).To(BeTrue())

		err := gitops.MarkSnapshotAsAddedToGlobalCandidateList(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		foundStatusCondition := meta.FindStatusCondition(hasSnapshot.Status.Conditions, gitops.SnapshotAddedToGlobalCandidateListCondition)
		Expect(foundStatusCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(foundStatusCondition.Message).To(Equal("Test message"))

		Expect(gitops.IsSnapshotMarkedAsAddedToGlobalCandidateList(hasSnapshot)).To(BeTrue())
	})

	It("ensures the Snapshots can be checked for the AppStudioTestSucceededCondition", func() {
		checkResult := gitops.HaveAppStudioTestsFinished(hasSnapshot)
		Expect(checkResult).To(BeFalse())
	})

	It("ensures the Snapshots can be checked for the AppStudioTestSucceededCondition", func() {
		checkResult := gitops.HaveAppStudioTestsSucceeded(hasSnapshot)
		Expect(checkResult).To(BeFalse())
	})

	It("returns true if only AppStudioTestSucceededCondition is set", func() {
		appStudioTestSucceededCondition := "AppStudioTestSucceeded" // Local variable
		condition := metav1.Condition{
			Type:   appStudioTestSucceededCondition,
			Status: metav1.ConditionTrue,
		}
		meta.SetStatusCondition(&hasSnapshot.Status.Conditions, condition)
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
	})

	It("returns true if only LegacyTestSucceededCondition is set", func() {
		legacyTestSucceededCondition := "HACBSStudioTestSucceeded" // Local variable
		condition := metav1.Condition{
			Type:   legacyTestSucceededCondition,
			Status: metav1.ConditionTrue,
		}
		meta.SetStatusCondition(&hasSnapshot.Status.Conditions, condition)
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
	})

	It("returns the LastTransitionTime when AppStudioTestSucceededCondition is set", func() {
		appStudioTestSucceededCondition := "AppStudioTestSucceeded" // Local variable
		testTime := metav1.NewTime(time.Now())
		condition := metav1.Condition{
			Type:               appStudioTestSucceededCondition,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: testTime,
		}
		meta.SetStatusCondition(&hasSnapshot.Status.Conditions, condition)

		returnedTime, ok := gitops.GetAppStudioTestsFinishedTime(hasSnapshot)
		Expect(ok).To(BeTrue())
		Expect(returnedTime).To(Equal(testTime))
	})

	It("returns the LastTransitionTime when LegacyTestSucceededCondition is set", func() {
		legacyTestSucceededCondition := "HACBSStudioTestSucceeded"
		testTime := metav1.NewTime(time.Now())
		condition := metav1.Condition{
			Type:               legacyTestSucceededCondition,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: testTime,
		}
		meta.SetStatusCondition(&hasSnapshot.Status.Conditions, condition)

		returnedTime, ok := gitops.GetAppStudioTestsFinishedTime(hasSnapshot)
		Expect(ok).To(BeTrue())
		Expect(returnedTime).To(Equal(testTime))
	})

	It("returns zero time when neither condition is set", func() {
		returnedTime, ok := gitops.GetAppStudioTestsFinishedTime(hasSnapshot)
		Expect(ok).To(BeFalse())
		Expect(returnedTime).To(Equal(metav1.Time{})) // Empty or zero time
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

	It("ensures the different Snapshots can be successfully compared if they have different event-type", func() {
		expectedSnapshot := hasSnapshot.DeepCopy()
		expectedSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodeMergeRequestType
		comparisonResult := gitops.CompareSnapshots(hasSnapshot, expectedSnapshot)
		Expect(comparisonResult).To(BeFalse())
	})

	It("ensures the different Snapshots can be successfully compared if missing event-type label", func() {
		err := metadata.DeleteLabel(hasSnapshot, gitops.PipelineAsCodeEventTypeLabel)
		Expect(err).ToNot(HaveOccurred())
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
		err := gitops.MarkSnapshotIntegrationStatusAsFinished(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(gitops.IsSnapshotValid(hasSnapshot)).To(BeTrue())
	})

	It("ensures the Snapshots status can be reset", func() {
		err := gitops.MarkSnapshotIntegrationStatusAsFinished(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		err = gitops.MarkSnapshotAsPassed(ctx, k8sClient, hasSnapshot, "test passed")
		Expect(err).ToNot(HaveOccurred())
		Expect(gitops.HaveAppStudioTestsFinished(hasSnapshot)).To(BeTrue())
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())
		Expect(gitops.ResetSnapshotStatusConditions(ctx, k8sClient, hasSnapshot, "in progress")).To(Succeed())
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeFalse())
		Expect(gitops.HaveAppStudioTestsFinished(hasSnapshot)).To(BeFalse())
	})

	It("ensures the a decision can be made to promote the Snapshot based on its status", func() {
		err := gitops.MarkSnapshotIntegrationStatusAsFinished(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())

		err = gitops.MarkSnapshotAsPassed(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())

		canBePromoted, reasons := gitops.CanSnapshotBePromoted(hasSnapshot)
		Expect(canBePromoted).To(BeTrue())
		Expect(reasons).To(BeEmpty())
	})

	It("ensures the a decision can be made to NOT promote the Snapshot based on its status", func() {
		err := gitops.MarkSnapshotIntegrationStatusAsFinished(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot).NotTo(BeNil())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())

		err = gitops.MarkSnapshotAsFailed(ctx, k8sClient, hasSnapshot, "Test message")
		Expect(err).ToNot(HaveOccurred())
		Expect(hasSnapshot.Status.Conditions).NotTo(BeNil())

		canBePromoted, reasons := gitops.CanSnapshotBePromoted(hasSnapshot)
		Expect(canBePromoted).To(BeFalse())
		Expect(reasons).To(HaveLen(1))

		hasSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodePullRequestType
		hasSnapshot.Labels[gitops.PipelineAsCodePullRequestAnnotation] = "1"
		canBePromoted, reasons = gitops.CanSnapshotBePromoted(hasSnapshot)
		Expect(canBePromoted).To(BeFalse())
		Expect(reasons).To(HaveLen(2))

		gitops.SetSnapshotIntegrationStatusAsInvalid(hasSnapshot, "Test message")
		canBePromoted, reasons = gitops.CanSnapshotBePromoted(hasSnapshot)
		Expect(canBePromoted).To(BeFalse())
		Expect(reasons).To(HaveLen(3))

		hasSnapshot.Labels[gitops.AutoReleaseLabel] = "false"
		canBePromoted, reasons = gitops.CanSnapshotBePromoted(hasSnapshot)
		Expect(canBePromoted).To(BeFalse())
		Expect(reasons).To(HaveLen(4))

	})

	It("Return false when the image url contains invalid digest", func() {
		imageUrl := "quay.io/redhat-appstudio/sample-image:latest"
		Expect(gitops.ValidateImageDigest(imageUrl)).NotTo(Succeed())
	})

	It("Return true when the image url contains valid digest", func() {
		// Prepare a valid image with digest
		imageUrl := "quay.io/redhat-appstudio/sample-image@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		Expect(gitops.ValidateImageDigest(imageUrl)).To(Succeed())
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
		snapshot, err := gitops.PrepareSnapshot(ctx, k8sClient, hasApp, allApplicationComponents, hasComp, imagePullSpec, componentSource)
		Expect(snapshot).NotTo(BeNil())
		Expect(err).ToNot(HaveOccurred())
		Expect(snapshot.Spec.Components).To(HaveLen(1), "One component should have been added to snapshot.  Other component should have been omited due to empty ContainerImage field or missing valid digest")
		Expect(snapshot.Spec.Components[0].Name).To(Equal(hasComp.Name), "The built component should have been added to the snapshot")
		Expect(snapshot.GetAnnotations()).To(HaveKeyWithValue(gitops.SnapshotGitSourceRepoURLAnnotation, componentSource.GitSource.URL), "The git source repo URL annotation is added")
	})

	It("ensure error is returned if the ContainerImage digest is invalid", func() {
		imagePullSpec := "quay.io/redhat-appstudio/sample-image@invaliDigest"
		componentSource := &applicationapiv1alpha1.ComponentSource{
			ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
				GitSource: &applicationapiv1alpha1.GitSource{
					URL:      SampleRepoLink,
					Revision: SampleCommit,
				},
			},
		}
		allApplicationComponents := &[]applicationapiv1alpha1.Component{*hasComp}
		snapshot, err := gitops.PrepareSnapshot(ctx, k8sClient, hasApp, allApplicationComponents, hasComp, imagePullSpec, componentSource)
		Expect(snapshot).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("quay.io/redhat-appstudio/sample-image@invaliDigest is invalid container image digest from component component-sample"))
	})

	It("ensure that an existing component with invalid image won't be added to the new Snapshot", func() {
		validImagePullSpec := "quay.io/redhat-appstudio/sample-image@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		invalidImagePullSpec := "quay.io/redhat-appstudio/sample-image"

		hasComp2 := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "second-component",
				Namespace: namespace,
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "second-component",
				Application:    applicationName,
				ContainerImage: invalidImagePullSpec,
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
		componentSource := &applicationapiv1alpha1.ComponentSource{
			ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
				GitSource: &applicationapiv1alpha1.GitSource{
					URL:      SampleRepoLink,
					Revision: SampleCommit,
				},
			},
		}
		allApplicationComponents := &[]applicationapiv1alpha1.Component{*hasComp, *hasComp2, *badComp}
		snapshot, err := gitops.PrepareSnapshot(ctx, k8sClient, hasApp, allApplicationComponents, hasComp, validImagePullSpec, componentSource)
		Expect(snapshot).NotTo(BeNil())
		Expect(err).NotTo(HaveOccurred())

		Expect(snapshot.Spec.Components).To(HaveLen(1))
		for _, snapshotComponent := range snapshot.Spec.Components {
			snapshotComponent := snapshotComponent
			Expect(snapshotComponent.ContainerImage).NotTo(Equal(invalidImagePullSpec))
		}
		Expect(snapshot.Annotations["test.appstudio.openshift.io/create-snapshot-status"]).To(Equal("Component(s) 'second-component, bad-component' is(are) not included in snapshot due to missing valid containerImage or git source"))
	})

	It("Return false when the image url contains invalid digest", func() {
		imageUrl := "quay.io/redhat-appstudio/sample-image:latest"
		Expect(gitops.ValidateImageDigest(imageUrl)).NotTo(Succeed())
	})

	It("ensure ComponentSource can returned when component have Status.LastBuiltCommit defined or not", func() {
		componentSource, err := gitops.GetComponentSourceFromComponent(hasComp)
		Expect(componentSource.GitSource.Revision).To(Equal("a2ba645d50e471d5f084b"))
		Expect(err).ShouldNot(HaveOccurred())

		hasComp.Status = applicationapiv1alpha1.ComponentStatus{
			LastBuiltCommit: "lastbuildcommit",
		}
		//Expect(k8sClient.Status().Update(ctx, hasComp)).Should(Succeed())
		componentSource, err = gitops.GetComponentSourceFromComponent(hasComp)
		Expect(componentSource.GitSource.Revision).To(Equal("lastbuildcommit"))
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("ensure ComponentSource can not returned when component have not git source ", func() {
		_, err := gitops.GetComponentSourceFromComponent(badComp)
		Expect(err).Should(HaveOccurred())
	})

	It("ensure existing snapshot can be found", func() {
		allSnapshots := &[]applicationapiv1alpha1.Snapshot{*hasSnapshot}
		existingSnapshot := gitops.FindMatchingSnapshot(hasApp, allSnapshots, hasSnapshot)
		Expect(existingSnapshot.Name).To(Equal(hasSnapshot.Name))
	})

	Context("GetIntegrationTestRunLabelValue tests", func() {

		It("snapshot has no label defined", func() {
			_, ok := gitops.GetIntegrationTestRunLabelValue(hasSnapshot)
			Expect(ok).To(BeFalse())
		})

		It("snaphost has label defined", func() {
			testScenario := "test-scenario"
			hasSnapshot.Labels[gitops.SnapshotIntegrationTestRun] = testScenario
			val, ok := gitops.GetIntegrationTestRunLabelValue(hasSnapshot)
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal(testScenario))
		})
	})

	Context("AddIntegrationTestRerunLabel tests", func() {

		It("add run label to snapshot", func() {
			testScenario := "test-scenario"
			err := gitops.AddIntegrationTestRerunLabel(ctx, k8sClient, hasSnapshot, testScenario)
			Expect(err).ToNot(HaveOccurred())
			val, ok := gitops.GetIntegrationTestRunLabelValue(hasSnapshot)
			Expect(ok).To(BeTrue())
			Expect(val).To(Equal(testScenario))
		})

	})

	Context("RemoveIntegrationTestRerunLabel tests", func() {

		It("won't fail if re-run label is not present", func() {
			err := gitops.RemoveIntegrationTestRerunLabel(ctx, k8sClient, hasSnapshot)
			Expect(err).To(Succeed())
		})

		When("Snapshot has re-run label", func() {
			testScenario := "test-scenario"
			var (
				snapshotRerun *applicationapiv1alpha1.Snapshot
			)

			BeforeEach(func() {
				// cannot create real object, reconciliation would just fetch it and process it
				// rerun label would be removed
				snapshotRerun = hasSnapshot.DeepCopy()
				snapshotRerun.Labels[gitops.SnapshotIntegrationTestRun] = testScenario
			})

			It("removes re-run label from snapshot and saves result into DB", func() {
				m := MatchKeys(IgnoreExtras, Keys{
					gitops.SnapshotIntegrationTestRun: Equal(testScenario),
				})
				Expect(snapshotRerun.GetLabels()).Should(m, "have re-run label")
				err := gitops.RemoveIntegrationTestRerunLabel(ctx, k8sClient, snapshotRerun)
				Expect(err).To(Succeed())
				Expect(snapshotRerun.GetLabels()).ShouldNot(m, "shouldn't have re-run label")
			})
		})

	})

	Context("Override snapshot tests", func() {
		When("Snapshot has snapshot type label", func() {
			var overrideSnapshot *applicationapiv1alpha1.Snapshot
			BeforeEach(func() {
				overrideSnapshot = hasSnapshot.DeepCopy()
				overrideSnapshot.Labels[gitops.SnapshotTypeLabel] = gitops.SnapshotOverrideType
				Expect(controllerutil.HasControllerReference(overrideSnapshot)).To(BeFalse())
			})

			It("make sure correct label is returned in overrideSnapshot", func() {
				isOverrideSnapshot := gitops.IsOverrideSnapshot(overrideSnapshot)
				Expect(isOverrideSnapshot).To(BeTrue())
			})

			It("Can set owner reference for override snapshot", func() {
				overrideSnapshot, err := gitops.SetOwnerReference(ctx, k8sClient, overrideSnapshot, hasApp)
				Expect(controllerutil.HasControllerReference(overrideSnapshot)).To(BeTrue())
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("Filter integration tests for a given Snapshot based on their context", func() {
		When("There are a number of integration test scenarios with different contexts", func() {
			integrationTestScenario := &v1beta2.IntegrationTestScenario{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-pass",
					Namespace: "default",

					Labels: map[string]string{
						"test.appstudio.openshift.io/optional": "false",
					},
				},
				Spec: v1beta2.IntegrationTestScenarioSpec{
					Application: "application-sample",
					ResolverRef: v1beta2.ResolverRef{
						Resolver: "git",
						Params: []v1beta2.ResolverParameter{
							{
								Name:  "url",
								Value: "https://github.com/redhat-appstudio/integration-examples.git",
							},
							{
								Name:  "revision",
								Value: "main",
							},
							{
								Name:  "pathInRepo",
								Value: "pipelineruns/integration_pipelinerun_pass.yaml",
							},
						},
					},
				},
			}
			applicationScenario := integrationTestScenario.DeepCopy()
			applicationScenario.Spec.Contexts = []v1beta2.TestContext{{Name: "application", Description: "Application Testing"}}
			componentScenario := integrationTestScenario.DeepCopy()
			componentScenario.Spec.Contexts = []v1beta2.TestContext{{Name: "component", Description: "Component"}}
			componentSampleScenario := integrationTestScenario.DeepCopy()
			componentSampleScenario.Spec.Contexts = []v1beta2.TestContext{{Name: "component_component-sample", Description: "Component component-sample"}}
			componentSample2Scenario := integrationTestScenario.DeepCopy()
			componentSample2Scenario.Spec.Contexts = []v1beta2.TestContext{{Name: "component_component-sample-2", Description: "Component component-sample-2"}}
			pullRequestScenario := integrationTestScenario.DeepCopy()
			pullRequestScenario.Spec.Contexts = []v1beta2.TestContext{{Name: "pull_request", Description: "Pull Request"}}
			pushScenario := integrationTestScenario.DeepCopy()
			pushScenario.Spec.Contexts = []v1beta2.TestContext{{Name: "push", Description: "Component"}}
			groupScenario := integrationTestScenario.DeepCopy()
			groupScenario.Spec.Contexts = []v1beta2.TestContext{{Name: "group", Description: "PR Group Testing"}}
			overrideScenario := integrationTestScenario.DeepCopy()
			overrideScenario.Spec.Contexts = []v1beta2.TestContext{{Name: "override", Description: "Override Snapshot testing"}}
			componentAndGroupScenario := integrationTestScenario.DeepCopy()
			componentAndGroupScenario.Spec.Contexts = []v1beta2.TestContext{{Name: "group"}, {Name: "component"}}
			unsupportedScenario := integrationTestScenario.DeepCopy()
			unsupportedScenario.Spec.Contexts = []v1beta2.TestContext{{Name: "n/a"}}

			allScenarios := []v1beta2.IntegrationTestScenario{*integrationTestScenario, *applicationScenario,
				*componentScenario, *componentSampleScenario, *componentSample2Scenario, *pullRequestScenario,
				*pushScenario, *groupScenario, *componentAndGroupScenario, *unsupportedScenario}

			It("Returns only the scenarios matching the context for a given kind of Snapshot", func() {
				// A component Snapshot for a push event referencing the component-sample
				filteredScenarios := gitops.FilterIntegrationTestScenariosWithContext(&allScenarios, hasSnapshot)
				Expect(*filteredScenarios).To(HaveLen(6))

				// A component Snapshot for pull request event referencing the component-sample
				hasSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodePullRequestType
				hasSnapshot.Labels[gitops.PipelineAsCodePullRequestAnnotation] = "1"
				filteredScenarios = gitops.FilterIntegrationTestScenariosWithContext(&allScenarios, hasSnapshot)
				Expect(*filteredScenarios).To(HaveLen(6))

				// A group Snapshot for pull request event referencing component-sample-2
				hasSnapshot.Labels[gitops.SnapshotComponentLabel] = "component-sample-2"
				filteredScenarios = gitops.FilterIntegrationTestScenariosWithContext(&allScenarios, hasSnapshot)
				Expect(*filteredScenarios).To(HaveLen(6))

				// A group Snapshot for pull request event for a PR group
				hasSnapshot.Labels[gitops.SnapshotTypeLabel] = "group"
				hasSnapshot.Labels[gitops.SnapshotComponentLabel] = ""
				filteredScenarios = gitops.FilterIntegrationTestScenariosWithContext(&allScenarios, hasSnapshot)
				Expect(*filteredScenarios).To(HaveLen(5))

				// An override Snapshot
				hasSnapshot.Labels[gitops.SnapshotTypeLabel] = "override"
				filteredScenarios = gitops.FilterIntegrationTestScenariosWithContext(&allScenarios, hasSnapshot)
				Expect(*filteredScenarios).To(HaveLen(3))
			})

			It("Testing annotating snapshot", func() {
				hasSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodePullRequestType
				componentSnapshotInfos := []gitops.ComponentSnapshotInfo{
					{
						Component:        "com1",
						Snapshot:         "snapshot1",
						BuildPipelineRun: "buildPLR1",
						Namespace:        "default",
					},
					{
						Component:        "com2",
						Snapshot:         "snapshot2",
						BuildPipelineRun: "buildPLR2",
						Namespace:        "default",
					},
				}
				snapshot, err := gitops.SetAnnotationAndLabelForGroupSnapshot(hasSnapshot, hasSnapshot, componentSnapshotInfos)
				Expect(err).ToNot(HaveOccurred())
				Expect(componentSnapshotInfos).To(HaveLen(2))
				Expect(snapshot.Labels[gitops.SnapshotTypeLabel]).To(Equal("group"))
				Expect(gitops.IsSnapshotCreatedByPACPushEvent(snapshot)).To(BeFalse())

			})

			It("Testing UnmarshalJSON", func() {
				infoString := "[{\"namespace\":\"default\",\"component\":\"devfile-sample-java-springboot-basic-8969\",\"buildPipelineRun\":\"build-plr-java-qjfxz\",\"snapshot\":\"app-8969-bbn7d\"},{\"namespace\":\"default\",\"component\":\"devfile-sample-go-basic-8969\",\"buildPipelineRun\":\"build-plr-go-jmsjq\",\"snapshot\":\"app-8969-kzq2l\"}]"
				componentSnapshotInfos, err := gitops.UnmarshalJSON([]byte(infoString))
				Expect(err).ToNot(HaveOccurred())
				Expect(componentSnapshotInfos[0].Namespace).To(Equal("default"))
				Expect(componentSnapshotInfos).To(HaveLen(2))
			})
		})
	})

	Context("Group snapshot creation tests", func() {
		When("Snapshot has snapshot type label", func() {
			expectedPRGroup := "feature1"
			expectedPRGoupSha := "feature1hash"
			BeforeEach(func() {
				hasComSnapshot1.Labels[gitops.PRGroupHashLabel] = expectedPRGoupSha
				hasComSnapshot1.Annotations[gitops.PRGroupAnnotation] = expectedPRGroup
				hasComSnapshot1.Annotations[gitops.PRGroupCreationAnnotation] = "group snapshot is created"
			})

			It("make sure pr group annotation/label can be found in group", func() {
				prGroupSha, prGroup := gitops.GetPRGroup(hasComSnapshot1)
				Expect(prGroup).To(Equal(expectedPRGroup))
				Expect(prGroupSha).To(Equal(expectedPRGoupSha))
				Expect(gitops.HasPRGroupProcessed(hasComSnapshot1)).To(BeTrue())

				hasComSnapshot1.Annotations[gitops.PRGroupCreationAnnotation] = "a new build PLR component-sample-on-pull-request-jhctk is running for component component-sample, waiting for it to create a new group Snapshot for PR group test-branch"
				Expect(gitops.HasPRGroupProcessed(hasComSnapshot1)).To(BeFalse())
			})

			It("Can find the correct snapshotComponent for the given component name", func() {
				FoundSnapshotComponent := gitops.FindMatchingSnapshotComponent(hasComSnapshot1, hasComp)
				Expect(FoundSnapshotComponent.Name).To(Equal(hasComp.Name))
			})

			It("Can sort the snapshots according to annotation test.appstudio.openshift.io/pipelinerunstarttime", func() {
				snapshots := []applicationapiv1alpha1.Snapshot{*hasComSnapshot1, *hasComSnapshot2, *hasComSnapshot3}
				sortedSnapshots := gitops.SortSnapshots(snapshots)
				Expect(sortedSnapshots[0].Name).To(Equal(hasComSnapshot3.Name))
				snapshots = []applicationapiv1alpha1.Snapshot{*hasComSnapshot2, *hasComSnapshot1, *hasComSnapshot3}
				sortedSnapshots = gitops.SortSnapshots(snapshots)
				Expect(sortedSnapshots[0].Name).To(Equal(hasComSnapshot3.Name))
			})

			It("Can sort the snapshots according to its creation time when it is not component so there is not annotation test.appstudio.openshift.io/pipelinerunstarttime", func() {
				// CreationTimestamp new>old: 1>2>3
				hasComSnapshot1.CreationTimestamp = metav1.NewTime(time.Now().Add(time.Hour * 2))
				hasComSnapshot2.CreationTimestamp = metav1.NewTime(time.Now().Add(time.Hour * 1))
				hasComSnapshot3.CreationTimestamp = metav1.NewTime(time.Now())
				Expect(metadata.DeleteAnnotation(hasComSnapshot1, gitops.BuildPipelineRunStartTime)).To(Succeed())
				Expect(metadata.DeleteAnnotation(hasComSnapshot2, gitops.BuildPipelineRunStartTime)).To(Succeed())
				Expect(metadata.DeleteAnnotation(hasComSnapshot3, gitops.BuildPipelineRunStartTime)).To(Succeed())
				snapshots := []applicationapiv1alpha1.Snapshot{*hasComSnapshot1, *hasComSnapshot2, *hasComSnapshot3}
				sortedSnapshots := gitops.SortSnapshots(snapshots)
				Expect(sortedSnapshots[0].Name).To(Equal(hasComSnapshot1.Name))
				snapshots = []applicationapiv1alpha1.Snapshot{*hasComSnapshot2, *hasComSnapshot1, *hasComSnapshot3}
				sortedSnapshots = gitops.SortSnapshots(snapshots)
				Expect(sortedSnapshots[0].Name).To(Equal(hasComSnapshot1.Name))
			})

			It("Can notify all component snapshots group snapshot creation status", func() {
				Expect(metadata.HasAnnotation(hasComSnapshot2, gitops.PRGroupCreationAnnotation)).To(BeFalse())
				Expect(metadata.HasAnnotation(hasComSnapshot3, gitops.PRGroupCreationAnnotation)).To(BeFalse())
				componentSnapshotInfos := []gitops.ComponentSnapshotInfo{
					{
						Namespace:         "default",
						Component:         hasComSnapshot2Name,
						BuildPipelineRun:  "plr2",
						Snapshot:          hasComSnapshot2.Name,
						RepoUrl:           SampleRepoLink,
						PullRequestNumber: "1",
					},
					{
						Namespace:         "default",
						Component:         hasComSnapshot1Name,
						BuildPipelineRun:  "plr3",
						Snapshot:          hasComSnapshot3.Name,
						RepoUrl:           SampleRepoLink,
						PullRequestNumber: "1",
					},
				}
				err := gitops.NotifyComponentSnapshotsInGroupSnapshot(ctx, k8sClient, componentSnapshotInfos, "group snapshot created")

				Eventually(func() bool {
					_ = k8sClient.Get(ctx, types.NamespacedName{
						Name:      hasComSnapshot2.Name,
						Namespace: namespace,
					}, hasComSnapshot2)
					return metadata.HasAnnotationWithValue(hasComSnapshot2, gitops.PRGroupCreationAnnotation, "group snapshot created")
				}, time.Second*10).Should(BeTrue())
				Eventually(func() bool {
					_ = k8sClient.Get(ctx, types.NamespacedName{
						Name:      hasComSnapshot3.Name,
						Namespace: namespace,
					}, hasComSnapshot3)
					return metadata.HasAnnotationWithValue(hasComSnapshot3, gitops.PRGroupCreationAnnotation, "group snapshot created")
				}, time.Second*10).Should(BeTrue())
				Expect(err).ShouldNot(HaveOccurred())
			})

			It("Can return correct source repo owner for snapshot", func() {
				sourceRepoOwner := gitops.GetSourceRepoOwnerFromSnapshot(hasSnapshot)
				Expect(sourceRepoOwner).To(Equal(""))
				Expect(metadata.SetAnnotation(hasSnapshot, gitops.PipelineAsCodeGitSourceURLAnnotation, "https://github.com/devfile-sample/devfile-sample-go-basic")).To(Succeed())
				sourceRepoOwner = gitops.GetSourceRepoOwnerFromSnapshot(hasSnapshot)
				Expect(sourceRepoOwner).To(Equal("devfile-sample"))
			})

			It("Can copy snapshot labels and annotations", func() {
				buildPipelineRun := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pipelinerun-build-sample",
						Namespace: "default",
						Labels: map[string]string{
							"pipelines.appstudio.openshift.io/type":    "build",
							"pipelines.openshift.io/used-by":           "build-cloud",
							"pipelines.openshift.io/runtime":           "nodejs",
							"pipelines.openshift.io/strategy":          "s2i",
							"appstudio.openshift.io/component":         "component-sample",
							"build.appstudio.redhat.com/target_branch": "main",
							"pipelinesascode.tekton.dev/event-type":    "pull_request",
							"pipelinesascode.tekton.dev/pull-request":  "1",
						},
						Annotations: map[string]string{
							"appstudio.redhat.com/updateComponentOnSuccess": "false",
							"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
							"build.appstudio.openshift.io/repo":             "https://github.com/devfile-samples/devfile-sample-go-basic?rev=c713067b0e65fb3de50d1f7c457eb51c2ab0dbb0",
							"chains.tekton.dev/signed":                      "true",
							"pipelinesascode.tekton.dev/source-branch":      "sourceBranch",
							"pipelinesascode.tekton.dev/url-org":            "redhat",
						},
					},
					Spec: tektonv1.PipelineRunSpec{},
				}
				tempGroupSnapshot := &applicationapiv1alpha1.Snapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tempGroupSnapshot",
						Namespace: buildPipelineRun.Namespace,
					},
				}
				prefixes := []string{gitops.BuildPipelineRunPrefix}
				gitops.CopyTempGroupSnapshotLabelsAndAnnotations(hasApp, tempGroupSnapshot, hasComp.Name, &buildPipelineRun.ObjectMeta, prefixes)
				Expect(metadata.HasLabel(tempGroupSnapshot, "pac.test.appstudio.openshift.io/event-type")).To(BeTrue())
				Expect(metadata.HasLabel(tempGroupSnapshot, "appstudio.openshift.io/component")).To(BeFalse())
			})

			It("can mark snapshot as cancelled", func() {
				Expect(gitops.MarkSnapshotAsCanceled(ctx, k8sClient, hasSnapshot, "Canceled")).Should(Succeed())
				Eventually(func() bool {
					_ = k8sClient.Get(ctx, types.NamespacedName{
						Name:      hasSnapshot.Name,
						Namespace: namespace,
					}, hasSnapshot)
					return gitops.IsSnapshotMarkedAsCanceled(hasSnapshot)
				}, time.Second*15).Should(BeTrue())
			})

			It("can prepare temp group snapshot", func() {
				tempGroupSnapshot := gitops.PrepareTempGroupSnapshot(hasApp, hasSnapshot)
				Expect(metadata.HasLabelWithValue(tempGroupSnapshot, gitops.SnapshotTypeLabel, gitops.SnapshotGroupType)).To(BeTrue())
			})
		})
	})
})
