package status

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/git/github"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GitHubReporter reports status back to GitHub for a PipelineRun.
type GitHubReporter struct {
	logger    logr.Logger
	k8sClient client.Client
	client    github.ClientInterface
}

// GitHubReporterOption is used to extend GitHubReporter with optional parameters.
type GitHubReporterOption = func(r *GitHubReporter)

func WithGitHubClient(client github.ClientInterface) GitHubReporterOption {
	return func(r *GitHubReporter) {
		r.client = client
	}
}

// NewGitHubReporter returns a struct implementing the Reporter interface for GitHub
func NewGitHubReporter(logger logr.Logger, k8sClient client.Client, opts ...GitHubReporterOption) *GitHubReporter {
	reporter := GitHubReporter{
		logger:    logger,
		k8sClient: k8sClient,
		client:    github.NewClient(logger),
	}

	for _, opt := range opts {
		opt(&reporter)
	}

	return &reporter
}

type appCredentials struct {
	AppID          int64
	InstallationID int64
	PrivateKey     []byte
}

func (r *GitHubReporter) getAppCredentials(ctx context.Context, object client.Object) (*appCredentials, error) {
	var err error
	var found bool
	appInfo := appCredentials{}

	appInfo.InstallationID, err = strconv.ParseInt(object.GetAnnotations()[gitops.PipelineAsCodeInstallationIDAnnotation], 10, 64)
	if err != nil {
		return nil, err
	}

	// Get the global pipelines as code secret
	pacSecret := v1.Secret{}
	err = r.k8sClient.Get(ctx, types.NamespacedName{Namespace: "pipelines-as-code", Name: "pipelines-as-code-secret"}, &pacSecret)
	if err != nil {
		return nil, err
	}

	// Get the App ID from the secret
	ghAppIDBytes, found := pacSecret.Data["github-application-id"]
	if !found {
		return nil, errors.New("failed to find github-application-id secret key")
	}

	appInfo.AppID, err = strconv.ParseInt(string(ghAppIDBytes), 10, 64)
	if err != nil {
		return nil, err
	}

	// Get the App's private key from the secret
	appInfo.PrivateKey, found = pacSecret.Data["github-private-key"]
	if !found {
		return nil, errors.New("failed to find github-private-key secret key")
	}

	return &appInfo, nil
}

func (r *GitHubReporter) getToken(ctx context.Context, object client.Object, namespace string) (string, error) {
	var err error

	// List all the Repository CRs in the PipelineRun's namespace
	repos := pacv1alpha1.RepositoryList{}
	if err = r.k8sClient.List(ctx, &repos, &client.ListOptions{Namespace: namespace}); err != nil {
		return "", err
	}

	// Get the full repo URL
	url, found := object.GetAnnotations()[gitops.PipelineAsCodeRepoURLAnnotation]
	if !found {
		return "", fmt.Errorf("object annotation not found %q", gitops.PipelineAsCodeRepoURLAnnotation)
	}

	// Find a Repository CR with a matching URL and get its secret details
	var repoSecret *pacv1alpha1.Secret
	for _, repo := range repos.Items {
		if url == repo.Spec.URL {
			repoSecret = repo.Spec.GitProvider.Secret
			break
		}
	}

	if repoSecret == nil {
		return "", fmt.Errorf("failed to find a Repository matching URL: %q", url)
	}

	// Get the pipelines as code secret from the namespace
	pacSecret := v1.Secret{}
	err = r.k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: repoSecret.Name}, &pacSecret)
	if err != nil {
		return "", err
	}

	// Get the personal access token from the secret
	token, found := pacSecret.Data[repoSecret.Key]
	if !found {
		return "", fmt.Errorf("failed to find %s secret key", repoSecret.Key)
	}

	return string(token), nil
}

