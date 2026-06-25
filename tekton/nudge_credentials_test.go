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

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
		return scheme
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
	})

	// ---------- bestMatchingSCMSecret ----------

	Describe("bestMatchingSCMSecret", func() {
		ctx := context.Background()

		It("returns direct match from scm.repository annotation", func() {
			secrets := []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "wildcard-secret",
						Annotations: map[string]string{
							scmSecretRepositoryAnnotation: "org/*",
						},
					},
					Data: map[string][]byte{
						corev1.BasicAuthUsernameKey: []byte("user"),
						corev1.BasicAuthPasswordKey: []byte("pass"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "exact-secret",
						Annotations: map[string]string{
							scmSecretRepositoryAnnotation: "org/repo",
						},
					},
					Data: map[string][]byte{
						corev1.BasicAuthUsernameKey: []byte("user-exact"),
						corev1.BasicAuthPasswordKey: []byte("pass-exact"),
					},
				},
			}
			result := bestMatchingSCMSecret(ctx, "org/repo", secrets)
			Expect(result).NotTo(BeNil())
			Expect(result.Name).To(Equal("exact-secret"))
		})

		It("returns wildcard match when no direct match", func() {
			secrets := []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "wildcard-secret",
						Annotations: map[string]string{
							scmSecretRepositoryAnnotation: "org/*",
						},
					},
					Data: map[string][]byte{
						corev1.BasicAuthUsernameKey: []byte("user"),
						corev1.BasicAuthPasswordKey: []byte("pass"),
					},
				},
			}
			result := bestMatchingSCMSecret(ctx, "org/other-repo", secrets)
			Expect(result).NotTo(BeNil())
			Expect(result.Name).To(Equal("wildcard-secret"))
		})

		It("returns host-only secret as fallback", func() {
			secrets := []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "host-only-secret",
						Annotations: map[string]string{},
					},
					Data: map[string][]byte{
						corev1.BasicAuthUsernameKey: []byte("user"),
						corev1.BasicAuthPasswordKey: []byte("pass"),
					},
				},
			}
			result := bestMatchingSCMSecret(ctx, "org/repo", secrets)
			Expect(result).NotTo(BeNil())
			Expect(result.Name).To(Equal("host-only-secret"))
		})

		It("returns nil when no secrets match", func() {
			secrets := []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "wrong-repo-secret",
						Annotations: map[string]string{
							scmSecretRepositoryAnnotation: "other-org/other-repo",
						},
					},
					Data: map[string][]byte{
						corev1.BasicAuthUsernameKey: []byte("user"),
						corev1.BasicAuthPasswordKey: []byte("pass"),
					},
				},
			}
			result := bestMatchingSCMSecret(ctx, "org/repo", secrets)
			Expect(result).To(BeNil())
		})

		It("handles leading slashes in repository annotation", func() {
			secrets := []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "prefixed-secret",
						Annotations: map[string]string{
							scmSecretRepositoryAnnotation: "/org/repo",
						},
					},
					Data: map[string][]byte{
						corev1.BasicAuthUsernameKey: []byte("user"),
						corev1.BasicAuthPasswordKey: []byte("pass"),
					},
				},
			}
			result := bestMatchingSCMSecret(ctx, "org/repo", secrets)
			Expect(result).NotTo(BeNil())
			Expect(result.Name).To(Equal("prefixed-secret"))
		})

		It("handles comma-separated repositories in annotation", func() {
			secrets := []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "multi-repo-secret",
						Annotations: map[string]string{
							scmSecretRepositoryAnnotation: "org/repo-a, org/repo-b, org/repo-c",
						},
					},
					Data: map[string][]byte{
						corev1.BasicAuthUsernameKey: []byte("user"),
						corev1.BasicAuthPasswordKey: []byte("pass"),
					},
				},
			}
			result := bestMatchingSCMSecret(ctx, "org/repo-b", secrets)
			Expect(result).NotTo(BeNil())
			Expect(result.Name).To(Equal("multi-repo-secret"))
		})
	})

	// ---------- GetNudgeTargetsGithubApp ----------

	Describe("GetNudgeTargetsGithubApp", func() {
		var (
			ctx    context.Context
			scheme *runtime.Scheme
		)

		BeforeEach(func() {
			ctx = context.Background()
			scheme = newCredentialScheme()
		})

		It("returns nil when PaC secret does not exist", func() {
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
			targets := GetNudgeTargetsGithubApp(ctx, c, components, "quay.io", "user", "pass")
			Expect(targets).To(BeNil())
		})

		It("returns nil when PaC secret exists but GitHub App is not configured", func() {
			pacSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tektonconsts.PipelinesAsCodeGitHubAppSecretName,
					Namespace: tektonconsts.BuildServiceNamespaceName,
				},
				Data: map[string][]byte{
					"some-other-key": []byte("some-value"),
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pacSecret).Build()
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
			targets := GetNudgeTargetsGithubApp(ctx, c, components, "quay.io", "user", "pass")
			Expect(targets).To(BeNil())
		})

		It("returns nil when PaC secret has GitHub App configured (not yet implemented)", func() {
			pacSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tektonconsts.PipelinesAsCodeGitHubAppSecretName,
					Namespace: tektonconsts.BuildServiceNamespaceName,
				},
				Data: map[string][]byte{
					tektonconsts.PipelinesAsCodeGithubAppIdKey:   []byte("12345"),
					tektonconsts.PipelinesAsCodeGithubPrivateKey: []byte("-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----"),
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pacSecret).Build()
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
			// Currently returns nil because the implementation is a TODO stub
			targets := GetNudgeTargetsGithubApp(ctx, c, components, "quay.io", "user", "pass")
			Expect(targets).To(BeNil())
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
			scmSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scm-github-secret",
					Namespace: testNamespace,
					Labels: map[string]string{
						scmCredentialsSecretLabel: "scm",
						scmSecretHostnameLabel:    "github.com",
					},
					Annotations: map[string]string{
						scmSecretRepositoryAnnotation: "org/repo",
					},
				},
				Type: corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					corev1.BasicAuthUsernameKey: []byte("bot-user"),
					corev1.BasicAuthPasswordKey: []byte("ghp_secret-token"),
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(scmSecret).Build()
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
			scmSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scm-secret",
					Namespace: testNamespace,
					Labels: map[string]string{
						scmCredentialsSecretLabel: "scm",
						scmSecretHostnameLabel:    "github.com",
					},
				},
				Type: corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					corev1.BasicAuthUsernameKey: {},
					corev1.BasicAuthPasswordKey: []byte("token"),
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(scmSecret).Build()
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
			scmSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scm-secret",
					Namespace: testNamespace,
					Labels: map[string]string{
						scmCredentialsSecretLabel: "scm",
						scmSecretHostnameLabel:    "github.com",
					},
					Annotations: map[string]string{
						scmSecretRepositoryAnnotation: "org/repo-a",
					},
				},
				Type: corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					corev1.BasicAuthUsernameKey: []byte("user"),
					corev1.BasicAuthPasswordKey: []byte("token"),
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(scmSecret).Build()
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
			scmSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scm-secret",
					Namespace: testNamespace,
					Labels: map[string]string{
						scmCredentialsSecretLabel: "scm",
						scmSecretHostnameLabel:    "github.com",
					},
				},
				Type: corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					corev1.BasicAuthUsernameKey: []byte("user"),
					corev1.BasicAuthPasswordKey: []byte("token"),
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(scmSecret).Build()
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
			scmSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scm-secret",
					Namespace: testNamespace,
					Labels: map[string]string{
						scmCredentialsSecretLabel: "scm",
						scmSecretHostnameLabel:    "github.com",
					},
				},
				Type: corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					corev1.BasicAuthUsernameKey: []byte("my-bot"),
					corev1.BasicAuthPasswordKey: []byte("token"),
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(scmSecret).Build()
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
	})

	// ---------- lookupSCMCredentials ----------

	Describe("lookupSCMCredentials", func() {
		var (
			ctx    context.Context
			scheme *runtime.Scheme
		)

		BeforeEach(func() {
			ctx = context.Background()
			scheme = newCredentialScheme()
		})

		It("returns credentials from matching BasicAuth secret", func() {
			scmSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scm-secret",
					Namespace: testNamespace,
					Labels: map[string]string{
						scmCredentialsSecretLabel: "scm",
						scmSecretHostnameLabel:    "github.com",
					},
					Annotations: map[string]string{
						scmSecretRepositoryAnnotation: "org/repo",
					},
				},
				Type: corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					corev1.BasicAuthUsernameKey: []byte("user"),
					corev1.BasicAuthPasswordKey: []byte("token"),
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(scmSecret).Build()

			username, password, err := lookupSCMCredentials(ctx, c, testNamespace, "github.com", "org/repo")
			Expect(err).NotTo(HaveOccurred())
			Expect(username).To(Equal("user"))
			Expect(password).To(Equal("token"))
		})

		It("returns error when no secrets match the host", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			_, _, err := lookupSCMCredentials(ctx, c, testNamespace, "github.com", "org/repo")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no SCM basic auth secrets found"))
		})

		It("skips non-BasicAuth secrets", func() {
			opaqueSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opaque-scm",
					Namespace: testNamespace,
					Labels: map[string]string{
						scmCredentialsSecretLabel: "scm",
						scmSecretHostnameLabel:    "github.com",
					},
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"token": []byte("some-token"),
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(opaqueSecret).Build()
			_, _, err := lookupSCMCredentials(ctx, c, testNamespace, "github.com", "org/repo")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no SCM basic auth secrets found"))
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
