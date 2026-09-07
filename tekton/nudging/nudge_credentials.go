/*
Copyright 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nudging

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	ghinstallation "github.com/bradleyfalzon/ghinstallation/v2"
	ghapi "github.com/google/go-github/v45/github"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// gitProviderAnnotationName is the annotation on a Component that overrides git provider detection.
	gitProviderAnnotationName = "git-provider"
)

// repositoryCredentials holds parsed docker config auth data for a single registry entry.
type repositoryCredentials struct {
	secretName string
	repoName   string
	username   string
	password   string
}

// repositoryConfigAuth mirrors the docker config auth JSON structure.
type repositoryConfigAuth struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"`
}

// dockerConfigJSON mirrors the top-level .dockerconfigjson structure.
type dockerConfigJSON struct {
	Auths map[string]repositoryConfigAuth `json:"auths"`
}

// applicationInstallation holds GitHub App installation data for a specific repository.
type applicationInstallation struct {
	Token        string
	ID           int64
	Repositories []*ghapi.Repository
}

// newGitHubAppClientFn creates an authenticated GitHub App client and fetches the App slug.
// Replaced in tests to avoid real GitHub API calls.
var newGitHubAppClientFn = newGitHubAppClient

// gitHubAppInstallationForRepo resolves a GitHub App installation token for a given repository URL.
// Replaced in tests to avoid real GitHub API calls.
var gitHubAppInstallationForRepo = getGitHubAppInstallation

// getGitHubBotUserIDFn returns the numeric GitHub user ID for an App's bot account.
// Replaced in tests to avoid real GitHub API calls.
var getGitHubBotUserIDFn = getGitHubBotUserID

// GetNudgeTargetsGithubApp returns NudgeTargets for components that use GitHub App authentication.
//
// It reads the global PaC secret from the integration-service namespace (INTEGRATION_NS env var),
// verifies that GitHub App credentials are configured, and for each GitHub-hosted component it
// obtains a repo-scoped installation token via the GitHub App JWT + installation token flow.
func GetNudgeTargetsGithubApp(ctx context.Context, c client.Client, targetComponents []applicationapiv1alpha1.Component, imageRepoHost, imageRepoUser, imageRepoPwd string) []NudgeTarget {
	log := ctrllog.FromContext(ctx)

	integrationNS := os.Getenv("INTEGRATION_NS")
	if integrationNS == "" {
		integrationNS = "integration-service"
	}

	pacSecret := corev1.Secret{}
	globalPaCSecretKey := types.NamespacedName{
		Namespace: integrationNS,
		Name:      tektonconsts.PipelinesAsCodeGitHubAppSecretName,
	}
	if err := c.Get(ctx, globalPaCSecretKey, &pacSecret); err != nil {
		log.Info("GitHub App PaC secret not found, skipping GitHub App auth path",
			"namespace", globalPaCSecretKey.Namespace,
			"secret", globalPaCSecretKey.Name,
			"error", err.Error())
		return nil
	}

	// Check if GitHub App is actually configured
	if !isPaCGitHubAppConfigured(pacSecret.Data) {
		log.Info("GitHub App is not configured in PaC secret, skipping GitHub App auth path")
		return nil
	}

	appIDStr := string(pacSecret.Data[tektonconsts.PipelinesAsCodeGithubAppIdKey])
	privateKeyPEM := pacSecret.Data[tektonconsts.PipelinesAsCodeGithubPrivateKey]

	appClient, appSlug, err := newGitHubAppClientFn(ctx, appIDStr, privateKeyPEM)
	if err != nil {
		log.Error(err, "failed to initialize GitHub App client, skipping GitHub App auth path")
		return nil
	}

	var appBotID int64
	if botID, botErr := getGitHubBotUserIDFn(ctx, appSlug); botErr != nil {
		log.Error(botErr, "failed to get GitHub App bot user ID, commits will use ID 0 in GitAuthor",
			"slug", appSlug)
	} else {
		appBotID = botID
	}

	var targets []NudgeTarget

	for i := range targetComponents {
		component := &targetComponents[i]

		gitProvider, err := getGitProvider(*component)
		if err != nil || gitProvider != "github" {
			continue
		}

		repoURL := getGitRepoURL(component)
		if repoURL == "" {
			log.Info("component has no git source URL, skipping", "ComponentName", component.Name)
			continue
		}

		// Only github.com is supported for GitHub App auth; ghinstallation connects to api.github.com.
		repoHost := getGitRepoHost(repoURL)
		if repoHost != "github.com" {
			log.Info("skipping GitHub App auth for non-github.com host",
				"ComponentName", component.Name, "host", repoHost)
			continue
		}

		log.Info("getting GitHub App installation token for component",
			"ComponentName", component.Name,
			"RepositoryUrl", repoURL)
		installation, err := gitHubAppInstallationForRepo(ctx, appClient, repoURL)
		if err != nil {
			log.Error(err, "failed to get GitHub App installation for component",
				"ComponentName", component.Name)
			continue
		}

		appBotName := fmt.Sprintf("%s[bot]", appSlug)
		repoPath := parseGitRepoPath(repoURL)

		branch := ""
		if component.Spec.Source.GitSource != nil && component.Spec.Source.GitSource.Revision != "" {
			branch = component.Spec.Source.GitSource.Revision
		} else if len(installation.Repositories) > 0 {
			branch = installation.Repositories[0].GetDefaultBranch()
		}

		customOpts, err := ReadCustomRenovateConfigMap(ctx, c, component)
		if err != nil {
			log.Error(err, "failed to read custom renovate config map, will still continue with nudging",
				"ComponentName", component.Name,
				"ComponentNamespace", component.Namespace)
		}

		targets = append(targets, NudgeTarget{
			ComponentName:                  component.Name,
			ComponentCustomRenovateOptions: customOpts,
			GitProvider:                    gitProvider,
			Username:                       appBotName,
			GitAuthor:                      fmt.Sprintf("%s <%d+%s@users.noreply.github.com>", appSlug, appBotID, appBotName),
			Token:                          installation.Token,
			Endpoint:                       buildAPIEndpoint(gitProvider, repoHost),
			Repositories: []RenovateRepository{{
				Repository:   repoPath,
				BaseBranches: branchSlice(branch),
			}},
			ImageRepositoryHost:     imageRepoHost,
			ImageRepositoryUsername: imageRepoUser,
			ImageRepositoryPassword: imageRepoPwd,
		})
		log.Info("component to update via GitHub App",
			"component", component.Name,
			"repositories", repoPath)
	}

	return targets
}

// GetNudgeTargetsBasicAuth returns NudgeTargets for components using basic auth (token-based) credentials.
//
// For each target component it:
// 1. Detects the git provider from the component's source URL
// 2. Looks up SCM credentials from namespace Secrets matching the repo host
// 3. Reads optional custom Renovate configuration
// 4. Builds a NudgeTarget with the gathered information
func GetNudgeTargetsBasicAuth(ctx context.Context, c client.Client, targetComponents []applicationapiv1alpha1.Component, imageRepoHost, imageRepoUser, imageRepoPwd string) []NudgeTarget {
	log := ctrllog.FromContext(ctx)
	targets := []NudgeTarget{}

	for i := range targetComponents {
		component := &targetComponents[i]

		gitProvider, err := getGitProvider(*component)
		if err != nil {
			log.Error(err, "error detecting git provider",
				"ComponentName", component.Name,
				"ComponentNamespace", component.Namespace)
			continue
		}

		repoURL := getGitRepoURL(component)
		if repoURL == "" {
			log.Info("component has no git source URL, skipping",
				"ComponentName", component.Name)
			continue
		}

		repoHost := getGitRepoHost(repoURL)
		if repoHost == "" {
			log.Error(fmt.Errorf("cannot parse host from URL %q", repoURL),
				"error parsing git repo host",
				"ComponentName", component.Name)
			continue
		}

		repoPath := parseGitRepoPath(repoURL)

		// Look up SCM credentials
		username, token, err := lookupSCMCredentialsViaRepository(ctx, c, component.Namespace, repoURL)
		if err != nil {
			log.Error(err, "error getting basic auth credentials for component",
				"ComponentName", component.Name,
				"ComponentNamespace", component.Namespace,
				"RepositoryUrl", repoURL)
			continue
		}

		if username == "" {
			username = tektonconsts.DefaultRenovateUser
		}

		// Determine branch
		branch := ""
		if component.Spec.Source.GitSource != nil && component.Spec.Source.GitSource.Revision != "" {
			branch = component.Spec.Source.GitSource.Revision
		}

		repositories := []RenovateRepository{
			{
				Repository:   repoPath,
				BaseBranches: branchSlice(branch),
			},
		}

		// Read custom Renovate config
		customOpts, err := ReadCustomRenovateConfigMap(ctx, c, component)
		if err != nil {
			log.Error(err, "failed to read custom renovate config map, will still continue with nudging",
				"ComponentName", component.Name,
				"ComponentNamespace", component.Namespace)
		}

		endpoint := buildAPIEndpoint(gitProvider, repoHost)

		targets = append(targets, NudgeTarget{
			ComponentName:                  component.Name,
			ComponentCustomRenovateOptions: customOpts,
			GitProvider:                    gitProvider,
			Username:                       username,
			GitAuthor:                      fmt.Sprintf("%s <%s@users.noreply.%s>", username, username, repoHost),
			Token:                          token,
			Endpoint:                       endpoint,
			Repositories:                   repositories,
			ImageRepositoryHost:            imageRepoHost,
			ImageRepositoryUsername:        imageRepoUser,
			ImageRepositoryPassword:        imageRepoPwd,
		})
		log.Info("component to update for basic auth",
			"component", component.Name,
			"repositories", repositories)
	}

	return targets
}

// GetImageRegistryCredentials resolves image registry credentials for a component by examining
// docker config secrets linked to the specified ServiceAccount.
//
// It parses the component's ContainerImage to determine the registry host, reads the
// ServiceAccount's linked secrets, and returns matching credentials.
func GetImageRegistryCredentials(ctx context.Context, c client.Client, component *applicationapiv1alpha1.Component, saName string) (host, username, password string, err error) {
	log := ctrllog.FromContext(ctx)

	if component.Spec.ContainerImage == "" {
		return "", "", "", fmt.Errorf("component %s/%s has no container image set", component.Namespace, component.Name)
	}

	// Parse registry host from container image
	host = parseImageHost(component.Spec.ContainerImage)
	if host == "" {
		return "", "", "", fmt.Errorf("cannot parse registry host from container image %q", component.Spec.ContainerImage)
	}

	namespace := component.Namespace

	// Read the ServiceAccount
	sa := &corev1.ServiceAccount{}
	if err := c.Get(ctx, types.NamespacedName{Name: saName, Namespace: namespace}, sa); err != nil {
		return "", "", "", fmt.Errorf("failed to read service account %s in namespace %s: %w", saName, namespace, err)
	}

	// Preserve ServiceAccount declaration order so conflicting registry
	// credentials from multiple linked secrets are resolved deterministically.
	// .secrets entries take precedence over .imagePullSecrets; first-seen wins.
	linkedSecretNames := linkedSecretNamesFromServiceAccount(sa)
	if len(linkedSecretNames) == 0 {
		return "", "", "", fmt.Errorf("no secrets linked to service account %s in namespace %s", saName, namespace)
	}

	// Parse credentials from linked docker config secrets
	var allCreds []repositoryCredentials
	for _, linkedSecretName := range linkedSecretNames {
		linkedSecret := &corev1.Secret{}
		err = c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: linkedSecretName}, linkedSecret)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return "", "", "", fmt.Errorf("failed to read secret %s in namespace %s: %w", linkedSecretName, namespace, err)
		}

		if linkedSecret.Type != corev1.SecretTypeDockerConfigJson {
			continue
		}

		dockerConfig := &dockerConfigJSON{}
		configData, ok := linkedSecret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			continue
		}
		if err := json.Unmarshal(configData, dockerConfig); err != nil {
			log.Error(err, "unable to parse docker json config",
				"secretName", linkedSecret.Name)
			continue
		}
		repoNames := make([]string, 0, len(dockerConfig.Auths))
		for repoName := range dockerConfig.Auths {
			repoNames = append(repoNames, repoName)
		}
		sort.Strings(repoNames)

		for _, repoName := range repoNames {
			repoAuth := dockerConfig.Auths[repoName]
			if repoAuth.Username != "" && repoAuth.Password != "" {
				allCreds = append(allCreds, repositoryCredentials{
					secretName: linkedSecret.Name,
					repoName:   repoName,
					username:   repoAuth.Username,
					password:   repoAuth.Password,
				})
			} else if repoAuth.Auth != "" {
				decoded, err := base64.StdEncoding.DecodeString(repoAuth.Auth)
				if err != nil {
					log.Error(err, "unable to decode docker config json auth",
						"repository", repoName,
						"secretName", linkedSecret.Name)
					continue
				}
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					allCreds = append(allCreds, repositoryCredentials{
						secretName: linkedSecret.Name,
						repoName:   repoName,
						username:   parts[0],
						password:   parts[1],
					})
				}
			}
		}
	}

	// Find the best matching credential for the image
	username, password, err = matchCredentialForImage(ctx, component.Spec.ContainerImage, allCreds)
	if err != nil {
		return host, "", "", fmt.Errorf("no credentials found for image %q in service account %s: %w",
			component.Spec.ContainerImage, saName, err)
	}

	return host, username, password, nil
}

// linkedSecretNamesFromServiceAccount returns unique secret names linked on the
// ServiceAccount, in declaration order: .secrets first, then .imagePullSecrets.
func linkedSecretNamesFromServiceAccount(sa *corev1.ServiceAccount) []string {
	names := make([]string, 0, len(sa.Secrets)+len(sa.ImagePullSecrets))
	seen := make(map[string]struct{}, len(sa.Secrets)+len(sa.ImagePullSecrets))
	appendUnique := func(name string) {
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for _, ref := range sa.Secrets {
		appendUnique(ref.Name)
	}
	for _, ref := range sa.ImagePullSecrets {
		appendUnique(ref.Name)
	}
	return names
}

// ---------- Helper functions ----------

// newGitHubAppClient creates an authenticated GitHub App transport and returns a client and the App slug.
// The transport handles RS256 JWT signing automatically (iss=appID, 10-min expiry).
func newGitHubAppClient(ctx context.Context, appIDStr string, privateKeyPEM []byte) (*ghapi.Client, string, error) {
	appID, err := strconv.ParseInt(strings.TrimSpace(appIDStr), 10, 64)
	if err != nil {
		return nil, "", fmt.Errorf("invalid GitHub App ID %q: %w", appIDStr, err)
	}
	itr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, privateKeyPEM)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create GitHub App transport: %w", err)
	}
	appClient := ghapi.NewClient(&http.Client{Transport: itr})
	githubApp, _, err := appClient.Apps.Get(ctx, "")
	if err != nil {
		return nil, "", fmt.Errorf("failed to get GitHub App metadata: %w", err)
	}
	return appClient, githubApp.GetSlug(), nil
}

// getGitHubAppInstallation finds the installation for repoURL and returns a repo-scoped token.
//
// It uses Repositories (name-based) in InstallationTokenOptions rather than RepositoryIDs to avoid
// implicitly minting a broad installation token via NewFromAppsTransport. The token response
// includes repository metadata (DefaultBranch) that callers use for branch resolution.
func getGitHubAppInstallation(ctx context.Context, appClient *ghapi.Client, repoURL string) (*applicationInstallation, error) {
	repoPath := parseGitRepoPath(repoURL)
	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("cannot parse owner/repo from URL %q", repoURL)
	}
	owner, repo := parts[0], parts[1]

	installation, _, err := appClient.Apps.FindRepositoryInstallation(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("GitHub App not installed for %s/%s: %w", owner, repo, err)
	}

	scopedToken, _, err := appClient.Apps.CreateInstallationToken(ctx, installation.GetID(),
		&ghapi.InstallationTokenOptions{
			Repositories: []string{repo},
			Permissions: &ghapi.InstallationPermissions{
				Contents:     ghapi.String("write"),
				PullRequests: ghapi.String("write"),
			},
		})
	if err != nil {
		return nil, fmt.Errorf("failed to create scoped installation token for %s/%s: %w", owner, repo, err)
	}

	return &applicationInstallation{
		Token:        scopedToken.GetToken(),
		ID:           installation.GetID(),
		Repositories: scopedToken.Repositories,
	}, nil
}

// getGitHubBotUserID returns the numeric GitHub user ID of the App's bot account.
// The ID is needed for the GitAuthor field format GitHub uses for App commits:
// "{slug} <{id}+{slug}[bot]@users.noreply.github.com>"
func getGitHubBotUserID(ctx context.Context, slug string) (int64, error) {
	c := ghapi.NewClient(nil) // public endpoint, no auth needed
	botName := fmt.Sprintf("%s[bot]", slug)
	user, _, err := c.Users.Get(ctx, botName)
	if err != nil {
		return 0, fmt.Errorf("failed to get GitHub bot user %q: %w", botName, err)
	}
	return user.GetID(), nil
}

// getGitProvider returns the git provider name (github, gitlab, bitbucket) based on
// the component's git-provider annotation or by inspecting the repository URL hostname.
func getGitProvider(component applicationapiv1alpha1.Component) (string, error) {
	allowedProviders := []string{"github", "gitlab", "bitbucket"}

	if component.Spec.Source.GitSource == nil {
		return "", fmt.Errorf("git source is not set for component %s/%s",
			component.Namespace, component.Name)
	}

	// Check annotation override first
	if component.Annotations != nil {
		if ann, ok := component.Annotations[gitProviderAnnotationName]; ok && ann != "" {
			for _, p := range allowedProviders {
				if ann == p {
					return p, nil
				}
			}
			return "", fmt.Errorf("unsupported git-provider annotation value %q on component %s/%s",
				ann, component.Namespace, component.Name)
		}
	}

	// Detect from URL hostname
	sourceURL := component.Spec.Source.GitSource.URL
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", fmt.Errorf("cannot parse git source URL %q: %w", sourceURL, err)
	}
	host := u.Hostname()

	for _, provider := range allowedProviders {
		if strings.Contains(host, provider) {
			return provider, nil
		}
	}

	return "", fmt.Errorf("cannot determine git provider from URL %q for component %s/%s, "+
		"set the %q annotation on the component",
		sourceURL, component.Namespace, component.Name, gitProviderAnnotationName)
}

// getGitRepoURL returns the normalized git repository URL from a component, stripping
// trailing slashes and .git suffix.
func getGitRepoURL(component *applicationapiv1alpha1.Component) string {
	if component.Spec.Source.GitSource == nil {
		return ""
	}
	return normalizeGitRepoURL(component.Spec.Source.GitSource.URL)
}

// parseGitRepoPath extracts the "org/repo" path from a full git URL.
// For example, "https://github.com/org/repo.git" returns "org/repo".
func parseGitRepoPath(gitURL string) string {
	u, err := url.Parse(normalizeGitRepoURL(gitURL))
	if err != nil {
		return ""
	}
	return strings.Trim(u.Path, "/")
}

// getGitRepoHost extracts the hostname from a git URL.
// For example, "https://github.com/org/repo" returns "github.com".
func getGitRepoHost(gitURL string) string {
	u, err := url.Parse(gitURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// buildAPIEndpoint returns the API endpoint URL for the given git provider and host.
// This replicates the logic from build-service's git.BuildAPIEndpoint.
func buildAPIEndpoint(provider, host string) string {
	switch provider {
	case "github":
		return fmt.Sprintf("https://api.%s/", host)
	case "gitlab":
		return fmt.Sprintf("https://%s/api/v4/", host)
	case "bitbucket":
		return fmt.Sprintf("https://api.%s/2.0/", host)
	default:
		return ""
	}
}

// normalizeGitRepoURL normalizes the git repository URL to not contain the trailing slash or .git suffix
func normalizeGitRepoURL(gitURL string) string {
	return strings.TrimSuffix(strings.TrimSuffix(gitURL, "/"), ".git")
}

// extractCredentialsFromRepositorySecret extracts the credentials from the repository secret based on its type
func extractCredentialsFromRepositorySecret(gp *pacv1alpha1.GitProvider, secret *corev1.Secret, key string) (string, string, error) {
	if secret.Type == corev1.SecretTypeBasicAuth {
		password, ok := secret.Data[corev1.BasicAuthPasswordKey]
		if !ok || len(password) == 0 {
			return "", "", fmt.Errorf("key %q not found in secret %s", corev1.BasicAuthPasswordKey, secret.Name)
		}
		return string(secret.Data[corev1.BasicAuthUsernameKey]), string(password), nil
	}
	token, ok := secret.Data[key]
	if !ok || len(token) == 0 {
		return "", "", fmt.Errorf("key %q not found in secret %s", key, secret.Name)
	}
	username := gp.User // GitProvider.user on the Repository CR
	return username, string(token), nil
}

// lookupSCMCredentialsViaRepository searches for Repository CRs in the tenant namespace whose Spec.URL matches the component URL.
// It then reads the secret reference from the Repository CR and extracts credentials from it.
func lookupSCMCredentialsViaRepository(ctx context.Context, c client.Client, namespace, repoURL string) (username, token string, err error) {
	repos := pacv1alpha1.RepositoryList{}
	if err := c.List(ctx, &repos, client.InNamespace(namespace)); err != nil {
		return "", "", fmt.Errorf("failed to list Repository CRs in %s: %w", namespace, err)
	}

	normalizedTarget := normalizeGitRepoURL(repoURL)
	var matches []pacv1alpha1.Repository
	for i := range repos.Items {
		if normalizeGitRepoURL(repos.Items[i].Spec.URL) == normalizedTarget {
			matches = append(matches, repos.Items[i])
		}
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("no Repository CR matching URL %q in namespace %s", repoURL, namespace)
	}
	if len(matches) > 1 {
		names := make([]string, len(matches))
		for i, repo := range matches {
			names[i] = repo.Name
		}
		sort.Strings(names)
		return "", "", fmt.Errorf("multiple Repository CRs match URL %q in namespace %s: %s", repoURL, namespace, strings.Join(names, ", "))
	}
	matched := &matches[0]
	if matched.Spec.GitProvider == nil || matched.Spec.GitProvider.Secret == nil {
		return "", "", fmt.Errorf("repository %s has no git_provider.secret configured", matched.Name)
	}

	ref := matched.Spec.GitProvider.Secret
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
		return "", "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, ref.Name, err)
	}

	username, token, err = extractCredentialsFromRepositorySecret(matched.Spec.GitProvider, secret, ref.Key)
	if err != nil {
		return "", "", err
	}
	return username, token, nil
}

// parseImageHost extracts the registry host from a container image reference.
// For "quay.io/org/repo:tag" it returns "quay.io".
// For "quay.io/org/repo@sha256:abc" it returns "quay.io".
func parseImageHost(image string) string {
	// Strip tag or digest
	ref := image
	if idx := strings.Index(ref, "@"); idx >= 0 {
		ref = ref[:idx]
	}
	if idx := strings.Index(ref, ":"); idx >= 0 {
		// Only strip if it looks like a tag (no slashes after the colon position)
		afterColon := ref[idx+1:]
		if !strings.Contains(afterColon, "/") {
			ref = ref[:idx]
		}
	}

	parts := strings.Split(ref, "/")
	if len(parts) >= 2 {
		// First part is the host if it contains a dot or colon (port)
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
			return parts[0]
		}
	}
	// Docker Hub shorthand (e.g., "library/nginx")
	return "docker.io"
}

// matchCredentialForImage finds the best matching credential for an image reference.
// It first tries an exact repo path match, then progressively shorter partial matches.
// When several credentials match at the same specificity, the first entry in creds wins.
func matchCredentialForImage(ctx context.Context, outputImage string, creds []repositoryCredentials) (string, string, error) {
	log := ctrllog.FromContext(ctx)

	// Normalize image to just host/path (no tag or digest)
	repoPath := outputImage
	if idx := strings.Index(repoPath, "@"); idx >= 0 {
		repoPath = repoPath[:idx]
	} else if idx := strings.LastIndex(repoPath, ":"); idx >= 0 {
		// Only strip if this looks like a tag, not a port
		afterColon := repoPath[idx+1:]
		if !strings.Contains(afterColon, "/") {
			repoPath = repoPath[:idx]
		}
	}
	repoPath = strings.TrimSuffix(repoPath, "/")

	// Try exact match first
	for _, cred := range creds {
		credRepo := strings.TrimSuffix(cred.repoName, "/")
		if repoPath == credRepo {
			log.Info("found full match of repository in auth",
				"repo", repoPath,
				"secretName", cred.secretName)
			return cred.username, cred.password, nil
		}
	}

	// Try progressively shorter partial matches
	repoParts := strings.Split(repoPath, "/")
	for len(repoParts) > 1 {
		repoParts = repoParts[:len(repoParts)-1]
		partialRepo := strings.Join(repoParts, "/")

		for _, cred := range creds {
			credRepo := strings.TrimSuffix(cred.repoName, "/")
			if partialRepo == credRepo && cred.username != "" && cred.password != "" {
				log.Info("partial match found of repository in auth",
					"repo", partialRepo,
					"secretName", cred.secretName)
				return cred.username, cred.password, nil
			}
		}
	}

	return "", "", fmt.Errorf("no credentials found for repository %s", repoPath)
}

// isPaCGitHubAppConfigured checks if the PaC secret has GitHub App credentials configured.
func isPaCGitHubAppConfigured(secretData map[string][]byte) bool {
	return len(secretData[tektonconsts.PipelinesAsCodeGithubAppIdKey]) > 0 &&
		len(secretData[tektonconsts.PipelinesAsCodeGithubPrivateKey]) > 0
}

// branchSlice returns a single-element slice with the branch if non-empty, or nil.
func branchSlice(branch string) []string {
	if branch == "" {
		return nil
	}
	return []string{branch}
}
