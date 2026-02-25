/*
Copyright 2026 Red Hat Inc.

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
	"sync/atomic"
	"time"

	forgejo "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v2"
	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	"github.com/tonglil/buflogr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/status"
)

var _ = Describe("ForgejoReporter", func() {

	const (
		repoUrl     = "https://codeberg.org/example/example"
		digest      = "12a4a35ccd08194595179815e4646c3a6c08bb77"
		pullRequest = "45"
		owner       = "example"
		repo        = "example"
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
					"pac.test.appstudio.openshift.io/url-org":        owner,
					"pac.test.appstudio.openshift.io/url-repository": repo,
					"pac.test.appstudio.openshift.io/sha":            digest,
					"pac.test.appstudio.openshift.io/event-type":     "Pull Request",
				},
				Annotations: map[string]string{
					"build.appstudio.redhat.com/commit_sha":         digest,
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
					"pac.test.appstudio.openshift.io/git-provider":  "forgejo",
					"pac.test.appstudio.openshift.io/repo-url":      repoUrl,
					"pac.test.appstudio.openshift.io/pull-request":  pullRequest,
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
		reporter := status.NewForgejoReporter(log, mockK8sClient)
		Expect(reporter.GetReporterName()).To(Equal("ForgejoReporter"))
	})

	It("can detect if forgejo reporter should be used", func() {
		reporter := status.NewForgejoReporter(log, mockK8sClient)
		hasSnapshot.Annotations["pac.test.appstudio.openshift.io/git-provider"] = "forgejo"
		Expect(reporter.Detect(hasSnapshot)).To(BeTrue())

		hasSnapshot.Annotations["pac.test.appstudio.openshift.io/git-provider"] = "not-forgejo"
		Expect(reporter.Detect(hasSnapshot)).To(BeFalse())

		hasSnapshot.Labels["pac.test.appstudio.openshift.io/git-provider"] = "forgejo"
		Expect(reporter.Detect(hasSnapshot)).To(BeTrue())

		hasSnapshot.Labels["pac.test.appstudio.openshift.io/git-provider"] = "not-forgejo"
		Expect(reporter.Detect(hasSnapshot)).To(BeFalse())
	})

	It("can detect when PaC is configured as gitea (workaround for Forgejo)", func() {
		reporter := status.NewForgejoReporter(log, mockK8sClient)
		hasSnapshot.Annotations["pac.test.appstudio.openshift.io/git-provider"] = "gitea"
		Expect(reporter.Detect(hasSnapshot)).To(BeTrue())

		hasSnapshot.Annotations["pac.test.appstudio.openshift.io/git-provider"] = "other"
		hasSnapshot.Labels["pac.test.appstudio.openshift.io/git-provider"] = "gitea"
		Expect(reporter.Detect(hasSnapshot)).To(BeTrue())
	})

	Context("when provided Forgejo webhook integration credentials", func() {

		var (
			secretData    map[string][]byte
			repoCR        pacv1alpha1.Repository
			reporter      *status.ForgejoReporter
			defaultAPIURL = "/api/v1"
			mux           *http.ServeMux
			server        *httptest.Server
		)

		BeforeEach(func() {
			mux = http.NewServeMux()
			apiHandler := http.NewServeMux()
			apiHandler.Handle(defaultAPIURL+"/", http.StripPrefix(defaultAPIURL, mux))

			// Mock the /version endpoint that the forgejo client calls during initialization
			mux.HandleFunc("/version", func(rw http.ResponseWriter, r *http.Request) {
				rw.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(rw, `{"version": "1.20.0"}`)
			})

			// server is a test HTTP server used to provide mock API responses
			server = httptest.NewServer(apiHandler)

			// mock URL with httptest server URL, but include the owner/repo path for parsing
			mockRepoURL := fmt.Sprintf("%s/%s/%s", server.URL, owner, repo)
			hasSnapshot.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = mockRepoURL

			repoCR = pacv1alpha1.Repository{
				Spec: pacv1alpha1.RepositorySpec{
					URL: mockRepoURL,
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
						repoList.Items = []pacv1alpha1.Repository{repoCR}
					}
				},
			}

			secretData = map[string][]byte{
				"example-token": []byte("example-personal-access-token"),
			}

			reporter = status.NewForgejoReporter(log, mockK8sClient)

			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(0))

		})

		AfterEach(func() {
			server.Close()
		})

		DescribeTable("test handling of missing labels/annotations", func(missingKey string, isLabel bool) {
			testSnapshot := hasSnapshot.DeepCopy()
			if isLabel {
				delete(testSnapshot.Labels, missingKey)
			} else {
				delete(testSnapshot.Annotations, missingKey)
			}
			testReporter := status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := testReporter.Initialize(context.TODO(), testSnapshot)
			Expect(err).ToNot(Succeed())
			Expect(statusCode).To(Equal(0))
		},
			Entry("Missing repo_url", gitops.PipelineAsCodeRepoURLAnnotation, false),
			Entry("Missing SHA", gitops.PipelineAsCodeSHALabel, true),
		)

		It("sends valid success commit status payload to the API", func() {
			summary := "Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 passed"
			muxForgejoCommitStatusPost(mux, owner, repo, digest, summary, "success")

			statusCode, err := reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:     "fullname/scenario1",
					ScenarioName: "scenario1",
					Status:       integrationteststatus.IntegrationTestStatusTestPassed,
					Summary:      summary,
					Text:         "detailed text here",
				})
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(200))
		})

		It("creates a commit status for push snapshot with correct textual data without comments", func() {
			pushSnapshot := hasSnapshot.DeepCopy()
			delete(pushSnapshot.Annotations, gitops.PipelineAsCodePullRequestAnnotation)
			pushSnapshot.Annotations[gitops.PipelineAsCodeEventTypeLabel] = "Push"

			pushEventReporter := status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := pushEventReporter.Initialize(context.TODO(), pushSnapshot)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(0))

			summary := "Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 passed"
			muxForgejoCommitStatusPost(mux, owner, repo, digest, summary, "")

			statusCode, err = pushEventReporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:     "fullname/scenario1",
					ScenarioName: "scenario1",
					Status:       integrationteststatus.IntegrationTestStatusTestPassed,
					Summary:      summary,
					Text:         "detailed text here",
				})
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(200))
		})

		It("creates a commit status for snapshot with TargetURL in CommitStatus", func() {
			PipelineRunName := "TestPipeline"
			expectedURL := status.FormatPipelineURL(PipelineRunName, hasSnapshot.Namespace, logr.Discard())

			muxForgejoCommitStatusPost(mux, owner, repo, digest, expectedURL, "")
			muxForgejoCommitStatusesGet(mux, owner, repo, digest, nil)

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

		It("sends failure commit status payload and comment text body to the API", func() {
			summary := "Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 failed"
			muxForgejoCommitStatusPost(mux, owner, repo, digest, summary, "error")

			commentPrefix := status.GenerateTestSummaryPrefixForComponent("component-sample")
			commentText, err := status.GenerateSummaryForAllScenarios(integrationteststatus.SnapshotCreationFailed, "component-sample")
			Expect(err).To(Succeed())
			Expect(commentText).ToNot(BeEmpty())
			muxForgejoIssueComments(mux, owner, repo, pullRequest, commentText)

			statusCode, err := reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:     "fullname/scenario1",
					ScenarioName: "scenario1",
					Status:       integrationteststatus.IntegrationTestStatusTestFail,
					Summary:      summary,
					Text:         "detailed text here",
				})
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(200))

			statusCode, err = reporter.UpdateStatusInComment(commentPrefix, commentText)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(201))
		})

		It("does not create a commit status for snapshot with existing matching status in pending state", func() {
			summary := "Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 is pending"
			report := status.TestReport{
				FullName:     "fullname/scenario1",
				ScenarioName: "scenario1",
				Status:       integrationteststatus.IntegrationTestStatusPending,
				Summary:      summary,
				Text:         "detailed text here",
			}
			muxForgejoCommitStatusesGet(mux, owner, repo, digest, &report)

			statusCode, err := reporter.ReportStatus(context.TODO(), report)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(200))
		})

		It("can get an existing commitStatus that matches the report", func() {
			summary := "Integration test for component component-sample snapshot snapshot-sample and scenario scenario1 passed"
			report := status.TestReport{
				FullName:     "fullname/scenario1",
				ScenarioName: "scenario1",
				Status:       integrationteststatus.IntegrationTestStatusTestPassed,
				Summary:      summary,
				Text:         "detailed text here",
			}
			commitStatus := forgejo.Status{}
			commitStatus.ID = 123
			commitStatus.Context = report.FullName
			commitStatus.State = forgejo.StatusState("pending")
			commitStatus.Description = report.Summary
			commitStatuses := []*forgejo.Status{&commitStatus}

			existingCommitStatus := reporter.GetExistingCommitStatus(commitStatuses, report.FullName)
			Expect(existingCommitStatus.Context).To(Equal(commitStatus.Context))
			Expect(existingCommitStatus.ID).To(Equal(commitStatus.ID))
			Expect(existingCommitStatus.State).To(Equal(commitStatus.State))
		})

		It("can delete issue comments that match the report then create a new comment", func() {
			reporter := status.NewForgejoReporter(log, mockK8sClient)
			_, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(Succeed())
			commentPrefix := status.GenerateTestSummaryPrefixForComponent("component-sample")
			commentText := "Integration test report for component component-sample: All tests passed"
			muxForgejoIssueComments(mux, owner, repo, pullRequest, commentText)
			statusCode, err := reporter.UpdateStatusInComment(commentPrefix, commentText)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(201))
		})
	})

	Context("when testing retry behavior", func() {
		var (
			secretData    map[string][]byte
			repoCR        pacv1alpha1.Repository
			reporter      *status.ForgejoReporter
			defaultAPIURL = "/api/v1"
			mux           *http.ServeMux
			server        *httptest.Server
			savedBackoff  wait.Backoff
		)

		BeforeEach(func() {
			buf.Reset()

			savedBackoff = status.GetReporterRetryBackoff()
			status.SetReporterRetryBackoff(wait.Backoff{
				Steps:    5,
				Duration: 1 * time.Millisecond,
				Factor:   1.0,
				Jitter:   0.0,
			})

			mux = http.NewServeMux()
			apiHandler := http.NewServeMux()
			apiHandler.Handle(defaultAPIURL+"/", http.StripPrefix(defaultAPIURL, mux))

			mux.HandleFunc("/version", func(rw http.ResponseWriter, r *http.Request) {
				rw.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(rw, `{"version": "1.20.0"}`)
			})

			server = httptest.NewServer(apiHandler)

			mockRepoURL := fmt.Sprintf("%s/%s/%s", server.URL, owner, repo)
			hasSnapshot.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = mockRepoURL

			repoCR = pacv1alpha1.Repository{
				Spec: pacv1alpha1.RepositorySpec{
					URL: mockRepoURL,
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{
							Name: "example-secret-name",
							Key:  "example-token",
						},
					},
				},
			}

			secretData = map[string][]byte{
				"example-token": []byte("example-personal-access-token"),
			}

			mockK8sClient = &MockK8sClient{
				getInterceptor: func(key client.ObjectKey, obj client.Object) {
					if secret, ok := obj.(*v1.Secret); ok {
						secret.Data = secretData
					}
				},
				listInterceptor: func(list client.ObjectList) {
					if repoList, ok := list.(*pacv1alpha1.RepositoryList); ok {
						repoList.Items = []pacv1alpha1.Repository{repoCR}
					}
				},
			}

			reporter = status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(0))
		})

		AfterEach(func() {
			server.Close()
			status.SetReporterRetryBackoff(savedBackoff)
		})

		It("retries on transient 500 error and succeeds on retry", func() {
			var callCount int32

			path := fmt.Sprintf("/repos/%s/%s/statuses/%s", owner, repo, digest)
			mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					rw.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				call := atomic.AddInt32(&callCount, 1)
				if call == 1 {
					rw.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintf(rw, `{"error": "internal server error"}`)
					return
				}
				rw.WriteHeader(http.StatusOK)
				rw.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(rw, `{"id": 123, "state": "success", "context": "test"}`)
			})

			statusCode, err := reporter.ReportStatus(context.TODO(), status.TestReport{
				FullName:     "fullname/scenario1",
				ScenarioName: "scenario1",
				Status:       integrationteststatus.IntegrationTestStatusTestPassed,
				Summary:      "test passed",
			})

			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(http.StatusOK))
			Expect(atomic.LoadInt32(&callCount)).To(BeNumerically("==", 2))
			Expect(buf.String()).To(ContainSubstring("retrying to set forgejo commit status after transient error"))
		})

		It("does not retry on 401 Unauthorized", func() {
			var callCount int32

			path := fmt.Sprintf("/repos/%s/%s/statuses/%s", owner, repo, digest)
			mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					rw.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				atomic.AddInt32(&callCount, 1)
				rw.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(rw, `{"error": "unauthorized"}`)
			})

			statusCode, err := reporter.ReportStatus(context.TODO(), status.TestReport{
				FullName:     "fullname/scenario1",
				ScenarioName: "scenario1",
				Status:       integrationteststatus.IntegrationTestStatusTestPassed,
				Summary:      "test passed",
			})

			Expect(err).To(HaveOccurred())
			Expect(statusCode).To(Equal(http.StatusUnauthorized))
			Expect(atomic.LoadInt32(&callCount)).To(BeNumerically("==", 1))
		})

		It("does not retry on 403 Forbidden", func() {
			var callCount int32

			path := fmt.Sprintf("/repos/%s/%s/statuses/%s", owner, repo, digest)
			mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					rw.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				atomic.AddInt32(&callCount, 1)
				rw.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(rw, `{"error": "forbidden"}`)
			})

			statusCode, err := reporter.ReportStatus(context.TODO(), status.TestReport{
				FullName:     "fullname/scenario1",
				ScenarioName: "scenario1",
				Status:       integrationteststatus.IntegrationTestStatusTestPassed,
				Summary:      "test passed",
			})

			Expect(err).To(HaveOccurred())
			Expect(statusCode).To(Equal(http.StatusForbidden))
			Expect(atomic.LoadInt32(&callCount)).To(BeNumerically("==", 1))
		})

		It("exhausts all retries on persistent 500 errors", func() {
			var callCount int32

			path := fmt.Sprintf("/repos/%s/%s/statuses/%s", owner, repo, digest)
			mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					rw.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				atomic.AddInt32(&callCount, 1)
				rw.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(rw, `{"error": "internal server error"}`)
			})

			statusCode, err := reporter.ReportStatus(context.TODO(), status.TestReport{
				FullName:     "fullname/scenario1",
				ScenarioName: "scenario1",
				Status:       integrationteststatus.IntegrationTestStatusTestPassed,
				Summary:      "test passed",
			})

			Expect(err).To(HaveOccurred())
			Expect(statusCode).To(Equal(http.StatusInternalServerError))
			Expect(atomic.LoadInt32(&callCount)).To(BeNumerically("==", 5))
			Expect(buf.String()).To(ContainSubstring("failed to set forgejo commit status after all retries"))
		})
	})

	Describe("Test helper functions", func() {
		DescribeTable(
			"reports correct forgejo statuses from test statuses",
			func(teststatus integrationteststatus.IntegrationTestStatus, fjState string) {
				state, err := status.GenerateForgejoCommitState(teststatus, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(state).To(Equal(fjState))
			},
			Entry("Provision error", integrationteststatus.IntegrationTestStatusEnvironmentProvisionError_Deprecated, "error"),
			Entry("Deployment error", integrationteststatus.IntegrationTestStatusDeploymentError_Deprecated, "error"),
			Entry("Deleted", integrationteststatus.IntegrationTestStatusDeleted, "error"),
			Entry("Success", integrationteststatus.IntegrationTestStatusTestPassed, "success"),
			Entry("Test failure", integrationteststatus.IntegrationTestStatusTestFail, "error"),
			Entry("In progress", integrationteststatus.IntegrationTestStatusInProgress, "pending"),
			Entry("Pending", integrationteststatus.IntegrationTestStatusPending, "pending"),
			Entry("Invalid", integrationteststatus.IntegrationTestStatusTestInvalid, "error"),
			Entry("BuildPLRInProgress", integrationteststatus.BuildPLRInProgress, "pending"),
			Entry("BuildPLRFailed", integrationteststatus.BuildPLRFailed, "error"),
			Entry("SnapshotCreationFailed", integrationteststatus.SnapshotCreationFailed, "error"),
			Entry("GroupSnapshotCreationFailed", integrationteststatus.GroupSnapshotCreationFailed, "error"),
		)

		It("check if all integration tests statuses are supported", func() {
			for _, teststatus := range integrationteststatus.IntegrationTestStatusValues() {
				_, err := status.GenerateForgejoCommitState(teststatus, false)
				Expect(err).ToNot(HaveOccurred())
			}
		})
		It("check if optional integration test returns warning status for failed test", func() {
			state, err := status.GenerateForgejoCommitState(integrationteststatus.IntegrationTestStatusTestFail, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal("warning"))
		})
		It("check if optional integration test returns success status for invalid test", func() {
			state, err := status.GenerateForgejoCommitState(integrationteststatus.IntegrationTestStatusTestInvalid, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal("success"))
		})
	})
})

// muxForgejoCommitStatusPost mocks commit status POST request.
// If catchStr is non-empty, the POST body must contain it.
// If expectedState is non-empty (e.g. "success" or "error"), the body must contain that state value.
func muxForgejoCommitStatusPost(mux *http.ServeMux, owner string, repo string, sha string, catchStr string, expectedState string) {
	path := fmt.Sprintf("/repos/%s/%s/statuses/%s", owner, repo, sha)
	mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		bit, _ := io.ReadAll(r.Body)
		s := string(bit)
		if catchStr != "" {
			Expect(s).To(ContainSubstring(catchStr))
		}
		if expectedState != "" {
			Expect(s).To(ContainSubstring(expectedState))
		}
		rw.WriteHeader(http.StatusOK)
		rw.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(rw, `{"id": 123, "state": "success", "context": "test"}`)
	})
}

// muxForgejoCommitStatusesGet mocks commit statuses GET request
func muxForgejoCommitStatusesGet(mux *http.ServeMux, owner string, repo string, sha string, report *status.TestReport) {
	path := fmt.Sprintf("/repos/%s/%s/commits/%s/statuses", owner, repo, sha)
	mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
		output := "[]"
		if report != nil {
			commitStatus := forgejo.Status{
				ID:          123,
				Context:     report.FullName,
				State:       forgejo.StatusState("pending"),
				Description: report.Summary,
			}
			jsonStatuses, _ := json.Marshal([]forgejo.Status{commitStatus})
			output = string(jsonStatuses)
		}
		rw.Header().Set("Content-Type", "application/json")
		fmt.Fprint(rw, output)
	})
}

// muxForgejoIssueComments mocks issue comment GET and POST requests
func muxForgejoIssueComments(mux *http.ServeMux, owner string, repo string, issueNum string, catchStr string) {
	path := fmt.Sprintf("/repos/%s/%s/issues/%s/comments", owner, repo, issueNum)
	mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			bit, _ := io.ReadAll(r.Body)
			s := string(bit)
			if catchStr != "" {
				Expect(s).To(ContainSubstring(catchStr))
			}
			rw.WriteHeader(http.StatusCreated)
			rw.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(rw, `{"id": 1000, "body": "new comment"}`)
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

	deleteEditPathPattern := fmt.Sprintf("/repos/%s/%s/issues/comments/", owner, repo)
	mux.HandleFunc(deleteEditPathPattern, func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "DELETE":
			rw.WriteHeader(http.StatusNoContent)
			return
		case "PATCH":
			bit, _ := io.ReadAll(r.Body)
			s := string(bit)
			if catchStr != "" {
				Expect(s).To(ContainSubstring(catchStr))
			}
			rw.WriteHeader(http.StatusOK)
			rw.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(rw, `{"id": 1000, "body": "updated comment"}`)
			return
		default:
			rw.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
