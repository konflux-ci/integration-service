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
	"bytes"
	"context"
	"os"
	"time"

	"github.com/go-logr/logr"
	ghapi "github.com/google/go-github/v45/github"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/operator-toolkit/metadata"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	"github.com/tonglil/buflogr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/git/github"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/pkg/integrationteststatus"
	"github.com/konflux-ci/integration-service/status"
)

type CreateAppInstallationTokenResult struct {
	Token string
	Error error
}

type CreateCheckRunResult struct {
	ID    *int64
	Error error
	cra   *github.CheckRunAdapter
}

type UpdateCheckRunResult struct {
	Error error
	cra   *github.CheckRunAdapter
}

type GetCheckRunIDResult struct {
	ID    *int64
	Error error
}

type GetCheckRunResult struct {
	cr *ghapi.CheckRun
}

type CreateCommentResult struct {
	ID          int64
	Error       error
	body        string
	issueNumber int
}

type EditCommentResult struct {
	ID    int64
	Error error
	body  string
}

type CreateCommitStatusResult struct {
	ID            int64
	Error         error
	state         string
	description   string
	statusContext string
	targetURL     string
}

type MockGitHubClient struct {
	CreateAppInstallationTokenResult
	CreateCheckRunResult
	UpdateCheckRunResult
	GetCheckRunIDResult
	GetCheckRunResult
	CreateCommentResult
	CreateCommitStatusResult
	EditCommentResult
}

func (c *MockGitHubClient) CreateAppInstallationToken(ctx context.Context, appID int64, installationID int64, privateKey []byte) (string, int, error) {
	return c.Token, 0, c.CreateAppInstallationTokenResult.Error
}

func (c *MockGitHubClient) SetOAuthToken(ctx context.Context, token string) {}

func (c *MockGitHubClient) CreateCheckRun(ctx context.Context, cra *github.CheckRunAdapter) (*int64, int, error) {
	c.CreateCheckRunResult.cra = cra
	return c.CreateCheckRunResult.ID, 200, c.CreateCheckRunResult.Error
}

func (c *MockGitHubClient) UpdateCheckRun(ctx context.Context, checkRunID int64, cra *github.CheckRunAdapter) (int, error) {
	c.UpdateCheckRunResult.cra = cra
	return 0, c.UpdateCheckRunResult.Error
}

func (c *MockGitHubClient) GetCheckRunID(context.Context, string, string, string, string, int64) (*int64, int, error) {
	return c.GetCheckRunIDResult.ID, 200, c.GetCheckRunIDResult.Error
}

func (c *MockGitHubClient) GetExistingCheckRun(checkRuns []*ghapi.CheckRun, cra *github.CheckRunAdapter) *ghapi.CheckRun {
	//nolint:staticcheck  // Ignore QF1008
	return c.GetCheckRunResult.cr
}

func (c *MockGitHubClient) CommitStatusExists(res []*ghapi.RepoStatus, commitStatus *github.CommitStatusAdapter) (bool, error) {
	for _, cs := range res {
		if *cs.State == commitStatus.State && *cs.Description == commitStatus.Description && *cs.Context == commitStatus.Context {
			return true, nil
		} else {
			return false, nil
		}
	}
	return false, nil
}

//nolint:staticcheck // Ignore QF1008
func (c *MockGitHubClient) CreateComment(ctx context.Context, owner string, repo string, issueNumber int, body string) (int64, int, error) {
	c.CreateCommentResult.body = body
	c.CreateCommentResult.issueNumber = issueNumber
	return c.CreateCommentResult.ID, 200, c.CreateCommentResult.Error
}

func (c *MockGitHubClient) EditComment(ctx context.Context, owner string, repo string, commentID int64, body string) (int64, int, error) {
	c.EditCommentResult.body = body
	c.EditCommentResult.ID = commentID
	return c.EditCommentResult.ID, 200, c.EditCommentResult.Error
}

func (c *MockGitHubClient) GetAllCommentsForPR(ctx context.Context, owner string, repo string, pr int) ([]*ghapi.IssueComment, int, error) {
	var id int64 = 20
	comments := []*ghapi.IssueComment{{ID: &id}}
	return comments, 200, nil
}

func (c *MockGitHubClient) GetExistingCommentID(comments []*ghapi.IssueComment, snapshotName, scenarioName string) *int64 {
	return nil
}

