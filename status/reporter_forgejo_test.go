/*
Copyright 2025 Red Hat Inc.

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

	"code.gitea.io/sdk/gitea"
	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	"github.com/tonglil/buflogr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		orgName     = "example"
		repoName    = "example"
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
					"pac.test.appstudio.openshift.io/url-org":        orgName,
					"pac.test.appstudio.openshift.io/url-repository": repoName,
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

		mockK8sClient = &MockK8sClient{}
	})

	When("Testing detect function", func() {

		It("should return true when the annotation is set to forgejo provider", func() {
			reporter := status.NewForgejoReporter(log, mockK8sClient)
			result := reporter.Detect(hasSnapshot)
			Expect(result).To(BeTrue())
		})

		It("should return true when the label is set to forgejo provider", func() {
			hasSnapshot.Labels[gitops.PipelineAsCodeGitProviderLabel] = gitops.PipelineAsCodeForgejoProviderType
			delete(hasSnapshot.Annotations, gitops.PipelineAsCodeGitProviderAnnotation)

			reporter := status.NewForgejoReporter(log, mockK8sClient)
			result := reporter.Detect(hasSnapshot)
			Expect(result).To(BeTrue())
		})

		It("should return false when the annotation/label is not set to forgejo provider", func() {
			hasSnapshot.Annotations[gitops.PipelineAsCodeGitProviderAnnotation] = gitops.PipelineAsCodeGitHubProviderType

			reporter := status.NewForgejoReporter(log, mockK8sClient)
			result := reporter.Detect(hasSnapshot)
			Expect(result).To(BeFalse())
		})

		It("should return false when there's no annotation/label for provider", func() {
			delete(hasSnapshot.Annotations, gitops.PipelineAsCodeGitProviderAnnotation)

			reporter := status.NewForgejoReporter(log, mockK8sClient)
			result := reporter.Detect(hasSnapshot)
			Expect(result).To(BeFalse())
		})
	})

	When("Testing GetReporterName function", func() {
		It("should return ForgejoReporter", func() {
			reporter := status.NewForgejoReporter(log, mockK8sClient)
			Expect(reporter.GetReporterName()).To(Equal("ForgejoReporter"))
		})
	})

	When("Testing Initialize function", func() {

		It("should initialize successfully", func() {
			// Use a test server URL to avoid authentication issues
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Return appropriate responses for API calls during initialization
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/version":
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, `{"version": "1.22.0"}`)
				default:
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, "{}")
				}
			}))
			defer server.Close()

			// Update repo URL to use test server
			testURL := server.URL + "/example/example"
			hasSnapshot.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = testURL

			// Update the mock repository to use the same URL
			repo := &pacv1alpha1.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pac-repo",
					Namespace: "default",
				},
				Spec: pacv1alpha1.RepositorySpec{
					URL: testURL, // Use the same URL
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{
							Name: "pac-secret",
							Key:  "password",
						},
					},
				},
			}

			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pac-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"password": []byte("my-token"),
				},
			}

			mockK8sClient = &MockK8sClient{
				getInterceptor: func(key client.ObjectKey, obj client.Object) {
					if s, ok := obj.(*v1.Secret); ok {
						*s = *secret
					}
				},
				listInterceptor: func(list client.ObjectList) {
					if repoList, ok := list.(*pacv1alpha1.RepositoryList); ok {
						repoList.Items = []pacv1alpha1.Repository{*repo}
					}
				},
			}

			reporter := status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(statusCode).To(Equal(0))
		})

		It("should fail when repo-url annotation is missing", func() {
			delete(hasSnapshot.Annotations, gitops.PipelineAsCodeRepoURLAnnotation)

			// Set up mock client with empty repositories for this error case
			mockK8sClient = &MockK8sClient{
				listInterceptor: func(list client.ObjectList) {
					if repoList, ok := list.(*pacv1alpha1.RepositoryList); ok {
						repoList.Items = []pacv1alpha1.Repository{} // Empty list
					}
				},
			}

			reporter := status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).To(Equal(0))
			Expect(err.Error()).To(ContainSubstring("failed to get value of pac.test.appstudio.openshift.io/repo-url annotation"))
		})

		It("should fail when sha label is missing", func() {
			delete(hasSnapshot.Labels, gitops.PipelineAsCodeSHALabel)

			// Set up working repository data since this error happens after token retrieval
			repo := &pacv1alpha1.Repository{
				ObjectMeta: metav1.ObjectMeta{Name: "pac-repo", Namespace: "default"},
				Spec: pacv1alpha1.RepositorySpec{
					URL: repoUrl,
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{Name: "pac-secret", Key: "password"},
					},
				},
			}
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "pac-secret", Namespace: "default"},
				Data:       map[string][]byte{"password": []byte("my-token")},
			}
			mockK8sClient = &MockK8sClient{
				getInterceptor: func(key client.ObjectKey, obj client.Object) {
					if s, ok := obj.(*v1.Secret); ok {
						*s = *secret
					}
				},
				listInterceptor: func(list client.ObjectList) {
					if repoList, ok := list.(*pacv1alpha1.RepositoryList); ok {
						repoList.Items = []pacv1alpha1.Repository{*repo}
					}
				},
			}

			reporter := status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).To(Equal(0))
			Expect(err.Error()).To(ContainSubstring("sha label not found"))
		})

		It("should fail when org label is missing", func() {
			delete(hasSnapshot.Labels, gitops.PipelineAsCodeURLOrgLabel)

			reporter := status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).To(Equal(0))
			Expect(err.Error()).To(ContainSubstring("org label not found"))
		})

		It("should fail when repository label is missing", func() {
			delete(hasSnapshot.Labels, gitops.PipelineAsCodeURLRepositoryLabel)

			reporter := status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).To(Equal(0))
			Expect(err.Error()).To(ContainSubstring("repository label not found"))
		})

		It("should work without pull-request annotation for push events", func() {
			// Use a test server URL to avoid authentication issues
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/version":
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, `{"version": "1.22.0"}`)
				default:
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, "{}")
				}
			}))
			defer server.Close()

			delete(hasSnapshot.Annotations, gitops.PipelineAsCodePullRequestAnnotation)
			hasSnapshot.Labels[gitops.PipelineAsCodeEventTypeLabel] = gitops.PipelineAsCodePushType
			testURL := server.URL + "/example/example"
			hasSnapshot.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = testURL

			// Set up mock repository with matching URL
			repo := &pacv1alpha1.Repository{
				ObjectMeta: metav1.ObjectMeta{Name: "pac-repo", Namespace: "default"},
				Spec: pacv1alpha1.RepositorySpec{
					URL: testURL,
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{Name: "pac-secret", Key: "password"},
					},
				},
			}
			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "pac-secret", Namespace: "default"},
				Data:       map[string][]byte{"password": []byte("my-token")},
			}
			mockK8sClient = &MockK8sClient{
				getInterceptor: func(key client.ObjectKey, obj client.Object) {
					if s, ok := obj.(*v1.Secret); ok {
						*s = *secret
					}
				},
				listInterceptor: func(list client.ObjectList) {
					if repoList, ok := list.(*pacv1alpha1.RepositoryList); ok {
						repoList.Items = []pacv1alpha1.Repository{*repo}
					}
				},
			}

			reporter := status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(statusCode).To(Equal(0))
		})

		It("should fail when pull-request annotation is missing for non-push events", func() {
			delete(hasSnapshot.Annotations, gitops.PipelineAsCodePullRequestAnnotation)
			// Make it a group snapshot to avoid the IsSnapshotCreatedByPACPushEvent logic
			hasSnapshot.Labels[gitops.SnapshotTypeLabel] = gitops.SnapshotGroupType

			reporter := status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).To(Equal(0))
			Expect(err.Error()).To(ContainSubstring("pull-request annotation not found"))
		})
	})

	When("Testing ReportStatus function", func() {
		var server *httptest.Server
		var reporter *status.ForgejoReporter

		BeforeEach(func() {
			// Mock server to handle Forgejo API calls
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/version":
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, `{"version": "1.22.0"}`)
				case fmt.Sprintf("/api/v1/repos/%s/%s/commits/%s/statuses", orgName, repoName, digest):
					// List statuses (GET)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, "[]")
				case fmt.Sprintf("/api/v1/repos/%s/%s/statuses/%s", orgName, repoName, digest):
					switch r.Method {
					case "POST":
						// Create status
						body, _ := io.ReadAll(r.Body)
						var status gitea.CreateStatusOption
						_ = json.Unmarshal(body, &status)

						response := gitea.Status{
							ID:          1,
							State:       status.State,
							Context:     status.Context,
							Description: status.Description,
							TargetURL:   status.TargetURL,
						}

						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusCreated)
						_ = json.NewEncoder(w).Encode(response)
					}
				case fmt.Sprintf("/api/v1/repos/%s/%s/issues/%s/comments", orgName, repoName, pullRequest):
					switch r.Method {
					case "GET":
						// Return empty list of comments
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						fmt.Fprint(w, "[]")
					case "POST":
						// Create comment
						response := gitea.Comment{
							ID:   1,
							Body: "Test comment",
						}

						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusCreated)
						_ = json.NewEncoder(w).Encode(response)
					}
				default:
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, "{}")
				}
			}))

			// Update snapshot to use the test server
			testURL := server.URL + "/example/example"
			hasSnapshot.Annotations[gitops.PipelineAsCodeRepoURLAnnotation] = testURL

			repo := &pacv1alpha1.Repository{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pac-repo",
					Namespace: "default",
				},
				Spec: pacv1alpha1.RepositorySpec{
					URL: testURL,
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{
							Name: "pac-secret",
							Key:  "password",
						},
					},
				},
			}

			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pac-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"password": []byte("my-token"),
				},
			}

			mockK8sClient = &MockK8sClient{
				getInterceptor: func(key client.ObjectKey, obj client.Object) {
					if s, ok := obj.(*v1.Secret); ok {
						*s = *secret
					}
				},
				listInterceptor: func(list client.ObjectList) {
					if repoList, ok := list.(*pacv1alpha1.RepositoryList); ok {
						repoList.Items = []pacv1alpha1.Repository{*repo}
					}
				},
			}

			reporter = status.NewForgejoReporter(log, mockK8sClient)
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).ToNot(HaveOccurred())
			Expect(statusCode).To(Equal(0))
		})

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		It("should report status successfully for a passed test", func() {
			testReport := status.TestReport{
				FullName:            "test-scenario",
				ScenarioName:        "test-scenario",
				SnapshotName:        "snapshot-sample",
				ComponentName:       "component-sample",
				Text:                "Test completed successfully",
				Status:              integrationteststatus.IntegrationTestStatusTestPassed,
				Summary:             "All tests passed",
				TestPipelineRunName: "test-pipeline-run",
			}

			statusCode, err := reporter.ReportStatus(context.TODO(), testReport)
			Expect(err).ToNot(HaveOccurred())
			Expect(statusCode).To(Equal(http.StatusCreated))
		})

		It("should report status successfully for a failed test", func() {
			testReport := status.TestReport{
				FullName:            "test-scenario",
				ScenarioName:        "test-scenario",
				SnapshotName:        "snapshot-sample",
				ComponentName:       "component-sample",
				Text:                "Test failed",
				Status:              integrationteststatus.IntegrationTestStatusTestFail,
				Summary:             "Tests failed",
				TestPipelineRunName: "test-pipeline-run",
			}

			statusCode, err := reporter.ReportStatus(context.TODO(), testReport)
			Expect(err).ToNot(HaveOccurred())
			Expect(statusCode).To(Equal(http.StatusCreated))
		})

		It("should report status successfully for a pending test", func() {
			testReport := status.TestReport{
				FullName:            "test-scenario",
				ScenarioName:        "test-scenario",
				SnapshotName:        "snapshot-sample",
				ComponentName:       "component-sample",
				Text:                "Test is pending",
				Status:              integrationteststatus.IntegrationTestStatusPending,
				Summary:             "Tests are pending",
				TestPipelineRunName: "test-pipeline-run",
			}

			statusCode, err := reporter.ReportStatus(context.TODO(), testReport)
			Expect(err).ToNot(HaveOccurred())
			Expect(statusCode).To(Equal(http.StatusCreated))
		})

		It("should fail when reporter is not initialized", func() {
			uninitializedReporter := status.NewForgejoReporter(log, mockK8sClient)

			testReport := status.TestReport{
				FullName:     "test-scenario",
				ScenarioName: "test-scenario",
				SnapshotName: "snapshot-sample",
				Status:       integrationteststatus.IntegrationTestStatusTestPassed,
			}

			statusCode, err := uninitializedReporter.ReportStatus(context.TODO(), testReport)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).To(Equal(0))
			Expect(err.Error()).To(ContainSubstring("forgejo reporter is not initialized"))
		})
	})

	When("Testing ReturnCodeIsUnrecoverable function", func() {
		It("should return true for unrecoverable status codes", func() {
			reporter := status.NewForgejoReporter(log, mockK8sClient)

			Expect(reporter.ReturnCodeIsUnrecoverable(http.StatusForbidden)).To(BeTrue())
			Expect(reporter.ReturnCodeIsUnrecoverable(http.StatusUnauthorized)).To(BeTrue())
			Expect(reporter.ReturnCodeIsUnrecoverable(http.StatusBadRequest)).To(BeTrue())
		})

		It("should return false for recoverable status codes", func() {
			reporter := status.NewForgejoReporter(log, mockK8sClient)

			Expect(reporter.ReturnCodeIsUnrecoverable(http.StatusInternalServerError)).To(BeFalse())
			Expect(reporter.ReturnCodeIsUnrecoverable(http.StatusTooManyRequests)).To(BeFalse())
			Expect(reporter.ReturnCodeIsUnrecoverable(http.StatusOK)).To(BeFalse())
		})
	})

	When("Testing GenerateForgejoCommitState function", func() {
		It("should return correct states for different integration test statuses", func() {
			testCases := []struct {
				input    integrationteststatus.IntegrationTestStatus
				expected gitea.StatusState
			}{
				{integrationteststatus.IntegrationTestStatusPending, gitea.StatusPending},
				{integrationteststatus.IntegrationTestStatusInProgress, gitea.StatusPending},
				{integrationteststatus.IntegrationTestStatusTestPassed, gitea.StatusSuccess},
				{integrationteststatus.IntegrationTestStatusTestFail, gitea.StatusFailure},
				{integrationteststatus.IntegrationTestStatusTestInvalid, gitea.StatusError},
				{integrationteststatus.IntegrationTestStatusDeleted, gitea.StatusFailure},
				{integrationteststatus.BuildPLRInProgress, gitea.StatusPending},
				{integrationteststatus.BuildPLRFailed, gitea.StatusFailure},
				{integrationteststatus.SnapshotCreationFailed, gitea.StatusFailure},
				{integrationteststatus.GroupSnapshotCreationFailed, gitea.StatusFailure},
			}

			for _, testCase := range testCases {
				result, err := status.GenerateForgejoCommitState(testCase.input)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(testCase.expected))
			}
		})

		It("should return error for unknown status", func() {
			unknownStatus := integrationteststatus.IntegrationTestStatus(99) // Use an invalid number
			_, err := status.GenerateForgejoCommitState(unknownStatus)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown status"))
		})
	})

	When("Testing GetExistingCommitStatus function", func() {
		It("should find existing commit status by context", func() {
			reporter := status.NewForgejoReporter(log, mockK8sClient)

			statuses := []*gitea.Status{
				{ID: 1, Context: "test-scenario-1", State: gitea.StatusSuccess},
				{ID: 2, Context: "test-scenario-2", State: gitea.StatusPending},
			}

			result := reporter.GetExistingCommitStatus(statuses, "test-scenario-1")
			Expect(result).ToNot(BeNil())
			Expect(result.ID).To(Equal(int64(1)))
			Expect(result.Context).To(Equal("test-scenario-1"))
		})

		It("should return nil when no matching commit status found", func() {
			reporter := status.NewForgejoReporter(log, mockK8sClient)

			statuses := []*gitea.Status{
				{ID: 1, Context: "test-scenario-1", State: gitea.StatusSuccess},
			}

			result := reporter.GetExistingCommitStatus(statuses, "non-existent")
			Expect(result).To(BeNil())
		})
	})

	When("Testing GetExistingCommentID function", func() {
		It("should find existing comment by scenario and snapshot name", func() {
			reporter := status.NewForgejoReporter(log, mockK8sClient)

			comments := []*gitea.Comment{
				{ID: 1, Body: "This is a comment for snapshot-sample and test-scenario"},
				{ID: 2, Body: "This is another comment"},
			}

			result := reporter.GetExistingCommentID(comments, "test-scenario", "snapshot-sample")
			Expect(result).ToNot(BeNil())
			Expect(*result).To(Equal(int64(1)))
		})

		It("should return nil when no matching comment found", func() {
			reporter := status.NewForgejoReporter(log, mockK8sClient)

			comments := []*gitea.Comment{
				{ID: 1, Body: "This is a different comment"},
			}

			result := reporter.GetExistingCommentID(comments, "test-scenario", "snapshot-sample")
			Expect(result).To(BeNil())
		})
	})
})
