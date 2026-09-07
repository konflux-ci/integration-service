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
	"errors"

	ghapi "github.com/google/go-github/v45/github"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Nudge credentials", func() {

	const (
		testNamespace = "test-ns"
	)

	newCredentialScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(applicationapiv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(pacv1alpha1.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

	repositoryCredentialObjects := func(namespace, repoName, repoURL, secretName, username, password string) []client.Object {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
			Type:       corev1.SecretTypeBasicAuth,
			Data: map[string][]byte{
				corev1.BasicAuthUsernameKey: []byte(username),
				corev1.BasicAuthPasswordKey: []byte(password),
			},
		}
		repo := &pacv1alpha1.Repository{
			ObjectMeta: metav1.ObjectMeta{Name: repoName, Namespace: namespace},
			Spec: pacv1alpha1.RepositorySpec{
				URL: repoURL,
				GitProvider: &pacv1alpha1.GitProvider{
					Secret: &pacv1alpha1.Secret{Name: secretName},
				},
			},
		}
		return []client.Object{secret, repo}
	}

	// ---------- getGitProvider ----------

	Describe("getGitProvider", func() {

		DescribeTable("detects provider from URL hostname",
			func(sourceURL, expectedProvider string) {
				comp := applicationapiv1alpha1.Component{
					ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: testNamespace},
					Spec: applicationapiv1alpha1.ComponentSpec{
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{URL: sourceURL},
							},
						},
					},
				}
				provider, err := getGitProvider(comp)
				Expect(err).NotTo(HaveOccurred())
				Expect(provider).To(Equal(expectedProvider))
			},
			Entry("github.com URL", "https://github.com/org/repo", "github"),
			Entry("gitlab.com URL", "https://gitlab.com/org/repo", "gitlab"),
			Entry("bitbucket.org URL", "https://bitbucket.org/org/repo", "bitbucket"),
			Entry("self-hosted GitHub", "https://github.example.com/org/repo", "github"),
			Entry("self-hosted GitLab", "https://gitlab.internal.company.com/org/repo", "gitlab"),
		)

		DescribeTable("uses annotation override",
			func(annotation, expectedProvider string) {
				comp := applicationapiv1alpha1.Component{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "c",
						Namespace:   testNamespace,
						Annotations: map[string]string{gitProviderAnnotationName: annotation},
					},
					Spec: applicationapiv1alpha1.ComponentSpec{
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{URL: "https://custom-git.example.com/org/repo"},
							},
						},
					},
				}
				provider, err := getGitProvider(comp)
				Expect(err).NotTo(HaveOccurred())
				Expect(provider).To(Equal(expectedProvider))
			},
			Entry("github annotation", "github", "github"),
			Entry("gitlab annotation", "gitlab", "gitlab"),
			Entry("bitbucket annotation", "bitbucket", "bitbucket"),
		)

		It("returns error for unsupported annotation value", func() {
			comp := applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "c",
					Namespace:   testNamespace,
					Annotations: map[string]string{gitProviderAnnotationName: "svn"},
				},
				Spec: applicationapiv1alpha1.ComponentSpec{
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{URL: "https://svn.example.com/org/repo"},
						},
					},
				},
			}
			_, err := getGitProvider(comp)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported git-provider annotation"))
		})

		It("returns error when git source is nil", func() {
			comp := applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: testNamespace},
				Spec:       applicationapiv1alpha1.ComponentSpec{},
			}
			_, err := getGitProvider(comp)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("git source is not set"))
		})

		It("returns error for unrecognized hostname without annotation", func() {
			comp := applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: testNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{URL: "https://custom-scm.example.com/org/repo"},
						},
					},
				},
			}
			_, err := getGitProvider(comp)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot determine git provider"))
		})
	})

	// ---------- parseGitRepoPath ----------

	Describe("parseGitRepoPath", func() {
		DescribeTable("extracts org/repo from URL",
			func(gitURL, expectedPath string) {
				Expect(parseGitRepoPath(gitURL)).To(Equal(expectedPath))
			},
			Entry("HTTPS URL", "https://github.com/org/repo", "org/repo"),
			Entry("HTTPS URL with .git", "https://github.com/org/repo.git", "org/repo"),
			Entry("HTTPS URL with trailing slash", "https://github.com/org/repo/", "org/repo"),
			Entry("HTTPS URL with .git and trailing slash", "https://github.com/org/repo.git/", "org/repo"),
			Entry("GitLab nested group", "https://gitlab.com/group/subgroup/repo", "group/subgroup/repo"),
			Entry("empty URL returns empty", "", ""),
		)
	})

	// ---------- getGitRepoHost ----------

	Describe("getGitRepoHost", func() {
		DescribeTable("extracts hostname",
			func(gitURL, expectedHost string) {
				Expect(getGitRepoHost(gitURL)).To(Equal(expectedHost))
			},
			Entry("github.com", "https://github.com/org/repo", "github.com"),
			Entry("gitlab.com", "https://gitlab.com/org/repo", "gitlab.com"),
			Entry("bitbucket.org", "https://bitbucket.org/org/repo", "bitbucket.org"),
			Entry("custom host", "https://git.internal.example.com/org/repo", "git.internal.example.com"),
			Entry("malformed URL returns empty", "://bad-url", ""),
		)
	})

	// ---------- buildAPIEndpoint ----------

	Describe("buildAPIEndpoint", func() {
		DescribeTable("returns correct API URL",
			func(provider, host, expected string) {
				Expect(buildAPIEndpoint(provider, host)).To(Equal(expected))
			},
			Entry("github", "github", "github.com", "https://api.github.com/"),
			Entry("gitlab", "gitlab", "gitlab.com", "https://gitlab.com/api/v4/"),
			Entry("bitbucket", "bitbucket", "bitbucket.org", "https://api.bitbucket.org/2.0/"),
			Entry("self-hosted github", "github", "github.example.com", "https://api.github.example.com/"),
			Entry("self-hosted gitlab", "gitlab", "gitlab.internal.co", "https://gitlab.internal.co/api/v4/"),
			Entry("unknown provider", "unknown", "example.com", ""),
		)
	})

	// ---------- parseImageHost ----------

	Describe("parseImageHost", func() {
		DescribeTable("extracts registry host from image reference",
			func(image, expectedHost string) {
				Expect(parseImageHost(image)).To(Equal(expectedHost))
			},
			Entry("quay.io with tag", "quay.io/org/repo:latest", "quay.io"),
			Entry("quay.io with digest", "quay.io/org/repo@sha256:abc123", "quay.io"),
			Entry("quay.io no tag", "quay.io/org/repo", "quay.io"),
			Entry("registry with port", "registry.example.com:5000/org/repo:v1", "registry.example.com:5000"),
			Entry("Docker Hub shorthand", "library/nginx", "docker.io"),
			Entry("gcr.io", "gcr.io/project/image:v1", "gcr.io"),
		)
	})

	// ---------- branchSlice ----------

	Describe("branchSlice", func() {
		It("returns nil for empty branch", func() {
			Expect(branchSlice("")).To(BeNil())
		})

		It("returns single-element slice for non-empty branch", func() {
			Expect(branchSlice("main")).To(Equal([]string{"main"}))
		})
	})

	// ---------- isPaCGitHubAppConfigured ----------

	Describe("isPaCGitHubAppConfigured", func() {
		It("returns true when both keys are present and non-empty", func() {
			data := map[string][]byte{
				tektonconsts.PipelinesAsCodeGithubAppIdKey:   []byte("12345"),
				tektonconsts.PipelinesAsCodeGithubPrivateKey: []byte("-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----"),
			}
			Expect(isPaCGitHubAppConfigured(data)).To(BeTrue())
		})

		It("returns false when app ID is missing", func() {
			data := map[string][]byte{
				tektonconsts.PipelinesAsCodeGithubPrivateKey: []byte("key-data"),
			}
			Expect(isPaCGitHubAppConfigured(data)).To(BeFalse())
		})

		It("returns false when private key is missing", func() {
			data := map[string][]byte{
				tektonconsts.PipelinesAsCodeGithubAppIdKey: []byte("12345"),
			}
			Expect(isPaCGitHubAppConfigured(data)).To(BeFalse())
		})

		It("returns false when both keys are empty", func() {
			data := map[string][]byte{
				tektonconsts.PipelinesAsCodeGithubAppIdKey:   {},
				tektonconsts.PipelinesAsCodeGithubPrivateKey: {},
			}
			Expect(isPaCGitHubAppConfigured(data)).To(BeFalse())
		})

		It("returns false for nil map", func() {
			Expect(isPaCGitHubAppConfigured(nil)).To(BeFalse())
		})
	})

	// ---------- matchCredentialForImage ----------

	Describe("matchCredentialForImage", func() {
		ctx := context.Background()

		It("returns exact match", func() {
			creds := []repositoryCredentials{
				{secretName: "sec1", repoName: "quay.io/org/repo", username: "u1", password: "p1"},
				{secretName: "sec2", repoName: "quay.io/other/repo", username: "u2", password: "p2"},
			}
			u, p, err := matchCredentialForImage(ctx, "quay.io/org/repo:latest", creds)
			Expect(err).NotTo(HaveOccurred())
			Expect(u).To(Equal("u1"))
			Expect(p).To(Equal("p1"))
		})

		It("returns partial match when no exact match", func() {
			creds := []repositoryCredentials{
				{secretName: "sec1", repoName: "quay.io/org", username: "u1", password: "p1"},
			}
			u, p, err := matchCredentialForImage(ctx, "quay.io/org/repo:v1", creds)
			Expect(err).NotTo(HaveOccurred())
			Expect(u).To(Equal("u1"))
			Expect(p).To(Equal("p1"))
		})

		It("returns host-level partial match", func() {
			creds := []repositoryCredentials{
				{secretName: "sec1", repoName: "quay.io", username: "u1", password: "p1"},
			}
			u, p, err := matchCredentialForImage(ctx, "quay.io/org/repo:v1", creds)
			Expect(err).NotTo(HaveOccurred())
			Expect(u).To(Equal("u1"))
			Expect(p).To(Equal("p1"))
		})

		It("returns error when no matching credential", func() {
			creds := []repositoryCredentials{
				{secretName: "sec1", repoName: "gcr.io/project", username: "u1", password: "p1"},
			}
			_, _, err := matchCredentialForImage(ctx, "quay.io/org/repo:v1", creds)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no credentials found"))
		})

		It("strips digest from image reference", func() {
			creds := []repositoryCredentials{
				{secretName: "sec1", repoName: "quay.io/org/repo", username: "u1", password: "p1"},
			}
			u, p, err := matchCredentialForImage(ctx, "quay.io/org/repo@sha256:abc123def456", creds)
			Expect(err).NotTo(HaveOccurred())
			Expect(u).To(Equal("u1"))
			Expect(p).To(Equal("p1"))
		})

		It("returns the first credential when several match at the same specificity", func() {
			creds := []repositoryCredentials{
				{secretName: "zebra-secret", repoName: "quay.io/org/repo", username: "first", password: "first-pass"},
				{secretName: "alpha-secret", repoName: "quay.io/org/repo", username: "second", password: "second-pass"},
			}
			u, p, err := matchCredentialForImage(ctx, "quay.io/org/repo:latest", creds)
			Expect(err).NotTo(HaveOccurred())
			Expect(u).To(Equal("first"))
			Expect(p).To(Equal("first-pass"))
		})
	})

	// ---------- GetNudgeTargetsGithubApp ----------

	Describe("GetNudgeTargetsGithubApp", func() {
		var (
			ctx                 context.Context
			scheme              *runtime.Scheme
			savedNewClientFn    func(context.Context, string, []byte) (*ghapi.Client, string, error)
			savedInstallationFn func(context.Context, *ghapi.Client, string) (*applicationInstallation, error)
			savedBotIDFn        func(context.Context, string) (int64, error)
		)

		defaultBranch := "main"
		repoID := int64(99999)
		fakeRepo := &ghapi.Repository{
			DefaultBranch: &defaultBranch,
			ID:            &repoID,
		}
		fakeInstallation := func(_ context.Context, _ *ghapi.Client, _ string) (*applicationInstallation, error) {
			return &applicationInstallation{
				Token:        "ghs_test-token",
				ID:           12345,
				Repositories: []*ghapi.Repository{fakeRepo},
			}, nil
		}
		fakeBotID := func(_ context.Context, _ string) (int64, error) {
			return int64(67890), nil
		}

		BeforeEach(func() {
			ctx = context.Background()
			scheme = newCredentialScheme()
			savedNewClientFn = newGitHubAppClientFn
			savedInstallationFn = gitHubAppInstallationForRepo
			savedBotIDFn = getGitHubBotUserIDFn
			newGitHubAppClientFn = func(_ context.Context, _ string, _ []byte) (*ghapi.Client, string, error) {
				return nil, "my-app", nil
			}
			gitHubAppInstallationForRepo = fakeInstallation
			getGitHubBotUserIDFn = fakeBotID
		})

		AfterEach(func() {
			newGitHubAppClientFn = savedNewClientFn
			gitHubAppInstallationForRepo = savedInstallationFn
			getGitHubBotUserIDFn = savedBotIDFn
		})

		pacSecretWithApp := func(scheme *runtime.Scheme) (*corev1.Secret, *fake.ClientBuilder) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tektonconsts.PipelinesAsCodeGitHubAppSecretName,
					Namespace: "integration-service",
				},
				Data: map[string][]byte{
					tektonconsts.PipelinesAsCodeGithubAppIdKey:   []byte("12345"),
					tektonconsts.PipelinesAsCodeGithubPrivateKey: []byte("fake-key"),
				},
			}
			return secret, fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret)
		}

		githubComponent := func(name, repoURL, revision string) applicationapiv1alpha1.Component {
			return applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{
								URL:      repoURL,
								Revision: revision,
							},
						},
					},
				},
			}
		}

		It("returns nil when PaC secret does not exist", func() {
			newGitHubAppClientFn = savedNewClientFn // don't mock — should never reach
			gitHubAppInstallationForRepo = savedInstallationFn
			getGitHubBotUserIDFn = savedBotIDFn
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("comp", "https://github.com/org/repo", ""),
			}, "quay.io", "user", "pass")
			Expect(targets).To(BeNil())
		})

		It("returns nil when PaC secret exists but GitHub App is not configured", func() {
			newGitHubAppClientFn = savedNewClientFn
			gitHubAppInstallationForRepo = savedInstallationFn
			getGitHubBotUserIDFn = savedBotIDFn
			unconfiguredSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tektonconsts.PipelinesAsCodeGitHubAppSecretName,
					Namespace: "integration-service",
				},
				Data: map[string][]byte{"some-other-key": []byte("value")},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(unconfiguredSecret).Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("comp", "https://github.com/org/repo", ""),
			}, "quay.io", "user", "pass")
			Expect(targets).To(BeNil())
		})

		It("returns targets for github component with valid installation", func() {
			_, builder := pacSecretWithApp(scheme)
			c := builder.Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("comp", "https://github.com/org/repo", "main"),
			}, "quay.io", "img-user", "img-pass")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].ComponentName).To(Equal("comp"))
			Expect(targets[0].GitProvider).To(Equal("github"))
			Expect(targets[0].Token).To(Equal("ghs_test-token"))
			Expect(targets[0].Username).To(Equal("my-app[bot]"))
			Expect(targets[0].Endpoint).To(Equal("https://api.github.com/"))
			Expect(targets[0].ImageRepositoryHost).To(Equal("quay.io"))
			Expect(targets[0].ImageRepositoryUsername).To(Equal("img-user"))
			Expect(targets[0].ImageRepositoryPassword).To(Equal("img-pass"))
			Expect(targets[0].Repositories).To(HaveLen(1))
			Expect(targets[0].Repositories[0].Repository).To(Equal("org/repo"))
		})

		It("builds correct GitAuthor format", func() {
			_, builder := pacSecretWithApp(scheme)
			c := builder.Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("comp", "https://github.com/org/repo", ""),
			}, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].GitAuthor).To(Equal("my-app <67890+my-app[bot]@users.noreply.github.com>"))
		})

		It("skips non-github components", func() {
			_, builder := pacSecretWithApp(scheme)
			c := builder.Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("gh-comp", "https://github.com/org/repo", ""),
				githubComponent("gl-comp", "https://gitlab.com/org/repo", ""),
			}, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].ComponentName).To(Equal("gh-comp"))
		})

		It("skips github.com Enterprise Server (non-github.com) components", func() {
			_, builder := pacSecretWithApp(scheme)
			c := builder.Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("ghe-comp", "https://github.example.com/org/repo", ""),
				githubComponent("gh-comp", "https://github.com/org/repo", ""),
			}, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].ComponentName).To(Equal("gh-comp"))
		})

		It("continues when installation lookup fails for one component", func() {
			callCount := 0
			gitHubAppInstallationForRepo = func(_ context.Context, _ *ghapi.Client, _ string) (*applicationInstallation, error) {
				callCount++
				if callCount == 1 {
					return nil, errors.New("installation not found")
				}
				return &applicationInstallation{
					Token:        "ghs_test-token",
					ID:           12345,
					Repositories: []*ghapi.Repository{fakeRepo},
				}, nil
			}
			_, builder := pacSecretWithApp(scheme)
			c := builder.Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("comp-a", "https://github.com/org/repo-a", ""),
				githubComponent("comp-b", "https://github.com/org/repo-b", ""),
			}, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].ComponentName).To(Equal("comp-b"))
		})

		It("returns empty slice when installation fails for all components", func() {
			gitHubAppInstallationForRepo = func(_ context.Context, _ *ghapi.Client, _ string) (*applicationInstallation, error) {
				return nil, errors.New("no installation")
			}
			_, builder := pacSecretWithApp(scheme)
			c := builder.Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("comp", "https://github.com/org/repo", ""),
			}, "quay.io", "u", "p")
			Expect(targets).To(BeEmpty())
		})

		It("uses component revision as branch when set", func() {
			_, builder := pacSecretWithApp(scheme)
			c := builder.Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("comp", "https://github.com/org/repo", "release-1.0"),
			}, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].Repositories[0].BaseBranches).To(Equal([]string{"release-1.0"}))
		})

		It("falls back to installation DefaultBranch when revision is empty", func() {
			_, builder := pacSecretWithApp(scheme)
			c := builder.Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("comp", "https://github.com/org/repo", ""),
			}, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].Repositories[0].BaseBranches).To(Equal([]string{"main"}))
		})

		It("caches slug and bot ID across multiple components", func() {
			installationCalls := 0
			botIDCalls := 0
			gitHubAppInstallationForRepo = func(_ context.Context, _ *ghapi.Client, _ string) (*applicationInstallation, error) {
				installationCalls++
				return &applicationInstallation{
					Token:        "ghs_test-token",
					ID:           12345,
					Repositories: []*ghapi.Repository{fakeRepo},
				}, nil
			}
			getGitHubBotUserIDFn = func(_ context.Context, _ string) (int64, error) {
				botIDCalls++
				return int64(67890), nil
			}
			_, builder := pacSecretWithApp(scheme)
			c := builder.Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("comp-a", "https://github.com/org/repo-a", ""),
				githubComponent("comp-b", "https://github.com/org/repo-b", ""),
			}, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(2))
			Expect(installationCalls).To(Equal(2))
			Expect(botIDCalls).To(Equal(1), "bot user ID should be looked up only once")
		})

		It("continues after bot user ID lookup failure, using ID 0 in GitAuthor", func() {
			getGitHubBotUserIDFn = func(_ context.Context, _ string) (int64, error) {
				return 0, errors.New("rate limited")
			}
			_, builder := pacSecretWithApp(scheme)
			c := builder.Build()
			targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{
				githubComponent("comp", "https://github.com/org/repo", ""),
			}, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].GitAuthor).To(Equal("my-app <0+my-app[bot]@users.noreply.github.com>"))
		})
	})

	// ---------- GetNudgeTargetsBasicAuth ----------

	Describe("GetNudgeTargetsBasicAuth", func() {
		var (
			ctx    context.Context
			scheme *runtime.Scheme
		)

		BeforeEach(func() {
			ctx = context.Background()
			scheme = newCredentialScheme()
		})

		It("creates a target for a component with matching SCM secret", func() {
			objects := repositoryCredentialObjects(testNamespace, "github-repo", "https://github.com/org/repo", "scm-github-secret", "bot-user", "ghp_secret-token")
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			components := []applicationapiv1alpha1.Component{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
					Spec: applicationapiv1alpha1.ComponentSpec{
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{
									URL:      "https://github.com/org/repo",
									Revision: "main",
								},
							},
						},
					},
				},
			}

			targets := GetNudgeTargetsBasicAuth(ctx, c, components, "quay.io", "img-user", "img-pass")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].ComponentName).To(Equal("comp"))
			Expect(targets[0].GitProvider).To(Equal("github"))
			Expect(targets[0].Username).To(Equal("bot-user"))
			Expect(targets[0].Token).To(Equal("ghp_secret-token"))
			Expect(targets[0].Endpoint).To(Equal("https://api.github.com/"))
			Expect(targets[0].ImageRepositoryHost).To(Equal("quay.io"))
			Expect(targets[0].ImageRepositoryUsername).To(Equal("img-user"))
			Expect(targets[0].ImageRepositoryPassword).To(Equal("img-pass"))
			Expect(targets[0].Repositories).To(HaveLen(1))
			Expect(targets[0].Repositories[0].Repository).To(Equal("org/repo"))
			Expect(targets[0].Repositories[0].BaseBranches).To(Equal([]string{"main"}))
		})

		It("uses default username when SCM secret has empty username", func() {
			objects := repositoryCredentialObjects(testNamespace, "github-repo", "https://github.com/org/repo", "scm-secret", "", "token")
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			components := []applicationapiv1alpha1.Component{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
					Spec: applicationapiv1alpha1.ComponentSpec{
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{URL: "https://github.com/org/repo"},
							},
						},
					},
				},
			}

			targets := GetNudgeTargetsBasicAuth(ctx, c, components, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].Username).To(Equal(tektonconsts.DefaultRenovateUser))
		})

		It("skips components without matching SCM secret", func() {
			// No secrets created in the namespace
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			components := []applicationapiv1alpha1.Component{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
					Spec: applicationapiv1alpha1.ComponentSpec{
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{URL: "https://github.com/org/repo"},
							},
						},
					},
				},
			}

			targets := GetNudgeTargetsBasicAuth(ctx, c, components, "quay.io", "u", "p")
			Expect(targets).To(BeEmpty())
		})

		It("handles multiple components where only some have credentials", func() {
			objects := repositoryCredentialObjects(testNamespace, "github-repo-a", "https://github.com/org/repo-a", "scm-secret", "user", "token")
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			components := []applicationapiv1alpha1.Component{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "comp-a", Namespace: testNamespace},
					Spec: applicationapiv1alpha1.ComponentSpec{
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{URL: "https://github.com/org/repo-a"},
							},
						},
					},
				},
				{
					// This component is on gitlab, no secret matches
					ObjectMeta: metav1.ObjectMeta{Name: "comp-b", Namespace: testNamespace},
					Spec: applicationapiv1alpha1.ComponentSpec{
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{URL: "https://gitlab.com/org/repo-b"},
							},
						},
					},
				},
			}

			targets := GetNudgeTargetsBasicAuth(ctx, c, components, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].ComponentName).To(Equal("comp-a"))
		})

		It("skips component with no git source", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			components := []applicationapiv1alpha1.Component{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
					Spec:       applicationapiv1alpha1.ComponentSpec{},
				},
			}

			targets := GetNudgeTargetsBasicAuth(ctx, c, components, "quay.io", "u", "p")
			Expect(targets).To(BeEmpty())
		})

		It("sets nil BaseBranches when revision is empty", func() {
			objects := repositoryCredentialObjects(testNamespace, "github-repo", "https://github.com/org/repo", "scm-secret", "user", "token")
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			components := []applicationapiv1alpha1.Component{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
					Spec: applicationapiv1alpha1.ComponentSpec{
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{
									URL:      "https://github.com/org/repo",
									Revision: "",
								},
							},
						},
					},
				},
			}

			targets := GetNudgeTargetsBasicAuth(ctx, c, components, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			Expect(targets[0].Repositories[0].BaseBranches).To(BeNil())
		})

		It("creates target with correct GitAuthor format", func() {
			objects := repositoryCredentialObjects(testNamespace, "github-repo", "https://github.com/org/repo", "scm-secret", "my-bot", "token")
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			components := []applicationapiv1alpha1.Component{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
					Spec: applicationapiv1alpha1.ComponentSpec{
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{URL: "https://github.com/org/repo"},
							},
						},
					},
				},
			}

			targets := GetNudgeTargetsBasicAuth(ctx, c, components, "quay.io", "u", "p")
			Expect(targets).To(HaveLen(1))
			expectedAuthor := "my-bot <my-bot@users.noreply.github.com>"
			Expect(targets[0].GitAuthor).To(Equal(expectedAuthor))
		})
	})

	// ---------- GetImageRegistryCredentials ----------

	Describe("GetImageRegistryCredentials", func() {
		var (
			ctx    context.Context
			scheme *runtime.Scheme
		)

		BeforeEach(func() {
			ctx = context.Background()
			scheme = newCredentialScheme()
		})

		It("returns credentials from SA-linked dockerconfigjson secret", func() {
			dockerConfig := dockerConfigJSON{
				Auths: map[string]repositoryConfigAuth{
					"quay.io/org/repo": {Username: "quay-user", Password: "quay-pass"},
				},
			}
			dockerConfigBytes, err := json.Marshal(dockerConfig)
			Expect(err).NotTo(HaveOccurred())

			dockerSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "docker-secret", Namespace: testNamespace},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: dockerConfigBytes,
				},
			}
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-sa", Namespace: testNamespace},
				Secrets:    []corev1.ObjectReference{{Name: "docker-secret"}},
			}
			comp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ContainerImage: "quay.io/org/repo:latest",
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dockerSecret, sa).Build()
			host, username, password, err := GetImageRegistryCredentials(ctx, c, comp, "pipeline-sa")
			Expect(err).NotTo(HaveOccurred())
			Expect(host).To(Equal("quay.io"))
			Expect(username).To(Equal("quay-user"))
			Expect(password).To(Equal("quay-pass"))
		})

		It("decodes base64 auth field when username/password not explicit", func() {
			encodedAuth := base64.StdEncoding.EncodeToString([]byte("b64-user:b64-pass"))
			dockerConfig := dockerConfigJSON{
				Auths: map[string]repositoryConfigAuth{
					"quay.io/org/repo": {Auth: encodedAuth},
				},
			}
			dockerConfigBytes, err := json.Marshal(dockerConfig)
			Expect(err).NotTo(HaveOccurred())

			dockerSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "docker-secret", Namespace: testNamespace},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: dockerConfigBytes,
				},
			}
			sa := &corev1.ServiceAccount{
				ObjectMeta:       metav1.ObjectMeta{Name: "pipeline-sa", Namespace: testNamespace},
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "docker-secret"}},
			}
			comp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ContainerImage: "quay.io/org/repo:latest",
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dockerSecret, sa).Build()
			host, username, password, err := GetImageRegistryCredentials(ctx, c, comp, "pipeline-sa")
			Expect(err).NotTo(HaveOccurred())
			Expect(host).To(Equal("quay.io"))
			Expect(username).To(Equal("b64-user"))
			Expect(password).To(Equal("b64-pass"))
		})

		It("returns error when ServiceAccount not found", func() {
			comp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ContainerImage: "quay.io/org/repo:latest",
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			_, _, _, err := GetImageRegistryCredentials(ctx, c, comp, "nonexistent-sa")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read service account"))
		})

		It("returns error when component has no container image", func() {
			comp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
				Spec:       applicationapiv1alpha1.ComponentSpec{},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			_, _, _, err := GetImageRegistryCredentials(ctx, c, comp, "sa")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("has no container image set"))
		})

		It("returns error when no matching registry in docker config", func() {
			dockerConfig := dockerConfigJSON{
				Auths: map[string]repositoryConfigAuth{
					"gcr.io/other-project": {Username: "gcr-user", Password: "gcr-pass"},
				},
			}
			dockerConfigBytes, err := json.Marshal(dockerConfig)
			Expect(err).NotTo(HaveOccurred())

			dockerSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "docker-secret", Namespace: testNamespace},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: dockerConfigBytes,
				},
			}
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-sa", Namespace: testNamespace},
				Secrets:    []corev1.ObjectReference{{Name: "docker-secret"}},
			}
			comp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ContainerImage: "quay.io/org/repo:latest",
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dockerSecret, sa).Build()
			host, _, _, err := GetImageRegistryCredentials(ctx, c, comp, "pipeline-sa")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no credentials found for image"))
			// Host is still returned even when credentials fail
			Expect(host).To(Equal("quay.io"))
		})

		It("returns error when SA has no linked secrets", func() {
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-sa", Namespace: testNamespace},
			}
			comp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ContainerImage: "quay.io/org/repo:latest",
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sa).Build()
			_, _, _, err := GetImageRegistryCredentials(ctx, c, comp, "empty-sa")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no secrets linked to service account"))
		})

		It("skips non-dockerconfigjson secrets", func() {
			opaqueSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "opaque-secret", Namespace: testNamespace},
				Type:       corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			}
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-sa", Namespace: testNamespace},
				Secrets:    []corev1.ObjectReference{{Name: "opaque-secret"}},
			}
			comp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ContainerImage: "quay.io/org/repo:latest",
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(opaqueSecret, sa).Build()
			_, _, _, err := GetImageRegistryCredentials(ctx, c, comp, "pipeline-sa")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no credentials found for image"))
		})

		It("handles image with digest instead of tag", func() {
			dockerConfig := dockerConfigJSON{
				Auths: map[string]repositoryConfigAuth{
					"quay.io/org/repo": {Username: "user", Password: "pass"},
				},
			}
			dockerConfigBytes, err := json.Marshal(dockerConfig)
			Expect(err).NotTo(HaveOccurred())

			dockerSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "docker-secret", Namespace: testNamespace},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: dockerConfigBytes,
				},
			}
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-sa", Namespace: testNamespace},
				Secrets:    []corev1.ObjectReference{{Name: "docker-secret"}},
			}
			comp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ContainerImage: "quay.io/org/repo@sha256:abc123def456",
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dockerSecret, sa).Build()
			host, username, password, err := GetImageRegistryCredentials(ctx, c, comp, "pipeline-sa")
			Expect(err).NotTo(HaveOccurred())
			Expect(host).To(Equal("quay.io"))
			Expect(username).To(Equal("user"))
			Expect(password).To(Equal("pass"))
		})

		It("prefers the first linked dockerconfigjson secret when credentials conflict", func() {
			firstConfig, err := json.Marshal(dockerConfigJSON{
				Auths: map[string]repositoryConfigAuth{
					"quay.io/org/repo": {Username: "first-user", Password: "first-pass"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			secondConfig, err := json.Marshal(dockerConfigJSON{
				Auths: map[string]repositoryConfigAuth{
					"quay.io/org/repo": {Username: "second-user", Password: "second-pass"},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			zebraSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "zebra-secret", Namespace: testNamespace},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data:       map[string][]byte{corev1.DockerConfigJsonKey: firstConfig},
			}
			alphaSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "alpha-secret", Namespace: testNamespace},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data:       map[string][]byte{corev1.DockerConfigJsonKey: secondConfig},
			}
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "pipeline-sa", Namespace: testNamespace},
				Secrets: []corev1.ObjectReference{
					{Name: "zebra-secret"},
					{Name: "alpha-secret"},
				},
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: "zebra-secret"},
				},
			}
			comp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp", Namespace: testNamespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					ContainerImage: "quay.io/org/repo:latest",
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(zebraSecret, alphaSecret, sa).Build()
			_, username, password, err := GetImageRegistryCredentials(ctx, c, comp, "pipeline-sa")
			Expect(err).NotTo(HaveOccurred())
			Expect(username).To(Equal("first-user"))
			Expect(password).To(Equal("first-pass"))
		})
	})

	// ---------- linkedSecretNamesFromServiceAccount ----------

	Describe("linkedSecretNamesFromServiceAccount", func() {
		It("returns unique names in .secrets then .imagePullSecrets order", func() {
			sa := &corev1.ServiceAccount{
				Secrets: []corev1.ObjectReference{
					{Name: "zebra-secret"},
					{Name: "alpha-secret"},
				},
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: "zebra-secret"},
					{Name: "pull-secret"},
				},
			}
			Expect(linkedSecretNamesFromServiceAccount(sa)).To(Equal([]string{"zebra-secret", "alpha-secret", "pull-secret"}))
		})
	})

	// ---------- extractCredentialsFromRepositorySecret ----------

	Describe("extractCredentialsFromRepositorySecret", func() {
		Context("When the secret is kubernetes.io/basic-auth", func() {
			It("should return username and password from a BasicAuth secret", func() {
				secret := &corev1.Secret{
					Type: corev1.SecretTypeBasicAuth,
					Data: map[string][]byte{
						corev1.BasicAuthUsernameKey: []byte("user"),
						corev1.BasicAuthPasswordKey: []byte("pass"),
					},
				}

				username, password, err := extractCredentialsFromRepositorySecret(&pacv1alpha1.GitProvider{}, secret, "ignored-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(username).To(Equal("user"))
				Expect(password).To(Equal("pass"))
			})

			It("should allow an empty username on a BasicAuth secret", func() {
				secret := &corev1.Secret{
					Type: corev1.SecretTypeBasicAuth,
					Data: map[string][]byte{
						corev1.BasicAuthPasswordKey: []byte("pass"),
					},
				}

				username, password, err := extractCredentialsFromRepositorySecret(&pacv1alpha1.GitProvider{}, secret, "ignored-key")
				Expect(err).NotTo(HaveOccurred())
				Expect(username).To(Equal(""))
				Expect(password).To(Equal("pass"))
			})

			It("should return error when BasicAuth secret password is missing", func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "scm-secret"},
					Type:       corev1.SecretTypeBasicAuth,
					Data: map[string][]byte{
						corev1.BasicAuthUsernameKey: []byte("user"),
					},
				}

				_, _, err := extractCredentialsFromRepositorySecret(&pacv1alpha1.GitProvider{}, secret, "ignored-key")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(`key "password" not found in secret scm-secret`))
			})

			It("should return error when BasicAuth secret password is empty", func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "scm-secret"},
					Type:       corev1.SecretTypeBasicAuth,
					Data: map[string][]byte{
						corev1.BasicAuthUsernameKey: []byte("user"),
						corev1.BasicAuthPasswordKey: {},
					},
				}

				_, _, err := extractCredentialsFromRepositorySecret(&pacv1alpha1.GitProvider{}, secret, "ignored-key")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(`key "password" not found in secret scm-secret`))
			})
		})

		Context("When the secret is Opaque", func() {
			It("should return GitProvider user and token from an opaque secret", func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "pac-secret"},
					Type:       corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"token": []byte("ghp_token"),
					},
				}

				username, password, err := extractCredentialsFromRepositorySecret(
					&pacv1alpha1.GitProvider{User: "pac-bot"},
					secret,
					"token",
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(username).To(Equal("pac-bot"))
				Expect(password).To(Equal("ghp_token"))
			})

			It("should return error when opaque secret key is missing", func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "pac-secret"},
					Type:       corev1.SecretTypeOpaque,
					Data:       map[string][]byte{},
				}

				_, _, err := extractCredentialsFromRepositorySecret(&pacv1alpha1.GitProvider{User: "pac-bot"}, secret, "token")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(`key "token" not found in secret pac-secret`))
			})

			It("should return error when opaque secret key value is empty", func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "pac-secret"},
					Type:       corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"token": {},
					},
				}

				_, _, err := extractCredentialsFromRepositorySecret(&pacv1alpha1.GitProvider{User: "pac-bot"}, secret, "token")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(`key "token" not found in secret pac-secret`))
			})
		})
	})

	// ---------- lookupSCMCredentialsViaRepository ----------

	Describe("lookupSCMCredentialsViaRepository", func() {
		var lookupNS *corev1.Namespace

		createAndWait := func(obj client.Object) {
			GinkgoHelper()
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
			}).Should(Succeed())
		}

		BeforeEach(func() {
			lookupNS = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "nudge-lookup-"}}
			createAndWait(lookupNS)
		})

		AfterEach(func() {
			if lookupNS != nil && lookupNS.Name != "" {
				Expect(k8sClient.Delete(ctx, lookupNS)).To(Succeed())
			}
		})

		Context("When a matching Repository CR references a BasicAuth secret", func() {
			It("should return credentials from matching Repository CR with BasicAuth secret", func() {
				objects := repositoryCredentialObjects(lookupNS.Name, "github-repo", "https://github.com/org/repo", "scm-secret", "user", "token")
				for _, obj := range objects {
					createAndWait(obj)
				}

				username, password, err := lookupSCMCredentialsViaRepository(ctx, k8sClient, lookupNS.Name, "https://github.com/org/repo")
				Expect(err).NotTo(HaveOccurred())
				Expect(username).To(Equal("user"))
				Expect(password).To(Equal("token"))
			})
		})

		Context("When the component URL has a .git suffix", func() {
			It("should match Repository URL when component URL has a .git suffix", func() {
				objects := repositoryCredentialObjects(lookupNS.Name, "github-repo", "https://github.com/org/repo", "scm-secret", "user", "token")
				for _, obj := range objects {
					createAndWait(obj)
				}

				username, password, err := lookupSCMCredentialsViaRepository(ctx, k8sClient, lookupNS.Name, "https://github.com/org/repo.git")
				Expect(err).NotTo(HaveOccurred())
				Expect(username).To(Equal("user"))
				Expect(password).To(Equal("token"))
			})
		})

		Context("When no Repository CR matches the URL", func() {
			It("should return error when no Repository CR matches the URL", func() {
				_, _, err := lookupSCMCredentialsViaRepository(ctx, k8sClient, lookupNS.Name, "https://github.com/org/repo")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no Repository CR matching URL"))
			})
		})

		Context("When multiple Repository CRs match the same URL", func() {
			It("should return error when multiple Repository CRs match the same URL", func() {
				repoA := &pacv1alpha1.Repository{
					ObjectMeta: metav1.ObjectMeta{Name: "github-repo-a", Namespace: lookupNS.Name},
					Spec: pacv1alpha1.RepositorySpec{
						URL: "https://github.com/org/repo",
					},
				}
				repoB := &pacv1alpha1.Repository{
					ObjectMeta: metav1.ObjectMeta{Name: "github-repo-b", Namespace: lookupNS.Name},
					Spec: pacv1alpha1.RepositorySpec{
						URL: "https://github.com/org/repo.git",
					},
				}
				createAndWait(repoA)
				createAndWait(repoB)

				_, _, err := lookupSCMCredentialsViaRepository(ctx, k8sClient, lookupNS.Name, "https://github.com/org/repo")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("multiple Repository CRs match URL"))
				Expect(err.Error()).To(ContainSubstring("github-repo-a, github-repo-b"))
			})
		})

		Context("When the Repository CR has no git_provider.secret", func() {
			It("should return error when Repository CR has no git_provider.secret", func() {
				repo := &pacv1alpha1.Repository{
					ObjectMeta: metav1.ObjectMeta{Name: "github-repo", Namespace: lookupNS.Name},
					Spec: pacv1alpha1.RepositorySpec{
						URL: "https://github.com/org/repo",
					},
				}
				createAndWait(repo)

				_, _, err := lookupSCMCredentialsViaRepository(ctx, k8sClient, lookupNS.Name, "https://github.com/org/repo")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("has no git_provider.secret configured"))
			})
		})

		Context("When the Repository CR references an opaque secret", func() {
			It("should return credentials from opaque secret using GitProvider user and secret key", func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "pac-secret", Namespace: lookupNS.Name},
					Type:       corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"token": []byte("ghp_token"),
					},
				}
				repo := &pacv1alpha1.Repository{
					ObjectMeta: metav1.ObjectMeta{Name: "github-repo", Namespace: lookupNS.Name},
					Spec: pacv1alpha1.RepositorySpec{
						URL: "https://github.com/org/repo",
						GitProvider: &pacv1alpha1.GitProvider{
							User: "pac-bot",
							Secret: &pacv1alpha1.Secret{
								Name: "pac-secret",
								Key:  "token",
							},
						},
					},
				}
				createAndWait(secret)
				createAndWait(repo)

				username, password, err := lookupSCMCredentialsViaRepository(ctx, k8sClient, lookupNS.Name, "https://github.com/org/repo")
				Expect(err).NotTo(HaveOccurred())
				Expect(username).To(Equal("pac-bot"))
				Expect(password).To(Equal("ghp_token"))
			})
		})
	})

	// ---------- getGitRepoURL ----------

	Describe("getGitRepoURL", func() {
		It("returns normalized URL stripping .git and trailing slash", func() {
			comp := &applicationapiv1alpha1.Component{
				Spec: applicationapiv1alpha1.ComponentSpec{
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{
								URL: "https://github.com/org/repo.git/",
							},
						},
					},
				},
			}
			Expect(getGitRepoURL(comp)).To(Equal("https://github.com/org/repo"))
		})

		It("returns empty string when git source is nil", func() {
			comp := &applicationapiv1alpha1.Component{
				Spec: applicationapiv1alpha1.ComponentSpec{},
			}
			Expect(getGitRepoURL(comp)).To(Equal(""))
		})
	})
})
