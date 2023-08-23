package status_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/status"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockReporter struct{}

func (r *MockReporter) ReportStatusForPipelineRun(client.Client, context.Context, *tektonv1beta1.PipelineRun) error {
	return nil
}

func (r *MockReporter) ReportStatusForSnapshot(client.Client, context.Context, *applicationapiv1alpha1.Snapshot, string, gitops.IntegrationTestStatus) error {
	return nil
}

var _ = Describe("Status Adapter", func() {

	var pipelineRun *tektonv1beta1.PipelineRun

	BeforeEach(func() {
		pipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"pac.test.appstudio.openshift.io/git-provider": "github",
				},
			},
		}
	})

	It("can get reporters from a PipelineRun", func() {
		adapter := status.NewAdapter(logr.Discard(), nil, status.WithGitHubReporter(&MockReporter{}))
		reporter, err := adapter.GetReporters(pipelineRun)
		Expect(err).To(BeNil())
		Expect(reporter).NotTo(BeNil())
	})
})
