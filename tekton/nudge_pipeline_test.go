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

package tekton_test

import (
	"context"
	"encoding/json"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	tekton "github.com/konflux-ci/integration-service/tekton"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// nudgeTestScheme builds a runtime.Scheme containing the types needed by the
// nudge pipeline tests (core, Tekton, Application API).
func nudgeTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	Expect(corev1.AddToScheme(s)).To(Succeed())
	Expect(tektonv1.AddToScheme(s)).To(Succeed())
	Expect(applicationapiv1alpha1.AddToScheme(s)).To(Succeed())
	return s
}

// nudgeBuildPLR returns a minimal build PipelineRun with the given results and
// annotations, suitable for passing to ExtractNudgeBuildResult.
func nudgeBuildPLR(name string, results []tektonv1.PipelineRunResult, annotations map[string]string) *tektonv1.PipelineRun {
	return &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			Annotations: annotations,
		},
		Status: tektonv1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
				Results: results,
			},
		},
	}
}

// nudgeSampleComponent returns a Component with optional annotations.
func nudgeSampleComponent(name, namespace string, annotations map[string]string) *applicationapiv1alpha1.Component {
	return &applicationapiv1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: applicationapiv1alpha1.ComponentSpec{
			ComponentName: name,
			Application:   "test-app",
		},
	}
}

