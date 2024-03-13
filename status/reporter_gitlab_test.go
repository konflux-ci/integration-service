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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/tonglil/buflogr"
	gitlab "github.com/xanzy/go-gitlab"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/pkg/integrationteststatus"
	"github.com/redhat-appstudio/integration-service/status"
)

var _ = Describe("GitLabReporter", func() {

	const (
		repoUrl      = "https://gitlab.com/example/example"
		digest       = "12a4a35ccd08194595179815e4646c3a6c08bb77"
		projectID    = "123"
		mergeRequest = "45"
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
					"pac.test.appstudio.openshift.io/git-provider":   "gitlab",
					"pac.test.appstudio.openshift.io/url-org":        "devfile-sample",
					"pac.test.appstudio.openshift.io/url-repository": "devfile-sample-go-basic",
					"pac.test.appstudio.openshift.io/sha":            "12a4a35ccd08194595179815e4646c3a6c08bb77",
					"pac.test.appstudio.openshift.io/event-type":     "Merge Request",
				},
				Annotations: map[string]string{
					"build.appstudio.redhat.com/commit_sha":             digest,
					"appstudio.redhat.com/updateComponentOnSuccess":     "false",
					"pac.test.appstudio.openshift.io/repo-url":          repoUrl,
					"pac.test.appstudio.openshift.io/source-project-id": projectID,
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

			err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(Succeed())

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
			Expect(reporter.Initialize(context.TODO(), hasSnapshot)).ToNot(Succeed())
		},
			Entry("Missing repo_url", gitops.PipelineAsCodeRepoURLAnnotation, false),
			Entry("Missing SHA", gitops.PipelineAsCodeSHALabel, true),
			Entry("Missing project ID", gitops.PipelineAsCodeSourceProjectIDAnnotation, false),
		)

		It("creates a commit status for snapshot with correct textual data", func() {

			summary := "Integration test for snapshot snapshot-sample and scenario scenario1 failed"

			muxCommitStatusPost(mux, projectID, digest, summary)
			muxMergeNotes(mux, projectID, mergeRequest, summary)

			Expect(reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:     "fullname/scenario1",
					ScenarioName: "scenario1",
					Status:       integrationteststatus.IntegrationTestStatusEnvironmentProvisionError_Deprecated,
					Summary:      summary,
					Text:         "detailed text here",
				})).To(Succeed())
		})

	})

	Describe("Test helper functions", func() {

		DescribeTable(
			"reports correct gitlab statuses from test statuses",
			func(teststatus integrationteststatus.IntegrationTestStatus, glState gitlab.BuildStateValue) {

				state, err := status.GenerateGitlabCommitState(teststatus)
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
		)

		It("check if all integration tests statuses are supported", func() {
			for _, teststatus := range integrationteststatus.IntegrationTestStatusValues() {
				_, err := status.GenerateGitlabCommitState(teststatus)
				Expect(err).ToNot(HaveOccurred())
			}
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

// muxMergeNotes mocks merge request notes GET and POST request, if catchStr is non-empty POST request must contain such substring
func muxMergeNotes(mux *http.ServeMux, pid string, sha string, catchStr string) {
	path := fmt.Sprintf("/projects/%s/merge_requests/%s/notes", pid, sha)
	mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			bit, _ := io.ReadAll(r.Body)
			s := string(bit)
			if catchStr != "" {
				Expect(s).To(ContainSubstring(catchStr))
			}
			fmt.Fprintf(rw, "{}")
		} else {
			fmt.Fprintf(rw, "[]")
		}
	})
}
