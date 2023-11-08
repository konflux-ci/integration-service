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
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/go-logr/logr"
	ghapi "github.com/google/go-github/v45/github"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/git/github"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/status"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tonglil/buflogr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (c *MockGitHubClient) CreateAppInstallationToken(ctx context.Context, appID int64, installationID int64, privateKey []byte) (string, error) {
	return c.CreateAppInstallationTokenResult.Token, c.CreateAppInstallationTokenResult.Error
}

func (c *MockGitHubClient) SetOAuthToken(ctx context.Context, token string) {}

func (c *MockGitHubClient) CreateCheckRun(ctx context.Context, cra *github.CheckRunAdapter) (*int64, error) {
	c.CreateCheckRunResult.cra = cra
	return c.CreateCheckRunResult.ID, c.CreateCheckRunResult.Error
}

func (c *MockGitHubClient) UpdateCheckRun(ctx context.Context, checkRunID int64, cra *github.CheckRunAdapter) error {
	c.UpdateCheckRunResult.cra = cra
	return c.UpdateCheckRunResult.Error
}

func (c *MockGitHubClient) GetCheckRunID(context.Context, string, string, string, string, int64) (*int64, error) {
	return c.GetCheckRunIDResult.ID, c.GetCheckRunIDResult.Error
}

func (c *MockGitHubClient) IsUpdateNeeded(existingCheckRun *ghapi.CheckRun, cra *github.CheckRunAdapter) bool {
	return true
}

