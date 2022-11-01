package status

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NamePrefix is a common name prefix for this service.
const NamePrefix = "HACBS Test"

// Reporter is a generic interface all status implementations must follow.
type Reporter interface {
	ReportStatus(context.Context) error
}

// Status is the interface of the main status Adapter.
type Status interface {
	GetReporters(context.Context, *tektonv1beta1.PipelineRun) ([]Reporter, error)
}

// Adapter is responsible for discovering supported Reporter implementations.
type Adapter struct {
	logger         logr.Logger
	k8sClient      client.Reader
	githubReporter GitHubPipelineRunReporterCreator
}

// AdapterOption is used to extend Adapter with optional parameters.
type AdapterOption = func(a *Adapter)

// WithGitHubPipelineRunReporterCreator is an option which allows for replacement of the GitHub PipelineRun reporter.
func WithGitHubPipelineRunReporterCreator(creator GitHubPipelineRunReporterCreator) AdapterOption {
	return func(a *Adapter) {
		a.githubReporter = creator
	}
}

// NewAdapter constructs an Adapter with optional params, if specified.
func NewAdapter(logger logr.Logger, k8sClient client.Reader, opts ...AdapterOption) *Adapter {
	adapter := Adapter{
		logger:         logger,
		k8sClient:      k8sClient,
		githubReporter: NewGitHubPipelineRunReporter,
	}

	for _, opt := range opts {
		opt(&adapter)
	}

	return &adapter
}

// GetReporters returns a list of enabled/supported status reporters for a PipelineRun.
func (a *Adapter) GetReporters(ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) ([]Reporter, error) {
	var reporters []Reporter

	if helpers.HasLabelWithValue(pipelineRun, gitops.PipelineAsCodeGitProviderLabel, gitops.PipelineAsCodeGitHubProviderType) {
		reporter, err := a.githubReporter(ctx, a.logger, a.k8sClient, pipelineRun)
		if err != nil {
			return nil, err
		}
		reporters = append(reporters, reporter)
	}

	return reporters, nil
}
