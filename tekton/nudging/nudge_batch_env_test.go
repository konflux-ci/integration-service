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

	"github.com/konflux-ci/integration-service/api/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			SourceComponentName: source,
			Digest:              "sha256:abc",
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
			Expect(updated.Status.ActiveBatches[0].Accumulated).To(HaveLen(1))
			Expect(updated.Status.ActiveBatches[0].Accumulated[0].BuildPipelineRun).To(Equal(buildPLR.Name))
		})

		It("should be idempotent when the same build PipelineRun is recorded twice", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Accumulated).To(HaveLen(1))
		})
	})

	Context("When clearing one-shot actions", func() {
		It("should succeed when spec.actions is already unset", func() {
			c := newClient(nudgeConfig)
			Expect(ClearNudgeConfigActions(ctx, c, nudgeConfig)).To(Succeed())
		})

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

		It("should reject force-fire when includePartial is false and the batch has failures", func() {
			now := metav1.Now()
			includePartial := false
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "blocked-partial",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
					Failed: []v1beta2.FailedEntry{
						{From: "other", BuildPipelineRun: "failed-plr", Reason: "fail", CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR)
			nudgeConfig.Spec.Actions = &v1beta2.Actions{
				ForceFire: &v1beta2.ForceFireAction{Target: target, IncludePartial: &includePartial},
			}
			Expect(ProcessForceFireAction(ctx, c, nudgeConfig)).To(MatchError(ContainSubstring("includePartial is false")))
		})

		It("should reject force-fire when the batch has no accumulated builds", func() {
			now := metav1.Now()
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "empty-batch",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
				},
			}
			c := newClient(nudgeConfig)
			nudgeConfig.Spec.Actions = &v1beta2.Actions{
				ForceFire: &v1beta2.ForceFireAction{Target: target},
			}
			Expect(ProcessForceFireAction(ctx, c, nudgeConfig)).To(MatchError(ContainSubstring("no accumulated builds")))
		})

		It("should complete force-fire when the nudge PipelineRun already exists", func() {
			buildPLR.UID = types.UID("force-fire-env-uid")
			plrName := NudgePipelineRunNameForBatchedTarget(buildPLR, target)
			existingPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: plrName, Namespace: namespace},
			}
			now := metav1.Now()
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "force-fire-existing-plr",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR, existingPLR)
			nudgeConfig.Spec.Actions = &v1beta2.Actions{
				ForceFire: &v1beta2.ForceFireAction{Target: target},
			}
			Expect(ProcessForceFireAction(ctx, c, nudgeConfig)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseCompleted))
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

	Context("When recording failed builds for batched targets", func() {
		It("should block the batch when failure policy is Block", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, "build failed")).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches).To(HaveLen(1))
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseBlocked))
			Expect(updated.Status.ActiveBatches[0].Failed).To(HaveLen(1))
			Expect(updated.Status.ActiveBatches[0].FireAt).To(BeNil())
		})

		It("should default the failure reason when none is provided", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, "")).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Failed[0].Reason).To(Equal("build failed"))
		})

		It("should be idempotent when the same failed build PipelineRun is recorded twice", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, "build failed")).To(Succeed())
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, "build failed")).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Failed).To(HaveLen(1))
		})

		It("should keep accumulating when failure policy is ProceedWithPartial", func() {
			nudgeConfig.Spec.BatchDefaults = &v1beta2.BatchDefaults{
				FailurePolicy: &[]v1beta2.FailurePolicyType{v1beta2.FailurePolicyProceedWithPartial}[0],
			}
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, "build failed")).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseAccumulating))
			Expect(updated.Status.ActiveBatches[0].Failed).To(HaveLen(1))
		})
	})

	Context("When a successful build follows a failed build from the same source", func() {
		It("should clear failed entries and return the batch to accumulating", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, "build failed")).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseBlocked))

			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseAccumulating))
			Expect(updated.Status.ActiveBatches[0].Failed).To(BeEmpty())
			Expect(updated.Status.ActiveBatches[0].FireAt).NotTo(BeNil())
		})
	})

	Context("When processing due batches", func() {
		It("should return the wait duration until the next debounce fire", func() {
			now := metav1.Now()
			futureFire := metav1.NewTime(now.Add(10 * time.Minute))
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "future-batch",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					FireAt:       &futureFire,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR)

			wake, err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(wake).To(BeNumerically(">", 0))
			Expect(wake).To(BeNumerically("<", 11*time.Minute))
		})

		It("should attempt to fire a due batch when failure policy is ProceedWithPartial", func() {
			now := metav1.Now()
			pastFire := metav1.NewTime(now.Add(-time.Minute))
			nudgeConfig.Spec.BatchDefaults = &v1beta2.BatchDefaults{
				FailurePolicy: &[]v1beta2.FailurePolicyType{v1beta2.FailurePolicyProceedWithPartial}[0],
			}
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "partial-due",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					FireAt:       &pastFire,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
					Failed: []v1beta2.FailedEntry{
						{From: "other-source", BuildPipelineRun: "failed-plr", Reason: "build failed", CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR)

			_, err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFailed))
		})

		It("should transition a due batch with failures to Blocked instead of firing", func() {
			now := metav1.Now()
			pastFire := metav1.NewTime(now.Add(-time.Minute))
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      "due-blocked",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					FireAt:       &pastFire,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
					},
					Failed: []v1beta2.FailedEntry{
						{From: "other-source", BuildPipelineRun: "failed-plr", Reason: "build failed", CapturedAt: now},
					},
				},
			}
			c := newClient(nudgeConfig, buildPLR)

			_, err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseBlocked))
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
	})

	Context("When resolving accumulated entries for batch fire", func() {
		It("should keep the latest entry per source component in batch order", func() {
			now := metav1.Now()
			later := metav1.NewTime(now.Add(time.Minute))
			entries := []v1beta2.AccumulatedEntry{
				{From: "source-a", ImageDigest: "sha256:1", BuildPipelineRun: "plr-a1", CapturedAt: now},
				{From: "source-b", ImageDigest: "sha256:2", BuildPipelineRun: "plr-b1", CapturedAt: now},
				{From: "source-a", ImageDigest: "sha256:3", BuildPipelineRun: "plr-a2", CapturedAt: later},
			}
			resolved := latestAccumulatedEntryPerSource(entries)
			Expect(resolved).To(HaveLen(2))
			Expect(resolved[0].From).To(Equal("source-a"))
			Expect(resolved[0].ImageDigest).To(Equal("sha256:3"))
			Expect(resolved[1].From).To(Equal("source-b"))
			Expect(resolved[1].ImageDigest).To(Equal("sha256:2"))
		})
	})

	Context("When batched PipelineRun names are derived", func() {
		It("should use distinct names per target and not alias the immediate per-build name", func() {
			buildPLR.UID = types.UID("multi-target-uid")
			otherTarget := "other-nudge-target"
			Expect(NudgePipelineRunNameForBatchedTarget(buildPLR, target)).NotTo(Equal(NudgePipelineRunNameForBatchedTarget(buildPLR, otherTarget)))
			Expect(NudgePipelineRunNameForBatchedTarget(buildPLR, target)).NotTo(Equal(NudgePipelineRunNameForBuild(buildPLR)))
		})

		It("should not treat an immediate nudge PipelineRun as the batched target's run", func() {
			buildPLR.UID = types.UID("immediate-vs-batch-uid")
			immediatePLRName := NudgePipelineRunNameForBuild(buildPLR)
			existingPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: immediatePLRName, Namespace: namespace},
			}
			now := metav1.Now()
			batch := &v1beta2.ActiveBatch{
				Target:       target,
				BatchID:      "batched-target-batch",
				Phase:        v1beta2.BatchPhaseAccumulating,
				CreatedAt:    now,
				HardDeadline: metav1.NewTime(now.Add(time.Hour)),
				Accumulated: []v1beta2.AccumulatedEntry{
					{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
				},
			}
			c := newClient(nudgeConfig, buildPLR, existingPLR)

			err := fireBatch(ctx, c, nudgeConfig, batch)
			Expect(err).To(HaveOccurred())
			Expect(batch.Phase).NotTo(Equal(v1beta2.BatchPhaseCompleted))
			Expect(batch.NudgePipelineRun).To(Equal(NudgePipelineRunNameForBatchedTarget(buildPLR, target)))
		})
	})

	Context("When firing an invalid batch", func() {
		It("should reject firing a batch with no accumulated builds", func() {
			now := metav1.Now()
			batch := &v1beta2.ActiveBatch{
				Target:       target,
				BatchID:      "empty-fire-batch",
				Phase:        v1beta2.BatchPhaseAccumulating,
				CreatedAt:    now,
				HardDeadline: metav1.NewTime(now.Add(time.Hour)),
			}
			c := newClient(nudgeConfig)
			Expect(fireBatch(ctx, c, nudgeConfig, batch)).To(MatchError(ContainSubstring("cannot fire empty batch")))
		})
	})

	Context("When firing a batch idempotently", func() {
		It("should complete without creating a duplicate when the nudge PipelineRun already exists", func() {
			buildPLR.UID = types.UID("batch-idempotency-uid")
			plrName := NudgePipelineRunNameForBatchedTarget(buildPLR, target)
			existingPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: plrName, Namespace: namespace},
			}
			now := metav1.Now()
			batch := &v1beta2.ActiveBatch{
				Target:       target,
				BatchID:      "idempotent-batch",
				Phase:        v1beta2.BatchPhaseAccumulating,
				CreatedAt:    now,
				HardDeadline: metav1.NewTime(now.Add(time.Hour)),
				Accumulated: []v1beta2.AccumulatedEntry{
					{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
				},
			}
			c := newClient(nudgeConfig, buildPLR, existingPLR)

			Expect(fireBatch(ctx, c, nudgeConfig, batch)).To(Succeed())
			Expect(batch.Phase).To(Equal(v1beta2.BatchPhaseCompleted))
			Expect(batch.NudgePipelineRun).To(Equal(plrName))

			list := &tektonv1.PipelineRunList{}
			Expect(c.List(ctx, list, client.InNamespace(namespace))).To(Succeed())
			nudgePLRCount := 0
			for _, plr := range list.Items {
				if plr.Name == plrName {
					nudgePLRCount++
				}
			}
			Expect(nudgePLRCount).To(Equal(1), "expected a single nudge PipelineRun named %q", plrName)
		})
	})

	Context("When a batch is stuck in Firing", func() {
		It("should resume and complete when the planned nudge PipelineRun already exists", func() {
			buildPLR.UID = types.UID("firing-recovery-uid")
			plrName := NudgePipelineRunNameForBatchedTarget(buildPLR, target)
			existingPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: plrName, Namespace: namespace},
			}
			now := metav1.Now()
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:           target,
					BatchID:          "firing-batch",
					Phase:            v1beta2.BatchPhaseFiring,
					NudgePipelineRun: plrName,
					CreatedAt:        now,
					HardDeadline:     metav1.NewTime(now.Add(time.Hour)),
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
	})
})
