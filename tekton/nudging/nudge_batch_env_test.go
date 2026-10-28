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
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
)

var _ = Describe("Nudge batch client operations", func() {
	const (
		namespace = "test-ns"
		target    = "nudge-target"
		source    = "nudge-source"
	)

	var (
		scheme      *runtime.Scheme
		nudgeConfig *v1beta2.NudgeConfig
		buildPLR    *tektonv1.PipelineRun
		buildResult *NudgeBuildResult
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(v1beta2.AddToScheme(scheme)).To(Succeed())
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())

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
		buildPLR = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nudge-batch-build-plr",
				Namespace: namespace,
			},
		}
		buildResult = &NudgeBuildResult{
			SourceComponentName:  source,
			Digest:               "sha256:abc",
			BuiltImageRepository: "registry.example/ns/source",
			BuiltImageTag:        "latest",
		}
	})

	newClient := func(objects ...client.Object) client.Client {
		builder := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1beta2.NudgeConfig{})
		if len(objects) > 0 {
			builder = builder.WithObjects(objects...)
		}
		return builder.Build()
	}

	Context("When recording builds for batched targets", func() {
		It("should require a build result", func() {
			c := newClient(nudgeConfig)
			err := RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, nil)
			Expect(err).To(MatchError(ContainSubstring("build result is required")))
		})

		It("should create an accumulating batch on the NudgeConfig status", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches).To(HaveLen(1))
			Expect(updated.Status.ActiveBatches[0].Target).To(Equal(target))
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseAccumulating))
			batch := updated.Status.ActiveBatches[0]
			Expect(batch.Accumulated).To(HaveLen(1))
			entry := batch.Accumulated[0]
			Expect(entry.BuildPipelineRun).To(Equal(buildPLR.Name))
			Expect(entry.BuiltImageRepository).To(Equal(buildResult.BuiltImageRepository))
			Expect(entry.BuildPipelineRunUID).To(Equal(string(buildPLR.UID)))
		})

		It("should be idempotent when the same build PipelineRun is recorded twice", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Accumulated).To(HaveLen(1))
		})

		It("should append a second source build to the same accumulating batch", func() {
			otherSource := "other-nudge-source"
			otherPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "other-batch-build-plr", Namespace: namespace},
			}
			otherResult := &NudgeBuildResult{
				SourceComponentName: otherSource,
				Digest:              "sha256:def",
			}

			c := newClient(nudgeConfig, buildPLR, otherPLR)
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, otherPLR, otherResult)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches).To(HaveLen(1))
			batch := updated.Status.ActiveBatches[0]
			Expect(batch.Accumulated).To(HaveLen(2))
			Expect(batch.Accumulated[0].From).To(Equal(source))
			Expect(batch.Accumulated[1].From).To(Equal(otherSource))
		})

		It("should reset fireAt on each new build without changing hardDeadline or batch identity", func() {
			debounce := metav1.Duration{Duration: 10 * time.Minute}
			maxWait := metav1.Duration{Duration: 2 * time.Hour}
			nudgeConfig.Spec.BatchDefaults = &v1beta2.BatchDefaults{
				DebounceTimeout: &debounce,
				MaxWaitTime:     &maxWait,
			}

			secondPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "debounce-reset-build-plr", Namespace: namespace},
			}
			secondResult := &NudgeBuildResult{
				SourceComponentName: "second-source",
				Digest:              "sha256:second",
			}

			c := newClient(nudgeConfig, buildPLR, secondPLR)
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())

			afterFirst := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, afterFirst)).To(Succeed())
			firstBatch := afterFirst.Status.ActiveBatches[0]
			firstFireAt := firstBatch.FireAt.Time
			firstHardDeadline := firstBatch.HardDeadline.Time
			firstBatchID := firstBatch.BatchID
			firstCreatedAt := firstBatch.CreatedAt.Time

			time.Sleep(1100 * time.Millisecond)

			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, secondPLR, secondResult)).To(Succeed())

			afterSecond := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, afterSecond)).To(Succeed())
			secondBatch := afterSecond.Status.ActiveBatches[0]
			Expect(secondBatch.BatchID).To(Equal(firstBatchID))
			Expect(secondBatch.CreatedAt.Time).To(Equal(firstCreatedAt))
			Expect(secondBatch.HardDeadline.Time).To(Equal(firstHardDeadline))
			Expect(secondBatch.FireAt.Time).To(BeTemporally(">", firstFireAt))
		})

		It("should replace the accumulated entry when the same source component rebuilds", func() {
			retryPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "nudge-batch-build-retry", Namespace: namespace},
			}
			c := newClient(nudgeConfig, buildPLR, retryPLR)
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())

			updatedDigest := &NudgeBuildResult{
				SourceComponentName: source,
				Digest:              "sha256:def",
			}
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, retryPLR, updatedDigest)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Accumulated).To(HaveLen(1))
			Expect(updated.Status.ActiveBatches[0].Accumulated[0].ImageDigest).To(Equal("sha256:def"))
			Expect(updated.Status.ActiveBatches[0].Accumulated[0].BuildPipelineRun).To(Equal(retryPLR.Name))
		})

		It("should block the batch when a source build fails under Block policy", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, "task failed")).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseBlocked))
			Expect(updated.Status.ActiveBatches[0].Failed).To(HaveLen(1))
			Expect(updated.Status.ActiveBatches[0].FireAt).To(BeNil())
		})
	})

	Context("When clearing one-shot actions", func() {
		It("should remove spec.actions from the NudgeConfig", func() {
			nudgeConfig.Spec.Actions = &v1beta2.Actions{
				ForceFire: &v1beta2.ForceFireAction{Target: target},
			}
			c := newClient(nudgeConfig)
			Expect(ClearNudgeConfigActions(ctx, c, nudgeConfig)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Spec.Actions).To(BeNil())
		})
	})

	Context("When force-firing a batch", func() {
		It("should return an error when no batch exists for the target", func() {
			c := newClient(nudgeConfig)
			nudgeConfig.Spec.Actions = &v1beta2.Actions{
				ForceFire: &v1beta2.ForceFireAction{Target: target},
			}
			Expect(ProcessForceFireAction(ctx, c, nudgeConfig)).To(MatchError(ContainSubstring("no force-fireable batch")))
		})

		It("should reject forceFire when includePartial is false and failures remain", func() {
			now := metav1.Now()
			includePartial := false
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "partial-batch",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
					Failed: []v1beta2.FailedEntry{
						{From: "other-operand", BuildPipelineRun: "plr-fail", Reason: "failed", CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR)
			nudgeConfig.Spec.Actions = &v1beta2.Actions{
				ForceFire: &v1beta2.ForceFireAction{Target: target, IncludePartial: &includePartial},
			}
			err := ProcessForceFireAction(ctx, c, nudgeConfig)
			Expect(err).To(MatchError(ContainSubstring("includePartial=false")))
		})

		It("should select a failed batch for retry", func() {
			now := metav1.Now()
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "failed-batch",
					Phase:        v1beta2.BatchPhaseFailed,
					CreatedAt:    now,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR)
			nudgeConfig.Spec.Actions = &v1beta2.Actions{
				ForceFire: &v1beta2.ForceFireAction{Target: target},
			}
			err := ProcessForceFireAction(ctx, c, nudgeConfig)
			Expect(err).NotTo(MatchError(ContainSubstring("no force-fireable batch")))
		})

		It("should select a blocked batch when includePartial defaults to true", func() {
			now := metav1.Now()
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "blocked-batch",
					Phase:        v1beta2.BatchPhaseBlocked,
					CreatedAt:    now,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR)
			nudgeConfig.Spec.Actions = &v1beta2.Actions{
				ForceFire: &v1beta2.ForceFireAction{Target: target},
			}
			err := ProcessForceFireAction(ctx, c, nudgeConfig)
			Expect(err).NotTo(MatchError(ContainSubstring("no force-fireable batch")))
		})
	})

	Context("When processing due batches", func() {
		It("should fire a batch when fireAt has elapsed with no failures (via idempotent PLR path)", func() {
			const batchID = "due-success-idempotent"
			plrName := BatchedNudgePipelineRunName(batchID, 0)
			existingPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: plrName, Namespace: namespace},
			}
			now := metav1.Now()
			pastFire := metav1.NewTime(now.Add(-time.Minute))
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      batchID,
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					FireAt:       &pastFire,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR, existingPLR)

			_, err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseCompleted))
			Expect(updated.Status.ActiveBatches[0].NudgePipelineRun).To(Equal(plrName))
		})

		It("should mark a due batch as failed when firing cannot complete", func() {
			now := metav1.Now()
			pastFire := metav1.NewTime(now.Add(-time.Minute))
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "due-batch",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					FireAt:       &pastFire,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR)

			_, err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches).To(HaveLen(1))
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFailed))
			Expect(updated.Status.ActiveBatches[0].Message).NotTo(BeEmpty())
		})

		It("should fire a batch when hardDeadline has elapsed even if fireAt is in the future", func() {
			now := metav1.Now()
			futureFire := metav1.NewTime(now.Add(time.Hour))
			pastDeadline := metav1.NewTime(now.Add(-time.Minute))
			plrName := BatchedNudgePipelineRunName("hard-deadline-due", 0)
			existingPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: plrName, Namespace: namespace},
			}
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "hard-deadline-due",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					FireAt:       &futureFire,
					HardDeadline: pastDeadline,
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR, existingPLR)

			wake, err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(wake).To(BeZero())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseCompleted))
			Expect(updated.Status.ActiveBatches[0].NudgePipelineRun).To(Equal(plrName))
		})

		It("should return a wake duration bounded by hardDeadline when it precedes fireAt", func() {
			now := metav1.Now()
			laterFire := metav1.NewTime(now.Add(2 * time.Hour))
			soonerDeadline := metav1.NewTime(now.Add(10 * time.Minute))
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "not-yet-due",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					FireAt:       &laterFire,
					HardDeadline: soonerDeadline,
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR)

			wake, err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())
			// Fails on base: returns ~2h (fireAt) instead of ~10m (hardDeadline).
			Expect(wake).To(BeNumerically(">", 0))
			Expect(wake).To(BeNumerically("<", 15*time.Minute))

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseAccumulating))
		})
	})

	Context("When pruning completed batches", func() {
		It("should remove a completed batch older than 24h", func() {
			old := metav1.NewTime(time.Now().Add(-25 * time.Hour))
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "old-done",
					Phase:        v1beta2.BatchPhaseCompleted,
					CreatedAt:    old,
					HardDeadline: metav1.NewTime(old.Add(4 * time.Hour)),
				},
			}
			c := newClient(nudgeConfig)

			Expect(PruneCompletedBatches(ctx, c, nudgeConfig)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches).To(BeEmpty())
		})

		It("should keep a completed batch still within the 24h TTL", func() {
			recent := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "recent-done",
					Phase:        v1beta2.BatchPhaseCompleted,
					CreatedAt:    recent,
					HardDeadline: metav1.NewTime(recent.Add(4 * time.Hour)),
				},
			}
			c := newClient(nudgeConfig)

			Expect(PruneCompletedBatches(ctx, c, nudgeConfig)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches).To(HaveLen(1))
		})

		It("should remove the completed batch for a target when a new build is recorded", func() {
			old := metav1.NewTime(time.Now().Add(-2 * time.Hour))
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "prev-done",
					Phase:        v1beta2.BatchPhaseCompleted,
					CreatedAt:    old,
					HardDeadline: metav1.NewTime(old.Add(4 * time.Hour)),
				},
			}
			c := newClient(nudgeConfig, buildPLR)

			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			// Old Completed batch replaced by new Accumulating batch.
			Expect(updated.Status.ActiveBatches).To(HaveLen(1))
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseAccumulating))
			Expect(updated.Status.ActiveBatches[0].BatchID).NotTo(Equal("prev-done"))
		})
	})
})

