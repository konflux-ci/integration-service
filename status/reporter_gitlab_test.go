/*
Copyright 2024 Red Hat Inc.

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	"github.com/tonglil/buflogr"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/status"
)

var _ = Describe("GitLabReporter", func() {

	const (
		repoUrl         = "https://gitlab.com/example/example"
		digest          = "12a4a35ccd08194595179815e4646c3a6c08bb77"
		sourceProjectID = "123"
		targetProjectID = "456"
		mergeRequest    = "45"
	)

	var (
		hasSnapshot   *applicationapiv1alpha1.Snapshot
		mockK8sClient *MockK8sClient
		buf           bytes.Buffer
		log           logr.Logger
	)

	BeforeEach(func() {
		log = buflogr.NewWithBuffer(&buf)

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					"test.appstudio.openshift.io/type":               "component",
					"appstudio.openshift.io/component":               "component-sample",
					"build.appstudio.redhat.com/pipeline":            "enterprise-contract",
					"pac.test.appstudio.openshift.io/url-org":        "devfile-sample",
					"pac.test.appstudio.openshift.io/url-repository": "devfile-sample-go-basic",
					"pac.test.appstudio.openshift.io/sha":            "12a4a35ccd08194595179815e4646c3a6c08bb77",
					"pac.test.appstudio.openshift.io/event-type":     "Merge Request",
				},
				Annotations: map[string]string{
					"build.appstudio.redhat.com/commit_sha":             digest,
					"appstudio.redhat.com/updateComponentOnSuccess":     "false",
					"pac.test.appstudio.openshift.io/git-provider":      "gitlab",
					"pac.test.appstudio.openshift.io/repo-url":          repoUrl,
					"pac.test.appstudio.openshift.io/target-project-id": targetProjectID,
					"pac.test.appstudio.openshift.io/source-project-id": sourceProjectID,
					"pac.test.appstudio.openshift.io/pull-request":      mergeRequest,
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

	})

	It("Reporter can return name uninitialized", func() {
		reporter := status.NewGitLabReporter(log, mockK8sClient)
		Expect(reporter.GetReporterName()).To(Equal("GitlabReporter"))
	})

	It("can detect if gitlab reporter should be used", func() {
		reporter := status.NewGitLabReporter(log, mockK8sClient)
		hasSnapshot.Annotations["pac.test.appstudio.openshift.io/git-provider"] = "gitlab"
		Expect(reporter.Detect(hasSnapshot)).To(BeTrue())

		hasSnapshot.Annotations["pac.test.appstudio.openshift.io/git-provider"] = "not-gitlab"
		Expect(reporter.Detect(hasSnapshot)).To(BeFalse())

		hasSnapshot.Labels["pac.test.appstudio.openshift.io/git-provider"] = "gitlab"
		Expect(reporter.Detect(hasSnapshot)).To(BeTrue())

		hasSnapshot.Labels["pac.test.appstudio.openshift.io/git-provider"] = "not-gitlab"
		Expect(reporter.Detect(hasSnapshot)).To(BeFalse())
	})

	Context("when provided Gitlab webhook integration credentials", func() {

		var (
			secretData    map[string][]byte
			repo          pacv1alpha1.Repository
			reporter      *status.GitLabReporter
			defaultAPIURL = "/api/v4"
			mux           *http.ServeMux
			server        *httptest.Server
		)

		BeforeEach(func() {
			mux = http.NewServeMux()
			apiHandler := http.NewServeMux()
			apiHandler.Handle(defaultAPIURL+"/", http.StripPrefix(defaultAPIURL, mux))

			// server is a test HTTP server used to provide mock API responses
			server = httptest.NewServer(apiHandler)

			// mock URL with httptest server URL
			hasSnapshot.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = server.URL

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

			reporter = status.NewGitLabReporter(log, mockK8sClient)

			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(0))

		})

		AfterEach(func() {
			server.Close()
		})

		DescribeTable("test handling of missing labels/annotations", func(missingKey string, isLabel bool) {
			if isLabel {
				delete(hasSnapshot.Labels, missingKey)
			} else {
				delete(hasSnapshot.Annotations, missingKey)
			}
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).ToNot(Succeed())
			Expect(statusCode).To(Equal(0))
		},
			Entry("Missing repo_url", gitops.PipelineAsCodeRepoURLAnnotation, false),
			Entry("Missing SHA", gitops.PipelineAsCodeSHALabel, true),
			Entry("Missing target project ID", gitops.PipelineAsCodeTargetProjectIDAnnotation, false),
			Entry("Missing source project ID", gitops.PipelineAsCodeSourceProjectIDAnnotation, false),
		)

		It("creates a commit status for snapshot with correct textual data", func() {

			summary := "Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 failed"

			muxCommitStatusPost(mux, sourceProjectID, digest, summary)

			statusCode, err := reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:     "fullname/scenario1",
					ScenarioName: "scenario1",
					Status:       integrationteststatus.IntegrationTestStatusEnvironmentProvisionError_Deprecated,
					Summary:      summary,
					Text:         "detailed text here",
				})
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(200))
		})

		It("creates a commit status for push snapshot with correct textual data without comments", func() {

			pushSnapshot := hasSnapshot.DeepCopy()
			// Removing the pull request annotation and adding the push label
			delete(pushSnapshot.Annotations, gitops.PipelineAsCodePullRequestAnnotation)
			pushSnapshot.Annotations[gitops.PipelineAsCodeEventTypeLabel] = "Push"

			pushEventReporter := status.NewGitLabReporter(log, mockK8sClient)

			statusCode, err := pushEventReporter.Initialize(context.TODO(), pushSnapshot)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(0))

			summary := "Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 failed"

			muxCommitStatusPost(mux, sourceProjectID, digest, summary)

			statusCode, err = pushEventReporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:     "fullname/scenario1",
					ScenarioName: "scenario1",
					Status:       integrationteststatus.IntegrationTestStatusEnvironmentProvisionError_Deprecated,
					Summary:      summary,
					Text:         "detailed text here",
				})
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(200))
		})

		It("creates a commit status for snapshot with TargetURL in CommitStatus", func() {

			PipelineRunName := "TestPipeline"
			expectedURL := status.FormatPipelineURL(PipelineRunName, hasSnapshot.Namespace, logr.Discard())

			muxCommitStatusPost(mux, sourceProjectID, digest, expectedURL)
			muxCommitStatusesGet(mux, sourceProjectID, digest, nil)

			statusCode, err := reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:            "fullname/scenario1",
					ScenarioName:        "scenario1",
					TestPipelineRunName: PipelineRunName,
					Status:              integrationteststatus.IntegrationTestStatusInProgress,
					Summary:             "summary",
					Text:                "detailed text here",
				})
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(200))
		})

		It("does not create a commit status or comment for snapshot with existing matching checkRun in running state", func() {
			summary := "Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 is running"

			report := status.TestReport{
				FullName:     "fullname/scenario1",
				ScenarioName: "scenario1",
				Status:       integrationteststatus.IntegrationTestStatusInProgress,
				Summary:      summary,
				Text:         "detailed text here",
			}

			muxCommitStatusesGet(mux, sourceProjectID, digest, &report)

			statusCode, err := reporter.ReportStatus(context.TODO(), report)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(200))
		})

		It("can get an existing commitStatus that matches the report", func() {
			summary := "Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 failed"
			report := status.TestReport{
				FullName:     "fullname/scenario1",
				ScenarioName: "scenario1",
				Status:       integrationteststatus.IntegrationTestStatusTestPassed,
				Summary:      summary,
				Text:         "detailed text here",
			}

			commitStatus := gitlab.CommitStatus{}
			commitStatus.ID = 123
			commitStatus.Name = report.FullName
			commitStatus.Status = string(gitlab.Running)
			commitStatus.Description = report.Summary

			commitStatuses := []*gitlab.CommitStatus{
				&commitStatus,
			}

			existingCommitStatus := reporter.GetExistingCommitStatus(commitStatuses, report.FullName)

			Expect(existingCommitStatus.Name).To(Equal(commitStatus.Name))
			Expect(existingCommitStatus.ID).To(Equal(commitStatus.ID))
			Expect(existingCommitStatus.Status).To(Equal(commitStatus.Status))
		})

		It("can delete mergeRequest notes that match the report then create a new comment", func() {
			reporter := status.NewGitLabReporter(log, mockK8sClient)
			_, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(Succeed())
			commentPrefix := status.GenerateTestSummaryPrefixForComponent("component-sample")
			commentText, _ := status.GenerateSummaryForAllScenarios(integrationteststatus.IntegrationTestStatusTestPassed, "component-sample")
			muxMergeNotes(mux, sourceProjectID, mergeRequest, "")
			muxMergeNotes(mux, targetProjectID, mergeRequest, commentText)
			statusCode, err := reporter.UpdateStatusInComment(commentPrefix, commentText)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(201))
		})
	})
	Describe("Test helper functions", func() {

		DescribeTable(
			"reports correct gitlab statuses from test statuses",
			func(teststatus integrationteststatus.IntegrationTestStatus, glState gitlab.BuildStateValue) {

				state, err := status.GenerateGitlabCommitState(teststatus, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(state).To(Equal(glState))
			},
			Entry("Provision error", integrationteststatus.IntegrationTestStatusEnvironmentProvisionError_Deprecated, gitlab.Failed),
			Entry("Deployment error", integrationteststatus.IntegrationTestStatusDeploymentError_Deprecated, gitlab.Failed),
			Entry("Deleted", integrationteststatus.IntegrationTestStatusDeleted, gitlab.Canceled),
			Entry("Success", integrationteststatus.IntegrationTestStatusTestPassed, gitlab.Success),
			Entry("Test failure", integrationteststatus.IntegrationTestStatusTestFail, gitlab.Failed),
			Entry("In progress", integrationteststatus.IntegrationTestStatusInProgress, gitlab.Running),
			Entry("Pending", integrationteststatus.IntegrationTestStatusPending, gitlab.Pending),
			Entry("Invalid", integrationteststatus.IntegrationTestStatusTestInvalid, gitlab.Failed),
			Entry("BuildPLRInProgress", integrationteststatus.BuildPLRInProgress, gitlab.Pending),
			Entry("BuildPLRFailed", integrationteststatus.BuildPLRFailed, gitlab.Canceled),
			Entry("SnapshotCreationFailed", integrationteststatus.SnapshotCreationFailed, gitlab.Canceled),
			Entry("GroupSnapshotCreationFailed", integrationteststatus.GroupSnapshotCreationFailed, gitlab.Canceled),
		)

		It("check if all integration tests statuses are supported", func() {
			for _, teststatus := range integrationteststatus.IntegrationTestStatusValues() {
				_, err := status.GenerateGitlabCommitState(teststatus, false)
				Expect(err).ToNot(HaveOccurred())
			}
		})
		It("check if optional integration test returns skipped status for failed test", func() {
			state, err := status.GenerateGitlabCommitState(integrationteststatus.IntegrationTestStatusTestFail, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(gitlab.Skipped))
		})
		It("check if optional integration test returns skipped status for invalid test", func() {
			state, err := status.GenerateGitlabCommitState(integrationteststatus.IntegrationTestStatusTestInvalid, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(gitlab.Skipped))
		})
	})
})

// muxCommitStatusPost mocks commit status POST request, if catchStr is non-empty POST request must contain such substring
func muxCommitStatusPost(mux *http.ServeMux, pid string, sha string, catchStr string) {
	path := fmt.Sprintf("/projects/%s/statuses/%s", pid, sha)
	mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
		bit, _ := io.ReadAll(r.Body)
		s := string(bit)
		if catchStr != "" {
			Expect(s).To(ContainSubstring(catchStr))
		}
		fmt.Fprintf(rw, "{}")
	})
}

// muxCommitStatusesGet mocks commit statuses GET request,
// if report is non-empty GET request will return a matching commitStatus
func muxCommitStatusesGet(mux *http.ServeMux, pid string, sha string, report *status.TestReport) {
	path := fmt.Sprintf("/projects/%s/repository/commits/%s/statuses", pid, sha)
	mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
		output := "[]"
		if report != nil {
			commitStatus := gitlab.CommitStatus{
				ID:          123,
				Name:        report.FullName,
				Status:      string(gitlab.Running),
				Description: report.Summary,
			}

			jsonStatuses, _ := json.Marshal([]gitlab.CommitStatus{commitStatus})
			output = string(jsonStatuses)
		}
		fmt.Fprint(rw, output)
	})
}

// muxMergeNotes mocks merge request notes GET and POST requests, if catchStr is non-empty POST request must contain such substring
func muxMergeNotes(mux *http.ServeMux, pid string, mr string, catchStr string) {
	path := fmt.Sprintf("/projects/%s/merge_requests/%s/notes", pid, mr)
	mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			bit, _ := io.ReadAll(r.Body)
			s := string(bit)
			if catchStr != "" {
				Expect(s).To(ContainSubstring(catchStr))
			}
			rw.WriteHeader(http.StatusCreated) // Simulate 201 Created
			fmt.Fprintf(rw, `{"id": 1000, "body": "new comment"}`)

		case "DELETE":
			rw.WriteHeader(http.StatusNoContent) // 204 No Content
			fmt.Fprintf(rw, "")

		case "GET":
			rw.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(rw, `[
				{"id": 1, "body": "Integration test report for component component-sample"},
				{"id": 2, "body": "Integration test report for component other-component"}
			]`)

		default:
			rw.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	deletePath := path + "/"
	mux.HandleFunc(deletePath, func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			// 可以在这里验证是否删除了正确的 ID
			rw.WriteHeader(http.StatusNoContent)
			return
		}
	})
}
