/*
Copyright 2023 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions andF
limitations under the License.
*/

package gitops_test

import (
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	intgteststat "github.com/konflux-ci/integration-service/pkg/integrationteststatus"

	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/operator-toolkit/metadata"
)

var _ = Describe("Snapshot integration test statuses", func() {

	Context("SnapshotTestsStatus", func() {
		const (
			testScenarioName = "test-scenario"
			testDetails      = "test-details"
			namespace        = "default"
			applicationName  = "application-sample"
			componentName    = "component-sample"
			snapshotName     = "snapshot-sample"
			pipelineRunName  = "pipeline-run-abcdf"
		)
		var (
			sits     *intgteststat.SnapshotIntegrationTestStatuses
			snapshot *applicationapiv1alpha1.Snapshot
		)

		BeforeEach(func() {
			var err error
			sits, err = intgteststat.NewSnapshotIntegrationTestStatuses("")
			Expect(err).ToNot(HaveOccurred())

			snapshot = &applicationapiv1alpha1.Snapshot{
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
					Application: applicationName,
					Components: []applicationapiv1alpha1.SnapshotComponent{
						{
							Name:           componentName,
							ContainerImage: "quay.io/redhat-appstudio/sample-image:latest",
						},
					},
				},
			}
		})

		It("Creates empty statuses when a snaphost doesn't have test status annotation", func() {
			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(statuses.GetStatuses()).To(BeEmpty())
		})

		When("Snapshot contains empty test status annotation", func() {

			BeforeEach(func() {
				err := metadata.SetAnnotation(snapshot, gitops.SnapshotTestsStatusAnnotation, "[]")
				Expect(err).ToNot(HaveOccurred())
			})

			It("Returns empty test statuses", func() {
				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).ToNot(HaveOccurred())
				Expect(statuses.GetStatuses()).To(BeEmpty())
			})
		})

		When("Snapshot contains valid test status annotation", func() {
			BeforeEach(func() {
				sits.UpdateTestStatusIfChanged(testScenarioName, intgteststat.IntegrationTestStatusInProgress, testDetails)
				testAnnotation, err := json.Marshal(sits)
				Expect(err).ToNot(HaveOccurred())
				err = metadata.SetAnnotation(snapshot, gitops.SnapshotTestsStatusAnnotation, string(testAnnotation))
				Expect(err).ToNot(HaveOccurred())

			})

			It("Returns expected test statuses", func() {
				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).ToNot(HaveOccurred())
				Expect(statuses.GetStatuses()).To(HaveLen(1))

				statusDetail := statuses.GetStatuses()[0]
				Expect(statusDetail.Status).To(Equal(intgteststat.IntegrationTestStatusInProgress))
				Expect(statusDetail.ScenarioName).To(Equal(testScenarioName))
				Expect(statusDetail.Details).To(Equal(testDetails))
			})

		})

		When("Snapshot contains invalid test status annotation", func() {
			BeforeEach(func() {
				err := metadata.SetAnnotation(
					snapshot, gitops.SnapshotTestsStatusAnnotation, "[{\"invalid\":\"data\"}]")
				Expect(err).ToNot(HaveOccurred())
			})

			It("Returns error", func() {
				_, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).To(HaveOccurred())
			})
		})

		When("Snapshot contains invalid JSON test status annotation", func() {
			BeforeEach(func() {
				err := metadata.SetAnnotation(snapshot, gitops.SnapshotTestsStatusAnnotation, "{}")
				Expect(err).ToNot(HaveOccurred())
			})

			It("Returns error", func() {
				_, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("legacy shifted status encoding", func() {
			const (
				legacyScenario     = "scenario-1"
				updatedAfterShift  = "2026-04-02T12:00:00Z"
				updatedBeforeShift = "2026-01-15T12:00:00Z"
			)

			setStatusAnnotation := func(statusJSON string) {
				err := metadata.SetAnnotation(snapshot, gitops.SnapshotTestsStatusAnnotation, statusJSON)
				Expect(err).ToNot(HaveOccurred())
			}

			legacyStatusJSON := func(status, updated string) string {
				return fmt.Sprintf(
					`[{"scenario":"%s","status":"%s","lastUpdateTime":"%s","details":"legacy"}]`,
					legacyScenario, status, updated,
				)
			}

			DescribeTable("reads a shifted name as the status that produced it",
				func(stored, canonical string) {
					setStatusAnnotation(legacyStatusJSON(stored, updatedAfterShift))

					statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
					Expect(err).ToNot(HaveOccurred())
					detail, ok := statuses.GetScenarioStatus(legacyScenario)
					Expect(ok).To(BeTrue())
					Expect(detail.Status.String()).To(Equal(canonical))
					Expect(detail.Details).To(Equal("legacy"))
				},
				Entry("TestWarning stored as BuildPLRInProgress", "BuildPLRInProgress", "TestWarning"),
				Entry("BuildPLRInProgress stored as SnapshotCreationFailed", "SnapshotCreationFailed", "BuildPLRInProgress"),
				Entry("SnapshotCreationFailed stored as BuildPLRFailed", "BuildPLRFailed", "SnapshotCreationFailed"),
				Entry("BuildPLRFailed stored as GroupSnapshotCreationFailed", "GroupSnapshotCreationFailed", "BuildPLRFailed"),
				Entry("GroupSnapshotCreationFailed stored as IntegrationTestStatus(13)", "IntegrationTestStatus(13)", "GroupSnapshotCreationFailed"),
			)

			It("keeps a shifted name written before TestWarning was introduced", func() {
				setStatusAnnotation(legacyStatusJSON("BuildPLRInProgress", updatedBeforeShift))

				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).ToNot(HaveOccurred())
				detail, ok := statuses.GetScenarioStatus(legacyScenario)
				Expect(ok).To(BeTrue())
				Expect(detail.Status).To(Equal(intgteststat.BuildPLRInProgress))
			})

			It("translates IntegrationTestStatus(13) even when it was written before the enum shift", func() {
				setStatusAnnotation(legacyStatusJSON("IntegrationTestStatus(13)", updatedBeforeShift))

				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).ToNot(HaveOccurred())
				detail, ok := statuses.GetScenarioStatus(legacyScenario)
				Expect(ok).To(BeTrue())
				Expect(detail.Status).To(Equal(intgteststat.GroupSnapshotCreationFailed))
			})

			It("reads the current spelling when the enum version annotation is set", func() {
				setStatusAnnotation(legacyStatusJSON("BuildPLRInProgress", updatedAfterShift))
				err := metadata.SetAnnotation(snapshot, gitops.SnapshotTestsStatusEnumVersionAnnotation, gitops.SnapshotTestsStatusEnumVersion)
				Expect(err).ToNot(HaveOccurred())

				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).ToNot(HaveOccurred())
				detail, ok := statuses.GetScenarioStatus(legacyScenario)
				Expect(ok).To(BeTrue())
				Expect(detail.Status).To(Equal(intgteststat.BuildPLRInProgress))
			})

			It("translates only the scenarios updated after the enum shift", func() {
				setStatusAnnotation(fmt.Sprintf(
					`[{"scenario":"old","status":"BuildPLRInProgress","lastUpdateTime":"%s","details":"before"},{"scenario":"new","status":"BuildPLRInProgress","lastUpdateTime":"%s","details":"after"}]`,
					updatedBeforeShift, updatedAfterShift,
				))

				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).ToNot(HaveOccurred())

				oldDetail, ok := statuses.GetScenarioStatus("old")
				Expect(ok).To(BeTrue())
				Expect(oldDetail.Status).To(Equal(intgteststat.BuildPLRInProgress))

				newDetail, ok := statuses.GetScenarioStatus("new")
				Expect(ok).To(BeTrue())
				Expect(newDetail.Status).To(Equal(intgteststat.IntegrationTestStatusTestWarning))
			})

			DescribeTable("reports invalid status JSON when a shifted name has no usable lastUpdateTime",
				func(statusJSON string) {
					setStatusAnnotation(statusJSON)

					_, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to get integration tests statuses from snapshot"))
					Expect(err.Error()).NotTo(ContainSubstring("failed to translate legacy integration test statuses"))
				},
				Entry("empty lastUpdateTime", `[{"scenario":"scenario-1","status":"BuildPLRInProgress","lastUpdateTime":""}]`),
				Entry("missing lastUpdateTime", `[{"scenario":"scenario-1","status":"BuildPLRInProgress"}]`),
				Entry("non-string lastUpdateTime", `[{"scenario":"scenario-1","status":"BuildPLRInProgress","lastUpdateTime":1}]`),
				Entry("unparseable lastUpdateTime", `[{"scenario":"scenario-1","status":"BuildPLRInProgress","lastUpdateTime":"not-a-time"}]`),
			)
		})

		Context("Writes data into snapshot", func() {

			// Make sure that snapshot is written to k8s for following tests
			BeforeEach(func() {
				Expect(k8sClient.Create(ctx, snapshot)).Should(Succeed())

				Eventually(func() error {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      snapshot.Name,
						Namespace: namespace,
					}, snapshot)
					return err
				}, time.Second*10).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				err := k8sClient.Delete(ctx, snapshot)
				Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
			})

			It("Test results are written into snapshot", func() {
				sits.UpdateTestStatusIfChanged(testScenarioName, intgteststat.IntegrationTestStatusInProgress, testDetails)

				err := gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, snapshot, sits, k8sClient)
				Expect(err).ToNot(HaveOccurred())
				Expect(sits.IsDirty()).To(BeFalse())

				// fetch updated snapshot
				Eventually(func() error {
					if err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      snapshot.Name,
						Namespace: namespace,
					}, snapshot); err != nil {
						return err
					}
					// race condition, sometimes it fetched old object
					annotations := snapshot.GetAnnotations()
					if _, ok := annotations[gitops.SnapshotTestsStatusAnnotation]; ok != true {
						return fmt.Errorf("Snapshot doesn't contain the expected annotation")
					}
					if annotations[gitops.SnapshotTestsStatusEnumVersionAnnotation] != gitops.SnapshotTestsStatusEnumVersion {
						return fmt.Errorf("Snapshot doesn't contain the enum version annotation")
					}
					return nil
				}, time.Second*10).ShouldNot(HaveOccurred())

				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).ToNot(HaveOccurred())
				Expect(statuses.GetStatuses()).To(HaveLen(1))
				Expect(statuses.GetStatuses()[0].Status).To(Equal(intgteststat.IntegrationTestStatusInProgress))
			})

			It("saves a legacy shifted status with the current spelling and enum version", func() {
				err := gitops.AnnotateSnapshot(ctx, snapshot, gitops.SnapshotTestsStatusAnnotation,
					`[{"scenario":"test-scenario","status":"BuildPLRInProgress","lastUpdateTime":"2026-04-02T12:00:00Z","details":"legacy"}]`,
					k8sClient)
				Expect(err).ToNot(HaveOccurred())

				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).ToNot(HaveOccurred())
				detail, ok := statuses.GetScenarioStatus(testScenarioName)
				Expect(ok).To(BeTrue())
				Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusTestWarning))

				statuses.UpdateTestStatusIfChanged(testScenarioName, intgteststat.IntegrationTestStatusTestWarning, "legacy-updated")
				err = gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, snapshot, statuses, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, types.NamespacedName{
						Name:      snapshot.Name,
						Namespace: namespace,
					}, snapshot)).To(Succeed())
					annotations := snapshot.GetAnnotations()
					g.Expect(annotations[gitops.SnapshotTestsStatusEnumVersionAnnotation]).To(Equal(gitops.SnapshotTestsStatusEnumVersion))
					g.Expect(annotations[gitops.SnapshotTestsStatusAnnotation]).To(ContainSubstring(`"status":"TestWarning"`))
					g.Expect(annotations[gitops.SnapshotTestsStatusAnnotation]).NotTo(ContainSubstring("BuildPLRInProgress"))
				}, time.Second*10).Should(Succeed())

				reread, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).ToNot(HaveOccurred())
				detail, ok = reread.GetScenarioStatus(testScenarioName)
				Expect(ok).To(BeTrue())
				Expect(detail.Status).To(Equal(intgteststat.IntegrationTestStatusTestWarning))
				Expect(detail.Details).To(Equal("legacy-updated"))
			})

			It("does not patch the snapshot when statuses are unchanged", func() {
				Expect(sits.IsDirty()).To(BeFalse())

				err := gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, snapshot, sits, k8sClient)
				Expect(err).ToNot(HaveOccurred())

				fetched := &applicationapiv1alpha1.Snapshot{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      snapshot.Name,
					Namespace: namespace,
				}, fetched)).To(Succeed())
				Expect(fetched.GetAnnotations()).NotTo(HaveKey(gitops.SnapshotTestsStatusAnnotation))
				Expect(fetched.GetAnnotations()).NotTo(HaveKey(gitops.SnapshotTestsStatusEnumVersionAnnotation))
			})

			It("returns an error when a status time is outside the RFC3339 year range", func() {
				sits.UpdateTestStatusIfChanged(testScenarioName, intgteststat.IntegrationTestStatusInProgress, testDetails)
				detail, ok := sits.GetScenarioStatus(testScenarioName)
				Expect(ok).To(BeTrue())
				detail.LastUpdateTime = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)

				err := gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, snapshot, sits, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to marshal test results into JSON"))
				Expect(sits.IsDirty()).To(BeTrue())

				fetched := &applicationapiv1alpha1.Snapshot{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      snapshot.Name,
					Namespace: namespace,
				}, fetched)).To(Succeed())
				Expect(fetched.GetAnnotations()).NotTo(HaveKey(gitops.SnapshotTestsStatusAnnotation))
				Expect(fetched.GetAnnotations()).NotTo(HaveKey(gitops.SnapshotTestsStatusEnumVersionAnnotation))
			})

			It("returns the API error when the snapshot cannot be patched", func() {
				Expect(k8sClient.Delete(ctx, snapshot)).To(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{
						Name:      snapshot.Name,
						Namespace: namespace,
					}, &applicationapiv1alpha1.Snapshot{})
					return errors.IsNotFound(err)
				}, time.Second*10).Should(BeTrue())

				sits.UpdateTestStatusIfChanged(testScenarioName, intgteststat.IntegrationTestStatusInProgress, testDetails)
				err := gitops.WriteIntegrationTestStatusesIntoSnapshot(ctx, snapshot, sits, k8sClient)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
				Expect(sits.IsDirty()).To(BeTrue())
			})
		})

	})

})
