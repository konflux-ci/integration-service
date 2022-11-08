package status_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/git/github"
	"github.com/redhat-appstudio/integration-service/status"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

type MockAppClient struct {
	CreateCheckRunResult
	UpdateCheckRunResult
	GetCheckRunIDResult
}

func (c *MockAppClient) CreateCheckRun(ctx context.Context, cra *github.CheckRunAdapter) (*int64, error) {
	c.CreateCheckRunResult.cra = cra
	return c.CreateCheckRunResult.ID, c.CreateCheckRunResult.Error
}

func (c *MockAppClient) UpdateCheckRun(ctx context.Context, checkRunID int64, cra *github.CheckRunAdapter) error {
	c.UpdateCheckRunResult.cra = cra
	return c.UpdateCheckRunResult.Error
}

func (c *MockAppClient) GetCheckRunID(context.Context, string, string, string, string) (*int64, error) {
	return c.GetCheckRunIDResult.ID, c.GetCheckRunIDResult.Error
}

type MockK8sClient struct {
	getInterceptor func(obj client.Object)
	err            error
}

func (c *MockK8sClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.err
}

func (c *MockK8sClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	if c.getInterceptor != nil {
		c.getInterceptor(obj)
	}
	return c.err
}

func MockAppClientCreator(client *MockAppClient) github.AppClientCreator {
	return func(context.Context, logr.Logger, int64, int64, []byte, ...github.AppClientOption) (github.AppClientInterface, error) {
		return client, nil
	}
}