//nolint:staticcheck  // Ignore QF1008
func (c *MockGitHubClient) CreateCommitStatus(ctx context.Context, owner string, repo string, SHA string, state string, description string, statusContext string, targetURL string) (int64, int, error) {
	var id int64 = 60
	c.CreateCommitStatusResult.ID = id
	c.CreateCommitStatusResult.state = state
	c.CreateCommitStatusResult.description = description
	c.CreateCommitStatusResult.statusContext = statusContext
	c.CreateCommitStatusResult.targetURL = targetURL
	return c.CreateCommitStatusResult.ID, 200, c.CreateCommitStatusResult.Error
}

func (c *MockGitHubClient) GetAllCheckRunsForRef(
	ctx context.Context, owner string, repo string, ref string, appID int64,
) ([]*ghapi.CheckRun, int, error) {
	var id int64 = 20
	var externalID = "example-external-id"
	checkRuns := []*ghapi.CheckRun{{ID: &id, ExternalID: &externalID}}
	return checkRuns, 200, nil
}

func (c *MockGitHubClient) GetAllCommitStatusesForRef(
	ctx context.Context, owner, repo, sha string) ([]*ghapi.RepoStatus, int, error) {
	var id int64 = 60
	var state = "pending"
	var description = "Integration test for snapshot snapshot-sample and scenario scenario2 is pending"
	var statusContext = "test/scenario2"
	repoStatus := &ghapi.RepoStatus{ID: &id, State: &state, Context: &statusContext, Description: &description}
	return []*ghapi.RepoStatus{repoStatus}, 200, nil
}

func (c *MockGitHubClient) GetPullRequest(ctx context.Context, owner string, repo string, prID int) (*ghapi.PullRequest, int, error) {
	var id int64 = 60
	var state = "opened"
	pullRequest := &ghapi.PullRequest{ID: &id, State: &state}
	return pullRequest, 200, nil
}