func (c *MockGitHubClient) GetExistingCheckRun(checkRuns []*ghapi.CheckRun, cra *github.CheckRunAdapter) *ghapi.CheckRun {
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

func (c *MockGitHubClient) CreateComment(ctx context.Context, owner string, repo string, issueNumber int, body string) (int64, error) {
	c.CreateCommentResult.body = body
	c.CreateCommentResult.issueNumber = issueNumber
	return c.CreateCommentResult.ID, c.CreateCommentResult.Error
}

func (c *MockGitHubClient) EditComment(ctx context.Context, owner string, repo string, commentID int64, body string) (int64, error) {
	c.EditCommentResult.body = body
	c.EditCommentResult.ID = commentID
	return c.EditCommentResult.ID, c.EditCommentResult.Error
}

func (c *MockGitHubClient) GetAllCommentsForPR(ctx context.Context, owner string, repo string, pr int) ([]*ghapi.IssueComment, error) {
	var id int64 = 20
	comments := []*ghapi.IssueComment{{ID: &id}}
	return comments, nil
}

func (c *MockGitHubClient) GetExistingCommentID(comments []*ghapi.IssueComment, snapshotName, scenarioName string) *int64 {
	return nil
}

func (c *MockGitHubClient) CreateCommitStatus(ctx context.Context, owner string, repo string, SHA string, state string, description string, statusContext string) (int64, error) {
	var id int64 = 60
	c.CreateCommitStatusResult.ID = id
	c.CreateCommitStatusResult.state = state
	c.CreateCommitStatusResult.description = description
	c.CreateCommitStatusResult.statusContext = statusContext
	return c.CreateCommitStatusResult.ID, c.CreateCommitStatusResult.Error
}

func (c *MockGitHubClient) GetAllCheckRunsForRef(
	ctx context.Context, owner string, repo string, ref string, appID int64,
) ([]*ghapi.CheckRun, error) {
	var id int64 = 20
	var externalID string = "example-external-id"
	checkRuns := []*ghapi.CheckRun{{ID: &id, ExternalID: &externalID}}
	return checkRuns, nil
}

func (c *MockGitHubClient) GetAllCommitStatusesForRef(
	ctx context.Context, owner, repo, sha string) ([]*ghapi.RepoStatus, error) {
	var id int64 = 60
	var state = "pending"
	var description = "Integration test for snapshot snapshot-sample and scenario scenario2 is pending"
	var statusContext = "Red Hat Trusted App Test / snapshot-sample / scenario2"
	repoStatus := &ghapi.RepoStatus{ID: &id, State: &state, Context: &statusContext, Description: &description}
	return []*ghapi.RepoStatus{repoStatus}, nil
}

type MockK8sClient struct {
	getInterceptor     func(key client.ObjectKey, obj client.Object)
	listInterceptor    func(list client.ObjectList)
	genericInterceptor func(obj client.Object)
	err                error
}

func (c *MockK8sClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.listInterceptor != nil {
		c.listInterceptor(list)
	}
	return c.err
}

func (c *MockK8sClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if c.getInterceptor != nil {
		c.getInterceptor(key, obj)
	}
	return c.err
}

func (c *MockK8sClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.genericInterceptor != nil {
		c.genericInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.genericInterceptor != nil {
		c.genericInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if c.genericInterceptor != nil {
		c.genericInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.genericInterceptor != nil {
		c.genericInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	if c.genericInterceptor != nil {
		c.genericInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Status() client.SubResourceWriter {
	panic("implement me")
}

func (c *MockK8sClient) SubResource(subResource string) client.SubResourceClient {
	panic("implement me")
}

func (c *MockK8sClient) Scheme() *runtime.Scheme {
	panic("implement me")
}

func (c *MockK8sClient) RESTMapper() meta.RESTMapper {
	panic("implement me")
}

var _ = Describe("GitHubReporter", func() {

	var reporter *status.GitHubReporter
	var pipelineRun *tektonv1.PipelineRun
	var mockK8sClient *MockK8sClient
	var mockGitHubClient *MockGitHubClient
	var successfulTaskRun *tektonv1.TaskRun
	var failedTaskRun *tektonv1.TaskRun
	var skippedTaskRun *tektonv1.TaskRun
	var hasSnapshot *applicationapiv1alpha1.Snapshot
	var logger helpers.IntegrationLogger

	BeforeEach(func() {
		now := time.Now()

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
	})

	Context("when provided GitHub app credentials", func() {

		var secretData map[string][]byte

		BeforeEach(func() {
			hasSnapshot.Annotations["pac.test.appstudio.openshift.io/installation-id"] = "123"

			secretData = map[string][]byte{
				"github-application-id": []byte("456"),
				"github-private-key":    []byte("example-private-key"),
			}

			mockK8sClient = &MockK8sClient{
				getInterceptor: func(key client.ObjectKey, obj client.Object) {
					if secret, ok := obj.(*v1.Secret); ok {
						secret.Data = secretData
					}
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

			mockGitHubClient = &MockGitHubClient{}
			reporter = status.NewGitHubReporter(logr.Discard(), mockK8sClient, status.WithGitHubClient(mockGitHubClient))
		})

		It("doesn't report status for non-pull request events", func() {
			delete(hasSnapshot.Labels, "pac.test.appstudio.openshift.io/event-type")
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
		})

		It("doesn't report status when the credentials are invalid/missing", func() {
			// Invalid installation ID value
			hasSnapshot.Annotations["pac.test.appstudio.openshift.io/installation-id"] = "bad-installation-id"
			err := reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)
			Expect(err).ToNot(BeNil())
			hasSnapshot.Annotations["pac.test.appstudio.openshift.io/installation-id"] = "123"

			// Invalid app ID value
			secretData["github-application-id"] = []byte("bad-app-id")
			err = reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)
			Expect(err).ToNot(BeNil())
			secretData["github-application-id"] = []byte("456")

			// Missing app ID value
			delete(secretData, "github-application-id")
			err = reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)
			Expect(err).ToNot(BeNil())
			secretData["github-application-id"] = []byte("456")

			// Missing private key
			delete(secretData, "github-private-key")
			err = reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)
			Expect(err).ToNot(BeNil())
		})

		It("reports snapshot tests status via CheckRuns", func() {
			// Create an pending CheckRun
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"Pending\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"pending\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Summary).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 is pending"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Conclusion).To(Equal(""))

			// Update existing CheckRun w/inprogress
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"Failed to find deploymentTargetClass with right provisioner for copy of existingEnvironment\"}]"
			var id int64 = 1
			var externalID string = "example-external-id"
			conclusion := ""
			mockGitHubClient.GetCheckRunResult.cr = &ghapi.CheckRun{ID: &id, ExternalID: &externalID, Conclusion: &conclusion}
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Summary).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 is in progress"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Conclusion).To(Equal(""))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.StartTime.IsZero()).To(BeFalse())

			// Update existing CheckRun w/failure
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"EnvironmentProvisionError\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"Failed to find deploymentTargetClass with right provisioner for copy of existingEnvironment\"}]"
			mockGitHubClient.GetCheckRunResult.cr = &ghapi.CheckRun{ID: &id, ExternalID: &externalID, Conclusion: &conclusion}
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Summary).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 experienced an error when provisioning environment"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Conclusion).To(Equal(gitops.IntegrationTestStatusFailureGithub))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.ExternalID).To(Equal("scenario1"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Owner).To(Equal("devfile-sample"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Repository).To(Equal("devfile-sample-go-basic"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.SHA).To(Equal("12a4a35ccd08194595179815e4646c3a6c08bb77"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Name).To(Equal("Red Hat Trusted App Test / snapshot-sample / scenario1"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.StartTime.IsZero()).To(BeFalse())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.CompletionTime.IsZero()).To(BeFalse())

			// Update existing CheckRun w/failure
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"DeploymentError\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"error\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Summary).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 experienced an error when deploying snapshotEnvironmentBinding"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Conclusion).To(Equal(gitops.IntegrationTestStatusFailureGithub))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.CompletionTime.IsZero()).To(BeFalse())

			// Update existing CheckRun w/deleted
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"Deleted\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"deleted\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Summary).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 was deleted before the pipelineRun could finish"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Conclusion).To(Equal(gitops.IntegrationTestStatusFailureGithub))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.CompletionTime.IsZero()).To(BeFalse())

			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"TestFail\",\"testPipelineRunName\":\"test-pipelinerun\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"failed\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Summary).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 has failed"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Conclusion).To(Equal(gitops.IntegrationTestStatusFailureGithub))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.CompletionTime.IsZero()).To(BeFalse())

			// Update existing CheckRun w/success
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"TestPassed\",\"testPipelineRunName\":\"test-pipelinerun\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"failed\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Summary).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 has passed"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Conclusion).To(Equal(gitops.IntegrationTestStatusSuccessGithub))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Title).To(Equal("Succeeded"))
		})
	})

	Context("when provided GitHub webhook integration credentials", func() {

		var secretData map[string][]byte
		var repo pacv1alpha1.Repository
		var buf bytes.Buffer
		log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

		BeforeEach(func() {
			hasSnapshot.Annotations["pac.test.appstudio.openshift.io/pull-request"] = "999"

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
			reporter = status.NewGitHubReporter(logr.Discard(), mockK8sClient, status.WithGitHubClient(mockGitHubClient))
		})

		It("creates a commit status for snapshot", func() {
			// EnvironmentProvisionError
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"EnvironmentProvisionError\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"failed\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal(gitops.IntegrationTestStatusErrorGithub))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 experienced an error when provisioning environment"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("Red Hat Trusted App Test / snapshot-sample / scenario1"))
			Expect(mockGitHubClient.CreateCommentResult.body).Should(ContainSubstring("experienced an error when provisioning environment"))

			// DeploymentError
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"DeploymentError\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"failed\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal(gitops.IntegrationTestStatusErrorGithub))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 experienced an error when deploying snapshotEnvironmentBinding"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("Red Hat Trusted App Test / snapshot-sample / scenario1"))
			Expect(mockGitHubClient.CreateCommentResult.body).Should(ContainSubstring("experienced an error when deploying snapshotEnvironmentBinding"))

			// Deleted
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"Deleted\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"deleted\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal(gitops.IntegrationTestStatusErrorGithub))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 was deleted before the pipelineRun could finish"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("Red Hat Trusted App Test / snapshot-sample / scenario1"))
			Expect(mockGitHubClient.CreateCommentResult.body).Should(ContainSubstring("was deleted before the pipelineRun could finish"))

			// Success
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"TestPassed\",\"testPipelineRunName\":\"test-pipelinerun\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"passed\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal(gitops.IntegrationTestStatusSuccessGithub))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 has passed"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("Red Hat Trusted App Test / snapshot-sample / scenario1"))
			Expect(mockGitHubClient.CreateCommentResult.body).Should(ContainSubstring("has passed"))

			// TestFail
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"TestFail\",\"testPipelineRunName\":\"test-pipelinerun\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"failed\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal(gitops.IntegrationTestStatusFailureGithub))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 has failed"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("Red Hat Trusted App Test / snapshot-sample / scenario1"))
			Expect(mockGitHubClient.CreateCommentResult.body).Should(ContainSubstring("has failed"))

			// InProgress
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"InProgress\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"In progress\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &logger, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal(gitops.IntegrationTestStatusPendingGithub))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 is in progress"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("Red Hat Trusted App Test / snapshot-sample / scenario1"))

			// Pending
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario1\",\"status\":\"Pending\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"pending\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &log, hasSnapshot)).To(BeNil())
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal(gitops.IntegrationTestStatusPendingGithub))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("Integration test for snapshot snapshot-sample and scenario scenario1 is pending"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("Red Hat Trusted App Test / snapshot-sample / scenario1"))

			// One commitStatus exists already
			hasSnapshot.Annotations["test.appstudio.openshift.io/status"] = "[{\"scenario\":\"scenario2\",\"status\":\"Pending\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"pending\"}]"
			Expect(reporter.ReportStatusForSnapshot(mockK8sClient, context.TODO(), &log, hasSnapshot)).To(BeNil())
			expectedLogEntry := "found existing commitStatus for scenario test status of snapshot, no need to create new commit status"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})
	})
})