var _ = Describe("Nudge Pipeline", func() {

	// ------------------------------------------------------------------
	// ExtractNudgeBuildResult
	// ------------------------------------------------------------------
	Describe("ExtractNudgeBuildResult", func() {
		var component *applicationapiv1alpha1.Component

		BeforeEach(func() {
			component = nudgeSampleComponent("my-component", "default", nil)
		})

		It("extracts IMAGE_URL and IMAGE_DIGEST from PLR results (happy path)", func() {
			plr := nudgeBuildPLR("build-plr-1", []tektonv1.PipelineRunResult{
				{Name: tektonconsts.PipelineRunImageUrlParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "quay.io/org/repo:v1.0"}},
				{Name: tektonconsts.PipelineRunImageDigestParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "sha256:abc123"}},
			}, nil)

			result, err := tekton.ExtractNudgeBuildResult(plr, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.BuiltImageRepository).To(Equal("quay.io/org/repo"))
			Expect(result.BuiltImageTag).To(Equal("v1.0"))
			Expect(result.Digest).To(Equal("sha256:abc123"))
			Expect(result.SourceComponentName).To(Equal("my-component"))
		})

		It("handles IMAGE_URL without a tag", func() {
			plr := nudgeBuildPLR("build-plr-notag", []tektonv1.PipelineRunResult{
				{Name: tektonconsts.PipelineRunImageUrlParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "quay.io/org/repo"}},
				{Name: tektonconsts.PipelineRunImageDigestParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "sha256:def456"}},
			}, nil)

			result, err := tekton.ExtractNudgeBuildResult(plr, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.BuiltImageRepository).To(Equal("quay.io/org/repo"))
			Expect(result.BuiltImageTag).To(BeEmpty())
		})

		It("returns an error when IMAGE_URL is missing", func() {
			plr := nudgeBuildPLR("build-plr-nourl", []tektonv1.PipelineRunResult{
				{Name: tektonconsts.PipelineRunImageDigestParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "sha256:abc123"}},
			}, nil)

			_, err := tekton.ExtractNudgeBuildResult(plr, component)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("IMAGE_URL"))
		})

		It("returns an error when IMAGE_DIGEST is missing", func() {
			plr := nudgeBuildPLR("build-plr-nodigest", []tektonv1.PipelineRunResult{
				{Name: tektonconsts.PipelineRunImageUrlParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "quay.io/org/repo:v1"}},
			}, nil)

			_, err := tekton.ExtractNudgeBuildResult(plr, component)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("IMAGE_DIGEST"))
		})

		It("uses DefaultNudgeFiles when nudge-files annotation is absent", func() {
			plr := nudgeBuildPLR("build-plr-defaults", []tektonv1.PipelineRunResult{
				{Name: tektonconsts.PipelineRunImageUrlParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "quay.io/org/repo:latest"}},
				{Name: tektonconsts.PipelineRunImageDigestParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "sha256:aaa"}},
			}, nil)

			result, err := tekton.ExtractNudgeBuildResult(plr, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.FileMatches).To(Equal(tektonconsts.DefaultNudgeFiles))
		})

		It("uses the nudge-files annotation when present", func() {
			plr := nudgeBuildPLR("build-plr-custom-files", []tektonv1.PipelineRunResult{
				{Name: tektonconsts.PipelineRunImageUrlParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "quay.io/org/repo:latest"}},
				{Name: tektonconsts.PipelineRunImageDigestParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "sha256:bbb"}},
			}, map[string]string{
				tektonconsts.NudgeFilesAnnotation: ".*\\.yaml",
			})

			result, err := tekton.ExtractNudgeBuildResult(plr, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.FileMatches).To(Equal(".*\\.yaml"))
		})

		It("reads the git-repo-at-sha annotation", func() {
			plr := nudgeBuildPLR("build-plr-sha", []tektonv1.PipelineRunResult{
				{Name: tektonconsts.PipelineRunImageUrlParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "quay.io/org/repo:tag"}},
				{Name: tektonconsts.PipelineRunImageDigestParamName, Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "sha256:ccc"}},
			}, map[string]string{
				tektonconsts.GitRepoAtShaAnnotation: "https://github.com/org/repo/commit/abc123def456abc123def456abc123def456abcd",
			})

			result, err := tekton.ExtractNudgeBuildResult(plr, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.GitRepoAtShaLink).To(Equal("https://github.com/org/repo/commit/abc123def456abc123def456abc123def456abcd"))
		})
	})

	// ------------------------------------------------------------------
	// GenerateRenovateConfig
	// ------------------------------------------------------------------
	Describe("GenerateRenovateConfig", func() {
		var (
			target      tekton.NudgeTarget
			buildResult *tekton.NudgeBuildResult
		)

		BeforeEach(func() {
			target = tekton.NudgeTarget{
				ComponentName: "target-comp",
				GitProvider:   "github",
				Username:      "test-user",
				GitAuthor:     "Test User <test@example.com>",
				Token:         "ghp_fake",
				Endpoint:      "https://api.github.com",
				Repositories:  []tekton.RenovateRepository{{Repository: "org/downstream"}},
			}
			buildResult = &tekton.NudgeBuildResult{
				BuiltImageRepository: "quay.io/org/image",
				BuiltImageTag:        "latest",
				Digest:               "sha256:abcdef",
				FileMatches:          ".*Dockerfile.*, .*.yaml",
				SourceComponentName:  "source-comp",
				GitRepoAtShaLink:     "https://github.com/org/repo/commit/aabbccddaabbccddaabbccddaabbccddaabbccdd",
			}
		})

		It("builds custom managers with correct regex match strings", func() {
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			Expect(config.CustomManagers).To(HaveLen(1))
			cm := config.CustomManagers[0]
			Expect(cm.CustomType).To(Equal("regex"))
			Expect(cm.DatasourceTemplate).To(Equal("docker"))
			Expect(cm.CurrentValueTemplate).To(Equal("latest"))
			Expect(cm.DepNameTemplate).To(Equal("quay.io/org/image"))
			Expect(cm.MatchStrings).To(HaveLen(1))
			Expect(cm.MatchStrings[0]).To(ContainSubstring(`quay\.io/org/image`))
			Expect(cm.MatchStrings[0]).To(ContainSubstring("(?<currentDigest>sha256:[a-f0-9]+)"))
		})

		It("produces file match parts by splitting on comma", func() {
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			Expect(config.CustomManagers[0].FileMatch).To(ConsistOf(".*Dockerfile.*", ".*.yaml"))
		})

		It("starts packageRules with DisableAll then an enable rule", func() {
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			Expect(config.PackageRules).To(HaveLen(2))
			Expect(config.PackageRules[0]).To(Equal(tekton.DisableAllPackageRules))
			Expect(config.PackageRules[1].Enabled).To(BeTrue())
			Expect(config.PackageRules[1].MatchPackageNames).To(ContainElement("quay.io/org/image"))
		})

		It("sets branch naming with component prefix when simpleBranchName is false", func() {
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			enableRule := config.PackageRules[1]
			Expect(enableRule.BranchPrefix).To(Equal(tektonconsts.BranchPrefix))
			Expect(enableRule.AdditionalBranchPrefix).To(Equal("target-comp-"))
			Expect(enableRule.BranchTopic).To(Equal("source-comp"))
		})

		It("sets empty additional branch prefix when simpleBranchName is true", func() {
			config := tekton.GenerateRenovateConfig(target, buildResult, true)

			enableRule := config.PackageRules[1]
			Expect(enableRule.AdditionalBranchPrefix).To(BeEmpty())
		})

		It("includes DefaultRenovateLabel in labels", func() {
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			Expect(config.Labels).To(ContainElement(tektonconsts.DefaultRenovateLabel))
		})

		It("appends custom labels from CustomRenovateOptions", func() {
			target.ComponentCustomRenovateOptions = &tekton.CustomRenovateOptions{
				Labels: []string{"extra-label-1", "extra-label-2"},
			}
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			Expect(config.Labels).To(ContainElement(tektonconsts.DefaultRenovateLabel))
			Expect(config.Labels).To(ContainElement("extra-label-1"))
			Expect(config.Labels).To(ContainElement("extra-label-2"))
		})

		It("creates registry aliases for distribution repositories", func() {
			buildResult.DistributionRepositories = []string{
				"registry.access.redhat.com/org/image",
				"registry.redhat.io/org/image",
			}
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			Expect(config.RegistryAliases).To(HaveLen(2))
			Expect(config.RegistryAliases["registry.access.redhat.com/org/image"]).To(Equal("quay.io/org/image"))
			Expect(config.RegistryAliases["registry.redhat.io/org/image"]).To(Equal("quay.io/org/image"))

			// Distribution repos should also appear in matchStrings and matchPackageNames
			Expect(config.CustomManagers[0].MatchStrings).To(HaveLen(3))
			Expect(config.PackageRules[1].MatchPackageNames).To(HaveLen(3))
		})

		It("uses custom fileMatch from CustomRenovateOptions when provided", func() {
			target.ComponentCustomRenovateOptions = &tekton.CustomRenovateOptions{
				FileMatch: []string{"custom-dir/.*\\.yaml"},
			}
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			Expect(config.CustomManagers[0].FileMatch).To(Equal([]string{"custom-dir/.*\\.yaml"}))
		})

		It("propagates automerge settings from CustomRenovateOptions", func() {
			target.ComponentCustomRenovateOptions = &tekton.CustomRenovateOptions{
				Automerge:         true,
				PlatformAutomerge: true,
				IgnoreTests:       true,
				AutomergeType:     "pr",
			}
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			Expect(config.Automerge).To(BeTrue())
			Expect(config.PlatformAutomerge).To(BeTrue())
			Expect(config.IgnoreTests).To(BeTrue())
			Expect(config.AutomergeType).To(Equal("pr"))
		})

		It("propagates commit message settings from CustomRenovateOptions", func() {
			target.ComponentCustomRenovateOptions = &tekton.CustomRenovateOptions{
				CommitMessagePrefix: "chore(deps):",
				CommitMessageSuffix: "[skip-ci]",
			}
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			enableRule := config.PackageRules[1]
			Expect(enableRule.CommitMessagePrefix).To(Equal("chore(deps):"))
			Expect(enableRule.CommitMessageSuffix).To(Equal("[skip-ci]"))
		})

		It("sets static fields correctly", func() {
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			Expect(config.GitProvider).To(Equal("github"))
			Expect(config.Username).To(Equal("test-user"))
			Expect(config.Onboarding).To(BeFalse())
			Expect(config.RequireConfig).To(Equal("ignored"))
			Expect(config.EnabledManagers).To(Equal([]string{"custom.regex"}))
			Expect(config.ForkProcessing).To(Equal("enabled"))
			Expect(config.DependencyDashboard).To(BeFalse())
			Expect(config.Extends).To(BeEmpty())
		})

		It("includes PRHeader and CommitBody with git-repo-at-sha link", func() {
			config := tekton.GenerateRenovateConfig(target, buildResult, false)

			enableRule := config.PackageRules[1]
			Expect(enableRule.PRHeader).To(ContainSubstring(buildResult.GitRepoAtShaLink))
			Expect(enableRule.CommitBody).To(ContainSubstring(buildResult.GitRepoAtShaLink))
			Expect(enableRule.CommitBody).To(ContainSubstring("Signed-off-by:"))
		})
	})

	// ------------------------------------------------------------------
	// CreateNudgePipelineRun
	// ------------------------------------------------------------------
	Describe("CreateNudgePipelineRun", func() {
		var (
			fakeClient  client.Client
			nudgingPLR  *tektonv1.PipelineRun
			buildResult *tekton.NudgeBuildResult
			targets     []tekton.NudgeTarget
			testCtx     context.Context
		)

		BeforeEach(func() {
			testCtx = context.Background()

			scheme := nudgeTestScheme()
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "appstudio-pipeline", Namespace: "test-ns"},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sa).
				Build()

			nudgingPLR = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "build-plr-source",
					Namespace: "test-ns",
				},
				Spec: tektonv1.PipelineRunSpec{
					TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{},
				},
			}

			buildResult = &tekton.NudgeBuildResult{
				BuiltImageRepository: "quay.io/org/image",
				BuiltImageTag:        "v1",
				Digest:               "sha256:aaa",
				FileMatches:          ".*.yaml",
				SourceComponentName:  "source-comp",
				GitRepoAtShaLink:     "https://github.com/org/repo/commit/aabbccddaabbccddaabbccddaabbccddaabbccdd",
			}

			targets = []tekton.NudgeTarget{
				{
					ComponentName:           "downstream-comp",
					GitProvider:             "github",
					Username:                "test-user",
					GitAuthor:               "Test <test@example.com>",
					Token:                   "ghp_token",
					Endpoint:                "https://api.github.com",
					Repositories:            []tekton.RenovateRepository{{Repository: "org/downstream"}},
					ImageRepositoryHost:     "quay.io",
					ImageRepositoryUsername: "robot",
					ImageRepositoryPassword: "secret",
				},
			}
		})

		It("returns nil when targets is empty", func() {
			err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, nil, buildResult, false)
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a PipelineRun, Secret, and ConfigMap", func() {
			err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
			Expect(err).NotTo(HaveOccurred())

			// List PipelineRuns to find the created one
			plrList := &tektonv1.PipelineRunList{}
			Expect(fakeClient.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(plrList.Items).To(HaveLen(1))
			createdPLR := plrList.Items[0]

			// Verify labels
			Expect(createdPLR.Labels[tektonconsts.NudgeTypeLabel]).To(Equal(tektonconsts.NudgePipelineRunTypeValue))

			// Verify annotations
			Expect(createdPLR.Annotations[tektonconsts.NudgingComponentAnnotation]).To(Equal("source-comp"))
			Expect(createdPLR.Annotations[tektonconsts.NudgingImageAnnotation]).To(Equal("quay.io/org/image:v1"))
			Expect(createdPLR.Annotations[tektonconsts.NudgingPipelineAnnotation]).To(Equal("build-plr-source"))
			Expect(createdPLR.Annotations[tektonconsts.NudgedComponentsAnnotation]).To(Equal("downstream-comp"))

			// Verify ServiceAccount falls back to default
			Expect(createdPLR.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("appstudio-pipeline"))

			// Verify security context
			step := createdPLR.Spec.PipelineSpec.Tasks[0].TaskSpec.Steps[0]
			Expect(step.SecurityContext).NotTo(BeNil())
			Expect(step.SecurityContext.RunAsNonRoot).NotTo(BeNil())
			Expect(*step.SecurityContext.RunAsNonRoot).To(BeTrue())
			Expect(*step.SecurityContext.AllowPrivilegeEscalation).To(BeFalse())
			Expect(step.SecurityContext.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))
			Expect(step.SecurityContext.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))

			// Verify Secret was created
			secretList := &corev1.SecretList{}
			Expect(fakeClient.List(testCtx, secretList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(secretList.Items).To(HaveLen(1))

			// Verify ConfigMap was created
			cmList := &corev1.ConfigMapList{}
			Expect(fakeClient.List(testCtx, cmList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(cmList.Items).To(HaveLen(1))
		})

		It("uses the ServiceAccount from the source PLR when specified", func() {
			nudgingPLR.Spec.TaskRunTemplate.ServiceAccountName = "custom-sa"

			customSA := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-sa", Namespace: "test-ns"},
			}
			Expect(fakeClient.Create(testCtx, customSA)).To(Succeed())

			err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
			Expect(err).NotTo(HaveOccurred())

			plrList := &tektonv1.PipelineRunList{}
			Expect(fakeClient.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(plrList.Items).To(HaveLen(1))
			Expect(plrList.Items[0].Spec.TaskRunTemplate.ServiceAccountName).To(Equal("custom-sa"))
		})

		It("returns an error when the service account does not exist", func() {
			nudgingPLR.Spec.TaskRunTemplate.ServiceAccountName = "nonexistent-sa"

			err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("service account nonexistent-sa not found"))
		})

		It("creates multiple renovate commands for multiple targets", func() {
			targets = append(targets, tekton.NudgeTarget{
				ComponentName:           "another-downstream",
				GitProvider:             "gitlab",
				Username:                "gl-user",
				GitAuthor:               "GL <gl@example.com>",
				Token:                   "glpat_token",
				Endpoint:                "https://gitlab.com/api/v4",
				Repositories:            []tekton.RenovateRepository{{Repository: "org/another"}},
				ImageRepositoryHost:     "quay.io",
				ImageRepositoryUsername: "robot2",
				ImageRepositoryPassword: "secret2",
			})

			err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
			Expect(err).NotTo(HaveOccurred())

			plrList := &tektonv1.PipelineRunList{}
			Expect(fakeClient.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(plrList.Items).To(HaveLen(1))

			// ConfigMap should have two entries (one per target)
			cmList := &corev1.ConfigMapList{}
			Expect(fakeClient.List(testCtx, cmList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(cmList.Items).To(HaveLen(1))
			Expect(cmList.Items[0].Data).To(HaveLen(2))

			// Annotation should list both component names
			Expect(plrList.Items[0].Annotations[tektonconsts.NudgedComponentsAnnotation]).To(ContainSubstring("downstream-comp"))
			Expect(plrList.Items[0].Annotations[tektonconsts.NudgedComponentsAnnotation]).To(ContainSubstring("another-downstream"))
		})

		It("extracts the commit hash from GitRepoAtShaLink into annotation", func() {
			err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
			Expect(err).NotTo(HaveOccurred())

			plrList := &tektonv1.PipelineRunList{}
			Expect(fakeClient.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(plrList.Items[0].Annotations[tektonconsts.NudgingCommitAnnotation]).To(Equal("aabbccddaabbccddaabbccddaabbccddaabbccdd"))
		})

		Context("volume and envFrom setup", func() {
			It("mounts the config ConfigMap as a volume and the secret as envFrom", func() {
				err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
				Expect(err).NotTo(HaveOccurred())

				plrList := &tektonv1.PipelineRunList{}
				Expect(fakeClient.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())
				createdPLR := plrList.Items[0]

				step := createdPLR.Spec.PipelineSpec.Tasks[0].TaskSpec.Steps[0]

				// Verify envFrom references the secret with TOKEN_ prefix
				Expect(step.EnvFrom).To(HaveLen(1))
				Expect(step.EnvFrom[0].Prefix).To(Equal("TOKEN_"))
				Expect(step.EnvFrom[0].SecretRef).NotTo(BeNil())
				Expect(step.EnvFrom[0].SecretRef.Name).To(Equal(createdPLR.Name))

				// Verify volume mount for configs
				var foundConfigMount bool
				for _, vm := range step.VolumeMounts {
					if vm.MountPath == "/configs" {
						foundConfigMount = true
						Expect(vm.Name).To(Equal(createdPLR.Name))
					}
				}
				Expect(foundConfigMount).To(BeTrue(), "expected /configs volume mount")

				// Verify volume definition
				volumes := createdPLR.Spec.PipelineSpec.Tasks[0].TaskSpec.Volumes
				var foundConfigVolume bool
				for _, v := range volumes {
					if v.Name == createdPLR.Name {
						foundConfigVolume = true
						Expect(v.ConfigMap).NotTo(BeNil())
						Expect(v.ConfigMap.Name).To(Equal(createdPLR.Name))
					}
				}
				Expect(foundConfigVolume).To(BeTrue(), "expected ConfigMap volume")
			})
		})

		Context("naming consistency", func() {
			It("creates Secret and ConfigMap with the same name as the PipelineRun", func() {
				err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
				Expect(err).NotTo(HaveOccurred())

				plrList := &tektonv1.PipelineRunList{}
				Expect(fakeClient.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())
				plrName := plrList.Items[0].Name

				// Secret should have the same name
				secret := &corev1.Secret{}
				Expect(fakeClient.Get(testCtx, types.NamespacedName{Name: plrName, Namespace: "test-ns"}, secret)).To(Succeed())

				// ConfigMap should have the same name
				cm := &corev1.ConfigMap{}
				Expect(fakeClient.Get(testCtx, types.NamespacedName{Name: plrName, Namespace: "test-ns"}, cm)).To(Succeed())
			})
		})

		Context("ConfigMap data shape", func() {
			It("contains valid JSON config entries keyed by component name", func() {
				err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
				Expect(err).NotTo(HaveOccurred())

				cmList := &corev1.ConfigMapList{}
				Expect(fakeClient.List(testCtx, cmList, client.InNamespace("test-ns"))).To(Succeed())
				Expect(cmList.Items).To(HaveLen(1))

				// Verify the data has exactly one entry keyed like "downstream-comp-<random>.json"
				Expect(cmList.Items[0].Data).To(HaveLen(1))
				for key, value := range cmList.Items[0].Data {
					Expect(key).To(HavePrefix("downstream-comp-"))
					Expect(key).To(HaveSuffix(".json"))

					// Verify it's valid JSON that can be deserialized into RenovateConfig
					var config tekton.RenovateConfig
					Expect(json.Unmarshal([]byte(value), &config)).To(Succeed())
					Expect(config.GitProvider).To(Equal("github"))
					Expect(config.Repositories).To(HaveLen(1))
				}
			})
		})

		Context("Secret data shape", func() {
			It("contains token entries for each target", func() {
				err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
				Expect(err).NotTo(HaveOccurred())

				secretList := &corev1.SecretList{}
				Expect(fakeClient.List(testCtx, secretList, client.InNamespace("test-ns"))).To(Succeed())
				Expect(secretList.Items).To(HaveLen(1))

				// The Secret is created with StringData (the fake client does not
				// auto-convert StringData to Data like a real API server).
				secret := secretList.Items[0]
				Expect(secret.StringData).To(HaveLen(2))

				// Verify the actual token values are stored
				var foundGitToken, foundRegistryPassword bool
				for _, v := range secret.StringData {
					if v == "ghp_token" {
						foundGitToken = true
					}
					if v == "secret" {
						foundRegistryPassword = true
					}
				}
				Expect(foundGitToken).To(BeTrue(), "expected git token to be stored in secret")
				Expect(foundRegistryPassword).To(BeTrue(), "expected registry password to be stored in secret")
			})
		})

		Context("renovate image", func() {
			It("uses default renovate image when env var is not set", func() {
				GinkgoT().Setenv(tektonconsts.RenovateImageEnvName, "")

				err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
				Expect(err).NotTo(HaveOccurred())

				plrList := &tektonv1.PipelineRunList{}
				Expect(fakeClient.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())

				image := plrList.Items[0].Spec.PipelineSpec.Tasks[0].TaskSpec.TaskSpec.Steps[0].Image
				Expect(image).To(Equal(tektonconsts.DefaultRenovateImageUrl))
			})

			It("uses custom renovate image when env var is set", func() {
				GinkgoT().Setenv(tektonconsts.RenovateImageEnvName, "my-custom-renovate:v99")

				err := tekton.CreateNudgePipelineRun(testCtx, fakeClient, nudgingPLR, targets, buildResult, false)
				Expect(err).NotTo(HaveOccurred())

				plrList := &tektonv1.PipelineRunList{}
				Expect(fakeClient.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())

				image := plrList.Items[0].Spec.PipelineSpec.Tasks[0].TaskSpec.TaskSpec.Steps[0].Image
				Expect(image).To(Equal("my-custom-renovate:v99"))
			})
		})

		Context("with CA bundle", func() {
			It("mounts the CA ConfigMap volume when a CA bundle exists in build-service namespace", func() {
				scheme := nudgeTestScheme()

				sa := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{Name: "appstudio-pipeline", Namespace: "test-ns"},
				}

				caCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "trusted-ca-bundle",
						Namespace: tektonconsts.BuildServiceNamespaceName,
						Labels:    map[string]string{tektonconsts.CaConfigMapLabel: "true"},
					},
					Data: map[string]string{
						tektonconsts.CaConfigMapKey: "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----",
					},
				}

				buildNS := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.BuildServiceNamespaceName},
				}

				caFakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(sa, caCM, buildNS).
					Build()

				err := tekton.CreateNudgePipelineRun(testCtx, caFakeClient, nudgingPLR, targets, buildResult, false)
				Expect(err).NotTo(HaveOccurred())

				plrList := &tektonv1.PipelineRunList{}
				Expect(caFakeClient.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())
				Expect(plrList.Items).To(HaveLen(1))
				createdPLR := plrList.Items[0]

				// Verify the CA volume was added
				volumes := createdPLR.Spec.PipelineSpec.Tasks[0].TaskSpec.Volumes
				var foundCAVolume bool
				for _, v := range volumes {
					if v.Name == tektonconsts.CaVolumeMountName {
						foundCAVolume = true
						Expect(v.ConfigMap).NotTo(BeNil())
					}
				}
				Expect(foundCAVolume).To(BeTrue(), "expected CA volume to be present")

				// Verify the CA volume mount was added
				volumeMounts := createdPLR.Spec.PipelineSpec.Tasks[0].TaskSpec.TaskSpec.Steps[0].VolumeMounts
				var foundCAMount bool
				for _, vm := range volumeMounts {
					if vm.Name == tektonconsts.CaVolumeMountName {
						foundCAMount = true
						Expect(vm.MountPath).To(Equal(tektonconsts.CaMountPath))
						Expect(vm.ReadOnly).To(BeTrue())
					}
				}
				Expect(foundCAMount).To(BeTrue(), "expected CA volume mount to be present")

				// Verify the CA ConfigMap was created in test-ns
				caConfigMaps := &corev1.ConfigMapList{}
				Expect(caFakeClient.List(testCtx, caConfigMaps, client.InNamespace("test-ns"))).To(Succeed())
				// Should have 2 ConfigMaps: the renovate config + the CA
				Expect(caConfigMaps.Items).To(HaveLen(2))

				var foundCACM bool
				for _, cm := range caConfigMaps.Items {
					if _, ok := cm.Data[tektonconsts.CaConfigMapKey]; ok {
						foundCACM = true
					}
				}
				Expect(foundCACM).To(BeTrue(), "expected CA ConfigMap to be created in test-ns")
			})
		})
	})

	// ------------------------------------------------------------------
	// ReadCustomRenovateConfigMap
	// ------------------------------------------------------------------
	Describe("ReadCustomRenovateConfigMap", func() {
		var (
			fakeClient client.Client
			component  *applicationapiv1alpha1.Component
			testCtx    context.Context
		)

		BeforeEach(func() {
			testCtx = context.Background()
			component = nudgeSampleComponent("test-comp", "test-ns", nil)
		})

		It("returns zero-value options when no ConfigMap exists", func() {
			fakeClient = fake.NewClientBuilder().WithScheme(nudgeTestScheme()).Build()

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts).NotTo(BeNil())
			Expect(opts.Automerge).To(BeFalse())
		})

		It("reads from per-component annotation-referenced ConfigMap", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-renovate-cm", Namespace: "test-ns"},
				Data: map[string]string{
					"automerge":   "true",
					"ignoreTests": "true",
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(nudgeTestScheme()).
				WithObjects(cm).
				Build()

			component.Annotations = map[string]string{
				tektonconsts.CustomRenovateConfigMapAnnotation: "custom-renovate-cm",
			}

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.Automerge).To(BeTrue())
			Expect(opts.IgnoreTests).To(BeTrue())
		})

		It("falls back to namespace-wide ConfigMap when no annotation is set", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.NamespaceWideRenovateConfigMapName, Namespace: "test-ns"},
				Data: map[string]string{
					"automergeType": "branch",
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(nudgeTestScheme()).
				WithObjects(cm).
				Build()

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.AutomergeType).To(Equal("branch"))
		})

		It("returns zero-value when annotation-referenced ConfigMap does not exist (not-found is not an error)", func() {
			fakeClient = fake.NewClientBuilder().WithScheme(nudgeTestScheme()).Build()

			component.Annotations = map[string]string{
				tektonconsts.CustomRenovateConfigMapAnnotation: "nonexistent-cm",
			}

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts).NotTo(BeNil())
		})

		It("defaults platformAutomerge to true when key is absent", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.NamespaceWideRenovateConfigMapName, Namespace: "test-ns"},
				Data:       map[string]string{},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(nudgeTestScheme()).
				WithObjects(cm).
				Build()

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.PlatformAutomerge).To(BeTrue())
		})

		It("parses platformAutomerge as false when explicitly set", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.NamespaceWideRenovateConfigMapName, Namespace: "test-ns"},
				Data: map[string]string{
					"platformAutomerge": "false",
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(nudgeTestScheme()).
				WithObjects(cm).
				Build()

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.PlatformAutomerge).To(BeFalse())
		})

		It("parses comma-separated fileMatch", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.NamespaceWideRenovateConfigMapName, Namespace: "test-ns"},
				Data: map[string]string{
					"fileMatch": ".*Dockerfile.*, .*.yaml, .*Containerfile.*",
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(nudgeTestScheme()).
				WithObjects(cm).
				Build()

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.FileMatch).To(Equal([]string{".*Dockerfile.*", ".*.yaml", ".*Containerfile.*"}))
		})

		It("parses semicolon-separated automergeSchedule", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.NamespaceWideRenovateConfigMapName, Namespace: "test-ns"},
				Data: map[string]string{
					"automergeSchedule": "after 10pm; before 5am",
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(nudgeTestScheme()).
				WithObjects(cm).
				Build()

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.AutomergeSchedule).To(Equal([]string{"after 10pm", "before 5am"}))
		})

		It("parses comma-separated labels", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.NamespaceWideRenovateConfigMapName, Namespace: "test-ns"},
				Data: map[string]string{
					"labels": "label-a, label-b",
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(nudgeTestScheme()).
				WithObjects(cm).
				Build()

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.Labels).To(Equal([]string{"label-a", "label-b"}))
		})

		It("reads commitMessagePrefix and commitMessageSuffix", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.NamespaceWideRenovateConfigMapName, Namespace: "test-ns"},
				Data: map[string]string{
					"commitMessagePrefix": "chore:",
					"commitMessageSuffix": "[auto]",
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(nudgeTestScheme()).
				WithObjects(cm).
				Build()

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.CommitMessagePrefix).To(Equal("chore:"))
			Expect(opts.CommitMessageSuffix).To(Equal("[auto]"))
		})

		It("parses gitLabIgnoreApprovals", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.NamespaceWideRenovateConfigMapName, Namespace: "test-ns"},
				Data: map[string]string{
					"gitLabIgnoreApprovals": "true",
				},
			}
			fakeClient = fake.NewClientBuilder().
				WithScheme(nudgeTestScheme()).
				WithObjects(cm).
				Build()

			opts, err := tekton.ReadCustomRenovateConfigMap(testCtx, fakeClient, component)
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.GitLabIgnoreApprovals).To(BeTrue())
		})
	})

	// ------------------------------------------------------------------
	// Helper functions (tested via exported API)
	// ------------------------------------------------------------------
	Describe("RandomString", func() {
		It("returns a string of the requested length", func() {
			s := tekton.RandomString(10)
			Expect(s).To(HaveLen(10))
		})

		It("returns different values on successive calls", func() {
			s1 := tekton.RandomString(16)
			s2 := tekton.RandomString(16)
			Expect(s1).NotTo(Equal(s2))
		})

		It("returns only hex characters", func() {
			s := tekton.RandomString(20)
			Expect(s).To(MatchRegexp("^[a-f0-9]+$"))
		})
	})

	// joinNudgedComponentNames and extractCommitHash are unexported, so we
	// test them indirectly through CreateNudgePipelineRun annotations.
	Describe("unexported helpers (tested indirectly via CreateNudgePipelineRun)", func() {
		It("joinNudgedComponentNames joins multiple target names with spaces", func() {
			testCtx := context.Background()
			scheme := nudgeTestScheme()
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "appstudio-pipeline", Namespace: "test-ns"},
			}
			fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sa).Build()

			plr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "build-plr", Namespace: "test-ns"},
				Spec:       tektonv1.PipelineRunSpec{},
			}
			buildResult := &tekton.NudgeBuildResult{
				BuiltImageRepository: "quay.io/org/img",
				BuiltImageTag:        "v1",
				Digest:               "sha256:aaa",
				FileMatches:          ".*.yaml",
				SourceComponentName:  "src",
				GitRepoAtShaLink:     "https://github.com/o/r/commit/aabbccddaabbccddaabbccddaabbccddaabbccdd",
			}
			targets := []tekton.NudgeTarget{
				{ComponentName: "comp-a", GitProvider: "github", Username: "u", GitAuthor: "A <a@b.c>",
					Token: "t", Endpoint: "https://api.github.com",
					Repositories:        []tekton.RenovateRepository{{Repository: "o/a"}},
					ImageRepositoryHost: "quay.io", ImageRepositoryUsername: "r", ImageRepositoryPassword: "s"},
				{ComponentName: "comp-b", GitProvider: "github", Username: "u", GitAuthor: "A <a@b.c>",
					Token: "t", Endpoint: "https://api.github.com",
					Repositories:        []tekton.RenovateRepository{{Repository: "o/b"}},
					ImageRepositoryHost: "quay.io", ImageRepositoryUsername: "r", ImageRepositoryPassword: "s"},
				{ComponentName: "comp-c", GitProvider: "github", Username: "u", GitAuthor: "A <a@b.c>",
					Token: "t", Endpoint: "https://api.github.com",
					Repositories:        []tekton.RenovateRepository{{Repository: "o/c"}},
					ImageRepositoryHost: "quay.io", ImageRepositoryUsername: "r", ImageRepositoryPassword: "s"},
			}

			Expect(tekton.CreateNudgePipelineRun(testCtx, fc, plr, targets, buildResult, false)).To(Succeed())

			plrList := &tektonv1.PipelineRunList{}
			Expect(fc.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())
			Expect(plrList.Items[0].Annotations[tektonconsts.NudgedComponentsAnnotation]).To(Equal("comp-a comp-b comp-c"))
		})

		DescribeTable("extractCommitHash tested via nudging-commit annotation",
			func(gitRepoAtShaLink, expectedCommit string) {
				testCtx := context.Background()
				scheme := nudgeTestScheme()
				sa := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{Name: "appstudio-pipeline", Namespace: "test-ns"},
				}
				fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sa).Build()

				plr := &tektonv1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{Name: "build-plr", Namespace: "test-ns"},
					Spec:       tektonv1.PipelineRunSpec{},
				}
				buildResult := &tekton.NudgeBuildResult{
					BuiltImageRepository: "quay.io/org/img",
					BuiltImageTag:        "v1",
					Digest:               "sha256:aaa",
					FileMatches:          ".*.yaml",
					SourceComponentName:  "src",
					GitRepoAtShaLink:     gitRepoAtShaLink,
				}
				targets := []tekton.NudgeTarget{{
					ComponentName: "d", GitProvider: "github", Username: "u", GitAuthor: "A <a@b.c>",
					Token: "t", Endpoint: "https://api.github.com",
					Repositories:        []tekton.RenovateRepository{{Repository: "o/d"}},
					ImageRepositoryHost: "quay.io", ImageRepositoryUsername: "r", ImageRepositoryPassword: "s",
				}}

				Expect(tekton.CreateNudgePipelineRun(testCtx, fc, plr, targets, buildResult, false)).To(Succeed())

				plrList := &tektonv1.PipelineRunList{}
				Expect(fc.List(testCtx, plrList, client.InNamespace("test-ns"))).To(Succeed())
				Expect(plrList.Items[0].Annotations[tektonconsts.NudgingCommitAnnotation]).To(Equal(expectedCommit))
			},
			Entry("valid GitHub commit URL",
				"https://github.com/org/repo/commit/aabbccddaabbccddaabbccddaabbccddaabbccdd",
				"aabbccddaabbccddaabbccddaabbccddaabbccdd",
			),
			Entry("URL without a valid hash at the end",
				"https://github.com/org/repo/tree/main",
				"",
			),
			Entry("empty string",
				"",
				"",
			),
		)
	})
})
