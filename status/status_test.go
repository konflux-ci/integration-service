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

package status_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/redhat-appstudio/integration-service/pkg/integrationteststatus"
	"github.com/redhat-appstudio/integration-service/status"
)

// Custom matcher for gomock, to match expected summary in TestReport
type hasSummary struct {
	expectedSummary string
}

// Matches do exact match of TestResult.Summary
func (m hasSummary) Matches(arg interface{}) bool {
	report, ok := arg.(status.TestReport)
	if !ok {
		return false
	}
	return report.Summary == m.expectedSummary
}

// String prints what we expected
func (m hasSummary) String() string {
	return fmt.Sprintf("TestReport.Summary: \"%s\"", m.expectedSummary)
}

// HasSummary matches if TestRepor.Summary has the expected value
func HasSummary(value string) gomock.Matcher {
	return hasSummary{expectedSummary: value}
}

var _ = Describe("Status Adapter", func() {

	var (
		githubSnapshot *applicationapiv1alpha1.Snapshot
		hasSnapshot    *applicationapiv1alpha1.Snapshot
		mockReporter   *status.MockReporterInterface

		pipelineRun       *tektonv1.PipelineRun
		successfulTaskRun *tektonv1.TaskRun
		failedTaskRun     *tektonv1.TaskRun
		skippedTaskRun    *tektonv1.TaskRun
		mockK8sClient     *MockK8sClient
	)

	BeforeEach(func() {
		now := time.Now()
		os.Setenv("CONSOLE_URL", "https://definetly.not.prod/preview/application-pipeline/ns/{{ .Namespace }}/pipelinerun/{{ .PipelineRunName }}")

		successfulTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-pass",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-pass",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
			Status: tektonv1.TaskRunStatus{
				TaskRunStatusFields: tektonv1.TaskRunStatusFields{
					StartTime:      &metav1.Time{Time: now},
					CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
					Results: []tektonv1.TaskRunResult{
						{
							Name: "TEST_OUTPUT",
							Value: *tektonv1.NewStructuredValues(`{
											"result": "SUCCESS",
											"timestamp": "1665405318",
											"failures": 0,
											"successes": 10,
											"warnings": 0
										}`),
						},
					},
				},
			},
		}

		failedTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-fail",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-fail",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
			Status: tektonv1.TaskRunStatus{
				TaskRunStatusFields: tektonv1.TaskRunStatusFields{
					StartTime:      &metav1.Time{Time: now},
					CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
					Results: []tektonv1.TaskRunResult{
						{
							Name: "TEST_OUTPUT",
							Value: *tektonv1.NewStructuredValues(`{
											"result": "FAILURE",
											"timestamp": "1665405317",
											"failures": 1,
											"successes": 0,
											"warnings": 0
										}`),
						},
					},
				},
			},
		}

		skippedTaskRun = &tektonv1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-skip",
				Namespace: "default",
			},
			Spec: tektonv1.TaskRunSpec{
				TaskRef: &tektonv1.TaskRef{
					Name: "test-taskrun-skip",
					ResolverRef: tektonv1.ResolverRef{
						Resolver: "bundle",
						Params: tektonv1.Params{
							{
								Name:  "bundle",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "quay.io/redhat-appstudio/example-tekton-bundle:test"},
							},
							{
								Name:  "name",
								Value: tektonv1.ParamValue{Type: "string", StringVal: "test-task"},
							},
						},
					},
				},
			},
			Status: tektonv1.TaskRunStatus{
				TaskRunStatusFields: tektonv1.TaskRunStatusFields{
					StartTime:      &metav1.Time{Time: now.Add(5 * time.Minute)},
					CompletionTime: &metav1.Time{Time: now.Add(10 * time.Minute)},
					Results: []tektonv1.TaskRunResult{
						{
							Name: "TEST_OUTPUT",
							Value: *tektonv1.NewStructuredValues(`{
											"result": "SKIPPED",
											"timestamp": "1665405318",
											"failures": 0,
											"successes": 0,
											"warnings": 0
										}`),
						},
					},
				},
			},
		}

		pipelineRun = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pipelinerun",
				Namespace: "default",
				Labels: map[string]string{
					"appstudio.openshift.io/component":               "devfile-sample-go-basic",
					"test.appstudio.openshift.io/scenario":           "example-pass",
					"pac.test.appstudio.openshift.io/git-provider":   "github",
					"pac.test.appstudio.openshift.io/url-org":        "devfile-sample",
					"pac.test.appstudio.openshift.io/url-repository": "devfile-sample-go-basic",
					"pac.test.appstudio.openshift.io/sha":            "12a4a35ccd08194595179815e4646c3a6c08bb77",
					"pac.test.appstudio.openshift.io/event-type":     "pull_request",
				},
				Annotations: map[string]string{
					"pac.test.appstudio.openshift.io/repo-url": "https://github.com/devfile-sample/devfile-sample-go-basic",
				},
			},
			Status: tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					StartTime: &metav1.Time{Time: time.Now()},
					ChildReferences: []tektonv1.ChildStatusReference{
						{
							Name:             successfulTaskRun.Name,
							PipelineTaskName: "pipeline1-task1",
						},
						{
							Name:             skippedTaskRun.Name,
							PipelineTaskName: "pipeline1-task2",
						},
					},
				},
			},
		}

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					"test.appstudio.openshift.io/type":               "component",
					"appstudio.openshift.io/component":               "component-sample",
					"build.appstudio.redhat.com/pipeline":            "enterprise-contract",
					"pac.test.appstudio.openshift.io/git-provider":   "github",
					"pac.test.appstudio.openshift.io/url-org":        "devfile-sample",
					"pac.test.appstudio.openshift.io/url-repository": "devfile-sample-go-basic",
					"pac.test.appstudio.openshift.io/sha":            "12a4a35ccd08194595179815e4646c3a6c08bb77",
					"pac.test.appstudio.openshift.io/event-type":     "pull_request",
				},
				Annotations: map[string]string{
					"build.appstudio.redhat.com/commit_sha":         "6c65b2fcaea3e1a0a92476c8b5dc89e92a85f025",
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
					"pac.test.appstudio.openshift.io/repo-url":      "https://github.com/devfile-sample/devfile-sample-go-basic",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "application-sample",
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           "component-sample",
						ContainerImage: "sample_image",
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{
									Revision: "sample_revision",
								},
							},
						},
					},
				},
			},
		}

		mockK8sClient = &MockK8sClient{
			getInterceptor: func(key client.ObjectKey, obj client.Object) {
				if taskRun, ok := obj.(*tektonv1.TaskRun); ok {
					if key.Name == successfulTaskRun.Name {
						taskRun.Status = successfulTaskRun.Status
					} else if key.Name == failedTaskRun.Name {
						taskRun.Status = failedTaskRun.Status
					} else if key.Name == skippedTaskRun.Name {
						taskRun.Status = skippedTaskRun.Status
					}
				}
				if plr, ok := obj.(*tektonv1.PipelineRun); ok {
					if key.Name == pipelineRun.Name {
						plr.Status = pipelineRun.Status
					}
				}
			},
			listInterceptor: func(list client.ObjectList) {},
		}

		githubSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"pac.test.appstudio.openshift.io/git-provider": "github",
				},
			},
		}

		ctrl := gomock.NewController(GinkgoT())
		mockReporter = status.NewMockReporterInterface(ctrl)
		mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
	})
	AfterEach(func() {
		os.Setenv("CONSOLE_URL", "")
	})

	It("can get reporters from a snapshot", func() {
		st := status.NewStatus(logr.Discard(), nil)
		reporter := st.GetReporter(githubSnapshot)
		Expect(reporter).ToNot(BeNil())
		Expect(reporter.GetReporterName()).To(Equal("GithubReporter"))
	})

	It("doesn't report anything when there are not test results", func() {

		mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(0)   // without test results reporter shouldn't be initialized
		mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0) // without test results reported shouldn't report status

		st := status.NewStatus(logr.Discard(), nil)
		err := st.ReportSnapshotStatus(context.Background(), mockReporter, githubSnapshot)
		Expect(err).NotTo(HaveOccurred())
	})

	It("doesn't report anything when data are older", func() {

		mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
		mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0) // data are older, status shouldn't be reported

		hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
		hasSnapshot.Annotations["test.appstudio.openshift.io/git-reporter-status"] = "{\"scenarios\":{\"scenario1\":{\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\"}}}"
		st := status.NewStatus(logr.Discard(), mockK8sClient)
		err := st.ReportSnapshotStatus(context.Background(), mockReporter, hasSnapshot)
		Expect(err).NotTo(HaveOccurred())
	})

	It("doesn't report anything when data are older (old way - migration test)", func() {

		mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
		mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(0) // data are older, status shouldn't be reported

		hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
		hasSnapshot.Annotations["test.appstudio.openshift.io/pr-last-update"] = "2023-08-26T17:57:50+02:00"
		st := status.NewStatus(logr.Discard(), mockK8sClient)
		err := st.ReportSnapshotStatus(context.Background(), mockReporter, hasSnapshot)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Report new status if it was updated", func() {

		mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
		mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(1)

		hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
		hasSnapshot.Annotations["test.appstudio.openshift.io/git-reporter-status"] = "{\"scenarios\":{\"scenario1\":{\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\"}}}"
		st := status.NewStatus(logr.Discard(), mockK8sClient)
		err := st.ReportSnapshotStatus(context.Background(), mockReporter, hasSnapshot)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Report new status if it was updated (old way - migration test)", func() {

		mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
		mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Times(1)

		hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
		hasSnapshot.Annotations["test.appstudio.openshift.io/pr-last-update"] = "2023-08-26T17:57:49+02:00"
		st := status.NewStatus(logr.Discard(), mockK8sClient)
		err := st.ReportSnapshotStatus(context.Background(), mockReporter, hasSnapshot)
		Expect(err).NotTo(HaveOccurred())
	})

	It("report expected textual data for InProgress test scenario", func() {
		hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
		t, err := time.Parse(time.RFC3339, "2023-07-26T16:57:49+02:00")
		Expect(err).NotTo(HaveOccurred())
		expectedTestReport := status.TestReport{
			FullName:      "Red Hat Konflux / scenario1 / component-sample",
			ScenarioName:  "scenario1",
			SnapshotName:  "snapshot-sample",
			ComponentName: "component-sample",
			Text:          "Test in progress",
			Summary:       "Integration test for snapshot snapshot-sample and scenario scenario1 is in progress",
			Status:        integrationteststatus.IntegrationTestStatusInProgress,
			StartTime:     &t,
		}
		mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
		mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Eq(expectedTestReport)).Times(1)

		st := status.NewStatus(logr.Discard(), mockK8sClient)
		err = st.ReportSnapshotStatus(context.Background(), mockReporter, hasSnapshot)
		Expect(err).NotTo(HaveOccurred())
	})

	It("report status for TestPassed test scenario", func() {
		hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"TestPassed\",\"testPipelineRunName\":\"test-pipelinerun\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:55+02:00\",\"details\":\"failed\"}]"
		delete(hasSnapshot.Labels, "appstudio.openshift.io/component")
		ts, err := time.Parse(time.RFC3339, "2023-07-26T16:57:49+02:00")
		Expect(err).NotTo(HaveOccurred())
		tc, err := time.Parse(time.RFC3339, "2023-07-26T17:57:49+02:00")
		Expect(err).NotTo(HaveOccurred())
		text := `<ul>
<li><b>Pipelinerun</b>: <a href="https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/test-pipelinerun">test-pipelinerun</a></li>
</ul>
<hr>

| Task | Duration | Test Suite | Status | Details |
| --- | --- | --- | --- | --- |
| pipeline1-task1 | 5m0s |  | :heavy_check_mark: SUCCESS | :heavy_check_mark: 10 success(es) |
| pipeline1-task2 | 5m0s |  | :white_check_mark: SKIPPED |  |

`
		expectedTestReport := status.TestReport{
			FullName:       "Red Hat Konflux / scenario1",
			ScenarioName:   "scenario1",
			SnapshotName:   "snapshot-sample",
			ComponentName:  "",
			Text:           text,
			Summary:        "Integration test for snapshot snapshot-sample and scenario scenario1 has passed",
			Status:         integrationteststatus.IntegrationTestStatusTestPassed,
			StartTime:      &ts,
			CompletionTime: &tc,
		}
		mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
		mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Eq(expectedTestReport)).Times(1)

		st := status.NewStatus(logr.Discard(), mockK8sClient)
		err = st.ReportSnapshotStatus(context.Background(), mockReporter, hasSnapshot)
		Expect(err).NotTo(HaveOccurred())
	})

	DescribeTable(
		"report right summary per status",
		func(expectedScenarioStatus integrationteststatus.IntegrationTestStatus, expectedTextEnding string) {

			statusAnnotationTempl := "[{\"scenario\":\"scenario1\",\"status\":\"%s\",\"testPipelineRunName\":\"test-pipelinerun\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:55+02:00\",\"details\":\"failed\"}]"
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = fmt.Sprintf(statusAnnotationTempl, expectedScenarioStatus)

			expectedSummary := fmt.Sprintf("Integration test for snapshot snapshot-sample and scenario scenario1 %s", expectedTextEnding)
			mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Times(1)
			mockReporter.EXPECT().ReportStatus(gomock.Any(), HasSummary(expectedSummary)).Times(1)

			st := status.NewStatus(logr.Discard(), mockK8sClient)
			err := st.ReportSnapshotStatus(context.Background(), mockReporter, hasSnapshot)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("Passed", integrationteststatus.IntegrationTestStatusTestPassed, "has passed"),
		Entry("Failed", integrationteststatus.IntegrationTestStatusTestFail, "has failed"),
		Entry("Provisioning error", integrationteststatus.IntegrationTestStatusEnvironmentProvisionError_Deprecated, "experienced an error when provisioning environment"),
		Entry("Deployment error", integrationteststatus.IntegrationTestStatusDeploymentError_Deprecated, "experienced an error when deploying snapshotEnvironmentBinding"),
		Entry("Deleted", integrationteststatus.IntegrationTestStatusDeleted, "was deleted before the pipelineRun could finish"),
		Entry("Pending", integrationteststatus.IntegrationTestStatusPending, "is pending"),
		Entry("In progress", integrationteststatus.IntegrationTestStatusInProgress, "is in progress"),
		Entry("Invalid", integrationteststatus.IntegrationTestStatusTestInvalid, "is invalid"),
	)

	It("check if GenerateSummary supports all integration test statuses", func() {
		for _, teststatus := range integrationteststatus.IntegrationTestStatusValues() {
			_, err := status.GenerateSummary(teststatus, "yolo", "yolo")
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("SnapshotReportStatus (SRS)", func() {
		const (
			scenarioName = "test-scenario"
		)
		var (
			hasSRS *status.SnapshotReportStatus
			now    time.Time
		)

		BeforeEach(func() {
			var err error

			now = time.Now().UTC()

			hasSRS, err = status.NewSnapshotReportStatus("")
			Expect(err).ToNot(HaveOccurred())
		})

		It("New SRS is not dirty", func() {
			Expect(hasSRS.IsDirty()).To(BeFalse())
		})

		It("Reseting dirty bit works", func() {
			hasSRS.SetLastUpdateTime(scenarioName, now)
			Expect(hasSRS.IsDirty()).To(BeTrue())

			hasSRS.ResetDirty()
			Expect(hasSRS.IsDirty()).To(BeFalse())

			// must keep scenarios
			Expect(hasSRS.Scenarios).To(
				HaveKeyWithValue(scenarioName, &status.ScenarioReportStatus{
					LastUpdateTime: &now,
				}))
		})

		It("New scenario can be added to SRS", func() {
			hasSRS.SetLastUpdateTime(scenarioName, now)
			Expect(hasSRS.IsDirty()).To(BeTrue())

			Expect(hasSRS.Scenarios).To(
				HaveKeyWithValue(scenarioName, &status.ScenarioReportStatus{
					LastUpdateTime: &now,
				}))
		})

		It("Additional scenario can be added to SRS", func() {
			extraScenarioName := "test-scenario-2"
			hasSRS.SetLastUpdateTime(scenarioName, now)
			hasSRS.SetLastUpdateTime(extraScenarioName, now)

			Expect(hasSRS.Scenarios).To(HaveLen(2))
		})

		It("New last updated time can be assigned to existing scenario", func() {
			tNew := now.Add(1 * time.Minute)
			hasSRS.SetLastUpdateTime(scenarioName, now)
			hasSRS.ResetDirty()

			hasSRS.SetLastUpdateTime(scenarioName, tNew)
			Expect(hasSRS.Scenarios).To(
				HaveKeyWithValue(scenarioName, &status.ScenarioReportStatus{
					LastUpdateTime: &tNew,
				}))
			Expect(hasSRS.Scenarios).To(HaveLen(1))
		})

		It("Detect newer update", func() {
			tNew := now.Add(1 * time.Minute)
			hasSRS.SetLastUpdateTime(scenarioName, now)

			Expect(hasSRS.IsNewer(scenarioName, tNew)).To(BeTrue())
		})

		It("Detect no new update", func() {
			tOld := now.Add(-1 * time.Minute)
			hasSRS.SetLastUpdateTime(scenarioName, now)

			Expect(hasSRS.IsNewer(scenarioName, tOld)).To(BeFalse())
		})

		It("Can export valid annotation", func() {
			hasSRS.SetLastUpdateTime(scenarioName, now)

			annotation, err := hasSRS.ToAnnotationString()
			Expect(err).ToNot(HaveOccurred())
			Expect(annotation).ToNot(BeEmpty())

			newSRS, err := status.NewSnapshotReportStatus(annotation)
			Expect(err).ToNot(HaveOccurred())

			Expect(newSRS.Scenarios).To(HaveKey(scenarioName))
			// comparing string because it was trying to compare pointer address and it changed
			Expect(newSRS.Scenarios[scenarioName].LastUpdateTime.UnixMicro()).To(Equal(now.UnixMicro()))
			Expect(newSRS.Scenarios).To(HaveLen(1))
		})

		It("Can read annotation from snapshot", func() {
			hasSnapshot.Annotations["test.appstudio.openshift.io/git-reporter-status"] = "{\"scenarios\":{\"test-scenario\":{\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\"}}}"
			newSRS, err := status.NewSnapshotReportStatusFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(newSRS.Scenarios).To(HaveKey(scenarioName))
			Expect(newSRS.Scenarios).To(HaveLen(1))
		})

	})

})