func (r *GitHubReporter) createCheckRunAdapterForPipelineRun(k8sClient client.Client, ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) (*github.CheckRunAdapter, error) {
	labels := pipelineRun.GetLabels()

	scenario, found := labels[gitops.SnapshotTestScenarioLabel]
	if !found {
		return nil, fmt.Errorf("PipelineRun label not found %q", gitops.SnapshotTestScenarioLabel)
	}

	component, found := labels[gitops.SnapshotComponentLabel]
	if !found {
		return nil, fmt.Errorf("PipelineRun label not found %q", gitops.SnapshotComponentLabel)
	}

	owner, found := labels[gitops.PipelineAsCodeURLOrgLabel]
	if !found {
		return nil, fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeURLOrgLabel)
	}

	repo, found := labels[gitops.PipelineAsCodeURLRepositoryLabel]
	if !found {
		return nil, fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeURLRepositoryLabel)
	}

	SHA, found := labels[gitops.PipelineAsCodeSHALabel]
	if !found {
		return nil, fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeSHALabel)
	}

	var title, conclusion string
	succeeded := pipelineRun.Status.GetCondition(apis.ConditionSucceeded)

	if succeeded.IsUnknown() {
		title = scenario + " has started"
	} else {
		outcome, err := helpers.CalculateIntegrationPipelineRunOutcome(k8sClient, ctx, r.logger, pipelineRun)

		if err != nil {
			return nil, err
		}

		if outcome {
			title = scenario + " has succeeded"
			conclusion = "success"
		} else {
			title = scenario + " has failed"
			conclusion = "failure"
		}
	}

	taskRuns, err := helpers.GetAllChildTaskRunsForPipelineRun(r.k8sClient, ctx, r.logger, pipelineRun)
	if err != nil {
		return nil, fmt.Errorf("error while getting all child taskRuns from pipelineRun %s: %w", pipelineRun.Name, err)
	}
	summary, err := FormatSummary(taskRuns)
	if err != nil {
		return nil, err
	}

	startTime := time.Time{}
	if start := pipelineRun.Status.StartTime; start != nil {
		startTime = start.Time
	}

	completionTime := time.Time{}
	if complete := pipelineRun.Status.CompletionTime; complete != nil {
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
		ExternalID:     pipelineRun.Name,
		Conclusion:     conclusion,
		Title:          title,
		Summary:        summary,
		Text:           text,
		StartTime:      startTime,
		CompletionTime: completionTime,
	}, nil
}

func (r *GitHubReporter) createCheckRunAdapterForSnapshot(k8sClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, scenarioName string, scenarioTestStatus gitops.IntegrationTestStatus) (*github.CheckRunAdapter, error) {
	labels := snapshot.GetLabels()

	owner, found := labels[gitops.PipelineAsCodeURLOrgLabel]
	if !found {
		return nil, fmt.Errorf("snapshot label not found %q", gitops.PipelineAsCodeURLOrgLabel)
	}

	repo, found := labels[gitops.PipelineAsCodeURLRepositoryLabel]
	if !found {
		return nil, fmt.Errorf("snapshot label not found %q", gitops.PipelineAsCodeURLRepositoryLabel)
	}

	SHA, found := labels[gitops.PipelineAsCodeSHALabel]
	if !found {
		return nil, fmt.Errorf("snapshot label not found %q", gitops.PipelineAsCodeSHALabel)
	}

	var title, conclusion string

	switch scenarioTestStatus {
	case gitops.IntegrationTestStatusEnvironmentProvisionError:
		title = "snapshot " + snapshot.Name + " experienced error when provisioning environment for integrationTestScenario " + scenarioName
		conclusion = "errorOccured"
	case gitops.IntegrationTestStatusDeploymentError:
		title = "snapshot " + snapshot.Name + " experienced error when deploying snapshotEnvironmentBinding for integrationTestScenario " + scenarioName
		conclusion = "errorOccured"
	case gitops.IntegrationTestStatusTestPassed:
		title = "snapshot " + snapshot.Name + " has passed integration test against integrationTestScenario " + scenarioName
		conclusion = "success"
	case gitops.IntegrationTestStatusTestFail:
		title = "snapshot " + snapshot.Name + " has failed integration test against integrationTestScenario " + scenarioName
		conclusion = "failure"
	default:
		return nil, fmt.Errorf("unknown status %s for integrationTestScenario %s/%s", scenarioTestStatus, snapshot.Namespace, scenarioName)
	}

	startTime := time.Now()
	// placeholder for the starttime
	// if start := pipelineRun.Status.StartTime; start != nil {
	// 	startTime = start.Time
	// }

	completionTime := time.Now()
	// placeholder for the completiontime
	// if complete := pipelineRun.Status.CompletionTime; complete != nil {
	// 	completionTime = complete.Time
	// }

	return &github.CheckRunAdapter{
		Owner:      owner,
		Repository: repo,
		Name:       NamePrefix + " / " + snapshot.Name + " / " + scenarioName,
		SHA:        SHA,
		ExternalID: scenarioName,
		Conclusion: conclusion,
		Title:      title,
		// Summary:        summary,
		// Text:           text,
		StartTime:      startTime,
		CompletionTime: completionTime,
	}, nil
}

