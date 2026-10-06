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

package nudging

import (
	"context"
	"fmt"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _ = Describe("Validated batched nudge recording", func() {
	const (
		namespace = "test-ns"
		target    = "nudge-target"
		source    = "nudge-source"
	)

	var (
		scheme      *runtime.Scheme
		nudgeConfig *v1beta2.NudgeConfig
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(v1beta2.AddToScheme(scheme)).To(Succeed())
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
		Expect(applicationapiv1alpha1.AddToScheme(scheme)).To(Succeed())

		nudgeConfig = &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      v1beta2.NudgeConfigSingletonName,
				Namespace: namespace,
			},
			Spec: v1beta2.NudgeConfigSpec{
				TargetConfig: []v1beta2.TargetConfig{
					{Target: target, BatchPolicy: &v1beta2.BatchPolicy{}},
				},
			},
		}
	})

	newClient := func(objects ...client.Object) client.Client {
		builder := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1beta2.NudgeConfig{})
		if len(objects) > 0 {
			builder = builder.WithObjects(objects...)
		}
		return builder.Build()
	}

	It("requires a digest for validated snapshot recording", func() {
		c := newClient(nudgeConfig)
		err := RecordValidatedSnapshotForBatchedNudge(ctx, c, nudgeConfig, target, source, "snap-1", "")
		Expect(err).To(MatchError(ContainSubstring("digest is required")))
	})

	It("builds a nudge result from persisted accumulation metadata", func() {
		sourceComponent := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: source, Namespace: namespace},
		}
		entry := v1beta2.AccumulatedEntry{
			From:                 source,
			ImageDigest:          "sha256:persisted",
			BuildPipelineRun:     "pruned-build-plr",
			BuiltImageRepository: "quay.io/org/image",
			BuiltImageTag:        "v3",
			NudgeFileMatches:     "Containerfile",
			GitRepoAtShaLink:     "https://github.com/org/repo/commit/abc",
		}
		c := newClient(nudgeConfig, sourceComponent)

		result, err := buildResultForAccumulatedEntry(ctx, c, namespace, entry, sourceComponent, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.BuiltImageRepository).To(Equal("quay.io/org/image"))
		Expect(result.BuiltImageTag).To(Equal("v3"))
		Expect(result.FileMatches).To(Equal("Containerfile"))
		Expect(result.GitRepoAtShaLink).To(Equal("https://github.com/org/repo/commit/abc"))
	})

	It("defaults file matches when accumulation metadata omits them", func() {
		sourceComponent := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: source, Namespace: namespace},
		}
		entry := v1beta2.AccumulatedEntry{
			From:                 source,
			ImageDigest:          "sha256:persisted",
			BuiltImageRepository: "quay.io/org/image",
		}
		c := newClient(nudgeConfig, sourceComponent)

		result, err := buildResultForAccumulatedEntry(ctx, c, namespace, entry, sourceComponent, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.FileMatches).To(Equal(tektonconsts.DefaultNudgeFiles))
	})

	It("records a validated snapshot digest on the active batch", func() {
		c := newClient(nudgeConfig)
		Expect(RecordValidatedSnapshotForBatchedNudge(ctx, c, nudgeConfig, target, source, "snap-1", "sha256:validated")).To(Succeed())

		updated := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
		Expect(updated.Status.ActiveBatches).To(HaveLen(1))
		Expect(updated.Status.ActiveBatches[0].Accumulated).To(HaveLen(1))
		Expect(updated.Status.ActiveBatches[0].Accumulated[0].ImageDigest).To(Equal("sha256:validated"))
		Expect(updated.Status.ActiveBatches[0].Accumulated[0].BuildPipelineRun).To(Equal("snap-1"))
	})

	It("records validated snapshot failures with a default reason", func() {
		c := newClient(nudgeConfig)
		Expect(RecordFailedValidatedSnapshotForBatchedNudge(ctx, c, nudgeConfig, target, source, "snap-fail", "")).To(Succeed())

		updated := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
		Expect(updated.Status.ActiveBatches[0].Failed).To(HaveLen(1))
		Expect(updated.Status.ActiveBatches[0].Failed[0].Reason).To(Equal("integration tests failed"))
	})

	It("builds renovate payload from accumulated digests when build PLRs are gone", func() {
		sourceComponent := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: source, Namespace: namespace},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ContainerImage: "quay.io/org/source:latest",
			},
		}
		targetComponent := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: target, Namespace: namespace},
			Spec: applicationapiv1alpha1.ComponentSpec{
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{URL: "https://github.com/org/repo"},
					},
				},
			},
		}
		referencePLR := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "reference-plr",
				Namespace: namespace,
				Annotations: map[string]string{
					tektonconsts.GitRepoAtShaAnnotation: "https://github.com/org/repo/commit/aabbccddaabbccddaabbccddaabbccddaabbccdd",
				},
			},
		}
		batch := &v1beta2.ActiveBatch{
			Target: target,
			Accumulated: []v1beta2.AccumulatedEntry{
				{
					From:             source,
					ImageDigest:      "sha256:from-status",
					BuildPipelineRun: "deleted-build-plr",
				},
			},
		}
		c := newClient(nudgeConfig, sourceComponent, targetComponent)

		result, err := buildResultForAccumulatedEntry(ctx, c, namespace, batch.Accumulated[0], sourceComponent, referencePLR)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Digest).To(Equal("sha256:from-status"))
		Expect(result.BuiltImageRepository).To(Equal("quay.io/org/source"))
		Expect(result.GitRepoAtShaLink).To(Equal(referencePLR.Annotations[tektonconsts.GitRepoAtShaAnnotation]))
	})

	It("honours component nudge file annotations when synthesizing build results from digests", func() {
		sourceComponent := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      source,
				Namespace: namespace,
				Annotations: map[string]string{
					tektonconsts.NudgeFilesAnnotation: "Dockerfile.dev",
				},
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ContainerImage: "quay.io/org/source:prod",
			},
		}
		batchEntry := v1beta2.AccumulatedEntry{
			From:             source,
			ImageDigest:      "sha256:only-digest",
			BuildPipelineRun: "missing-plr",
		}
		c := newClient(nudgeConfig, sourceComponent)

		result, err := buildResultForAccumulatedEntry(ctx, c, namespace, batchEntry, sourceComponent, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.FileMatches).To(Equal("Dockerfile.dev"))
		Expect(result.BuiltImageTag).To(Equal("prod"))
	})

	It("loads build results from an existing build PipelineRun", func() {
		sourceComponent := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: source, Namespace: namespace},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ContainerImage: "quay.io/org/source:latest",
			},
		}
		buildPLR := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "existing-build-plr", Namespace: namespace},
			Status: tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					Results: []tektonv1.PipelineRunResult{
						{Name: "IMAGE_DIGEST", Value: *tektonv1.NewStructuredValues("sha256:from-plr")},
						{Name: "IMAGE_URL", Value: *tektonv1.NewStructuredValues("quay.io/org/source:v2")},
					},
				},
			},
		}
		batchEntry := v1beta2.AccumulatedEntry{
			From:             source,
			ImageDigest:      "sha256:status-override",
			BuildPipelineRun: buildPLR.Name,
		}
		c := newClient(nudgeConfig, sourceComponent, buildPLR)

		result, err := buildResultForAccumulatedEntry(ctx, c, namespace, batchEntry, sourceComponent, buildPLR)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Digest).To(Equal("sha256:status-override"))
		Expect(result.BuiltImageRepository).To(Equal("quay.io/org/source"))
		Expect(result.BuiltImageTag).To(Equal("v2"))
	})

	It("returns an error when the build PipelineRun cannot be loaded", func() {
		sourceComponent := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: source, Namespace: namespace},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nudgeConfig, sourceComponent).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*tektonv1.PipelineRun); ok && key.Name == "bad-build-plr" {
					return fmt.Errorf("apiserver unavailable")
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).Build()
		batchEntry := v1beta2.AccumulatedEntry{
			From:             source,
			ImageDigest:      "sha256:status",
			BuildPipelineRun: "bad-build-plr",
		}

		_, err := buildResultForAccumulatedEntry(ctx, c, namespace, batchEntry, sourceComponent, nil)
		Expect(err).To(MatchError(ContainSubstring(`loading build PipelineRun "bad-build-plr"`)))
	})

	It("returns an error when a component has no container image for digest-only batching", func() {
		sourceComponent := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: source, Namespace: namespace},
		}
		batchEntry := v1beta2.AccumulatedEntry{
			From:             source,
			ImageDigest:      "sha256:only-digest",
			BuildPipelineRun: "missing-plr",
		}
		c := newClient(nudgeConfig, sourceComponent)

		_, err := buildResultForAccumulatedEntry(ctx, c, namespace, batchEntry, sourceComponent, nil)
		Expect(err).To(MatchError(ContainSubstring("no container image")))
	})
})
