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

package tekton

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// scmCredentialsSecretLabel is the label that marks a Secret as containing SCM credentials.
	// Matches build-service's appstudio.redhat.com/credentials label.
	scmCredentialsSecretLabel = "appstudio.redhat.com/credentials"

	// scmSecretHostnameLabel is the label whose value is the SCM hostname this secret applies to.
	scmSecretHostnameLabel = "appstudio.redhat.com/scm.host"

	// scmSecretRepositoryAnnotation is the annotation whose value is a comma-separated list of
	// repository paths (org/repo or org/*) this secret applies to.
	scmSecretRepositoryAnnotation = "appstudio.redhat.com/scm.repository"

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

// GetNudgeTargetsGithubApp returns NudgeTargets for components that use GitHub App authentication.
//
// It reads the global PaC secret from the build-service namespace, verifies that GitHub App
// credentials are configured, and for each GitHub-hosted component it obtains an installation
// token via the GitHub App API.
//
// TODO(STONEINTG-1671): Implement GitHub App JWT + installation token flow.
// This requires creating a JWT from the App ID + private key, calling
// GET /app/installations to find the installation for each repo, then
// POST /app/installations/{id}/access_tokens to get scoped tokens.
// For now this returns nil and logs a message. Basic auth covers the majority of use cases.
func GetNudgeTargetsGithubApp(ctx context.Context, c client.Client, targetComponents []applicationapiv1alpha1.Component, imageRepoHost, imageRepoUser, imageRepoPwd string) []NudgeTarget {
	log := ctrllog.FromContext(ctx)

	// Read the global PaC secret to check if GitHub App is configured
	pacSecret := corev1.Secret{}
	globalPaCSecretKey := types.NamespacedName{
		Namespace: tektonconsts.BuildServiceNamespaceName,
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

	// TODO(STONEINTG-1671): Implement GitHub App installation token flow.
	// The full implementation needs to:
	// 1. Parse github-application-id and github-private-key from the PaC secret
	// 2. Create a JWT signed with the private key
	// 3. For each github-hosted component, call the GitHub API to get an installation token
	// 4. Build NudgeTargets with those tokens
	//
	// For now, return nil. The basic auth path in GetNudgeTargetsBasicAuth handles
	// most real-world cases since PaC provisions per-repo secrets in the user namespace.
	log.Info("GitHub App auth path not yet implemented for nudge credential resolution, falling back to basic auth")
	return nil
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
		username, token, err := lookupSCMCredentials(ctx, c, component.Namespace, repoHost, repoPath)
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

	// Gather all linked secret names from both .secrets and .imagePullSecrets
	linkedSecretNames := map[string]bool{}
	for _, ref := range sa.Secrets {
		linkedSecretNames[ref.Name] = true
	}
	for _, ref := range sa.ImagePullSecrets {
		linkedSecretNames[ref.Name] = true
	}

	if len(linkedSecretNames) == 0 {
		return "", "", "", fmt.Errorf("no secrets linked to service account %s in namespace %s", saName, namespace)
	}

	// List all secrets in the namespace and filter to dockerconfigjson type
	allSecrets := &corev1.SecretList{}
	if err := c.List(ctx, allSecrets, client.InNamespace(namespace)); err != nil {
		return "", "", "", fmt.Errorf("failed to list secrets in %s namespace: %w", namespace, err)
	}

	// Parse credentials from linked docker config secrets
	var allCreds []repositoryCredentials
	for _, secret := range allSecrets.Items {
		if secret.Type != corev1.SecretTypeDockerConfigJson {
			continue
		}
		if !linkedSecretNames[secret.Name] {
			continue
		}

		dockerConfig := &dockerConfigJSON{}
		configData, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			continue
		}
		if err := json.Unmarshal(configData, dockerConfig); err != nil {
			log.Error(err, "unable to parse docker json config",
				"secretName", secret.Name)
			continue
		}

		for repoName, repoAuth := range dockerConfig.Auths {
			if repoAuth.Username != "" && repoAuth.Password != "" {
				allCreds = append(allCreds, repositoryCredentials{
					secretName: secret.Name,
					repoName:   repoName,
					username:   repoAuth.Username,
					password:   repoAuth.Password,
				})
			} else if repoAuth.Auth != "" {
				decoded, err := base64.StdEncoding.DecodeString(repoAuth.Auth)
				if err != nil {
					log.Error(err, "unable to decode docker config json auth",
						"repository", repoName,
						"secretName", secret.Name)
					continue
				}
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					allCreds = append(allCreds, repositoryCredentials{
						secretName: secret.Name,
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

// ---------- Helper functions ----------

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
	return strings.TrimSuffix(strings.TrimSuffix(component.Spec.Source.GitSource.URL, "/"), ".git")
}

// parseGitRepoPath extracts the "org/repo" path from a full git URL.
// For example, "https://github.com/org/repo.git" returns "org/repo".
func parseGitRepoPath(gitURL string) string {
	u, err := url.Parse(strings.TrimSuffix(strings.TrimSuffix(gitURL, "/"), ".git"))
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

// lookupSCMCredentials searches for basic auth secrets in the component's namespace
// that match the given repository host and path.
//
// It replicates build-service's GitCredentialProvider.LookupSecret logic:
//   - Lists secrets with label appstudio.redhat.com/credentials=scm and
//     appstudio.redhat.com/scm.host=<host>
//   - Filters for BasicAuth type secrets
//   - Selects the best match based on the scm.repository annotation
func lookupSCMCredentials(ctx context.Context, c client.Client, namespace, repoHost, repoPath string) (username, password string, err error) {
	log := ctrllog.FromContext(ctx)

	secretList := &corev1.SecretList{}
	if err := c.List(ctx, secretList,
		client.InNamespace(namespace),
		client.MatchingLabels{
			scmCredentialsSecretLabel: "scm",
			scmSecretHostnameLabel:    repoHost,
		},
	); err != nil {
		return "", "", fmt.Errorf("failed to list SCM secrets in %s namespace: %w", namespace, err)
	}

	log.Info("found SCM secrets matching host",
		"count", len(secretList.Items),
		"host", repoHost)

	// Filter to BasicAuth secrets with data
	var candidates []corev1.Secret
	for _, s := range secretList.Items {
		if s.Type == corev1.SecretTypeBasicAuth && len(s.Data) > 0 {
			candidates = append(candidates, s)
		}
	}

	if len(candidates) == 0 {
		return "", "", fmt.Errorf("no SCM basic auth secrets found for host %q in namespace %s", repoHost, namespace)
	}

	best := bestMatchingSCMSecret(ctx, repoPath, candidates)
	if best == nil {
		return "", "", fmt.Errorf("no matching SCM secret found for repo %q in namespace %s", repoPath, namespace)
	}

	return string(best.Data[corev1.BasicAuthUsernameKey]),
		string(best.Data[corev1.BasicAuthPasswordKey]),
		nil
}

// bestMatchingSCMSecret finds the best matching secret for a given repository path.
// Priority:
// 1. Direct match of scm.repository annotation to the component repository path
// 2. Wildcard match (org/*) with the longest intersection
// 3. Host-only match (no repository annotation)
func bestMatchingSCMSecret(ctx context.Context, componentRepo string, secrets []corev1.Secret) *corev1.Secret {
	log := ctrllog.FromContext(ctx)

	var hostOnlySecrets []corev1.Secret
	// Map from secret index to its best path intersection count
	potentialMatches := make(map[int]int, len(secrets))

	for idx, secret := range secrets {
		repoAnnotation, exists := secret.Annotations[scmSecretRepositoryAnnotation]
		if !exists || repoAnnotation == "" {
			hostOnlySecrets = append(hostOnlySecrets, secret)
			continue
		}

		secretRepos := strings.Split(repoAnnotation, ",")
		for i := range secretRepos {
			secretRepos[i] = strings.TrimPrefix(strings.TrimSpace(secretRepos[i]), "/")
		}

		// Check for direct match
		for _, r := range secretRepos {
			if r == componentRepo {
				log.Info("found direct SCM secret match",
					"secret", secret.Name,
					"repository", componentRepo)
				return &secrets[idx]
			}
		}

		// Check wildcard matches (e.g., "org/*")
		componentParts := strings.Split(componentRepo, "/")
		for _, r := range secretRepos {
			if !strings.HasSuffix(r, "*") {
				continue
			}
			wildcardParts := strings.Split(strings.TrimSuffix(r, "*"), "/")
			// Count matching path segments
			matched := 0
			for i, part := range wildcardParts {
				if i < len(componentParts) && part == componentParts[i] {
					matched++
				}
			}
			if matched > 0 && potentialMatches[idx] < matched {
				potentialMatches[idx] = matched
			}
		}
	}

	if len(potentialMatches) == 0 {
		if len(hostOnlySecrets) == 0 {
			return nil
		}
		log.Info("using host-only SCM secret",
			"secret", hostOnlySecrets[0].Name)
		return &hostOnlySecrets[0]
	}

	// Find the best wildcard match
	var bestIdx, bestCount int
	for idx, count := range potentialMatches {
		if count > bestCount {
			bestCount = count
			bestIdx = idx
		}
	}

	log.Info("using best wildcard-matched SCM secret",
		"secret", secrets[bestIdx].Name)
	return &secrets[bestIdx]
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