var _ = Describe("GitHubReporter", func() {

	var reporter *status.GitHubReporter
	var mockGitHubClient *MockGitHubClient
	var hasSnapshot *applicationapiv1alpha1.Snapshot
	var mockK8sClient *MockK8sClient

	BeforeEach(func() {
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
					"pac.test.appstudio.openshift.io/event-type":     "pull_request",
				},
				Annotations: map[string]string{
					"build.appstudio.redhat.com/commit_sha":           "6c65b2fcaea3e1a0a92476c8b5dc89e92a85f025",
					"appstudio.redhat.com/updateComponentOnSuccess":   "false",
					"pac.test.appstudio.openshift.io/git-provider":    "github",
					"pac.test.appstudio.openshift.io/repo-url":        "https://github.com/devfile-sample/devfile-sample-go-basic",
					"pac.test.appstudio.openshift.io/source-repo-url": "https://github.com/devfile-sample/devfile-sample-go-basic",
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

	Context("when provided GitHub app credentials (CheckRun)", func() {

		var secretData map[string][]byte
		var expectedLogEntry string
		var buf bytes.Buffer
		var log logr.Logger

		BeforeEach(func() {
			hasSnapshot.Annotations["pac.test.appstudio.openshift.io/installation-id"] = "123"

			buf.Reset()
			log = buflogr.NewWithBuffer(&buf)

			secretData = map[string][]byte{
				"github-application-id": []byte("456"),
				"github-private-key":    []byte("example-private-key"),
			}

			mockK8sClient = &MockK8sClient{
				getInterceptor: func(key client.ObjectKey, obj client.Object) {
					if secret, ok := obj.(*v1.Secret); ok {
						secret.Data = secretData
					}
				},
				listInterceptor: func(list client.ObjectList) {},
			}

			mockGitHubClient = &MockGitHubClient{}
			reporter = status.NewGitHubReporter(log, mockK8sClient, status.WithGitHubClient(mockGitHubClient))
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(Succeed())
			Expect(statusCode).NotTo(BeNil())
		})

		It("failed to initialize when invalid pac secret and private names is not found", func() {
			os.Setenv("INTEGRATION_NS", "invalid")
			os.Setenv("PAC_SECRET", "invalid")
			os.Setenv("GITHUBAPPLICATION_ID", "invalid")
			os.Setenv("GITHUBPRIVATE_KEY", "invalid")
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).NotTo(BeNil())
			os.Setenv("INTEGRATION_NS", "integration-service")
			os.Setenv("PAC_SECRET", "pipelines-as-code-secret")
			os.Setenv("GITHUBAPPLICATION_ID", "github-application-id")
			os.Setenv("GITHUBPRIVATE_KEY", "github-private-key")
		})

		It("can detect if github reporter should be used", func() {
			hasSnapshot.Annotations["pac.test.appstudio.openshift.io/git-provider"] = "github"
			Expect(reporter.Detect(hasSnapshot)).To(BeTrue())

			hasSnapshot.Annotations["pac.test.appstudio.openshift.io/git-provider"] = "not-github"
			Expect(reporter.Detect(hasSnapshot)).To(BeFalse())

			hasSnapshot.Labels["pac.test.appstudio.openshift.io/git-provider"] = "github"
			Expect(reporter.Detect(hasSnapshot)).To(BeTrue())

			hasSnapshot.Labels["pac.test.appstudio.openshift.io/git-provider"] = "not-github"
			Expect(reporter.Detect(hasSnapshot)).To(BeFalse())
		})

		It("doesn't report status when the credentials are invalid/missing", func() {
			// Invalid installation ID value
			hasSnapshot.Annotations["pac.test.appstudio.openshift.io/installation-id"] = "bad-installation-id"
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).NotTo(BeNil())
			Expect(statusCode).NotTo(BeNil())
			hasSnapshot.Annotations["pac.test.appstudio.openshift.io/installation-id"] = "123"

			// Invalid app ID value
			secretData["github-application-id"] = []byte("bad-app-id")
			statusCode, err = reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).NotTo(BeNil())
			secretData["github-application-id"] = []byte("456")

			// Missing app ID value
			delete(secretData, "github-application-id")
			statusCode, err = reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).NotTo(BeNil())
			secretData["github-application-id"] = []byte("456")

			// Missing private key
			delete(secretData, "github-private-key")
			statusCode, err = reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(HaveOccurred())
			Expect(statusCode).NotTo(BeNil())
		})

		DescribeTable(
			"reports correct github title and conclusion from test statuses",
			func(teststatus integrationteststatus.IntegrationTestStatus, title string, conclusion string) {

				statusCode, err := reporter.ReportStatus(
					context.TODO(),
					status.TestReport{
						ScenarioName: "scenario1",
						Status:       teststatus,
					})

				Expect(err).To(Succeed(), "ReportStatus should succeed")
				Expect(statusCode).To(Equal(200))
				Expect(mockGitHubClient.CreateCheckRunResult.cra).NotTo(BeNil())
				Expect(mockGitHubClient.CreateCheckRunResult.cra.Title).To(Equal(title))
				Expect(mockGitHubClient.CreateCheckRunResult.cra.Conclusion).To(Equal(conclusion))

			},
			Entry("Provision error", integrationteststatus.IntegrationTestStatusEnvironmentProvisionError_Deprecated, "Errored", gitops.IntegrationTestStatusFailureGithub),
			Entry("Deployment error", integrationteststatus.IntegrationTestStatusDeploymentError_Deprecated, "Errored", gitops.IntegrationTestStatusFailureGithub),
			Entry("Deleted", integrationteststatus.IntegrationTestStatusDeleted, "Deleted", gitops.IntegrationTestStatusFailureGithub),
			Entry("Success", integrationteststatus.IntegrationTestStatusTestPassed, "Succeeded", gitops.IntegrationTestStatusSuccessGithub),
			Entry("Test failure", integrationteststatus.IntegrationTestStatusTestFail, "Failed", gitops.IntegrationTestStatusFailureGithub),
			Entry("In progress", integrationteststatus.IntegrationTestStatusInProgress, "In Progress", ""),
			Entry("Pending", integrationteststatus.IntegrationTestStatusPending, "Pending", ""),
			Entry("Invalid", integrationteststatus.IntegrationTestStatusTestInvalid, "Errored", gitops.IntegrationTestStatusFailureGithub),
			Entry("BuildPLRInProgress", integrationteststatus.IntegrationTestStatusPending, "Pending", ""),
			Entry("BuildPLRFailed", integrationteststatus.IntegrationTestStatusTestFail, "Failed", gitops.IntegrationTestStatusFailureGithub),
			Entry("SnapshotCreationFailed", integrationteststatus.IntegrationTestStatusTestFail, "Failed", gitops.IntegrationTestStatusFailureGithub),
			Entry("GroupSnapshotCreationFailed", integrationteststatus.IntegrationTestStatusTestFail, "Failed", gitops.IntegrationTestStatusFailureGithub),
		)

		It("check if all integration tests statuses are supported", func() {
			for _, teststatus := range integrationteststatus.IntegrationTestStatusValues() {
				statusCode, err := reporter.ReportStatus(
					context.TODO(),
					status.TestReport{
						ScenarioName: "scenario1",
						Status:       teststatus,
					})

				Expect(err).To(Succeed(), "ReportStatus should succeed")
				Expect(statusCode).To(Equal(200))
			}
		})

		It("reports all details of snapshot tests status via CheckRuns", func() {
			now := time.Now()

			statusCode, err := reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:       "test-name",
					ScenarioName:   "scenario1",
					SnapshotName:   "snapshot-sample",
					ComponentName:  "component-sample",
					Status:         integrationteststatus.IntegrationTestStatusTestFail,
					Summary:        "Integration test for snapshot snapshot-sample and scenario scenario1 experienced an error when provisioning environment",
					StartTime:      &now,
					CompletionTime: &now,
				})

			Expect(err).To(Succeed(), "ReportStatus should succeed")
			Expect(statusCode).To(Equal(200))
			Expect(mockGitHubClient.CreateCheckRunResult.cra).NotTo(BeNil())
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Summary).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 experienced an error when provisioning environment"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Conclusion).To(Equal(gitops.IntegrationTestStatusFailureGithub))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.ExternalID).To(Equal("scenario1-component-sample"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Owner).To(Equal("devfile-sample"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Repository).To(Equal("devfile-sample-go-basic"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.SHA).To(Equal("12a4a35ccd08194595179815e4646c3a6c08bb77"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Name).To(Equal("test-name"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.StartTime.IsZero()).To(BeFalse())
			Expect(mockGitHubClient.CreateCheckRunResult.cra.CompletionTime.IsZero()).To(BeFalse())
		})

		It("reports all details of snapshot tests status via CheckRuns for a Snapshot without a component", func() {
			now := time.Now()

			statusCode, err := reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:       "test-name",
					ScenarioName:   "scenario1",
					SnapshotName:   "snapshot-sample",
					Status:         integrationteststatus.IntegrationTestStatusTestFail,
					Summary:        "Integration test for snapshot snapshot-sample and scenario scenario1 experienced an error when provisioning environment",
					StartTime:      &now,
					CompletionTime: &now,
				})

			Expect(err).To(Succeed(), "ReportStatus should succeed")
			Expect(statusCode).To(Equal(200))
			Expect(mockGitHubClient.CreateCheckRunResult.cra).NotTo(BeNil())
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Summary).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 experienced an error when provisioning environment"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Conclusion).To(Equal(gitops.IntegrationTestStatusFailureGithub))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.ExternalID).To(Equal("scenario1"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Owner).To(Equal("devfile-sample"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Repository).To(Equal("devfile-sample-go-basic"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.SHA).To(Equal("12a4a35ccd08194595179815e4646c3a6c08bb77"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Name).To(Equal("test-name"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.StartTime.IsZero()).To(BeFalse())
			Expect(mockGitHubClient.CreateCheckRunResult.cra.CompletionTime.IsZero()).To(BeFalse())
		})

		It("updates existing CheckRun wit all details of snapshot tests status", func() {
			now := time.Now()

			var id int64 = 1
			var externalID = "example-external-id"
			conclusion := ""

			// Update existing CheckRun w/failure
			//nolint:staticcheck  // Ignore QF1008
			mockGitHubClient.GetCheckRunResult.cr = &ghapi.CheckRun{ID: &id, ExternalID: &externalID, Conclusion: &conclusion}
			statusCode, err := reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:       "test-name",
					ScenarioName:   "scenario1",
					SnapshotName:   "snapshot-sample",
					ComponentName:  "component-sample",
					Status:         integrationteststatus.IntegrationTestStatusTestFail,
					Summary:        "Integration test for snapshot snapshot-sample and scenario scenario1 experienced an error when provisioning environment",
					StartTime:      &now,
					CompletionTime: &now,
				})

			Expect(err).To(Succeed(), "ReportStatus should succeed")
			Expect(statusCode).To(Equal(0))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra).NotTo(BeNil())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Summary).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 experienced an error when provisioning environment"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Conclusion).To(Equal(gitops.IntegrationTestStatusFailureGithub))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.ExternalID).To(Equal("scenario1-component-sample"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Owner).To(Equal("devfile-sample"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Repository).To(Equal("devfile-sample-go-basic"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.SHA).To(Equal("12a4a35ccd08194595179815e4646c3a6c08bb77"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Name).To(Equal("test-name"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.StartTime.IsZero()).To(BeFalse())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.CompletionTime.IsZero()).To(BeFalse())
		})

		It("creates a new checkrun when there exists a CheckRun with same External ID and with 'completed' status", func() {
			var id int64 = 1
			now := time.Now()
			var externalID = "scenario1-component-sample"
			checkrunStatus := "completed"

			// Create a pre-existing CheckRun with "completed" status
			//nolint:staticcheck  // Ignore QF1008
			mockGitHubClient.GetCheckRunResult.cr = &ghapi.CheckRun{ID: &id, ExternalID: &externalID, Status: &checkrunStatus}

			statusCode, err := reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:      "test-name",
					ScenarioName:  "scenario1",
					SnapshotName:  "snapshot-sample",
					ComponentName: "component-sample",
					Status:        integrationteststatus.IntegrationTestStatusInProgress,
					Summary:       "Integration test for snapshot snapshot-sample and scenario scenario1 is in progress",
					StartTime:     &now,
				})
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(200))

			expectedLogEntry = "found existing checkrun"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			expectedLogEntry = "The existing checkrun is already in completed state, re-creating a new checkrun for scenario test status of snapshot"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
			Expect(mockGitHubClient.CreateCheckRunResult.cra).NotTo(BeNil())
			Expect(mockGitHubClient.CreateCheckRunResult.cra.GetStatus()).To(Equal("in_progress"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Conclusion).To(Equal(""))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.ExternalID).To(Equal("scenario1-component-sample"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.CompletionTime.IsZero()).To(BeTrue())
		})
	})

	Context("when provided GitHub webhook integration credentials", func() {

		var secretData map[string][]byte
		var repo pacv1alpha1.Repository
		var buf bytes.Buffer
		var log logr.Logger

		BeforeEach(func() {
			hasSnapshot.Annotations["pac.test.appstudio.openshift.io/pull-request"] = "999"

			log = buflogr.NewWithBuffer(&buf)

			repo = pacv1alpha1.Repository{
				Spec: pacv1alpha1.RepositorySpec{
					URL: "https://github.com/devfile-sample/devfile-sample-go-basic",
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

			mockGitHubClient = &MockGitHubClient{}
			reporter = status.NewGitHubReporter(log, mockK8sClient, status.WithGitHubClient(mockGitHubClient))

			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(Succeed())
			Expect(statusCode).NotTo(BeNil())
		})

		It("creates a commit status for snapshot with correct textual data", func() {
			hasSnapshot.Labels["pac.test.appstudio.openshift.io/pull-request"] = "999"
			statusCode, err := reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:      "fullname/scenario1",
					ScenarioName:  "scenario1",
					SnapshotName:  "snapshot-sample",
					ComponentName: "component-sample",
					Status:        integrationteststatus.IntegrationTestStatusEnvironmentProvisionError_Deprecated,
					Summary:       "Integration test for snapshot snapshot-sample and scenario scenario1 failed",
					Text:          "detailed text here",
				})

			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(0))
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal(gitops.IntegrationTestStatusErrorGithub))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 failed"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("fullname/scenario1"))
			Expect(mockGitHubClient.CreateCommentResult.body).To(Equal("### Integration test for snapshot snapshot-sample and scenario scenario1 failed\n\ndetailed text here"))
		})

		It("creates a commit status for snapshot with correct textual data, but does not create a comment for push event", func() {
			delete(hasSnapshot.Annotations, "pac.test.appstudio.openshift.io/pull-request")
			hasSnapshot.Labels["pac.test.appstudio.openshift.io/event-type"] = "push"
			statusCode, err := reporter.ReportStatus(
				context.TODO(),
				status.TestReport{
					FullName:      "fullname/scenario1",
					ScenarioName:  "scenario1",
					SnapshotName:  "snapshot-sample",
					ComponentName: "component-sample",
					Status:        integrationteststatus.IntegrationTestStatusEnvironmentProvisionError_Deprecated,
					Summary:       "Integration test for snapshot snapshot-sample and scenario scenario1 failed",
					Text:          "detailed text here",
				})

			Expect(err).To(Succeed(), "ReportStatus should succeed")
			Expect(statusCode).To(Equal(0))
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal(gitops.IntegrationTestStatusErrorGithub))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 failed"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("fullname/scenario1"))
			Expect(mockGitHubClient.CreateCommentResult.body).To(BeEmpty(), "Expected no comment to be created for PAC push event")
		})

		DescribeTable(
			"reports correct github statuses from test statuses",
			func(teststatus integrationteststatus.IntegrationTestStatus, ghstatus string) {

				statusCode, err := reporter.ReportStatus(
					context.TODO(),
					status.TestReport{
						ScenarioName: "scenario1",
						Status:       teststatus,
					})
				Expect(err).To(Succeed(), "ReportStatus should succeed")
				Expect(statusCode).To(Equal(0))
				Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal(ghstatus))
			},
			Entry("Provision error", integrationteststatus.IntegrationTestStatusEnvironmentProvisionError_Deprecated, gitops.IntegrationTestStatusErrorGithub),
			Entry("Deployment error", integrationteststatus.IntegrationTestStatusDeploymentError_Deprecated, gitops.IntegrationTestStatusErrorGithub),
			Entry("Deleted", integrationteststatus.IntegrationTestStatusDeleted, gitops.IntegrationTestStatusErrorGithub),
			Entry("Success", integrationteststatus.IntegrationTestStatusTestPassed, gitops.IntegrationTestStatusSuccessGithub),
			Entry("Test failure", integrationteststatus.IntegrationTestStatusTestFail, gitops.IntegrationTestStatusFailureGithub),
			Entry("In progress", integrationteststatus.IntegrationTestStatusInProgress, gitops.IntegrationTestStatusPendingGithub),
			Entry("Pending", integrationteststatus.IntegrationTestStatusPending, gitops.IntegrationTestStatusPendingGithub),
			Entry("Invalid", integrationteststatus.IntegrationTestStatusTestInvalid, gitops.IntegrationTestStatusErrorGithub),
			Entry("BuildPLRInProgress", integrationteststatus.BuildPLRInProgress, gitops.IntegrationTestStatusPendingGithub),
			Entry("BuildPLRFailed", integrationteststatus.BuildPLRFailed, gitops.IntegrationTestStatusFailureGithub),
			Entry("SnapshotCreationFailed", integrationteststatus.SnapshotCreationFailed, gitops.IntegrationTestStatusFailureGithub),
			Entry("GroupSnapshotCreationFailed", integrationteststatus.GroupSnapshotCreationFailed, gitops.IntegrationTestStatusFailureGithub),
		)

		It("check if all integration tests statuses are supported", func() {
			for _, teststatus := range integrationteststatus.IntegrationTestStatusValues() {
				statusCode, err := reporter.ReportStatus(
					context.TODO(),
					status.TestReport{
						ScenarioName: "scenario1",
						Status:       teststatus,
					})

				Expect(err).To(Succeed(), "ReportStatus should succeed")
				Expect(statusCode).To(Equal(0))
			}
		})

		It("don't create a new commit status when already exist", func() {
			testReport := status.TestReport{
				ScenarioName: "scenario2",
				FullName:     "test/scenario2",
				Status:       integrationteststatus.IntegrationTestStatusPending,
				Summary:      "Integration test for snapshot snapshot-sample and scenario scenario2 is pending",
			}
			statusCode, err := reporter.ReportStatus(context.TODO(), testReport)
			Expect(err).To(Succeed(), "ReportStatus should succeed")
			Expect(statusCode).To(Equal(0))
			expectedLogEntry := "found existing commitStatus for scenario test status of snapshot, no need to create new commit status"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})

		It("don't create commit status when source and target repo owner are different", func() {
			Expect(metadata.DeleteAnnotation(hasSnapshot, gitops.PipelineAsCodeGitSourceURLAnnotation)).To(Succeed())
			testReport := status.TestReport{
				ScenarioName: "scenario2",
				FullName:     "test/scenario2",
				Status:       integrationteststatus.IntegrationTestStatusPending,
				Summary:      "Integration test for snapshot snapshot-sample and scenario scenario2 is pending",
			}
			statusCode, err := reporter.ReportStatus(context.TODO(), testReport)
			Expect(err).To(Succeed(), "ReportStatus should succeed")
			Expect(statusCode).To(Equal(0))

			expectedLogEntry := "Won't create/update commitStatus since there is access limitation for different source and target Repo Owner"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})
	})

	Context("when comment_strategy is set to disable_all", func() {
		var (
			integrationTestScenario *v1beta2.IntegrationTestScenario
			mockGitHubClient        *MockGitHubClient
			reporter                *status.GitHubReporter
			hasSnapshot             *applicationapiv1alpha1.Snapshot
		)

		BeforeEach(func() {
			// Create an IntegrationTestScenario with comment_strategy set to disable_all
			integrationTestScenario = &v1beta2.IntegrationTestScenario{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scenario",
					Namespace: "default",
				},
				Spec: v1beta2.IntegrationTestScenarioSpec{
					Application: "test-app",
					Settings: &v1beta2.Settings{
						Github: &v1beta2.GithubSettings{
							CommentStrategy: "disable_all",
						},
					},
				},
			}

			repo := pacv1alpha1.Repository{
				Spec: pacv1alpha1.RepositorySpec{
					URL: "https://github.com/testorg/testrepo",
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{
							Name: "example-secret-name",
							Key:  "example-token",
						},
					},
				},
			}

			secretData := map[string][]byte{
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
						repoList.Items = []pacv1alpha1.Repository{repo}
					}
				},
			}

			mockGitHubClient = &MockGitHubClient{
				CreateAppInstallationTokenResult: CreateAppInstallationTokenResult{
					Token: "example-token",
					Error: nil,
				},
				CreateCheckRunResult: CreateCheckRunResult{
					ID:    ghapi.Int64(12345),
					Error: nil,
				},
				CreateCommentResult: CreateCommentResult{
					ID:    0,
					Error: nil,
				},
			}

			hasSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snapshot-sample",
					Namespace: "default",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/url-org":        "testorg",
						"pac.test.appstudio.openshift.io/url-repository": "testrepo",
						"pac.test.appstudio.openshift.io/sha":            "testsha",
						"pac.test.appstudio.openshift.io/event-type":     "pull_request",
					},
					Annotations: map[string]string{
						"pac.test.appstudio.openshift.io/repo-url":     "https://github.com/testorg/testrepo",
						"pac.test.appstudio.openshift.io/pull-request": "1",
						"pac.test.appstudio.openshift.io/git-provider": "github",
					},
				},
			}

			reporter = status.NewGitHubReporter(logr.Discard(), mockK8sClient, status.WithGitHubClient(mockGitHubClient))
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(0))
		})

		It("should not post comments on pull requests", func() {
			testReport := status.TestReport{
				FullName:                "fullname/scenario1",
				ScenarioName:            "scenario1",
				SnapshotName:            "snapshot-sample",
				Status:                  integrationteststatus.IntegrationTestStatusTestFail,
				Summary:                 "Integration test for snapshot snapshot-sample and scenario scenario1 failed",
				Text:                    "detailed text here",
				IntegrationTestScenario: integrationTestScenario,
			}

			statusCode, err := reporter.ReportStatus(context.TODO(), testReport)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(0))

			// Verify that no comment was created
			Expect(mockGitHubClient.CreateCommentResult.body).To(BeEmpty(), "Expected no comment to be created when comment_strategy is disable_all")
			Expect(mockGitHubClient.CreateCommentResult.issueNumber).To(Equal(0), "Expected CreateComment to not be called")
		})
	})

	Context("when comment_strategy is empty or not set", func() {
		var (
			integrationTestScenario *v1beta2.IntegrationTestScenario
			mockGitHubClient        *MockGitHubClient
			reporter                *status.GitHubReporter
			hasSnapshot             *applicationapiv1alpha1.Snapshot
		)

		BeforeEach(func() {
			// Create an IntegrationTestScenario with no comment_strategy (default behavior)
			integrationTestScenario = &v1beta2.IntegrationTestScenario{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scenario",
					Namespace: "default",
				},
				Spec: v1beta2.IntegrationTestScenarioSpec{
					Application: "test-app",
					Settings: &v1beta2.Settings{
						Github: &v1beta2.GithubSettings{
							CommentStrategy: "",
						},
					},
				},
			}

			repo := pacv1alpha1.Repository{
				Spec: pacv1alpha1.RepositorySpec{
					URL: "https://github.com/testorg/testrepo",
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{
							Name: "example-secret-name",
							Key:  "example-token",
						},
					},
				},
			}

			secretData := map[string][]byte{
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
						repoList.Items = []pacv1alpha1.Repository{repo}
					}
				},
			}

			mockGitHubClient = &MockGitHubClient{
				CreateAppInstallationTokenResult: CreateAppInstallationTokenResult{
					Token: "example-token",
					Error: nil,
				},
				CreateCheckRunResult: CreateCheckRunResult{
					ID:    ghapi.Int64(12345),
					Error: nil,
				},
				CreateCommentResult: CreateCommentResult{
					ID:    67890,
					Error: nil,
				},
			}

			hasSnapshot = &applicationapiv1alpha1.Snapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "snapshot-sample",
					Namespace: "default",
					Labels: map[string]string{
						"pac.test.appstudio.openshift.io/url-org":        "testorg",
						"pac.test.appstudio.openshift.io/url-repository": "testrepo",
						"pac.test.appstudio.openshift.io/sha":            "testsha",
						"pac.test.appstudio.openshift.io/event-type":     "pull_request",
					},
					Annotations: map[string]string{
						"pac.test.appstudio.openshift.io/repo-url":     "https://github.com/testorg/testrepo",
						"pac.test.appstudio.openshift.io/pull-request": "1",
						"pac.test.appstudio.openshift.io/git-provider": "github",
					},
				},
			}

			reporter = status.NewGitHubReporter(logr.Discard(), mockK8sClient, status.WithGitHubClient(mockGitHubClient))
			statusCode, err := reporter.Initialize(context.TODO(), hasSnapshot)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(0))
		})

		It("should post comments on pull requests", func() {
			testReport := status.TestReport{
				FullName:                "fullname/scenario1",
				ScenarioName:            "scenario1",
				SnapshotName:            "snapshot-sample",
				Status:                  integrationteststatus.IntegrationTestStatusTestFail,
				Summary:                 "Integration test for snapshot snapshot-sample and scenario scenario1 failed",
				Text:                    "detailed text here",
				IntegrationTestScenario: integrationTestScenario,
			}

			statusCode, err := reporter.ReportStatus(context.TODO(), testReport)
			Expect(err).To(Succeed())
			Expect(statusCode).To(Equal(0))

			// Verify that a comment was created
			Expect(mockGitHubClient.CreateCommentResult.body).ToNot(BeEmpty(), "Expected a comment to be created when comment_strategy is empty")
			Expect(mockGitHubClient.CreateCommentResult.issueNumber).To(Equal(1), "Expected comment to be posted on PR #1")
		})
	})

	Context("Testing GenerateCheckRunConclusion", func() {
		It("Returns 'success' when optional tests pass", func() {
			conclusion, err := status.GenerateCheckRunConclusion(integrationteststatus.IntegrationTestStatusTestPassed, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(conclusion).To(Equal("success"))
		})

		It("Returns 'neutral' when optional tests fail", func() {
			conclusion, err := status.GenerateCheckRunConclusion(integrationteststatus.IntegrationTestStatusTestFail, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(conclusion).To(Equal("neutral"))
		})

		It("Returns 'success' when required tests pass", func() {
			conclusion, err := status.GenerateCheckRunConclusion(integrationteststatus.IntegrationTestStatusTestPassed, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(conclusion).To(Equal("success"))
		})

		It("Returns 'failure' when required tests fail", func() {
			conclusion, err := status.GenerateCheckRunConclusion(integrationteststatus.IntegrationTestStatusTestFail, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(conclusion).To(Equal("failure"))
		})
	})
})
