package status_test

import (
	"context"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	"github.com/redhat-appstudio/integration-service/git/github"
	"github.com/redhat-appstudio/integration-service/status"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
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

type CreateCommentResult struct {
	ID          int64
	Error       error
	body        string
	issueNumber int
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
	CreateCommentResult
	CreateCommitStatusResult
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

func (c *MockGitHubClient) CreateComment(ctx context.Context, owner string, repo string, issueNumber int, body string) (int64, error) {
	c.CreateCommentResult.body = body
	c.CreateCommentResult.issueNumber = issueNumber
	return c.CreateCommentResult.ID, c.CreateCommentResult.Error
}

func (c *MockGitHubClient) CreateCommitStatus(ctx context.Context, owner string, repo string, SHA string, state string, description string, statusContext string) (int64, error) {
	c.CreateCommitStatusResult.state = state
	c.CreateCommitStatusResult.description = description
	c.CreateCommitStatusResult.statusContext = statusContext
	return c.CreateCommitStatusResult.ID, c.CreateCommitStatusResult.Error
}

type MockK8sClient struct {
	getInterceptor  func(obj client.Object)
	listInterceptor func(list client.ObjectList)
	err             error
}

func (c *MockK8sClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.listInterceptor != nil {
		c.listInterceptor(list)
	}
	return c.err
}

func (c *MockK8sClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if c.getInterceptor != nil {
		c.getInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.getInterceptor != nil {
		c.getInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.getInterceptor != nil {
		c.getInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if c.getInterceptor != nil {
		c.getInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.getInterceptor != nil {
		c.getInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	if c.getInterceptor != nil {
		c.getInterceptor(obj)
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

func setPipelineRunOutcome(pipelineRun *tektonv1beta1.PipelineRun, taskRun *tektonv1beta1.TaskRun) {
	pipelineRun.Status = tektonv1beta1.PipelineRunStatus{
		PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
			CompletionTime: &metav1.Time{Time: time.Now()},
			ChildReferences: []tektonv1beta1.ChildStatusReference{
				{
					Name:             taskRun.Name,
					PipelineTaskName: "pipeline1-task1",
				},
			},
		},
	}
	pipelineRun.Status.SetCondition(&apis.Condition{
		Type:   apis.ConditionSucceeded,
		Status: "True",
	})
}

var _ = Describe("GitHubReporter", Ordered, func() {

	var reporter *status.GitHubReporter
	var pipelineRun *tektonv1beta1.PipelineRun
	var mockK8sClient *MockK8sClient
	var mockGitHubClient *MockGitHubClient
	var successfulTaskRun *tektonv1beta1.TaskRun
	var failedTaskRun *tektonv1beta1.TaskRun
	var skippedTaskRun *tektonv1beta1.TaskRun

	BeforeAll(func() {
		now := time.Now()

		successfulTaskRun = &tektonv1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-pass",
				Namespace: "default",
			},
			Spec: tektonv1beta1.TaskRunSpec{
				TaskRef: &tektonv1beta1.TaskRef{
					Name:   "test-taskrun-pass",
					Bundle: "quay.io/redhat-appstudio/example-tekton-bundle:test",
				},
			},
		}

		Expect(k8sClient.Create(ctx, successfulTaskRun)).Should(Succeed())

		successfulTaskRun.Status = tektonv1beta1.TaskRunStatus{
			TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				TaskRunResults: []tektonv1beta1.TaskRunResult{
					{
						Name: "HACBS_TEST_OUTPUT",
						Value: *tektonv1beta1.NewArrayOrString(`{
											"result": "SUCCESS",
											"timestamp": "1665405318",
											"failures": 0,
											"successes": 10
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, successfulTaskRun)).Should(Succeed())

		failedTaskRun = &tektonv1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-fail",
				Namespace: "default",
			},
			Spec: tektonv1beta1.TaskRunSpec{
				TaskRef: &tektonv1beta1.TaskRef{
					Name:   "test-taskrun-fail",
					Bundle: "quay.io/redhat-appstudio/example-tekton-bundle:test",
				},
			},
		}

		Expect(k8sClient.Create(ctx, failedTaskRun)).Should(Succeed())

		failedTaskRun.Status = tektonv1beta1.TaskRunStatus{
			TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				TaskRunResults: []tektonv1beta1.TaskRunResult{
					{
						Name: "HACBS_TEST_OUTPUT",
						Value: *tektonv1beta1.NewArrayOrString(`{
											"result": "FAILURE",
											"timestamp": "1665405317",
											"failures": 1,
											"successes": 0
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, failedTaskRun)).Should(Succeed())

		skippedTaskRun = &tektonv1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-skip",
				Namespace: "default",
			},
			Spec: tektonv1beta1.TaskRunSpec{
				TaskRef: &tektonv1beta1.TaskRef{
					Name:   "test-taskrun-skip",
					Bundle: "quay.io/redhat-appstudio/example-tekton-bundle:test",
				},
			},
		}

		Expect(k8sClient.Create(ctx, skippedTaskRun)).Should(Succeed())

		skippedTaskRun.Status = tektonv1beta1.TaskRunStatus{
			TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now.Add(5 * time.Minute)},
				CompletionTime: &metav1.Time{Time: now.Add(10 * time.Minute)},
				TaskRunResults: []tektonv1beta1.TaskRunResult{
					{
						Name: "HACBS_TEST_OUTPUT",
						Value: *tektonv1beta1.NewArrayOrString(`{
											"result": "SKIPPED",
											"timestamp": "1665405318",
											"failures": 0,
											"successes": 0
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, skippedTaskRun)).Should(Succeed())
	})

	BeforeEach(func() {
		pipelineRun = &tektonv1beta1.PipelineRun{
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
			Status: tektonv1beta1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
					StartTime: &metav1.Time{Time: time.Now()},
					ChildReferences: []tektonv1beta1.ChildStatusReference{
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
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, successfulTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, failedTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, skippedTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	Context("when provided GitHub app credentials", func() {

		var secretData map[string][]byte

		BeforeEach(func() {
			pipelineRun.Annotations["pac.test.appstudio.openshift.io/installation-id"] = "123"

			secretData = map[string][]byte{
				"github-application-id": []byte("456"),
				"github-private-key":    []byte("example-private-key"),
			}

			mockK8sClient = &MockK8sClient{
				getInterceptor: func(obj client.Object) {
					if secret, ok := obj.(*v1.Secret); ok {
						secret.Data = secretData
					}
				},
				listInterceptor: func(list client.ObjectList) {},
			}

			mockGitHubClient = &MockGitHubClient{}
			reporter = status.NewGitHubReporter(logr.Discard(), mockK8sClient, status.WithGitHubClient(mockGitHubClient))
		})

		It("doesn't report status for non-pull request events", func() {
			delete(pipelineRun.Labels, "pac.test.appstudio.openshift.io/event-type")
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
		})

		It("doesn't report status when the credentials are invalid/missing", func() {
			// Invalid installation ID value
			pipelineRun.Annotations["pac.test.appstudio.openshift.io/installation-id"] = "bad-installation-id"
			err := reporter.ReportStatus(k8sClient, ctx, pipelineRun)
			Expect(err).ToNot(BeNil())
			pipelineRun.Annotations["pac.test.appstudio.openshift.io/installation-id"] = "123"

			// Invalid app ID value
			secretData["github-application-id"] = []byte("bad-app-id")
			err = reporter.ReportStatus(k8sClient, ctx, pipelineRun)
			Expect(err).ToNot(BeNil())
			secretData["github-application-id"] = []byte("456")

			// Missing app ID value
			delete(secretData, "github-application-id")
			err = reporter.ReportStatus(k8sClient, ctx, pipelineRun)
			Expect(err).ToNot(BeNil())
			secretData["github-application-id"] = []byte("456")

			// Missing private key
			delete(secretData, "github-private-key")
			err = reporter.ReportStatus(k8sClient, ctx, pipelineRun)
			Expect(err).ToNot(BeNil())
		})

		It("reports status via CheckRuns", func() {
			// Create an in progress CheckRun
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Title).To(Equal("example-pass has started"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Conclusion).To(Equal(""))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.ExternalID).To(Equal(pipelineRun.Name))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Owner).To(Equal("devfile-sample"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Repository).To(Equal("devfile-sample-go-basic"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.SHA).To(Equal("12a4a35ccd08194595179815e4646c3a6c08bb77"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Name).To(Equal("HACBS Test / devfile-sample-go-basic / example-pass"))
			Expect(mockGitHubClient.CreateCheckRunResult.cra.StartTime.IsZero()).To(BeFalse())
			Expect(mockGitHubClient.CreateCheckRunResult.cra.CompletionTime.IsZero()).To(BeTrue())
			Expect(mockGitHubClient.CreateCheckRunResult.cra.Text).To(Equal(""))

			// Update existing CheckRun w/success
			setPipelineRunOutcome(pipelineRun, successfulTaskRun)
			pipelineRun.Status.SetCondition(&apis.Condition{
				Type:    apis.ConditionSucceeded,
				Message: "sample msg",
				Status:  "True",
			})
			var id int64 = 1
			mockGitHubClient.GetCheckRunIDResult.ID = &id
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Title).To(Equal("example-pass has succeeded"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Conclusion).To(Equal("success"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.CompletionTime.IsZero()).To(BeFalse())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Text).To(Equal("sample msg"))

			// Update existing CheckRun w/failure
			setPipelineRunOutcome(pipelineRun, failedTaskRun)
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Title).To(Equal("example-pass has failed"))
			Expect(mockGitHubClient.UpdateCheckRunResult.cra.Conclusion).To(Equal("failure"))
		})
	})

	Context("when provided GitHub webhook integration credentials", func() {

		var secretData map[string][]byte
		var repo pacv1alpha1.Repository

		BeforeEach(func() {
			pipelineRun.Annotations["pac.test.appstudio.openshift.io/pull-request"] = "999"

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
				getInterceptor: func(obj client.Object) {
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
			reporter = status.NewGitHubReporter(logr.Discard(), mockK8sClient, status.WithGitHubClient(mockGitHubClient))
		})

		It("doesn't report status for non-pull request events", func() {
			delete(pipelineRun.Labels, "pac.test.appstudio.openshift.io/event-type")
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
		})

		It("creates a comment for a succeeded PipelineRun", func() {
			pipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "True",
			})
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
			Expect(mockGitHubClient.CreateCommentResult.body).To(ContainSubstring("# example-pass has succeeded"))
			Expect(mockGitHubClient.CreateCommentResult.issueNumber).To(Equal(999))
		})

		It("creates a comment for a failed PipelineRun", func() {
			setPipelineRunOutcome(pipelineRun, failedTaskRun)
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
			called := strings.Contains(mockGitHubClient.CreateCommentResult.body, "# example-pass has failed")
			Expect(called).To(BeTrue())
			Expect(mockGitHubClient.CreateCommentResult.issueNumber).To(Equal(999))
		})

		It("doesn't create a comment for non-completed PipelineRuns", func() {
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
			Expect(mockGitHubClient.CreateCommentResult.body).To(Equal(""))
			Expect(mockGitHubClient.CreateCommentResult.issueNumber).To(Equal(0))
		})

		It("creates a commit status", func() {
			// In progress
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal("pending"))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("example-pass has started"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("HACBS Test / devfile-sample-go-basic / example-pass"))

			// Success
			pipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "True",
			})
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal("success"))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("example-pass has succeeded"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("HACBS Test / devfile-sample-go-basic / example-pass"))

			// Failure
			setPipelineRunOutcome(pipelineRun, failedTaskRun)
			Expect(reporter.ReportStatus(k8sClient, ctx, pipelineRun)).To(BeNil())
			Expect(mockGitHubClient.CreateCommitStatusResult.state).To(Equal("failure"))
			Expect(mockGitHubClient.CreateCommitStatusResult.description).To(Equal("example-pass has failed"))
			Expect(mockGitHubClient.CreateCommitStatusResult.statusContext).To(Equal("HACBS Test / devfile-sample-go-basic / example-pass"))
		})
	})

})
