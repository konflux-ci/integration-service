package status_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/status"

	"github.com/go-logr/logr"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MockReporter struct{}

func (r *MockReporter) ReportStatus(context.Context, *tektonv1beta1.PipelineRun) error {
	return nil
}

var _ = Describe("Status Adapter", func() {

	var pipelineRun *tektonv1beta1.PipelineRun

	BeforeEach(func() {
		pipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"pipelinesascode.tekton.dev/git-provider": "github",
				},
			},
		}
	})

	It("can get reporters from a PipelineRun", func() {
		adapter := status.NewAdapter(logr.Discard(), nil, status.WithGitHubReporter(&MockReporter{}))
		reporters, err := adapter.GetReporters(pipelineRun)
		Expect(err).To(BeNil())
		Expect(len(reporters)).To(Equal(1))
	})
})
