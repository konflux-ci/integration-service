/*
Copyright 2024 Red Hat Inc.

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

package oci

import (
	"bytes"
	"encoding/json"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/pkg/tracing"
	"github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("OCI Utilities", func() {

	Describe("UpdatePipelineRunNamespace", func() {
		var plr *tektonv1.PipelineRun

		BeforeEach(func() {
			plr = &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-plr",
					Namespace: "old-namespace",
				},
				Spec: tektonv1.PipelineRunSpec{},
			}
		})

		It("should update the namespace field", func() {
			UpdatePipelineRunNamespace(plr, "new-namespace")
			Expect(plr.Namespace).To(Equal("new-namespace"))
		})

		It("should handle empty original namespace", func() {
			plr.Namespace = ""
			UpdatePipelineRunNamespace(plr, "test-namespace")
			Expect(plr.Namespace).To(Equal("test-namespace"))
		})

		It("should overwrite existing namespace", func() {
			plr.Namespace = "original"
			UpdatePipelineRunNamespace(plr, "replacement")
			Expect(plr.Namespace).To(Equal("replacement"))
			Expect(plr.Namespace).To(Equal("replacement"))
		})

		It("should handle very long namespace names", func() {
			longNamespace := "very-long-namespace-name-with-many-characters-to-test-edge-cases"
			UpdatePipelineRunNamespace(plr, longNamespace)
			Expect(plr.Namespace).To(Equal(longNamespace))
			Expect(plr.Namespace).To(Equal(longNamespace))
		})

		It("should handle namespace with special characters", func() {
			specialNamespace := "test-ns-123"
			UpdatePipelineRunNamespace(plr, specialNamespace)
			Expect(plr.Namespace).To(Equal(specialNamespace))
			Expect(plr.Namespace).To(Equal(specialNamespace))
		})

		It("should handle multiple consecutive updates", func() {
			UpdatePipelineRunNamespace(plr, "ns1")
			Expect(plr.Namespace).To(Equal("ns1"))
			UpdatePipelineRunNamespace(plr, "ns2")
			Expect(plr.Namespace).To(Equal("ns2"))
			UpdatePipelineRunNamespace(plr, "ns3")
			Expect(plr.Namespace).To(Equal("ns3"))
		})

		It("should preserve other PipelineRun fields when updating namespace", func() {
			originalName := plr.Name
			UpdatePipelineRunNamespace(plr, "new-ns")
			Expect(plr.Name).To(Equal(originalName))
		})

		DescribeTable("should update namespace for various inputs",
			func(newNs string) {
				UpdatePipelineRunNamespace(plr, newNs)
				Expect(plr.Namespace).To(Equal(newNs))
				Expect(plr.Namespace).To(Equal(newNs))
			},
			Entry("simple namespace", "default"),
			Entry("hyphenated namespace", "test-namespace"),
			Entry("numeric suffix", "ns-123"),
			Entry("single character", "a"),
		)
	})

	Describe("PreparePipelineRunForPrebuiltUsage", func() {
		It("handles PipelineRuns without integration-service metadata", func() {
			plr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "old-plr",
					Namespace: "old-namespace",
				},
			}

			Expect(func() {
				PreparePipelineRunForPrebuiltUsage(plr, "new-namespace", "new-plr", "new-app", "new-component")
			}).NotTo(Panic())
			Expect(plr.Name).To(Equal("new-plr"))
			Expect(plr.Namespace).To(Equal("new-namespace"))
			Expect(plr.Labels).To(HaveKeyWithValue("appstudio.openshift.io/application", "new-app"))
			Expect(plr.Labels).To(HaveKeyWithValue("appstudio.openshift.io/component", "new-component"))
			Expect(string(plr.Spec.Status)).To(Equal(tektonv1.PipelineRunSpecStatusPending))
		})

		It("removes stale integration-service annotations and labels before reuse", func() {
			plr := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "old-plr",
					Namespace: "old-namespace",
					Annotations: map[string]string{
						consts.SnapshotNameLabel:                    "old-snapshot",
						consts.SnapshotNamesLabel:                   "old-snapshot",
						helpers.CreateSnapshotAnnotationName:        "old-status",
						helpers.SnapshotCreationReportAnnotation:    "old-report",
						gitops.PRGroupAnnotation:                    "old-pr-group",
						gitops.PRGroupCreationAnnotation:            "old-group-status",
						gitops.PRStatusAnnotation:                   "merged",
						gitops.AddedToGlobalCandidateListAnnotation: "true",
						gitops.PRGroupCancelledAnnotation:           "true",
						consts.NudgeProcessedAnnotation:             "component-b",
						tracing.TimingEmittedAnnotation:             "true",
						"test.appstudio.openshift.io/future-key":    "future-value",
						consts.PipelineRunChainsSignedAnnotation:    "true",
						"build.appstudio.openshift.io/repo":         "https://example.test/repo",
					},
					Labels: map[string]string{
						gitops.PRGroupHashLabel:                  "old-hash",
						"test.appstudio.openshift.io/future-key": "future-value",
						consts.PipelineRunTypeLabel:              consts.PipelineRunBuildType,
					},
				},
			}

			PreparePipelineRunForPrebuiltUsage(plr, "new-namespace", "new-plr", "new-app", "new-component")

			Expect(plr.Name).To(Equal("new-plr"))
			Expect(plr.Namespace).To(Equal("new-namespace"))
			Expect(plr.Annotations).NotTo(HaveKey(consts.SnapshotNameLabel))
			Expect(plr.Annotations).NotTo(HaveKey(consts.SnapshotNamesLabel))
			Expect(plr.Annotations).NotTo(HaveKey(helpers.CreateSnapshotAnnotationName))
			Expect(plr.Annotations).NotTo(HaveKey(helpers.SnapshotCreationReportAnnotation))
			Expect(plr.Annotations).NotTo(HaveKey(gitops.PRGroupAnnotation))
			Expect(plr.Annotations).NotTo(HaveKey(gitops.PRGroupCreationAnnotation))
			Expect(plr.Annotations).NotTo(HaveKey(gitops.PRStatusAnnotation))
			Expect(plr.Annotations).NotTo(HaveKey(gitops.AddedToGlobalCandidateListAnnotation))
			Expect(plr.Annotations).NotTo(HaveKey(gitops.PRGroupCancelledAnnotation))
			Expect(plr.Annotations).NotTo(HaveKey(consts.NudgeProcessedAnnotation))
			Expect(plr.Annotations).NotTo(HaveKey(tracing.TimingEmittedAnnotation))
			Expect(plr.Annotations).NotTo(HaveKey(consts.PipelineRunChainsSignedAnnotation))
			Expect(plr.Annotations).NotTo(HaveKey("test.appstudio.openshift.io/future-key"))
			Expect(plr.Labels).NotTo(HaveKey(gitops.PRGroupHashLabel))
			Expect(plr.Labels).NotTo(HaveKey("test.appstudio.openshift.io/future-key"))
			Expect(plr.Annotations).To(HaveKeyWithValue("build.appstudio.openshift.io/repo", "https://example.test/repo"))
			Expect(plr.Labels).To(HaveKeyWithValue("appstudio.openshift.io/application", "new-app"))
			Expect(plr.Labels).To(HaveKeyWithValue("appstudio.openshift.io/component", "new-component"))
			Expect(plr.Labels).To(HaveKeyWithValue(consts.PipelineRunTypeLabel, consts.PipelineRunBuildType))
		})
	})

	Describe("FetchPipelineRunFromOCIArtifact", func() {

		Context("when a PipelineRun artifact is found", func() {
			var (
				originalRemoteHead      func(name.Reference, ...remote.Option) (*v1.Descriptor, error)
				originalRemoteReferrers func(name.Digest, ...remote.Option) (v1.ImageIndex, error)
				originalRemoteGet       func(name.Reference, ...remote.Option) (v1.Image, error)
			)

			BeforeEach(func() {
				originalRemoteHead = remoteHead
				originalRemoteReferrers = remoteReferrers
				originalRemoteGet = remoteGet
			})

			AfterEach(func() {
				remoteHead = originalRemoteHead
				remoteReferrers = originalRemoteReferrers
				remoteGet = originalRemoteGet
			})

			It("should fetch and unmarshal the PipelineRun from the first referrer artifact", func() {
				subjectDigest := v1.Hash{Algorithm: "sha256", Hex: "1111111111111111111111111111111111111111111111111111111111111111"}
				artifactDigest := v1.Hash{Algorithm: "sha256", Hex: "2222222222222222222222222222222222222222222222222222222222222222"}

				expectedPipelineRun := &tektonv1.PipelineRun{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "tekton.dev/v1",
						Kind:       "PipelineRun",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pipelinerun",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"appstudio.openshift.io/component": "test-component",
						},
					},
				}
				pipelineRunJSON, err := json.Marshal(expectedPipelineRun)
				Expect(err).NotTo(HaveOccurred())

				pipelineRunLayer, err := tarball.LayerFromReader(bytes.NewReader(pipelineRunJSON), tarball.WithMediaType(types.OCILayer))
				Expect(err).NotTo(HaveOccurred())
				artifactImage, err := mutate.AppendLayers(empty.Image, pipelineRunLayer)
				Expect(err).NotTo(HaveOccurred())
				referrers := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{
					Add:        empty.Image,
					Descriptor: v1.Descriptor{Digest: artifactDigest},
				})

				remoteHead = func(ref name.Reference, options ...remote.Option) (*v1.Descriptor, error) {
					Expect(ref.Name()).To(Equal("registry.example.com/team/app:latest"))
					return &v1.Descriptor{Digest: subjectDigest}, nil
				}
				remoteReferrers = func(digest name.Digest, options ...remote.Option) (v1.ImageIndex, error) {
					Expect(digest.Name()).To(Equal("registry.example.com/team/app@" + subjectDigest.String()))
					return referrers, nil
				}
				remoteGet = func(ref name.Reference, options ...remote.Option) (v1.Image, error) {
					Expect(ref.Name()).To(Equal("registry.example.com/team/app@" + artifactDigest.String()))
					return artifactImage, nil
				}

				pipelineRun, err := FetchPipelineRunFromOCIArtifact("registry.example.com/team/app", "latest")
				Expect(err).NotTo(HaveOccurred())
				Expect(pipelineRun.Name).To(Equal(expectedPipelineRun.Name))
				Expect(pipelineRun.Namespace).To(Equal(expectedPipelineRun.Namespace))
				Expect(pipelineRun.Labels).To(HaveKeyWithValue("appstudio.openshift.io/component", "test-component"))
			})
		})

		DescribeTable("invalid image references",
			func(imageName, tag, expectedError string) {
				_, err := FetchPipelineRunFromOCIArtifact(imageName, tag)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedError))
			},
			Entry("malformed with special chars", "not a valid reference!!!!", "latest", "parse image reference"),
			Entry("malformed with spaces", "not valid ref", "latest", "parse image reference"),
			Entry("empty image", "", "latest", "parse image reference"),
			Entry("invalid URL format", ":::invalid", "latest", "parse image reference"),
			Entry("invalid image format", "quay.io/@invalid", "latest", "parse image reference"),
		)

		Context("when image reference is valid but image does not exist", func() {
			It("should return an error for non-existent image", func() {
				_, err := FetchPipelineRunFromOCIArtifact("quay.io/nonexistent/image", "nonexistent-tag-xyz")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("resolve image reference"))
			})

			It("should return an error with context about the failure", func() {
				_, err := FetchPipelineRunFromOCIArtifact("quay.io/library/nonexistent", "latest")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("error handling in FetchPipelineRunFromOCIArtifact", func() {
			It("should wrap parse errors with context", func() {
				_, err := FetchPipelineRunFromOCIArtifact("!!invalid!!", "latest")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("parse image reference"))
			})

			It("should handle image references with ports", func() {
				_, err := FetchPipelineRunFromOCIArtifact("localhost:5000/nonexistent", "latest")
				Expect(err).To(HaveOccurred())
			})

		})
	})
})
