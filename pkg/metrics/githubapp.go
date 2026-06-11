package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v45/github"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	MetricsNamespace                = "redhat_appstudio"
	MetricsSubsystem                = "integrationservice"
	IntegrationServiceNamespaceName = "integration-service"
	// Pipelines as Code GitHub appliaction configuration secret name.
	// The secret is located in integration-service namespace.
	PipelinesAsCodeGitHubAppSecretName = "pipelines-as-code-secret"
	// Keys of the GitHub app ID and the app private key in the pipelines-as-code-secret
	PipelinesAsCodeGithubAppIdKey   = "github-application-id"
	PipelinesAsCodeGithubPrivateKey = "github-private-key"
)

type GithubAppAvailabilityProbe struct {
	client                  client.Client
	gauge                   prometheus.Gauge
	getGithubAppCredentials func(ctx context.Context, client client.Client) (int64, []byte, error)
	getGithubApp            func(ctx context.Context, tr http.RoundTripper, appID int64, privateKey []byte) (*github.App, *github.Response, error)
}

func NewGithubAppAvailabilityProbe(client client.Client) *GithubAppAvailabilityProbe {
	return &GithubAppAvailabilityProbe{
		client: client,
		gauge: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: MetricsNamespace,
				Subsystem: MetricsSubsystem,
				Name:      "global_github_app_available",
				Help:      "The availability of the Github App",
			}),
		getGithubAppCredentials: getGithubAppCredentials,
		getGithubApp:            getGithubApp,
	}
}

func (g *GithubAppAvailabilityProbe) CheckAvailability(ctx context.Context) bool {
	githubAppId, privateKey, err := g.getGithubAppCredentials(ctx, g.client)
	if err != nil {
		return false
	}
	_, _, err = g.getGithubApp(ctx, http.DefaultTransport, githubAppId, privateKey)
	return err == nil
}

func getGithubAppCredentials(ctx context.Context, client client.Client) (int64, []byte, error) {
	pacSecret := corev1.Secret{}
	globalPaCSecretKey := types.NamespacedName{Namespace: IntegrationServiceNamespaceName, Name: PipelinesAsCodeGitHubAppSecretName}
	if err := client.Get(ctx, globalPaCSecretKey, &pacSecret); err != nil {
		return 0, nil, fmt.Errorf("pipelines as Code secret not found in %s namespace", IntegrationServiceNamespaceName)
	}
	config := pacSecret.Data
	githubAppIdStr := string(config[PipelinesAsCodeGithubAppIdKey])
	githubAppId, err := strconv.ParseInt(githubAppIdStr, 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create git client: failed to convert %s to int: %w", githubAppIdStr, err)
	}
	privateKey := config[PipelinesAsCodeGithubPrivateKey]
	if len(config[PipelinesAsCodeGithubPrivateKey]) == 0 {
		return 0, nil, fmt.Errorf("invalid configuration in Pipelines as Code secret")

	}
	return githubAppId, privateKey, nil
}

func getGithubApp(ctx context.Context, rt http.RoundTripper, appID int64, privateKey []byte) (*github.App, *github.Response, error) {

	transport, err := ghinstallation.NewAppsTransport(rt, appID, privateKey)
	if err != nil {
		// Inability to create transport based on a private key indicates that the key is badly formatted
		return nil, nil, err
	}
	client := github.NewClient(&http.Client{Transport: transport})
	app, resp, err := client.Apps.Get(ctx, "")
	if err != nil {
		ctrllog.FromContext(ctx).Error(err, "GitHub App communication error", "app", app, "resp", resp)
		return nil, nil, err
	}

	return app, resp, err
}

func (g *GithubAppAvailabilityProbe) AvailabilityGauge() prometheus.Gauge {
	return g.gauge
}
