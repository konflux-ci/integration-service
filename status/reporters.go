package status

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/redhat-appstudio/integration-service/git/github"
	"github.com/redhat-appstudio/integration-service/helpers"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GitHubPipelineRunReporter reports status back to GitHub for a PipelineRun.
type GitHubPipelineRunReporter struct {
	logger           logr.Logger
	pipelineRun      *tektonv1beta1.PipelineRun
	installationID   int64
	appID            int64
	privateKey       []byte
	appClientCreator github.AppClientCreator
}

// GitHubPipelineRunReporterOption is used to extend GitHubPipelineRunReporter with optional parameters.
type GitHubPipelineRunReporterOption = func(r *GitHubPipelineRunReporter)

// GitHubPipelineRunReporterCreator is the signature of the GitHub PipelineRun reporter constructor function.
type GitHubPipelineRunReporterCreator func(ctx context.Context, logger logr.Logger, k8sClient client.Reader, pipelineRun *tektonv1beta1.PipelineRun, opts ...GitHubPipelineRunReporterOption) (Reporter, error)

// WithAppClientCreator is an option which allows the replacement of the github App client constructor function.
func WithAppClientCreator(creator github.AppClientCreator) GitHubPipelineRunReporterOption {
	return func(r *GitHubPipelineRunReporter) {
		r.appClientCreator = creator
	}
}

// NewGitHubPipelineRunReporter returns a struct implementing the Reporter interface for GitHub
func NewGitHubPipelineRunReporter(ctx context.Context, logger logr.Logger, k8sClient client.Reader, pipelineRun *tektonv1beta1.PipelineRun, opts ...GitHubPipelineRunReporterOption) (Reporter, error) {
	var err error
	reporter := GitHubPipelineRunReporter{
		logger:           logger,
		pipelineRun:      pipelineRun,
		appClientCreator: github.NewAppClient,
	}

	for _, opt := range opts {
		opt(&reporter)
	}

	pacSecret := v1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "pipelines-as-code", Name: "pipelines-as-code-secret"}, &pacSecret)
	if err != nil {
		return nil, err
	}

	if helpers.HasAnnotation(pipelineRun, InstallationIDAnnotation) {
		reporter.installationID, err = strconv.ParseInt(pipelineRun.GetAnnotations()[InstallationIDAnnotation], 10, 64)
		if err != nil {
			return nil, err
		}

		ghAppIDBytes, found := pacSecret.Data["github-application-id"]
		if !found {
			return nil, errors.New("failed to find github-application-id secret key")
		}

		reporter.appID, err = strconv.ParseInt(string(ghAppIDBytes), 10, 64)
		if err != nil {
			return nil, err
		}

		reporter.privateKey, found = pacSecret.Data["github-private-key"]
		if !found {
			return nil, errors.New("failed to find github-private-key secret key")
		}
	}

	return &reporter, nil
}

func (r *GitHubPipelineRunReporter) createCheckRunAdapter() (*github.CheckRunAdapter, error) {
	scenario, found := r.pipelineRun.GetLabels()[TestScenarioLabel]
	if !found {
		return nil, fmt.Errorf("PipelineRun label not found %q", TestScenarioLabel)
	}

	component, found := r.pipelineRun.GetLabels()[ComponentLabel]
	if !found {
		return nil, fmt.Errorf("PipelineRun label not found %q", ComponentLabel)
	}

	owner, found := r.pipelineRun.Labels[OrgLabel]
	if !found {
		return nil, fmt.Errorf("PipelineRun label not found %q", OrgLabel)
	}

	repo, found := r.pipelineRun.Labels[RepositoryLabel]
	if !found {
		return nil, fmt.Errorf("PipelineRun label not found %q", RepositoryLabel)
	}

	SHA, found := r.pipelineRun.Labels[SHALabel]
	if !found {
		return nil, fmt.Errorf("PipelineRun label not found %q", SHALabel)
	}

	var title, conclusion string
	succeeded := r.pipelineRun.Status.GetCondition(apis.ConditionSucceeded)
	if succeeded.IsTrue() {
		title = scenario + " has succeeded"
		conclusion = "success"
	} else if succeeded.IsFalse() {
		title = scenario + " has failed"
		conclusion = "failure"
	} else {
		title = scenario + " has started"
	}

	results, err := helpers.GetHACBSTestResultsFromPipelineRun(r.logger, r.pipelineRun)
	if err != nil {
		return nil, err
	}

	summary, err := FormatSummary(results)
	if err != nil {
		return nil, err
	}

	startTime := time.Time{}
	if start := r.pipelineRun.Status.StartTime; start != nil {
		startTime = start.Time
	}

	completionTime := time.Time{}
	if complete := r.pipelineRun.Status.CompletionTime; complete != nil {
		completionTime = complete.Time
	}

	text := ""
	if !succeeded.IsUnknown() {
		text = succeeded.Message
	}

	return &github.CheckRunAdapter{
		Owner:          owner,
		Repository:     repo,
		Name:           NamePrefix + " / " + component + " / " + scenario,
		SHA:            SHA,
		ExternalID:     r.pipelineRun.Name,
		Conclusion:     conclusion,
		Title:          title,
		Summary:        summary,
		Text:           text,
		StartTime:      startTime,
		CompletionTime: completionTime,
	}, nil
}

// ReportStatus creates/updates CheckRuns when using GitHub App integration.
func (r *GitHubPipelineRunReporter) ReportStatus(ctx context.Context) error {
	if !helpers.HasLabelWithValue(r.pipelineRun, EventTypeLabel, PullRequestEventType) {
		return nil
	}

	if r.installationID > 0 {
		checkRun, err := r.createCheckRunAdapter()

		if err != nil {
			return err
		}

		client, err := r.appClientCreator(ctx, r.logger, r.appID, r.installationID, r.privateKey)

		if err != nil {
			return err
		}

		checkRunID, err := client.GetCheckRunID(ctx, checkRun.Owner, checkRun.Repository, checkRun.SHA, checkRun.ExternalID)

		if err != nil {
			return err
		}

		if checkRunID == nil {
			_, err = client.CreateCheckRun(ctx, checkRun)
		} else {
			err = client.UpdateCheckRun(ctx, *checkRunID, checkRun)
		}

		if err != nil {
			return err
		}
	}

	return nil
}
