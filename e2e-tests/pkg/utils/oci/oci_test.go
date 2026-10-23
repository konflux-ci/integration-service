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

package oci_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils/oci"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/pkg/tracing"
	"github.com/konflux-ci/integration-service/tekton/consts"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
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
			oci.UpdatePipelineRunNamespace(plr, "new-namespace")
			Expect(plr.Namespace).To(Equal("new-namespace"))
		})

		It("should update the ObjectMeta namespace field", func() {
			oci.UpdatePipelineRunNamespace(plr, "new-namespace")
			Expect(plr.Namespace).To(Equal("new-namespace"))
		})

		It("should handle empty original namespace", func() {
			plr.Namespace = ""
			oci.UpdatePipelineRunNamespace(plr, "test-namespace")
			Expect(plr.Namespace).To(Equal("test-namespace"))
		})

		It("should overwrite existing namespace", func() {
			plr.Namespace = "original"
			oci.UpdatePipelineRunNamespace(plr, "replacement")
			Expect(plr.Namespace).To(Equal("replacement"))
			Expect(plr.Namespace).To(Equal("replacement"))
		})

		It("should handle very long namespace names", func() {
			longNamespace := "very-long-namespace-name-with-many-characters-to-test-edge-cases"
			oci.UpdatePipelineRunNamespace(plr, longNamespace)
			Expect(plr.Namespace).To(Equal(longNamespace))
			Expect(plr.Namespace).To(Equal(longNamespace))
		})

		It("should handle namespace with special characters", func() {
			specialNamespace := "test-ns-123"
			oci.UpdatePipelineRunNamespace(plr, specialNamespace)
			Expect(plr.Namespace).To(Equal(specialNamespace))
			Expect(plr.Namespace).To(Equal(specialNamespace))
		})

		It("should handle multiple consecutive updates", func() {
			oci.UpdatePipelineRunNamespace(plr, "ns1")
			Expect(plr.Namespace).To(Equal("ns1"))
			oci.UpdatePipelineRunNamespace(plr, "ns2")
			Expect(plr.Namespace).To(Equal("ns2"))
			oci.UpdatePipelineRunNamespace(plr, "ns3")
			Expect(plr.Namespace).To(Equal("ns3"))
		})

		It("should preserve other PipelineRun fields when updating namespace", func() {
			originalName := plr.Name
			oci.UpdatePipelineRunNamespace(plr, "new-ns")
			Expect(plr.Name).To(Equal(originalName))
		})

		DescribeTable("should update namespace for various inputs",
			func(newNs string) {
				oci.UpdatePipelineRunNamespace(plr, newNs)
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
				oci.PreparePipelineRunForPrebuiltUsage(plr, "new-namespace", "new-plr", "new-app", "new-component")
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

			oci.PreparePipelineRunForPrebuiltUsage(plr, "new-namespace", "new-plr", "new-app", "new-component")

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
		DescribeTable("invalid image references",
			func(imageName, tag, expectedError string) {
				_, err := oci.FetchPipelineRunFromOCIArtifact(imageName, tag)
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
				_, err := oci.FetchPipelineRunFromOCIArtifact("quay.io/nonexistent/image", "nonexistent-tag-xyz")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("resolve image reference"))
			})

			It("should return an error with context about the failure", func() {
				_, err := oci.FetchPipelineRunFromOCIArtifact("quay.io/library/nonexistent", "latest")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("error handling in FetchPipelineRunFromOCIArtifact", func() {
			It("should wrap parse errors with context", func() {
				_, err := oci.FetchPipelineRunFromOCIArtifact("!!invalid!!", "latest")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("parse image reference"))
			})

			It("should handle image references with ports", func() {
				_, err := oci.FetchPipelineRunFromOCIArtifact("localhost:5000/nonexistent", "latest")
				Expect(err).To(HaveOccurred())
			})

		})

		// Note: Testing successful fetch and layer processing requires:
		// 1. A real OCI artifact at a registry with correct PipelineRun JSON structure
		// 2. Or complex mocking of the go-containerregistry library
		// Successful fetch is validated through e2e tests with actual OCI artifacts.
	})
})