var _ = Describe("buildResultFromComponentDigest", func() {
	It("returns a NudgeBuildResult populated from component image and digest", func() {
		comp := &applicationapiv1alpha1.Component{
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "my-source",
				ContainerImage: "registry.example/ns/my-source",
			},
		}
		comp.Name = "my-source"

		result, err := buildResultFromComponentDigest(comp, "sha256:deadbeef", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.BuiltImageRepository).To(Equal("registry.example/ns/my-source"))
		Expect(result.BuiltImageTag).To(BeEmpty())
		Expect(result.Digest).To(Equal("sha256:deadbeef"))
		Expect(result.SourceComponentName).To(Equal("my-source"))
		Expect(result.FileMatches).To(Equal(tektonconsts.DefaultNudgeFiles))
		Expect(result.GitRepoAtShaLink).To(BeEmpty())
	})

	It("returns an error when the component has no container image", func() {
		comp := &applicationapiv1alpha1.Component{}
		comp.Name = "empty-component"

		_, err := buildResultFromComponentDigest(comp, "sha256:abc", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no container image"))
	})

	It("parses the image tag from a container image URL", func() {
		comp := &applicationapiv1alpha1.Component{
			Spec: applicationapiv1alpha1.ComponentSpec{
				ContainerImage: "registry.example/ns/my-source:v1.2.3",
			},
		}
		comp.Name = "my-source"

		result, err := buildResultFromComponentDigest(comp, "sha256:abc", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.BuiltImageRepository).To(Equal("registry.example/ns/my-source"))
		Expect(result.BuiltImageTag).To(Equal("v1.2.3"))
	})
})