func (r *GitHubReporter) createCommitStatusForPipelineRun(k8sClient client.Client, ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) error {
	var (
		state       string
		description string
	)

	labels := pipelineRun.GetLabels()

	scenario, found := labels[gitops.SnapshotTestScenarioLabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.SnapshotTestScenarioLabel)
	}

	component, found := labels[gitops.SnapshotComponentLabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.SnapshotComponentLabel)
	}

	owner, found := labels[gitops.PipelineAsCodeURLOrgLabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeURLOrgLabel)
	}

	repo, found := labels[gitops.PipelineAsCodeURLRepositoryLabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeURLRepositoryLabel)
	}

	SHA, found := labels[gitops.PipelineAsCodeSHALabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeSHALabel)
	}

	statusContext := NamePrefix + " / " + component + " / " + scenario

	succeeded := pipelineRun.Status.GetCondition(apis.ConditionSucceeded)

	if succeeded.IsUnknown() {
		state = "pending"
		description = scenario + " has started"
	} else {
		outcome, err := helpers.CalculateIntegrationPipelineRunOutcome(k8sClient, ctx, r.logger, pipelineRun)
		if err != nil {
			return err
		}

		if outcome {
			state = "success"
			description = scenario + " has succeeded"
		} else {
			state = "failure"
			description = scenario + " has failed"
		}
	}

	_, err := r.client.CreateCommitStatus(ctx, owner, repo, SHA, state, description, statusContext)
	if err != nil {
		return err
	}

	return nil
}

func (r *GitHubReporter) createCommitStatusForSnapshot(k8sClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, scenarioName string, scenarioTestStatus gitops.IntegrationTestStatus) error {
	var (
		state       string
		description string
	)

	labels := snapshot.GetLabels()

	owner, found := labels[gitops.PipelineAsCodeURLOrgLabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeURLOrgLabel)
	}

	repo, found := labels[gitops.PipelineAsCodeURLRepositoryLabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeURLRepositoryLabel)
	}

	SHA, found := labels[gitops.PipelineAsCodeSHALabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeSHALabel)
	}

	statusContext := NamePrefix + " / " + snapshot.Name + " / " + scenarioName

	switch scenarioTestStatus {
	case gitops.IntegrationTestStatusEnvironmentProvisionError:
		state = "errorOccured"
		description = "snapshot " + snapshot.Name + " experienced error when provisioning environment for integrationTestScenario " + scenarioName
	case gitops.IntegrationTestStatusDeploymentError:
		state = "errorOccured"
		description = "snapshot " + snapshot.Name + " experienced error when deploying snapshotEnvironmentBinding for integrationTestScenario " + scenarioName
	case gitops.IntegrationTestStatusTestPassed:
		state = "success"
		description = "snapshot " + snapshot.Name + " has passed integration test against integrationTestScenario " + scenarioName
	case gitops.IntegrationTestStatusTestFail:
		state = "failure"
		description = "snapshot " + snapshot.Name + " has failed integration test against integrationTestScenario " + scenarioName
	default:
		return fmt.Errorf("unknown status %s for integrationTestScenario %s/%s", scenarioTestStatus, snapshot.Namespace, scenarioName)
	}

	_, err := r.client.CreateCommitStatus(ctx, owner, repo, SHA, state, description, statusContext)
	if err != nil {
		return err
	}

	return nil
}

func (r *GitHubReporter) createComment(k8sClient client.Client, ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) error {
	labels := pipelineRun.GetLabels()

	succeeded := pipelineRun.Status.GetCondition(apis.ConditionSucceeded)
	if succeeded.IsUnknown() {
		return nil
	}

	scenario, found := labels[gitops.SnapshotTestScenarioLabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.SnapshotTestScenarioLabel)
	}

	owner, found := labels[gitops.PipelineAsCodeURLOrgLabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeURLOrgLabel)
	}

	repo, found := labels[gitops.PipelineAsCodeURLRepositoryLabel]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeURLRepositoryLabel)
	}

	issueNumberStr, found := pipelineRun.GetAnnotations()[gitops.PipelineAsCodePullRequestAnnotation]
	if !found {
		return fmt.Errorf("PipelineRun label not found %q", gitops.PipelineAsCodeURLRepositoryLabel)
	}

	issueNumber, err := strconv.Atoi(issueNumberStr)
	if err != nil {
		return err
	}

	outcome, err := helpers.CalculateIntegrationPipelineRunOutcome(k8sClient, ctx, r.logger, pipelineRun)
	if err != nil {
		return err
	}

	var title string
	if outcome {
		title = scenario + " has succeeded"
	} else {
		title = scenario + " has failed"
	}

	taskRuns, err := helpers.GetAllChildTaskRunsForPipelineRun(r.k8sClient, ctx, r.logger, pipelineRun)
	if err != nil {
		return fmt.Errorf("error while getting all child taskRuns from pipelineRun %s: %w", pipelineRun.Name, err)
	}
	comment, err := FormatComment(title, taskRuns)
	if err != nil {
		return err
	}

	_, err = r.client.CreateComment(ctx, owner, repo, issueNumber, comment)
	if err != nil {
		return err
	}

	return nil
}

