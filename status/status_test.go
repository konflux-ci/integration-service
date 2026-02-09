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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/operator-toolkit/metadata"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/status"
	"k8s.io/apimachinery/pkg/api/errors"
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

func newIntegrationTestStatusDetail(expectedScenarioStatus integrationteststatus.IntegrationTestStatus) integrationteststatus.IntegrationTestStatusDetail {
	ts, _ := time.Parse(time.RFC3339, "2023-07-26T16:57:49+02:00")
	tc, _ := time.Parse(time.RFC3339, "2023-07-26T17:57:49+02:00")
	return integrationteststatus.IntegrationTestStatusDetail{
		ScenarioName:        "scenario1",
		Status:              expectedScenarioStatus,
		LastUpdateTime:      time.Now().UTC(),
		Details:             "failed",
		StartTime:           &ts,
		CompletionTime:      &tc,
		TestPipelineRunName: "test-pipelinerun",
	}
}

// muxMergeRequestGet mocks a GET request for a specific Merge Request.
// It returns a MergeRequest object with the specified state.
func muxMergeRequestGet(mux *http.ServeMux, pid, mrIID int, state string) {
	path := fmt.Sprintf("/projects/%d/merge_requests/%d", pid, mrIID)

	mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
		mrData := map[string]interface{}{
			"id":         1001,
			"iid":        mrIID,
			"project_id": pid,
			"state":      state,
			"sha":        "deadbeef",
			"title":      "Mock MR",
		}

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)

		jsonMR, _ := json.Marshal(mrData)
		fmt.Fprint(rw, string(jsonMR))
	})
}

