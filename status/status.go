package status

import (
	"context"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/operator-toolkit/metadata"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NamePrefix is a common name prefix for this service.
const NamePrefix = "Red Hat Trusted App Test"

// Reporter is a generic interface all status implementations must follow.
type Reporter interface {
	ReportStatus(client.Client, context.Context, *tektonv1beta1.PipelineRun) error
	ReportStatusForSnapshot(client.Client, context.Context, *helpers.IntegrationLogger, *applicationapiv1alpha1.Snapshot) error
}

// Status is the interface of the main status Adapter.
type Status interface {
	GetReporters(client.Object) ([]Reporter, error)
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
func (a *Adapter) GetReporters(object client.Object) ([]Reporter, error) {
	var reporters []Reporter

	if metadata.HasLabelWithValue(object, gitops.PipelineAsCodeGitProviderLabel, gitops.PipelineAsCodeGitHubProviderType) {
		reporters = append(reporters, a.githubReporter)
	}

	return reporters, nil
}