// ReportStatusForPipelineRun creates/updates CheckRuns when using GitHub App integration.
// When using GitHub webhook integration a commit status and, in some cases, a comment is created.
func (r *GitHubReporter) ReportStatusForPipelineRun(k8sClient client.Client, ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) error {
	if !helpers.HasLabelWithValue(pipelineRun, gitops.PipelineAsCodeEventTypeLabel, gitops.PipelineAsCodePullRequestType) {
		return nil
	}

	// Existence of the Pipelines as Code installation ID annotation signals configuration using GitHub App integration.
	// If it doesn't exist, GitHub webhook integration is configured.
	if helpers.HasAnnotation(pipelineRun, gitops.PipelineAsCodeInstallationIDAnnotation) {
		creds, err := r.getAppCredentials(ctx, pipelineRun)
		if err != nil {
			return err
		}

		token, err := r.client.CreateAppInstallationToken(ctx, creds.AppID, creds.InstallationID, creds.PrivateKey)
		if err != nil {
			return err
		}

		r.client.SetOAuthToken(ctx, token)

		checkRun, err := r.createCheckRunAdapterForPipelineRun(k8sClient, ctx, pipelineRun)
		if err != nil {
			return err
		}

		checkRunID, err := r.client.GetCheckRunID(ctx, checkRun.Owner, checkRun.Repository, checkRun.SHA, checkRun.ExternalID, creds.AppID)
		if err != nil {
			return err
		}

		if checkRunID == nil {
			_, err = r.client.CreateCheckRun(ctx, checkRun)
		} else {
			err = r.client.UpdateCheckRun(ctx, *checkRunID, checkRun)
		}

		if err != nil {
			return err
		}
	} else {
		token, err := r.getToken(ctx, pipelineRun, pipelineRun.Namespace)
		if err != nil {
			return err
		}

		r.client.SetOAuthToken(ctx, token)

		err = r.createCommitStatusForPipelineRun(k8sClient, ctx, pipelineRun)
		if err != nil {
			return err
		}

		err = r.createComment(k8sClient, ctx, pipelineRun)
		if err != nil {
			return err
		}
	}

	return nil
}

// ReportStatusForSnapshot creates CheckRuns when using GitHub App integration.
// When using GitHub webhook integration a commit status
func (r *GitHubReporter) ReportStatusForSnapshot(k8sClient client.Client, ctx context.Context, snapshot *applicationapiv1alpha1.Snapshot, scenarioName string, scenarioTestStatus gitops.IntegrationTestStatus) error {
	if !helpers.HasLabelWithValue(snapshot, gitops.PipelineAsCodeEventTypeLabel, gitops.PipelineAsCodePullRequestType) {
		return nil
	}

	// Existence of the Pipelines as Code installation ID annotation signals configuration using GitHub App integration.
	// If it doesn't exist, GitHub webhook integration is configured.
	if helpers.HasAnnotation(snapshot, gitops.PipelineAsCodeInstallationIDAnnotation) {
		creds, err := r.getAppCredentials(ctx, snapshot)
		if err != nil {
			return err
		}

		token, err := r.client.CreateAppInstallationToken(ctx, creds.AppID, creds.InstallationID, creds.PrivateKey)
		if err != nil {
			return err
		}

		r.client.SetOAuthToken(ctx, token)

		checkRun, err := r.createCheckRunAdapterForSnapshot(k8sClient, ctx, snapshot, scenarioName, scenarioTestStatus)
		if err != nil {
			return err
		}

		checkRunID, err := r.client.GetCheckRunID(ctx, checkRun.Owner, checkRun.Repository, checkRun.SHA, checkRun.ExternalID, creds.AppID)
		if err != nil {
			return err
		}

		if checkRunID == nil {
			_, err = r.client.CreateCheckRun(ctx, checkRun)
		} else {
			err = r.client.UpdateCheckRun(ctx, *checkRunID, checkRun)
		}

		if err != nil {
			return err
		}
	} else {
		token, err := r.getToken(ctx, snapshot, snapshot.Namespace)
		if err != nil {
			return err
		}

		r.client.SetOAuthToken(ctx, token)

		err = r.createCommitStatusForSnapshot(k8sClient, ctx, snapshot, scenarioName, scenarioTestStatus)
		if err != nil {
			return err
		}
	}

	return nil
}