var _ = Describe("Status Adapter", func() {

	var (
		githubSnapshot          *applicationapiv1alpha1.Snapshot
		hasSnapshot             *applicationapiv1alpha1.Snapshot
		hasComSnapshot2         *applicationapiv1alpha1.Snapshot
		hasComSnapshot3         *applicationapiv1alpha1.Snapshot
		groupSnapshot           *applicationapiv1alpha1.Snapshot
		integrationTestScenario *v1beta2.IntegrationTestScenario
		hasComponent            *applicationapiv1alpha1.Component
		mockReporter            *status.MockReporterInterface

		pipelineRun       *tektonv1.PipelineRun
		successfulTaskRun *tektonv1.TaskRun
		failedTaskRun     *tektonv1.TaskRun
		skippedTaskRun    *tektonv1.TaskRun
		mockK8sClient     *MockK8sClient
		repo              pacv1alpha1.Repository

		hasComSnapshot2Name = "hascomsnapshot2-sample"
		hasComSnapshot3Name = "hascomsnapshot3-sample"

		prGroup            = "feature1"
		prGroupSha         = "feature1hash"
		plrstarttime int64 = 1775992257000 // milliseconds (was 1775992257 seconds)
		SampleImage        = "quay.io/redhat-appstudio/sample-image@sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
		SampleDigest       = "sha256:841328df1b9f8c4087adbdcfec6cc99ac8308805dea83f6d415d6fb8d40227c1"
	)

	BeforeEach(func() {
		now := time.Now()
		os.Setenv("CONSOLE_URL", "https://definetly.not.prod/preview/application-pipeline/ns/{{ .Namespace }}/pipelinerun/{{ .PipelineRunName }}")
		os.Setenv("CONSOLE_URL_TASKLOG", "https://definetly.not.prod/preview/application-pipeline/ns/{{ .Namespace }}/pipelinerun/{{ .PipelineRunName }}/logs/{{ .TaskName }}")

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
											"timestamp": "2024-05-22T06:42:21+00:00",
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
											"timestamp": "2024-05-22T06:42:21+00:00",
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
											"timestamp": "2024-05-22T06:42:21+00:00",
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
					gitops.BuildPipelineRunStartTime:                strconv.FormatInt(plrstarttime+100000, 10),
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

		hasComSnapshot2 = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hasComSnapshot2Name,
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                         gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel:                    hasComSnapshot2Name,
					gitops.PipelineAsCodeEventTypeLabel:              gitops.PipelineAsCodePullRequestType,
					gitops.PRGroupHashLabel:                          prGroupSha,
					"pac.test.appstudio.openshift.io/url-org":        "testorg",
					"pac.test.appstudio.openshift.io/url-repository": "testrepo",
					gitops.PipelineAsCodeSHALabel:                    "sha",
					gitops.PipelineAsCodePullRequestAnnotation:       "1",
				},
				Annotations: map[string]string{
					"test.appstudio.openshift.io/pr-last-update":   "2023-08-26T17:57:50+02:00",
					gitops.BuildPipelineRunStartTime:               strconv.FormatInt(plrstarttime+150000, 10), // +100 seconds = +100000 milliseconds
					gitops.PRGroupAnnotation:                       prGroup,
					gitops.PipelineAsCodeGitProviderAnnotation:     "github",
					gitops.PipelineAsCodeInstallationIDAnnotation:  "123",
					gitops.PipelineAsCodeTargetProjectIDAnnotation: "142237",
					gitops.PipelineAsCodeSourceProjectIDAnnotation: "142237",
					gitops.PipelineAsCodePullRequestAnnotation:     "5",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "application-sample",
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComSnapshot2Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
					{
						Name:           hasComSnapshot3Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
				},
			},
		}

		hasComSnapshot3 = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      hasComSnapshot3Name,
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                         gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel:                    hasComSnapshot3Name,
					gitops.PipelineAsCodeEventTypeLabel:              gitops.PipelineAsCodePullRequestType,
					gitops.PRGroupHashLabel:                          prGroupSha,
					"pac.test.appstudio.openshift.io/url-org":        "testorg",
					"pac.test.appstudio.openshift.io/url-repository": "testrepo",
					gitops.PipelineAsCodeSHALabel:                    "sha",
					gitops.PipelineAsCodePullRequestAnnotation:       "1",
				},
				Annotations: map[string]string{
					"test.appstudio.openshift.io/pr-last-update":  "2023-08-26T17:57:50+02:00",
					gitops.BuildPipelineRunStartTime:              strconv.FormatInt(plrstarttime+200000, 10), // +200 seconds = +200000 milliseconds
					gitops.PRGroupAnnotation:                      prGroup,
					gitops.PipelineAsCodeGitProviderAnnotation:    "github",
					gitops.PipelineAsCodePullRequestAnnotation:    "1",
					gitops.PipelineAsCodeInstallationIDAnnotation: "123",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "application-sample",
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComSnapshot2Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
					{
						Name:           hasComSnapshot3Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
				},
			},
		}

		groupSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "groupsnapshot",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:            gitops.SnapshotGroupType,
					gitops.PipelineAsCodeEventTypeLabel: gitops.PipelineAsCodePullRequestType,
					gitops.PRGroupHashLabel:             prGroupSha,
				},
				Annotations: map[string]string{
					gitops.PRGroupAnnotation:             prGroup,
					gitops.GroupSnapshotInfoAnnotation:   "[{\"namespace\":\"default\",\"component\":\"component1-sample\",\"buildPipelineRun\":\"\",\"snapshot\":\"hascomsnapshot2-sample\"},{\"namespace\":\"default\",\"component\":\"component3-sample\",\"buildPipelineRun\":\"\",\"snapshot\":\"hascomsnapshot3-sample\"}]",
					gitops.SnapshotTestsStatusAnnotation: "[{\"scenario\":\"scenario-1\",\"status\":\"EnvironmentProvisionError\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"Failed to find deploymentTargetClass with right provisioner for copy of existingEnvironment\"}]",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "application-sample",
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComSnapshot2Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
					{
						Name:           hasComSnapshot3Name,
						ContainerImage: SampleImage + "@" + SampleDigest,
					},
				},
			},
		}

		hasComponent = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
				Labels: map[string]string{
					"appstudio.openshift.io/application": "application-sample",
				},
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName: "component-sample",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: "",
						},
					},
				},
			},
		}

		mockK8sClient = &MockK8sClient{
			getInterceptor: func(key client.ObjectKey, obj client.Object) {
				if taskRun, ok := obj.(*tektonv1.TaskRun); ok {
					switch key.Name {
					case successfulTaskRun.Name:
						taskRun.Status = successfulTaskRun.Status
					case failedTaskRun.Name:
						taskRun.Status = failedTaskRun.Status
					case skippedTaskRun.Name:
						taskRun.Status = skippedTaskRun.Status
					}
				}
				if plr, ok := obj.(*tektonv1.PipelineRun); ok {
					if key.Name == pipelineRun.Name {
						plr.Status = pipelineRun.Status
					}
				}
				if snapshot, ok := obj.(*applicationapiv1alpha1.Snapshot); ok {
					if key.Name == hasComSnapshot2.Name {
						snapshot.Name = hasComSnapshot2.Name
					}
					if key.Name == hasComSnapshot3.Name {
						snapshot.Name = hasComSnapshot3.Name
					}
				}
			},
			listInterceptor: func(list client.ObjectList) {
				if repoList, ok := list.(*pacv1alpha1.RepositoryList); ok {
					repoList.Items = []pacv1alpha1.Repository{repo}
				}
			},
		}

		githubSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"pac.test.appstudio.openshift.io/git-provider": "github",
				},
			},
		}

		integrationTestScenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-its",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
		}

		ctrl := gomock.NewController(GinkgoT())
		mockReporter = status.NewMockReporterInterface(ctrl)
		mockReporter.EXPECT().GetReporterName().Return("mocked-reporter").AnyTimes()
		mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Return(0, nil).AnyTimes()
		mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(0, nil).AnyTimes()

		os.Setenv("CONSOLE_NAME", "Red Hat Konflux")
	})
	AfterEach(func() {
		os.Setenv("CONSOLE_URL", "")
		os.Setenv("CONSOLE_NAME", "")
	})

	It("can get reporters from a snapshot", func() {
		st := status.NewStatus(logr.Discard(), nil)
		reporter := st.GetReporter(githubSnapshot)
		Expect(reporter).ToNot(BeNil())
		Expect(reporter.GetReporterName()).To(Equal("GithubReporter"))
	})

	It("returns ForgejoReporter when snapshot has gitea provider label", func() {
		giteaSnapshot := &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					gitops.PipelineAsCodeGitProviderLabel: gitops.PipelineAsCodeGiteaProviderType,
				},
			},
		}
		st := status.NewStatus(logr.Discard(), nil)
		reporter := st.GetReporter(giteaSnapshot)
		Expect(reporter).ToNot(BeNil())
		Expect(reporter.GetReporterName()).To(Equal(status.ForgejoProvider))
		_, ok := reporter.(*status.ForgejoReporter)
		Expect(ok).To(BeTrue())
	})

	It("returns ForgejoReporter when snapshot has forgejo provider label", func() {
		forgejoSnapshot := &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					gitops.PipelineAsCodeGitProviderLabel: gitops.PipelineAsCodeForgejoProviderType,
				},
			},
		}
		st := status.NewStatus(logr.Discard(), nil)
		reporter := st.GetReporter(forgejoSnapshot)
		Expect(reporter).ToNot(BeNil())
		Expect(reporter.GetReporterName()).To(Equal(status.ForgejoProvider))
		_, ok := reporter.(*status.ForgejoReporter)
		Expect(ok).To(BeTrue())
	})

	It("can migrate snapshot to reportStatus in old way - migration test)", func() {
		hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:50+02:00\",\"details\":\"Test in progress\"}]"
		hasSnapshot.Annotations["test.appstudio.openshift.io/pr-last-update"] = "2023-08-26T17:57:50+02:00"
		statuses, err := gitops.NewSnapshotIntegrationTestStatusesFromSnapshot(hasSnapshot)
		Expect(err).NotTo(HaveOccurred())
		integrationTestStatusDetails := statuses.GetStatuses()
		status.MigrateSnapshotToReportStatus(hasSnapshot, integrationTestStatusDetails)
		Expect(hasSnapshot.Annotations[gitops.SnapshotStatusReportAnnotation]).Should(ContainSubstring("lastUpdateTime"))
	})

	It("report status for TestPassed test scenario", func() {
		hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"TestPassed\",\"testPipelineRunName\":\"test-pipelinerun\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:55+02:00\",\"details\":\"failed\"}]"
		integrationTestStatusDetail := newIntegrationTestStatusDetail(integrationteststatus.IntegrationTestStatusTestPassed)
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
| <a href="https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/test-pipelinerun/logs/pipeline1-task1">pipeline1-task1</a> | 5m0s |  | :heavy_check_mark: SUCCESS | :heavy_check_mark: 10 success(es) |
| <a href="https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/test-pipelinerun/logs/pipeline1-task2">pipeline1-task2</a> | 5m0s |  | :white_check_mark: SKIPPED |  |


`
		expectedTestReport := status.TestReport{
			FullName:            "Red Hat Konflux / scenario1 / component-sample",
			ScenarioName:        "scenario1",
			SnapshotName:        "snapshot-sample",
			ComponentName:       "component-sample",
			Text:                text,
			ShortText:           "<ul>\n<li><b>Pipelinerun</b>: <a href=\"https://definetly.not.prod/preview/application-pipeline/ns/default/pipelinerun/test-pipelinerun\">test-pipelinerun</a></li>\n</ul>\n\n",
			Summary:             "Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 has passed",
			Status:              integrationteststatus.IntegrationTestStatusTestPassed,
			StartTime:           &ts,
			CompletionTime:      &tc,
			TestPipelineRunName: "test-pipelinerun",
		}

		testReport, err := status.GenerateTestReport(context.Background(), mockK8sClient, integrationTestStatusDetail, hasSnapshot, "component-sample")
		Expect(err).NotTo(HaveOccurred())
		Expect(testReport).To(Equal(&expectedTestReport))
	})

	DescribeTable(
		"report right summary per status",
		func(expectedScenarioStatus integrationteststatus.IntegrationTestStatus, expectedTextEnding string) {

			integrationTestStatusDetail := newIntegrationTestStatusDetail(expectedScenarioStatus)

			expectedSummary := fmt.Sprintf("Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 %s", expectedTextEnding)
			testReport, err := status.GenerateTestReport(context.Background(), mockK8sClient, integrationTestStatusDetail, hasSnapshot, "component-sample")
			Expect(err).NotTo(HaveOccurred())
			Expect(testReport.Summary).To(Equal(expectedSummary))
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

	DescribeTable(
		"report right summary when its have the same status per component",
		func(expectedScenarioStatus integrationteststatus.IntegrationTestStatus, expectedTextEnding string) {

			expectedSummary := fmt.Sprintf("Integration test for component component-sample and integration test scenarios %s", expectedTextEnding)
			summary, err := status.GenerateSummaryForAllScenarios(expectedScenarioStatus, "component-sample")
			Expect(err).NotTo(HaveOccurred())
			Expect(summary).To(Equal(expectedSummary))
		},
		Entry("Pending", integrationteststatus.IntegrationTestStatusPending, "is pending"),
		Entry("BuildPLRInProgress", integrationteststatus.BuildPLRInProgress, "is pending because build pipelinerun is still running and snapshot has not been created"),
		Entry("SnapshotCreationFailed", integrationteststatus.SnapshotCreationFailed, "has not run and is considered as failed because the snapshot was not created"),
		Entry("BuildPLRFailed", integrationteststatus.BuildPLRFailed, "has not run and is considered as failed because the build pipelinerun failed and snapshot was not created"),
		Entry("GroupSnapshotCreationFailed", integrationteststatus.GroupSnapshotCreationFailed, "has not run and is considered as failed because group snapshot was not created"),
	)

	DescribeTable(
		"report right summary per status",
		func(expectedScenarioStatus integrationteststatus.IntegrationTestStatus, expectedTextEnding string) {

			integrationTestStatusDetail := newIntegrationTestStatusDetail(expectedScenarioStatus)

			expectedSummary := fmt.Sprintf("Integration test for component component-sample snapshot scenario scenario1 %s", expectedTextEnding)
			testReport, err := status.GenerateTestReport(context.Background(), mockK8sClient, integrationTestStatusDetail, hasSnapshot, "component-sample")
			Expect(err).NotTo(HaveOccurred())
			Expect(testReport.Summary).To(Equal(expectedSummary))
		},
		Entry("BuildPLRInProgress", integrationteststatus.BuildPLRInProgress, "is pending because build pipelinerun is still running and snapshot has not been created"),
		Entry("SnapshotCreationFailed", integrationteststatus.SnapshotCreationFailed, "has not run and is considered as failed because the snapshot was not created"),
		Entry("BuildPLRFailed", integrationteststatus.BuildPLRFailed, "has not run and is considered as failed because the build pipelinerun failed and snapshot was not created"),
	)

	It("check if GenerateSummary supports all integration test statuses", func() {
		for _, teststatus := range integrationteststatus.IntegrationTestStatusValues() {
			_, err := status.GenerateSummary(teststatus, "yolo", "yolo", "yoyo")
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("check getting component snapshots from group snapshot", func() {
		componentSnapshots, err := status.GetComponentSnapshotsFromGroupSnapshot(context.Background(), mockK8sClient, groupSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(componentSnapshots).To(HaveLen(2))

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
			Expect(mockK8sClient.Create(context.Background(), hasSnapshot)).Should(Succeed())
			hasSRS.SetLastUpdateTime(scenarioName, hasSnapshot.Name, now)
			Expect(hasSRS.IsDirty()).To(BeTrue())

			hasSRS.ResetDirty()
			Expect(hasSRS.IsDirty()).To(BeFalse())

			// must keep scenarios
			Expect(hasSRS.Scenarios).To(
				HaveKeyWithValue(scenarioName+"-"+hasSnapshot.Name, &status.ScenarioReportStatus{
					LastUpdateTime: &now,
				}))
			Expect(hasSnapshot.Annotations[gitops.SnapshotStatusReportAnnotation]).To(Equal(""))
			err := status.WriteSnapshotReportStatus(context.Background(), mockK8sClient, hasSnapshot, hasSRS)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasSnapshot.Annotations[gitops.SnapshotStatusReportAnnotation]).NotTo(BeNil())
			err = mockK8sClient.Delete(context.Background(), hasSnapshot)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("New scenario can be added to SRS", func() {
			Expect(mockK8sClient.Create(context.Background(), hasSnapshot)).Should(Succeed())
			hasSRS.SetLastUpdateTime(scenarioName, hasSnapshot.Name, now)
			Expect(hasSRS.IsDirty()).To(BeTrue())

			Expect(hasSRS.Scenarios).To(
				HaveKeyWithValue(scenarioName+"-"+hasSnapshot.Name, &status.ScenarioReportStatus{
					LastUpdateTime: &now,
				}))

			Expect(hasSnapshot.Annotations[gitops.SnapshotStatusReportAnnotation]).To(Equal(""))
			err := status.WriteSnapshotReportStatus(context.Background(), mockK8sClient, hasSnapshot, hasSRS)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasSnapshot.Annotations[gitops.SnapshotStatusReportAnnotation]).NotTo(BeNil())
			err = mockK8sClient.Delete(context.Background(), hasSnapshot)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("Additional scenario can be added to SRS", func() {
			extraScenarioName := "test-scenario-2"
			hasSRS.SetLastUpdateTime(scenarioName, hasSnapshot.Name, now)
			hasSRS.SetLastUpdateTime(extraScenarioName, hasSnapshot.Name, now)

			Expect(hasSRS.Scenarios).To(HaveLen(2))
		})

		It("New last updated time can be assigned to existing scenario", func() {
			tNew := now.Add(1 * time.Minute)
			hasSRS.SetLastUpdateTime(scenarioName, hasSnapshot.Name, now)
			hasSRS.ResetDirty()

			hasSRS.SetLastUpdateTime(scenarioName, hasSnapshot.Name, tNew)
			Expect(hasSRS.Scenarios).To(
				HaveKeyWithValue(scenarioName+"-"+hasSnapshot.Name, &status.ScenarioReportStatus{
					LastUpdateTime: &tNew,
				}))
			Expect(hasSRS.Scenarios).To(HaveLen(1))
		})

		It("Detect newer update", func() {
			tNew := now.Add(1 * time.Minute)
			hasSRS.SetLastUpdateTime(scenarioName, hasSnapshot.Name, now)

			Expect(hasSRS.IsNewer(scenarioName, hasSnapshot.Name, tNew)).To(BeTrue())
		})

		It("Detect no new update", func() {
			tOld := now.Add(-1 * time.Minute)
			hasSRS.SetLastUpdateTime(scenarioName, hasSnapshot.Name, now)

			Expect(hasSRS.IsNewer(scenarioName, hasSnapshot.Name, tOld)).To(BeFalse())
		})

		It("Can export valid annotation", func() {
			hasSRS.SetLastUpdateTime(scenarioName, hasSnapshot.Name, now)

			annotation, err := hasSRS.ToAnnotationString()
			Expect(err).ToNot(HaveOccurred())
			Expect(annotation).ToNot(BeEmpty())

			newSRS, err := status.NewSnapshotReportStatus(annotation)
			Expect(err).ToNot(HaveOccurred())

			Expect(newSRS.Scenarios).To(HaveKey(scenarioName + "-" + hasSnapshot.Name))
			// comparing string because it was trying to compare pointer address and it changed
			Expect(newSRS.Scenarios[scenarioName+"-"+hasSnapshot.Name].LastUpdateTime.UnixMicro()).To(Equal(now.UnixMicro()))
			Expect(newSRS.Scenarios).To(HaveLen(1))
		})

		It("Can read annotation from snapshot", func() {
			hasSnapshot.Annotations["test.appstudio.openshift.io/git-reporter-status"] = "{\"scenarios\":{\"test-scenario-snapshot-sample\":{\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\"}}}"
			newSRS, err := status.NewSnapshotReportStatusFromSnapshot(hasSnapshot)
			Expect(err).ToNot(HaveOccurred())

			Expect(newSRS.Scenarios).To(HaveKey(scenarioName + "-" + hasSnapshot.Name))
			Expect(newSRS.Scenarios).To(HaveLen(1))
		})

		It("can return unrecoverable error when label is not defined for githubReporter", func() {
			metadataErr := metadata.DeleteLabel(hasSnapshot, gitops.PipelineAsCodeURLOrgLabel)
			Expect(metadataErr).ToNot(HaveOccurred())
			githubReporter := status.NewGitHubReporter(logr.Discard(), mockK8sClient)
			statusCode, err := githubReporter.Initialize(context.Background(), hasSnapshot)
			Expect(helpers.IsUnrecoverableMetadataError(err)).To(BeTrue())
			Expect(statusCode).To(Equal(0))

			err = metadata.SetLabel(hasSnapshot, gitops.PipelineAsCodeURLOrgLabel, "org")
			Expect(err).ToNot(HaveOccurred())
			err = metadata.DeleteLabel(hasSnapshot, gitops.PipelineAsCodeURLRepositoryLabel)
			Expect(err).ToNot(HaveOccurred())
			statusCode, err = githubReporter.Initialize(context.Background(), hasSnapshot)
			Expect(helpers.IsUnrecoverableMetadataError(err)).To(BeTrue())
			Expect(statusCode).To(Equal(0))

			err = metadata.SetLabel(hasSnapshot, gitops.PipelineAsCodeURLRepositoryLabel, "repo")
			Expect(err).ToNot(HaveOccurred())
			err = metadata.DeleteLabel(hasSnapshot, gitops.PipelineAsCodeSHALabel)
			Expect(err).ToNot(HaveOccurred())
			statusCode, err = githubReporter.Initialize(context.Background(), hasSnapshot)
			Expect(helpers.IsUnrecoverableMetadataError(err)).To(BeTrue())
			Expect(statusCode).To(Equal(0))

		})

		It("can return unrecoverable error when label/annotation is not defined for gitlabReporter", func() {
			err := metadata.DeleteAnnotation(hasSnapshot, gitops.PipelineAsCodeRepoURLAnnotation)
			Expect(err).ToNot(HaveOccurred())
			gitlabReporter := status.NewGitLabReporter(logr.Discard(), mockK8sClient)
			statusCode, err := gitlabReporter.Initialize(context.Background(), hasSnapshot)
			Expect(helpers.IsUnrecoverableMetadataError(err)).To(BeTrue())
			Expect(statusCode).To(Equal(0))

			err = metadata.SetAnnotation(hasSnapshot, gitops.PipelineAsCodeRepoURLAnnotation, "https://test-repo.example.com")
			Expect(err).ToNot(HaveOccurred())
			err = metadata.SetAnnotation(hasSnapshot, gitops.PipelineAsCodeSourceProjectIDAnnotation, "qqq")
			Expect(err).ToNot(HaveOccurred())
			statusCode, err = gitlabReporter.Initialize(context.Background(), hasSnapshot)
			Expect(helpers.IsUnrecoverableMetadataError(err)).To(BeTrue())
			Expect(statusCode).To(Equal(0))
		})
	})

	It("can report status in IterateIntegrationTestScenarioWithSameStatus", func() {
		integrationTestStatusDetail := integrationteststatus.IntegrationTestStatusDetail{
			Status:  integrationteststatus.GroupSnapshotCreationFailed,
			Details: "details",
		}
		ctrl := gomock.NewController(GinkgoT())
		mockReporter = status.NewMockReporterInterface(ctrl)
		mockReporter.EXPECT().GetReporterName().Return(status.GitLabProvider).AnyTimes()
		mockReporter.EXPECT().Initialize(gomock.Any(), gomock.Any()).Return(0, nil).AnyTimes()
		mockReporter.EXPECT().ReportStatus(gomock.Any(), gomock.Any()).Return(0, nil).AnyTimes()
		commentText, _ := status.GenerateSummaryForAllScenarios(integrationTestStatusDetail.Status, "component-sample")
		commentText, _ = status.FormatComment(commentText, integrationTestStatusDetail.Details)
		mockReporter.EXPECT().UpdateStatusInComment(status.GenerateTestSummaryPrefixForComponent("component-sample"), commentText).Return(0, nil).AnyTimes()
		hasSnapshot.Labels["pac.test.appstudio.openshift.io/git-provider"] = "gitlab"
		hasSnapshot.Annotations[gitops.PipelineAsCodePullRequestAnnotation] = "123"

		statusCode, err := status.IterateIntegrationTestScenarioWithSameStatus(context.Background(), mockK8sClient, mockReporter, hasSnapshot, &[]v1beta2.IntegrationTestScenario{*integrationTestScenario}, integrationTestStatusDetail, hasComponent, "component-sample")
		Expect(err).Should(Succeed())
		Expect(statusCode).To(BeZero())
	})
	Context("can get correct MR status", func() {
		var (
			defaultAPIURL = "/api/v4"
			mux           *http.ServeMux
			server        *httptest.Server
			secretData    map[string][]byte
		)
		BeforeEach(func() {
			mux = http.NewServeMux()
			apiHandler := http.NewServeMux()
			apiHandler.Handle(defaultAPIURL+"/", http.StripPrefix(defaultAPIURL, mux))

			// server is a test HTTP server used to provide mock API responses
			server = httptest.NewServer(apiHandler)

			repo = pacv1alpha1.Repository{
				Spec: pacv1alpha1.RepositorySpec{
					URL: server.URL, // mocked URL
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{
							Name: "example-secret-name",
							Key:  "example-token",
						},
					},
				},
			}

			mockK8sClient = &MockK8sClient{
				getInterceptor: func(key client.ObjectKey, obj client.Object) {
					if secret, ok := obj.(*v1.Secret); ok {
						secret.Data = secretData
					}
				},
				listInterceptor: func(list client.ObjectList) {
					if repoList, ok := list.(*pacv1alpha1.RepositoryList); ok {
						repoList.Items = []pacv1alpha1.Repository{repo}
					}
				},
			}

			secretData = map[string][]byte{
				"example-token": []byte("example-personal-access-token"),
			}
		})
		AfterEach(func() {
			server.Close()
		})

		It("can get correct MR status when processed snapshot is in the snapshot list", func() {
			// mock URL with httptest server URL
			hasComSnapshot3.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = server.URL
			st := status.NewStatus(logr.Discard(), mockK8sClient)
			hasComSnapshot2.Annotations[gitops.PipelineAsCodePullRequestAnnotation] = hasComSnapshot3.Annotations[gitops.PipelineAsCodePullRequestAnnotation]
			hasComSnapshot2.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = hasComSnapshot3.Annotations[gitops.PipelineAsCodeRepoURLAnnotation]
			tmpSnapshot, statusCode, err := st.FindSnapshotWithOpenedPR(context.Background(), &[]applicationapiv1alpha1.Snapshot{*hasSnapshot, *hasComSnapshot2, *hasComSnapshot3}, hasComSnapshot3)
			Expect(err).Should(Succeed())
			Expect(statusCode).To(BeZero())
			Expect(tmpSnapshot.Name).To(Equal(hasComSnapshot3.Name))
		})

		It("can get correct MR status when processed snapshot is not in the snapshot list", func() {
			hasComSnapshot2.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = server.URL
			hasComSnapshot2.Annotations[gitops.PipelineAsCodeGitProviderLabel] = gitops.PipelineAsCodeGitLabProviderType
			st := status.NewStatus(logr.Discard(), mockK8sClient)
			reporter := status.NewGitLabReporter(logr.Discard(), mockK8sClient)
			_, err := reporter.Initialize(context.TODO(), hasComSnapshot2)
			Expect(err).Should(Succeed())
			pid := 142237
			mrIID := 5
			muxMergeRequestGet(mux, pid, mrIID, "opened")
			tmpSnapshot, statusCode, err := st.FindSnapshotWithOpenedPR(context.Background(), &[]applicationapiv1alpha1.Snapshot{*hasSnapshot, *hasComSnapshot2}, hasComSnapshot3)
			Expect(err).Should(Succeed())
			Expect(statusCode).To(Equal(200))
			Expect(tmpSnapshot.Name).To(Equal(hasComSnapshot2.Name))
		})
	})
})
