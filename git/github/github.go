package github

import (
	"context"
	"net/http"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/go-logr/logr"
	ghapi "github.com/google/go-github/v45/github"
	"golang.org/x/oauth2"
)

// CheckRunAdapter is an abstraction for the github.CheckRun struct.
type CheckRunAdapter struct {
	Owner          string
	Repository     string
	Name           string
	SHA            string
	ExternalID     string
	Conclusion     string
	Title          string
	Summary        string
	Text           string
	StartTime      time.Time
	CompletionTime time.Time
}

// GetStatus returns the appropriate status based on conclusion and start time.
func (s *CheckRunAdapter) GetStatus() string {
	if s.Conclusion == "success" || s.Conclusion == "failure" {
		return "completed"
	} else if s.StartTime.IsZero() {
		return "queued"
	}
	return "in_progress"
}

// AppsService defines the methods used in the github Apps service.
type AppsService interface {
	CreateInstallationToken(ctx context.Context, id int64, opts *ghapi.InstallationTokenOptions) (*ghapi.InstallationToken, *ghapi.Response, error)
}

// ChecksService defines the methods used in the github Checks service.
type ChecksService interface {
	CreateCheckRun(ctx context.Context, owner string, repo string, opts ghapi.CreateCheckRunOptions) (*ghapi.CheckRun, *ghapi.Response, error)
	ListCheckRunsForRef(ctx context.Context, owner string, repo string, ref string, opts *ghapi.ListCheckRunsOptions) (*ghapi.ListCheckRunsResults, *ghapi.Response, error)
	UpdateCheckRun(ctx context.Context, owner string, repo string, checkRunID int64, opts ghapi.UpdateCheckRunOptions) (*ghapi.CheckRun, *ghapi.Response, error)
}

// AppClientCreator is the signature of the constructor for GitHub App-based clients
type AppClientCreator func(ctx context.Context, logger logr.Logger, appID int64, installationID int64, privateKey []byte, opts ...AppClientOption) (AppClientInterface, error)

// AppClientInterface defines the methods that should be implemented by a GitHub App client
type AppClientInterface interface {
	CreateCheckRun(context.Context, *CheckRunAdapter) (*int64, error)
	UpdateCheckRun(context.Context, int64, *CheckRunAdapter) error
	GetCheckRunID(context.Context, string, string, string, string) (*int64, error)
}

// AppClient is an abstraction around the github API client.
type AppClient struct {
	logger         logr.Logger
	client         *ghapi.Client
	appID          int64
	installationID int64
	apps           AppsService
	checks         ChecksService
}

// AppClientOption is used to extend AppClient with optional parameters.
type AppClientOption = func(c *AppClient)

// WithAppsService is an option which allows for overriding the github client's default Apps service.
func WithAppsService(svc AppsService) AppClientOption {
	return func(c *AppClient) {
		c.apps = svc
	}
}

// WithChecksService is an option which allows for overriding the github client's default Checks service.
func WithChecksService(svc ChecksService) AppClientOption {
	return func(c *AppClient) {
		c.checks = svc
	}
}

// NewAppClient constructs a GitHub App client with valid session tokens.
func NewAppClient(
	ctx context.Context, logger logr.Logger, appID int64, installationID int64, privateKey []byte, opts ...AppClientOption,
) (AppClientInterface, error) {
	transport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, privateKey)
	if err != nil {
		return nil, err
	}

	client := ghapi.NewClient(&http.Client{Transport: transport})

	appClient := AppClient{
		logger:         logger,
		appID:          appID,
		installationID: installationID,
		apps:           client.Apps,
		checks:         client.Checks,
	}

	for _, opt := range opts {
		opt(&appClient)
	}

	installToken, _, err := appClient.apps.CreateInstallationToken(
		ctx,
		installationID,
		&ghapi.InstallationTokenOptions{},
	)

	if err != nil {
		return nil, err
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: installToken.GetToken()},
	)

	client = ghapi.NewClient(oauth2.NewClient(ctx, ts))
	appClient.client = client

	// Reapply options using the latest client's services as defaults
	appClient.apps = client.Apps
	appClient.checks = client.Checks
	for _, opt := range opts {
		opt(&appClient)
	}

	return &appClient, nil
}

// CreateCheckRun creates a new CheckRun via the GitHub API.
func (c *AppClient) CreateCheckRun(ctx context.Context, cra *CheckRunAdapter) (*int64, error) {
	status := cra.GetStatus()

	options := ghapi.CreateCheckRunOptions{
		Name:       cra.Name,
		HeadSHA:    cra.SHA,
		ExternalID: &cra.ExternalID,
		Status:     &status,
		Output: &ghapi.CheckRunOutput{
			Title:   &cra.Title,
			Summary: &cra.Summary,
			Text:    &cra.Text,
		},
	}

	if cra.Conclusion != "" {
		options.Conclusion = &cra.Conclusion
	}

	if !cra.StartTime.IsZero() {
		options.StartedAt = &ghapi.Timestamp{Time: cra.StartTime}
	}

	if !cra.CompletionTime.IsZero() {
		options.CompletedAt = &ghapi.Timestamp{Time: cra.CompletionTime}
	}

	cr, _, err := c.checks.CreateCheckRun(ctx, cra.Owner, cra.Repository, options)

	if err != nil {
		return nil, err
	}

	c.logger.Info("Created CheckRun",
		"ID", cr.ID,
		"CheckName", cr.Name,
		"Status", cr.Status,
		"Conclusion", cr.Conclusion,
	)

	return cr.ID, nil
}

// UpdateCheckRun updates and existing CheckRun via the GitHub API.
func (c *AppClient) UpdateCheckRun(ctx context.Context, checkRunID int64, cra *CheckRunAdapter) error {
	status := cra.GetStatus()

	options := ghapi.UpdateCheckRunOptions{
		Name:   cra.Name,
		Status: &status,
		Output: &ghapi.CheckRunOutput{
			Title:   &cra.Title,
			Summary: &cra.Summary,
			Text:    &cra.Text,
		},
	}

	if cra.Conclusion != "" {
		options.Conclusion = &cra.Conclusion
	}

	if !cra.CompletionTime.IsZero() {
		options.CompletedAt = &ghapi.Timestamp{Time: cra.CompletionTime}
	}

	cr, _, err := c.checks.UpdateCheckRun(ctx, cra.Owner, cra.Repository, checkRunID, options)

	if err != nil {
		return err
	}

	c.logger.Info("Updated CheckRun",
		"ID", cr.ID,
		"CheckName", cr.Name,
		"Status", cr.Status,
		"Conclusion", cr.Conclusion,
	)
	return nil

}

// GetCheckRunID returns an existing GitHub CheckRun ID if a match is found for the SHA and externalID
func (c *AppClient) GetCheckRunID(ctx context.Context, owner string, repo string, SHA string, externalID string) (*int64, error) {
	filter := "all"

	res, _, err := c.checks.ListCheckRunsForRef(
		ctx,
		owner,
		repo,
		SHA,
		&ghapi.ListCheckRunsOptions{
			AppID:  &c.appID,
			Filter: &filter,
		},
	)

	if err != nil {
		return nil, err
	}

	if *res.Total == 0 {
		c.logger.Info("Found no CheckRuns for the ref", "SHA", SHA)
		return nil, nil
	}

	for _, cr := range res.CheckRuns {
		if *cr.ExternalID == externalID {
			return cr.ID, nil
		}
	}
	c.logger.Info("Found no CheckRuns with a matching ExternalID", "ExternalID", externalID)

	return nil, nil
}