var _ = Describe("GitHubPipelineRunReporter", Ordered, func() {

	var pipelineRun *tektonv1beta1.PipelineRun
	var secretData map[string][]byte
	var mockK8sClient *MockK8sClient

	BeforeAll(func() {
		mockK8sClient = &MockK8sClient{
			getInterceptor: func(obj client.Object) {
				if secret, ok := obj.(*v1.Secret); ok {
					secret.Data = secretData
				}
			},
		}
	})

	BeforeEach(func() {
		pipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pipelinerun",
				Labels: map[string]string{
					"test.appstudio.openshift.io/component":     "devfile-sample-go-basic",
					"test.appstudio.openshift.io/scenario":      "example-pass",
					"pipelinesascode.tekton.dev/git-provider":   "github",
					"pipelinesascode.tekton.dev/url-org":        "devfile-sample",
					"pipelinesascode.tekton.dev/url-repository": "devfile-sample-go-basic",
					"pipelinesascode.tekton.dev/sha":            "12a4a35ccd08194595179815e4646c3a6c08bb77",
					"pipelinesascode.tekton.dev/event-type":     "pull_request",
				},
				Annotations: map[string]string{
					"pipelinesascode.tekton.dev/installation-id": "123",
				},
			},
		}

		pipelineRun.Status.StartTime = &metav1.Time{Time: time.Now()}

		secretData = map[string][]byte{
			"github-application-id": []byte("456"),
			"github-private-key":    []byte("example-key"),
		}
	})

	It("can be initialized with app credentials", func() {
		// Happy path w/installation ID
		_, err := status.NewGitHubPipelineRunReporter(context.TODO(), logr.Discard(), mockK8sClient, pipelineRun)
		Expect(err).To(BeNil())

		// Invalid installation ID value
		pipelineRun.Annotations["pipelinesascode.tekton.dev/installation-id"] = "bad-installation-id"
		_, err = status.NewGitHubPipelineRunReporter(context.TODO(), logr.Discard(), mockK8sClient, pipelineRun)
		Expect(err).ToNot(BeNil())
		pipelineRun.Annotations["pipelinesascode.tekton.dev/installation-id"] = "123"

		// Invalid app ID value
		secretData["github-application-id"] = []byte("bad-app-id")
		_, err = status.NewGitHubPipelineRunReporter(context.TODO(), logr.Discard(), mockK8sClient, pipelineRun)
		Expect(err).ToNot(BeNil())
		secretData["github-application-id"] = []byte("456")

		// Missing app ID value
		delete(secretData, "github-application-id")
		_, err = status.NewGitHubPipelineRunReporter(context.TODO(), logr.Discard(), mockK8sClient, pipelineRun)
		Expect(err).ToNot(BeNil())
		secretData["github-application-id"] = []byte("456")

		// Missing private key
		delete(secretData, "github-private-key")
		_, err = status.NewGitHubPipelineRunReporter(context.TODO(), logr.Discard(), mockK8sClient, pipelineRun)
		Expect(err).ToNot(BeNil())
	})

	It("doesn't report status for non-pull request events", func() {
		delete(pipelineRun.Labels, "pipelinesascode.tekton.dev/event-type")
		reporter, err := status.NewGitHubPipelineRunReporter(context.TODO(), logr.Discard(), mockK8sClient, pipelineRun)
		Expect(err).To(BeNil())
		Expect(reporter.ReportStatus(context.TODO())).To(BeNil())
	})

	It("reports status", func() {
		// Create an in progress CheckRun
		client := MockAppClient{}
		opt := status.WithAppClientCreator(MockAppClientCreator(&client))
		reporter, err := status.NewGitHubPipelineRunReporter(context.TODO(), logr.Discard(), mockK8sClient, pipelineRun, opt)
		Expect(err).To(BeNil())
		Expect(reporter.ReportStatus(context.TODO())).To(BeNil())
		Expect(client.CreateCheckRunResult.cra.Title).To(Equal("example-pass has started"))
		Expect(client.CreateCheckRunResult.cra.Conclusion).To(Equal(""))
		Expect(client.CreateCheckRunResult.cra.ExternalID).To(Equal(pipelineRun.Name))
		Expect(client.CreateCheckRunResult.cra.Owner).To(Equal("devfile-sample"))
		Expect(client.CreateCheckRunResult.cra.Repository).To(Equal("devfile-sample-go-basic"))
		Expect(client.CreateCheckRunResult.cra.SHA).To(Equal("12a4a35ccd08194595179815e4646c3a6c08bb77"))
		Expect(client.CreateCheckRunResult.cra.Name).To(Equal("HACBS Test / devfile-sample-go-basic / example-pass"))
		Expect(client.CreateCheckRunResult.cra.StartTime.IsZero()).To(BeFalse())
		Expect(client.CreateCheckRunResult.cra.CompletionTime.IsZero()).To(BeTrue())
		Expect(client.CreateCheckRunResult.cra.Text).To(Equal(""))

		// Update existing CheckRun w/success
		pipelineRun.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				CompletionTime: &metav1.Time{Time: time.Now()},
				TaskRuns: map[string]*tektonv1beta1.PipelineRunTaskRunStatus{
					"task1": {
						PipelineTaskName: "task-passed",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "HACBS_TEST_OUTPUT",
										Value: "{\"result\":\"SUCCESS\"}",
									},
								},
							},
						},
					},
					"task2": {
						PipelineTaskName: "task-skipped",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "HACBS_TEST_OUTPUT",
										Value: "{\"result\":\"SKIPPED\"}",
									},
								},
							},
						},
					},
				},
			},
		}
		pipelineRun.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Message: "sample msg",
			Status:  "True",
		})
		var id int64 = 1
		client.GetCheckRunIDResult.ID = &id
		Expect(reporter.ReportStatus(context.TODO())).To(BeNil())
		Expect(client.UpdateCheckRunResult.cra.Title).To(Equal("example-pass has succeeded"))
		Expect(client.UpdateCheckRunResult.cra.Conclusion).To(Equal("success"))
		Expect(client.UpdateCheckRunResult.cra.CompletionTime.IsZero()).To(BeFalse())
		Expect(client.UpdateCheckRunResult.cra.Text).To(Equal("sample msg"))

		// Update existing CheckRun w/failure
		pipelineRun.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				CompletionTime: &metav1.Time{Time: time.Now()},
				TaskRuns: map[string]*tektonv1beta1.PipelineRunTaskRunStatus{
					"task1": {
						PipelineTaskName: "task-passed",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "HACBS_TEST_OUTPUT",
										Value: "{\"result\":\"FAILURE\"}",
									},
								},
							},
						},
					},
				},
			},
		}
		pipelineRun.Status.SetCondition(&apis.Condition{
			Type:   apis.ConditionSucceeded,
			Status: "True",
		})
		Expect(reporter.ReportStatus(context.TODO())).To(BeNil())
		Expect(client.UpdateCheckRunResult.cra.Title).To(Equal("example-pass has failed"))
		Expect(client.UpdateCheckRunResult.cra.Conclusion).To(Equal("failure"))
	})
})
