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
const NamePrefix = "Red Hat Trusted App Test"

// Reporter is a generic interface all status implementations must follow.
type Reporter interface {
	ReportStatus(client.Client, context.Context, *tektonv1beta1.PipelineRun) error
}

// Status is the interface of the main status Adapter.
type Status interface {
	GetReporters(*tektonv1beta1.PipelineRun) ([]Reporter, error)
}

// Adapter is responsible for discovering supported Reporter implementations.
type Adapter struct {
	logger         logr.Logger
	k8sClient      client.Reader
	githubReporter Reporter
}

// AdapterOption is used to extend Adapter with optional parameters.
type AdapterOption = func(a *Adapter)

// WithGitHubReporter is an option which allows for replacement of the GitHub PipelineRun reporter.
func WithGitHubReporter(reporter Reporter) AdapterOption {
	return func(a *Adapter) {
		a.githubReporter = reporter
	}
}

// NewAdapter constructs an Adapter with optional params, if specified.
func NewAdapter(logger logr.Logger, k8sClient client.Client, opts ...AdapterOption) *Adapter {
	adapter := Adapter{
		logger:         logger,
		k8sClient:      k8sClient,
		githubReporter: NewGitHubReporter(logger, k8sClient),
	}

	for _, opt := range opts {
		opt(&adapter)
	}

	return &adapter
}

// GetReporters returns a list of enabled/supported status reporters for a PipelineRun.
// All potential reporters must be added to this function for them to be utilized.
func (a *Adapter) GetReporters(pipelineRun *tektonv1beta1.PipelineRun) ([]Reporter, error) {
	var reporters []Reporter

	if helpers.HasLabelWithValue(pipelineRun, gitops.PipelineAsCodeGitProviderLabel, gitops.PipelineAsCodeGitHubProviderType) {
		reporters = append(reporters, a.githubReporter)
	}

	return reporters, nil
}
