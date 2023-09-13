/*
Copyright 2023.

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

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/operator-toolkit/metadata"
)

var _ = Describe("Snapshot integration test statuses", func() {

	Context("TestStatusDetail", func() {
		var (
			statusDetailPending gitops.IntegrationTestStatusDetail
		)

		BeforeEach(func() {
			statusDetailPending = gitops.IntegrationTestStatusDetail{Status: gitops.IntegrationTestStatusPending}
		})

		Describe("JSON operations", func() {
			It("Struct can be transformed to JSON", func() {
				jsonData, err := json.Marshal(statusDetailPending)
				Expect(err).To(BeNil())
				Expect(jsonData).Should(ContainSubstring("Pending"))
			})

			It("From JSON back to struct", func() {
				jsonData, err := json.Marshal(statusDetailPending)
				Expect(err).To(BeNil())
				var statusDetailFromJSON gitops.IntegrationTestStatusDetail
				err = json.Unmarshal(jsonData, &statusDetailFromJSON)
				Expect(err).To(BeNil())
				Expect(statusDetailFromJSON).Should(Equal(statusDetailPending))
			})
		})
	})

	Context("SnapshotTestsStatus", func() {
		const (
			testScenarioName = "test-scenario"
			testDetails      = "test-details"
			namespace        = "default"
			applicationName  = "application-sample"
			componentName    = "component-sample"
			snapshotName     = "snapshot-sample"
		)
		var (
			sits                    *gitops.SnapshotIntegrationTestStatuses
			integrationTestScenario *v1beta1.IntegrationTestScenario
			snapshot                *applicationapiv1alpha1.Snapshot
		)

		BeforeEach(func() {
			sits = gitops.NewSnapshotIntegrationTestStatuses()

			integrationTestScenario = &v1beta1.IntegrationTestScenario{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-pass",
					Namespace: "default",

					Labels: map[string]string{
						"test.appstudio.openshift.io/optional": "false",
					},
				},
				Spec: v1beta1.IntegrationTestScenarioSpec{
					Application: "application-sample",
					ResolverRef: v1beta1.ResolverRef{
						Resolver: "git",
						Params: []v1beta1.ResolverParameter{
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
					Environment: v1beta1.TestEnvironment{
						Name: "envname",
						Type: "POC",
						Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
							Env: []applicationapiv1alpha1.EnvVarPair{},
						},
					},
				},
			}

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

		It("Can add new scenario test status", func() {
			Expect(len(sits.GetStatuses())).To(Equal(0))
			sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusPending, testDetails)
			Expect(len(sits.GetStatuses())).To(Equal(1))

			detail, ok := sits.GetScenarioStatus(testScenarioName)
			Expect(ok).To(BeTrue())
			Expect(detail.ScenarioName).To(Equal(testScenarioName))
			Expect(detail.Status).To(Equal(gitops.IntegrationTestStatusPending))
		})

		DescribeTable("Test expected additons of startTime",
			func(st gitops.IntegrationTestStatus, shouldAdd bool) {
				sits.UpdateTestStatusIfChanged(testScenarioName, st, testDetails)
				detail, ok := sits.GetScenarioStatus(testScenarioName)
				Expect(ok).To(BeTrue())
				if shouldAdd {
					Expect(detail.StartTime).NotTo(BeNil())
				} else {
					Expect(detail.StartTime).To(BeNil())
				}
			},
			Entry("When status is Pending", gitops.IntegrationTestStatusPending, false),
			Entry("When status is InProgress", gitops.IntegrationTestStatusInProgress, true),
			Entry("When status is EnvironmentProvisionError", gitops.IntegrationTestStatusEnvironmentProvisionError, false),
			Entry("When status is DeploymentError", gitops.IntegrationTestStatusDeploymentError, false),
			Entry("When status is TestFail", gitops.IntegrationTestStatusTestFail, false),
			Entry("When status is TestPass", gitops.IntegrationTestStatusTestPassed, false),
		)

		DescribeTable("Test expected additons of completionTime",
			func(st gitops.IntegrationTestStatus, shouldAdd bool) {
				sits.UpdateTestStatusIfChanged(testScenarioName, st, testDetails)
				detail, ok := sits.GetScenarioStatus(testScenarioName)
				Expect(ok).To(BeTrue())
				if shouldAdd {
					Expect(detail.CompletionTime).NotTo(BeNil())
				} else {
					Expect(detail.CompletionTime).To(BeNil())
				}
			},
			Entry("When status is Pending", gitops.IntegrationTestStatusPending, false),
			Entry("When status is InProgress", gitops.IntegrationTestStatusInProgress, false),
			Entry("When status is EnvironmentProvisionError", gitops.IntegrationTestStatusEnvironmentProvisionError, true),
			Entry("When status is DeploymentError", gitops.IntegrationTestStatusDeploymentError, true),
			Entry("When status is TestFail", gitops.IntegrationTestStatusTestFail, true),
			Entry("When status is TestPass", gitops.IntegrationTestStatusTestPassed, true),
		)

		It("Change back to InProgress updates timestamps accordingly", func() {
			sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusTestPassed, testDetails)
			originalDetail, ok := sits.GetScenarioStatus(testScenarioName)
			Expect(ok).To(BeTrue())
			Expect(originalDetail.CompletionTime).ToNot(BeNil())
			originalStartTime := originalDetail.StartTime // copy time, it's all in pointers

			sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusInProgress, testDetails)
			newDetail, ok := sits.GetScenarioStatus(testScenarioName)
			Expect(ok).To(BeTrue())
			Expect(originalDetail.CompletionTime).To(BeNil())
			Expect(newDetail.StartTime).NotTo(Equal(originalStartTime))
		})

		It("not changing status keeps starting time the same", func() {
			newDetails := "something important"
			sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusInProgress, testDetails)
			originalDetail, ok := sits.GetScenarioStatus(testScenarioName)
			Expect(ok).To(BeTrue())
			originalStartTime := originalDetail.StartTime // copy time, it's all in pointers
			originalLastUpdateTime := originalDetail.LastUpdateTime

			time.Sleep(time.Duration(100)) // wait 100ns to avoid race condition when test is too quick and timestamp is the same
			sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusInProgress, newDetails)
			newDetail, ok := sits.GetScenarioStatus(testScenarioName)
			Expect(ok).To(BeTrue())
			Expect(newDetail.StartTime).To(Equal(originalStartTime))
			// but details and lastUpdateTimestamp must changed
			Expect(newDetail.Details).To(Equal(newDetails))
			Expect(newDetail.LastUpdateTime).NotTo(Equal(originalLastUpdateTime))
		})

		It("Can export valid JSON without start and completion time (Pending)", func() {
			sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusPending, testDetails)
			detail, ok := sits.GetScenarioStatus(testScenarioName)
			Expect(ok).To(BeTrue())

			expectedFormatStr := `[
				{
					"scenario": "%s",
					"status": "Pending",
					"lastUpdateTime": "%s",
					"details": "%s"
				}
			]`
			marshaledTime, err := detail.LastUpdateTime.MarshalText()
			Expect(err).To(BeNil())
			expectedStr := fmt.Sprintf(expectedFormatStr, testScenarioName, marshaledTime, testDetails)
			expected := []byte(expectedStr)

			Expect(json.Marshal(sits)).To(MatchJSON(expected))
		})

		It("Can export valid JSON with start time (InProgress)", func() {
			sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusInProgress, testDetails)

			detail, ok := sits.GetScenarioStatus(testScenarioName)
			Expect(ok).To(BeTrue())

			expectedFormatStr := `[
				{
					"scenario": "%s",
					"status": "InProgress",
					"lastUpdateTime": "%s",
					"details": "%s",
					"startTime": "%s"
				}
			]`
			marshaledTime, err := detail.LastUpdateTime.MarshalText()
			Expect(err).To(BeNil())
			marshaledStartTime, err := detail.StartTime.MarshalText()
			Expect(err).To(BeNil())
			expectedStr := fmt.Sprintf(expectedFormatStr, testScenarioName, marshaledTime, testDetails, marshaledStartTime)
			expected := []byte(expectedStr)

			Expect(json.Marshal(sits)).To(MatchJSON(expected))
		})

		It("Can export valid JSON with completion time (TestFailed)", func() {
			sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusTestFail, testDetails)

			detail, ok := sits.GetScenarioStatus(testScenarioName)
			Expect(ok).To(BeTrue())

			expectedFormatStr := `[
				{
					"scenario": "%s",
					"status": "TestFail",
					"lastUpdateTime": "%s",
					"details": "%s",
					"completionTime": "%s"
				}
			]`
			marshaledTime, err := detail.LastUpdateTime.MarshalText()
			Expect(err).To(BeNil())
			marshaledCompletionTime, err := detail.CompletionTime.MarshalText()
			Expect(err).To(BeNil())
			expectedStr := fmt.Sprintf(expectedFormatStr, testScenarioName, marshaledTime, testDetails, marshaledCompletionTime)
			expected := []byte(expectedStr)

			Expect(json.Marshal(sits)).To(MatchJSON(expected))
		})

		It("Can export valid JSON with startTime and completion time", func() {
			sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusInProgress, "yolo")
			sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusTestFail, testDetails)

			detail, ok := sits.GetScenarioStatus(testScenarioName)
			Expect(ok).To(BeTrue())

			expectedFormatStr := `[
				{
					"scenario": "%s",
					"status": "TestFail",
					"lastUpdateTime": "%s",
					"details": "%s",
					"startTime": "%s",
					"completionTime": "%s"
				}
			]`
			marshaledTime, err := detail.LastUpdateTime.MarshalText()
			Expect(err).To(BeNil())
			marshaledStartTime, err := detail.StartTime.MarshalText()
			Expect(err).To(BeNil())
			marshaledCompletionTime, err := detail.CompletionTime.MarshalText()
			Expect(err).To(BeNil())
			expectedStr := fmt.Sprintf(expectedFormatStr, testScenarioName, marshaledTime, testDetails, marshaledStartTime, marshaledCompletionTime)
			expected := []byte(expectedStr)

			Expect(json.Marshal(sits)).To(MatchJSON(expected))
		})

		When("Contains updates to status", func() {

			BeforeEach(func() {
				sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusPending, testDetails)
			})

			It("Status is marked as dirty", func() {
				Expect(sits.IsDirty()).To(BeTrue())
			})

			It("Status can be reseted as non-dirty", func() {
				sits.ResetDirty()
				Expect(sits.IsDirty()).To(BeFalse())
			})

			It("Adding the same update keeps status non-dirty", func() {
				sits.ResetDirty()
				sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusPending, testDetails)
				Expect(sits.IsDirty()).To(BeFalse())
			})

			It("Updating status of scenario is reflected", func() {
				oldSt, ok := sits.GetScenarioStatus(testScenarioName)
				Expect(ok).To(BeTrue())

				oldTimestamp := oldSt.LastUpdateTime

				sits.ResetDirty()
				// needs different status
				sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusInProgress, testDetails)
				Expect(sits.IsDirty()).To(BeTrue())

				newSt, ok := sits.GetScenarioStatus(testScenarioName)
				Expect(ok).To(BeTrue())
				Expect(newSt.Status).To(Equal(gitops.IntegrationTestStatusInProgress))
				// timestamp must be updated too
				Expect(newSt.LastUpdateTime).NotTo(Equal(oldTimestamp))

				// no changes to nuber of records
				Expect(len(sits.GetStatuses())).To(Equal(1))
			})

			It("Updating details of scenario is reflected", func() {
				newDetails := "_Testing details_"
				oldSt, ok := sits.GetScenarioStatus(testScenarioName)
				Expect(ok).To(BeTrue())

				oldTimestamp := oldSt.LastUpdateTime

				sits.ResetDirty()
				// needs the same status but different details
				sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusPending, newDetails)
				Expect(sits.IsDirty()).To(BeTrue())

				newSt, ok := sits.GetScenarioStatus(testScenarioName)
				Expect(ok).To(BeTrue())
				Expect(newSt.Details).To(Equal(newDetails))
				// timestamp must be updated too
				Expect(newSt.LastUpdateTime).NotTo(Equal(oldTimestamp))

				// no changes to nuber of records
				Expect(len(sits.GetStatuses())).To(Equal(1))
			})

			It("Scenario can be deleted", func() {
				sits.ResetDirty()
				sits.DeleteStatus(testScenarioName)
				Expect(len(sits.GetStatuses())).To(Equal(0))
				Expect(sits.IsDirty()).To(BeTrue())
			})

			It("Initialization with empty scneario list will remove data", func() {
				sits.ResetDirty()
				sits.InitStatuses(&[]v1beta1.IntegrationTestScenario{})
				Expect(len(sits.GetStatuses())).To(Equal(0))
				Expect(sits.IsDirty()).To(BeTrue())
			})

		})

		It("Initialization with new test scenario creates pending status", func() {
			sits.ResetDirty()
			sits.InitStatuses(&[]v1beta1.IntegrationTestScenario{*integrationTestScenario})

			Expect(sits.IsDirty()).To(BeTrue())
			Expect(len(sits.GetStatuses())).To(Equal(1))

			statusDetail, ok := sits.GetScenarioStatus(integrationTestScenario.Name)
			Expect(ok).To(BeTrue())
			Expect(statusDetail.ScenarioName).To(Equal(integrationTestScenario.Name))
			Expect(statusDetail.Status).To(Equal(gitops.IntegrationTestStatusPending))
		})

		It("Creates empty statuses when a snaphost doesn't have test status annotation", func() {
			statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
			Expect(err).To(BeNil())
			Expect(len(statuses.GetStatuses())).To(Equal(0))
		})

		When("Snapshot contains empty test status annotation", func() {

			BeforeEach(func() {
				err := metadata.SetAnnotation(snapshot, gitops.SnapshotTestsStatusAnnotation, "[]")
				Expect(err).To(BeNil())
			})

			It("Returns empty test statuses", func() {
				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).To(BeNil())
				Expect(len(statuses.GetStatuses())).To(Equal(0))
			})
		})

		When("Snapshot contains valid test status annotation", func() {
			BeforeEach(func() {
				sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusInProgress, testDetails)
				testAnnotation, err := json.Marshal(sits)
				Expect(err).To(BeNil())
				err = metadata.SetAnnotation(snapshot, gitops.SnapshotTestsStatusAnnotation, string(testAnnotation))
				Expect(err).To(BeNil())

			})

			It("Returns expected test statuses", func() {
				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).To(BeNil())
				Expect(len(statuses.GetStatuses())).To(Equal(1))

				statusDetail := statuses.GetStatuses()[0]
				Expect(statusDetail.Status).To(Equal(gitops.IntegrationTestStatusInProgress))
				Expect(statusDetail.ScenarioName).To(Equal(testScenarioName))
				Expect(statusDetail.Details).To(Equal(testDetails))
			})

		})

		When("Snapshot contains invalid test status annotation", func() {
			BeforeEach(func() {
				err := metadata.SetAnnotation(
					snapshot, gitops.SnapshotTestsStatusAnnotation, "[{\"invalid\":\"data\"}]")
				Expect(err).To(BeNil())
			})

			It("Returns error", func() {
				_, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).NotTo(BeNil())
			})
		})

		When("Snapshot contains invalid JSON test status annotation", func() {
			BeforeEach(func() {
				err := metadata.SetAnnotation(snapshot, gitops.SnapshotTestsStatusAnnotation, "{}")
				Expect(err).To(BeNil())
			})

			It("Returns error", func() {
				_, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).NotTo(BeNil())
			})
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
				sits.UpdateTestStatusIfChanged(testScenarioName, gitops.IntegrationTestStatusInProgress, testDetails)

				err := gitops.WriteIntegrationTestStatusesIntoSnapshot(snapshot, sits, k8sClient, ctx)
				Expect(err).To(BeNil())
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
					return nil
				}, time.Second*10).ShouldNot(HaveOccurred())

				statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(snapshot)
				Expect(err).To(BeNil())
				Expect(len(statuses.GetStatuses())).To(Equal(1))
			})
		})

	})

})
