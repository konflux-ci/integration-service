/*
Copyright 2026 Red Hat Inc.

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

package conversion

import (
	newapi "github.com/konflux-ci/application-api/api/konflux/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ConvertNewToOld", func() {
	It("should convert a full component with all fields", func() {
		src := newapi.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "my-component",
				Namespace:       "default",
				ResourceVersion: "123",
				Annotations: map[string]string{
					"test-key": "test-value",
				},
			},
			Spec: newapi.ComponentSpec{
				Source: newapi.ComponentSource{
					GitURL:         "https://github.com/org/repo",
					DockerfilePath: "docker/Dockerfile",
					Versions: []newapi.ComponentVersion{
						{
							Name:           "v1",
							Revision:       "main",
							Context:        "src",
							DockerfilePath: "src/Dockerfile",
							SkipBuilds:     false,
							BuildPipeline: &newapi.ComponentBuildPipeline{
								PullAndPush: &newapi.PipelineDefinition{
									PipelineRefName: "my-pipeline",
								},
							},
						},
						{
							Name:       "v2",
							Revision:   "release-2.0",
							SkipBuilds: true,
						},
					},
				},
				ContainerImage:    "quay.io/org/repo",
				SkipOffboardingPr: true,
				RepositorySettings: newapi.RepositorySettings{
					CommentStrategy:          "always",
					GithubAppTokenScopeRepos: []string{"extra-repo"},
				},
				DefaultBuildPipeline: &newapi.ComponentBuildPipeline{
					Push: &newapi.PipelineDefinition{
						PipelineSpecFromBundle: &newapi.PipelineSpecFromBundle{
							Bundle: "latest",
							Name:   "docker-build",
						},
					},
					Pull: &newapi.PipelineDefinition{
						PipelineRefGit: &newapi.PipelineRefGit{
							PathInRepo: "pipeline/pull.yaml",
							Revision:   "main",
							Url:        "https://github.com/pipelines/repo",
						},
					},
				},
				Actions: newapi.ComponentActions{
					TriggerBuild:  "v1",
					TriggerBuilds: []string{"v2"},
					CreateConfiguration: newapi.ComponentCreatePipelineConfiguration{
						AllVersions: true,
					},
				},
			},
			Status: newapi.ComponentStatus{
				Message:       "all good",
				PacRepository: "my-component-pac",
				RepositorySettings: newapi.RepositorySettings{
					CommentStrategy: "always",
				},
				Versions: []newapi.ComponentVersionStatus{
					{
						Name:                  "v1",
						Revision:              "main",
						OnboardingStatus:      "succeeded",
						OnboardingTime:        "01 Jan 2026 00:00:00 UTC",
						ConfigurationMergeURL: "https://github.com/org/repo/pull/1",
					},
				},
			},
		}

		dst := ConvertNewToOld(src)

		// ObjectMeta
		Expect(dst.Name).To(Equal("my-component"))
		Expect(dst.Namespace).To(Equal("default"))
		Expect(dst.ResourceVersion).To(Equal("123"))
		Expect(dst.Annotations["test-key"]).To(Equal("test-value"))

		// Source
		Expect(dst.Spec.Source.GitURL).To(Equal("https://github.com/org/repo"))
		Expect(dst.Spec.Source.GitSource).NotTo(BeNil())
		Expect(dst.Spec.Source.GitSource.URL).To(Equal("https://github.com/org/repo"))
		Expect(dst.Spec.Source.DockerfileURI).To(Equal("docker/Dockerfile"))

		// Versions
		Expect(dst.Spec.Source.Versions).To(HaveLen(2))
		v1 := dst.Spec.Source.Versions[0]
		Expect(v1.Name).To(Equal("v1"))
		Expect(v1.Revision).To(Equal("main"))
		Expect(v1.Context).To(Equal("src"))
		Expect(v1.DockerfileURI).To(Equal("src/Dockerfile"))
		Expect(v1.BuildPipeline).NotTo(BeNil())
		Expect(v1.BuildPipeline.PullAndPush).NotTo(BeNil())
		Expect(v1.BuildPipeline.PullAndPush.PipelineRefName).To(Equal("my-pipeline"))
		v2 := dst.Spec.Source.Versions[1]
		Expect(v2.Name).To(Equal("v2"))
		Expect(v2.SkipBuilds).To(BeTrue())

		// ContainerImage
		Expect(dst.Spec.ContainerImage).To(Equal("quay.io/org/repo"))

		// Actions
		Expect(dst.Spec.Actions.TriggerBuild).To(Equal("v1"))
		Expect(dst.Spec.Actions.TriggerBuilds).To(HaveLen(1))
		Expect(dst.Spec.Actions.TriggerBuilds[0]).To(Equal("v2"))
		Expect(dst.Spec.Actions.CreateConfiguration.AllVersions).To(BeTrue())

		// SkipOffboardingPr
		Expect(dst.Spec.SkipOffboardingPr).To(BeTrue())

		// RepositorySettings
		Expect(dst.Spec.RepositorySettings.CommentStrategy).To(Equal("always"))
		Expect(dst.Spec.RepositorySettings.GithubAppTokenScopeRepos).To(HaveLen(1))

		// DefaultBuildPipeline
		Expect(dst.Spec.DefaultBuildPipeline).NotTo(BeNil())
		Expect(dst.Spec.DefaultBuildPipeline.Push).NotTo(BeNil())
		Expect(dst.Spec.DefaultBuildPipeline.Push.PipelineSpecFromBundle).NotTo(BeNil())
		Expect(dst.Spec.DefaultBuildPipeline.Push.PipelineSpecFromBundle.Bundle).To(Equal("latest"))
		Expect(dst.Spec.DefaultBuildPipeline.Pull).NotTo(BeNil())
		Expect(dst.Spec.DefaultBuildPipeline.Pull.PipelineRefGit).NotTo(BeNil())
		Expect(dst.Spec.DefaultBuildPipeline.Pull.PipelineRefGit.Url).To(Equal("https://github.com/pipelines/repo"))

		// Status
		Expect(dst.Status.Message).To(Equal("all good"))
		Expect(dst.Status.PacRepository).To(Equal("my-component-pac"))
		Expect(dst.Status.RepositorySettings.CommentStrategy).To(Equal("always"))
		Expect(dst.Status.Versions).To(HaveLen(1))
		sv := dst.Status.Versions[0]
		Expect(sv.Name).To(Equal("v1"))
		Expect(sv.OnboardingStatus).To(Equal("succeeded"))
		Expect(sv.ConfigurationMergeURL).To(Equal("https://github.com/org/repo/pull/1"))

		// Legacy fields should be zero-valued
		Expect(dst.Spec.Application).To(BeEmpty())
		Expect(dst.Status.LastPromotedImage).To(BeEmpty())
		Expect(dst.Status.LastBuiltCommit).To(BeEmpty())
	})

	It("should handle empty source", func() {
		src := newapi.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "ns"},
			Spec: newapi.ComponentSpec{
				Source: newapi.ComponentSource{},
			},
		}

		dst := ConvertNewToOld(src)

		Expect(dst.Spec.Source.GitSource).NotTo(BeNil())
		Expect(dst.Spec.Source.GitSource.URL).To(BeEmpty())
		Expect(dst.Spec.Source.Versions).To(BeNil())
	})

	It("should handle nil build pipeline", func() {
		src := newapi.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "no-pipeline", Namespace: "ns"},
			Spec: newapi.ComponentSpec{
				Source: newapi.ComponentSource{
					GitURL: "https://github.com/org/repo",
					Versions: []newapi.ComponentVersion{
						{Name: "v1", Revision: "main"},
					},
				},
			},
		}

		dst := ConvertNewToOld(src)

		Expect(dst.Spec.DefaultBuildPipeline).To(BeNil())
		Expect(dst.Spec.Source.Versions[0].BuildPipeline).To(BeNil())
	})

	It("should deep copy ObjectMeta", func() {
		src := newapi.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "copy-test",
				Namespace:   "ns",
				Annotations: map[string]string{"key": "value"},
			},
			Spec: newapi.ComponentSpec{
				Source: newapi.ComponentSource{GitURL: "https://github.com/org/repo"},
			},
		}

		dst := ConvertNewToOld(src)

		dst.Annotations["key"] = "mutated"
		Expect(src.Annotations["key"]).To(Equal("value"))
	})
})
