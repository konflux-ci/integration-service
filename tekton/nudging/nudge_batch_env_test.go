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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
			Expect(batch.NudgePipelineRun).To(Equal(BatchedNudgePipelineRunName(batch.BatchID, 0)))
			Expect(batch.BatchID).To(MatchRegexp(`^` + target + `_\d+$`))
			Expect(batch.Accumulated).To(HaveLen(1))
			entry := batch.Accumulated[0]
			Expect(entry.BuildPipelineRun).To(Equal(buildPLR.Name))
			Expect(entry.BuiltImageRepository).To(Equal(buildResult.BuiltImageRepository))
			Expect(entry.BuildPipelineRunUID).To(Equal(string(buildPLR.UID)))
			Expect(entry.BuildServiceAccountName).To(Equal(buildPLR.Spec.TaskRunTemplate.ServiceAccountName))
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

		It("should resume accumulating when a previously failed source build succeeds", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, "task failed")).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseBlocked))

			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())

			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseAccumulating))
			Expect(updated.Status.ActiveBatches[0].Failed).To(BeEmpty())
			Expect(updated.Status.ActiveBatches[0].Accumulated).To(HaveLen(1))
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

		It("should default the failure reason when none is provided", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, "")).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Failed[0].Reason).To(Equal("build pipeline failed"))
		})

		It("should no-op when the same failed build is recorded again", func() {
			c := newClient(nudgeConfig, buildPLR)
			reason := "task failed"
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, reason)).To(Succeed())
			Expect(RecordFailedBuildForBatchedNudge(ctx, c, nudgeConfig, target, source, buildPLR, reason)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Failed).To(HaveLen(1))
		})
	})

	Context("When handling force-fire conditions and no-op actions", func() {
		It("should no-op ProcessForceFireAction when spec.actions is nil", func() {
			c := newClient(nudgeConfig)
			Expect(ProcessForceFireAction(ctx, c, nudgeConfig)).To(Succeed())
		})

		It("should return ErrForceFireIgnored when the target batch is already firing", func() {
			now := metav1.Now()
			nudgeConfig.Status.ActiveBatches = []v1beta2.ActiveBatch{{
				Target:           target,
				BatchID:          "firing",
				Phase:            v1beta2.BatchPhaseFiring,
				CreatedAt:        now,
				NudgePipelineRun: "nudge-batch-firing",
			}}
			c := newClient(nudgeConfig)
			nudgeConfig.Spec.Actions = &v1beta2.Actions{
				ForceFire: &v1beta2.ForceFireAction{Target: target},
			}
			err := ProcessForceFireAction(ctx, c, nudgeConfig)
			Expect(errors.Is(err, ErrForceFireIgnored)).To(BeTrue())
		})

		It("should record force-fire rejected conditions", func() {
			c := newClient(nudgeConfig)
			Expect(SetForceFireRejectedCondition(ctx, c, nudgeConfig, "rejected")).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.Conditions).To(ContainElement(And(
				HaveField("Type", v1beta2.NudgeConfigConditionForceFireRejected),
				HaveField("Status", metav1.ConditionTrue),
				HaveField("Message", "rejected"),
			)))
		})

		It("should no-op when clearing absent force-fire conditions", func() {
			c := newClient(nudgeConfig)
			Expect(ClearForceFireRejectedCondition(ctx, c, nudgeConfig)).To(Succeed())
			Expect(ClearForceFireIgnoredCondition(ctx, c, nudgeConfig)).To(Succeed())
		})

		It("should record force-fire ignored conditions", func() {
			c := newClient(nudgeConfig)
			Expect(SetForceFireIgnoredCondition(ctx, c, nudgeConfig, "ignored")).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.Conditions).To(ContainElement(And(
				HaveField("Type", v1beta2.NudgeConfigConditionForceFireIgnored),
				HaveField("Status", metav1.ConditionTrue),
				HaveField("Message", "ignored"),
			)))
		})

		It("should clear force-fire ignored conditions after they were set", func() {
			c := newClient(nudgeConfig)
			Expect(SetForceFireIgnoredCondition(ctx, c, nudgeConfig, "ignored")).To(Succeed())
			Expect(ClearForceFireIgnoredCondition(ctx, c, nudgeConfig)).To(Succeed())
		})

		It("should clear force-fire rejected conditions after they were set", func() {
			c := newClient(nudgeConfig)
			Expect(SetForceFireRejectedCondition(ctx, c, nudgeConfig, "rejected")).To(Succeed())
			Expect(ClearForceFireRejectedCondition(ctx, c, nudgeConfig)).To(Succeed())
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

		It("should reject forceFire on a blocked batch when includePartial is false", func() {
			now := metav1.Now()
			includePartial := false
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
			Expect(err).NotTo(MatchError(ContainSubstring("no force-fireable batch")))
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

	Context("When resolving reference build PipelineRuns for firing", func() {
		It("should use another accumulated entry when the first build PLR was pruned", func() {
			livePLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "live-build-plr", Namespace: namespace},
			}
			c := newClient(livePLR)
			plr, err := resolveReferenceBuildPLR(ctx, c, namespace, []v1beta2.AccumulatedEntry{
				{From: "gone", BuildPipelineRun: "pruned-build-plr"},
				{From: source, BuildPipelineRun: livePLR.Name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(plr.Name).To(Equal(livePLR.Name))
		})

		It("should skip accumulated entries with no build PipelineRun name", func() {
			livePLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "live-build-plr", Namespace: namespace},
			}
			c := newClient(livePLR)
			plr, err := resolveReferenceBuildPLR(ctx, c, namespace, []v1beta2.AccumulatedEntry{
				{From: source, BuildPipelineRun: ""},
				{From: source, BuildPipelineRun: livePLR.Name},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(plr.Name).To(Equal(livePLR.Name))
		})

		It("should return an empty reference when every build PLR is missing", func() {
			c := newClient()
			plr, err := resolveReferenceBuildPLR(ctx, c, namespace, []v1beta2.AccumulatedEntry{
				{From: source, BuildPipelineRun: "pruned-build-plr"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(plr.Name).To(BeEmpty())
			Expect(plr.Namespace).To(Equal(namespace))
		})

		It("should surface unexpected errors while loading a reference build PLR", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*tektonv1.PipelineRun); ok && key.Name == "bad-build-plr" {
						return fmt.Errorf("apiserver unavailable")
					}
					return cl.Get(ctx, key, obj, opts...)
				},
			}).Build()

			_, err := resolveReferenceBuildPLR(ctx, c, namespace, []v1beta2.AccumulatedEntry{
				{From: source, BuildPipelineRun: "bad-build-plr"},
			})
			Expect(err).To(MatchError(ContainSubstring(`loading reference build PipelineRun "bad-build-plr"`)))
		})
	})

	Context("When processing due batches", func() {
		It("should reconcile an existing nudge PLR into Firing and record its name on status", func() {
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
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFiring))
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
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFiring))
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

	Context("When firing batches end-to-end", func() {
		const batchID = "nudge-target_1700000000"

		batchedFireScheme := func() *runtime.Scheme {
			s := runtime.NewScheme()
			Expect(v1beta2.AddToScheme(s)).To(Succeed())
			Expect(tektonv1.AddToScheme(s)).To(Succeed())
			Expect(corev1.AddToScheme(s)).To(Succeed())
			Expect(applicationapiv1alpha1.AddToScheme(s)).To(Succeed())
			Expect(pacv1alpha1.AddToScheme(s)).To(Succeed())
			return s
		}

		batchedFireClient := func(nc *v1beta2.NudgeConfig, build *tektonv1.PipelineRun) client.Client {
			dockerConfigBytes, err := json.Marshal(dockerConfigJSON{
				Auths: map[string]repositoryConfigAuth{
					"quay.io/org/source": {Username: "quay-user", Password: "quay-pass"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			dockerSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "docker-secret", Namespace: namespace},
				Type:       corev1.SecretTypeDockerConfigJson,
				Data:       map[string][]byte{corev1.DockerConfigJsonKey: dockerConfigBytes},
			}
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultPipelineServiceAccount, Namespace: namespace},
				Secrets:    []corev1.ObjectReference{{Name: "docker-secret"}},
			}
			scmSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "scm-github-secret", Namespace: namespace},
				Type:       corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					corev1.BasicAuthUsernameKey: []byte("bot-user"),
					corev1.BasicAuthPasswordKey: []byte("ghp_secret-token"),
				},
			}
			repo := &pacv1alpha1.Repository{
				ObjectMeta: metav1.ObjectMeta{Name: "downstream-repo", Namespace: namespace},
				Spec: pacv1alpha1.RepositorySpec{
					URL: "https://github.com/org/downstream",
					GitProvider: &pacv1alpha1.GitProvider{
						Secret: &pacv1alpha1.Secret{Name: "scm-github-secret"},
					},
				},
			}
			targetComp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: target, Namespace: namespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					Application:   "test-app",
					ComponentName: target,
					Source: applicationapiv1alpha1.ComponentSource{
						ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
							GitSource: &applicationapiv1alpha1.GitSource{
								URL:      "https://github.com/org/downstream",
								Revision: "main",
							},
						},
					},
				},
			}
			sourceComp := &applicationapiv1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: source, Namespace: namespace},
				Spec: applicationapiv1alpha1.ComponentSpec{
					Application:    "test-app",
					ComponentName:  source,
					ContainerImage: "quay.io/org/source:latest",
				},
			}
			return fake.NewClientBuilder().
				WithScheme(batchedFireScheme()).
				WithStatusSubresource(&v1beta2.NudgeConfig{}).
				WithObjects(nc, build, sa, dockerSecret, scmSecret, repo, targetComp, sourceComp).
				Build()
		}

		accumulatedEntry := func(now metav1.Time) v1beta2.AccumulatedEntry {
			return v1beta2.AccumulatedEntry{
				From:                 source,
				ImageDigest:          buildResult.Digest,
				BuildPipelineRun:     buildPLR.Name,
				BuiltImageRepository: buildResult.BuiltImageRepository,
				BuiltImageTag:        buildResult.BuiltImageTag,
				NudgeFileMatches:     ".*.yaml",
				GitRepoAtShaLink:     "https://github.com/org/source/commit/abc",
				CapturedAt:           now,
			}
		}

		It("should create the aggregated nudge PipelineRun when a due batch fires", func() {
			now := metav1.Now()
			pastFire := metav1.NewTime(now.Add(-time.Minute))
			nc := nudgeConfig.DeepCopy()
			nc.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:       target,
					BatchID:      batchID,
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					FireAt:       &pastFire,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated:  []v1beta2.AccumulatedEntry{accumulatedEntry(now)},
				},
			}
			plr := buildPLR.DeepCopy()
			plr.Spec.TaskRunTemplate.ServiceAccountName = tektonconsts.DefaultPipelineServiceAccount
			c := batchedFireClient(nc, plr)

			_, err := ProcessDueNudgeBatches(ctx, c, nc)
			Expect(err).NotTo(HaveOccurred())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFiring))
			plrName := BatchedNudgePipelineRunName(batchID, 0)
			Expect(updated.Status.ActiveBatches[0].NudgePipelineRun).To(Equal(plrName))

			created := &tektonv1.PipelineRun{}
			Expect(c.Get(ctx, types.NamespacedName{Name: plrName, Namespace: namespace}, created)).To(Succeed())
			Expect(created.Annotations[tektonconsts.NudgeBatchIDAnnotation]).To(Equal(batchID))
			Expect(created.Annotations).NotTo(HaveKey(tektonconsts.NudgingComponentAnnotation))
		})

		It("should move a Firing batch to Completed after the nudge PipelineRun succeeds", func() {
			now := metav1.Now()
			pastFire := metav1.NewTime(now.Add(-time.Minute))
			plrName := BatchedNudgePipelineRunName(batchID, 0)
			existingPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: plrName, Namespace: namespace},
			}
			existingPLR.Status.MarkSucceeded(tektonv1.PipelineRunReasonCompleted.String(), "done")
			nc := nudgeConfig.DeepCopy()
			nc.Status.ActiveBatches = []v1beta2.ActiveBatch{
				{
					Target:           target,
					BatchID:          batchID,
					Phase:            v1beta2.BatchPhaseAccumulating,
					CreatedAt:        now,
					FireAt:           &pastFire,
					HardDeadline:     metav1.NewTime(now.Add(time.Hour)),
					Accumulated:      []v1beta2.AccumulatedEntry{accumulatedEntry(now)},
					NudgePipelineRun: "",
				},
			}
			c := batchedFireClient(nc, buildPLR)
			Expect(c.Create(ctx, existingPLR)).To(Succeed())

			_, err := ProcessDueNudgeBatches(ctx, c, nc)
			Expect(err).NotTo(HaveOccurred())

			Expect(ProcessFiringNudgeBatches(ctx, c, nc)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseCompleted))
			Expect(updated.Status.ActiveBatches[0].NudgePipelineRun).To(Equal(plrName))
		})
	})

	Context("When processing firing batches", func() {
		const (
			batchID      = "firing-batch-id"
			nudgePLRName = "nudge-batch-firing-batch-id"
		)

		firingBatch := func() *v1beta2.NudgeConfig {
			nc := nudgeConfig.DeepCopy()
			nc.Status.ActiveBatches = []v1beta2.ActiveBatch{{
				Target:           target,
				BatchID:          batchID,
				Phase:            v1beta2.BatchPhaseFiring,
				NudgePipelineRun: nudgePLRName,
				Accumulated: []v1beta2.AccumulatedEntry{
					{From: source, ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: metav1.Now()},
				},
			}}
			return nc
		}

		It("should transition the batch to Completed when the nudge PipelineRun succeeds", func() {
			nc := firingBatch()
			nudgePLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nudgePLRName,
					Namespace: namespace,
					Annotations: map[string]string{
						tektonconsts.NudgeBatchIDAnnotation: batchID,
					},
				},
			}
			nudgePLR.Status.MarkSucceeded(tektonv1.PipelineRunReasonCompleted.String(), "done")
			c := newClient(nc, nudgePLR)

			Expect(ProcessFiringNudgeBatches(ctx, c, nc)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseCompleted))
			Expect(updated.Status.ActiveBatches[0].Message).To(ContainSubstring("succeeded"))
		})

		It("should leave the batch in Firing while the nudge PipelineRun is still running", func() {
			nc := firingBatch()
			nudgePLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: nudgePLRName, Namespace: namespace},
			}
			nudgePLR.Status.MarkRunning(tektonv1.PipelineRunReasonRunning.String(), "in progress")
			c := newClient(nc, nudgePLR)

			Expect(ProcessFiringNudgeBatches(ctx, c, nc)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFiring))
		})

		It("should fail the batch when the nudge PipelineRun batch-id annotation disagrees", func() {
			nc := firingBatch()
			nudgePLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nudgePLRName,
					Namespace: namespace,
					Annotations: map[string]string{
						tektonconsts.NudgeBatchIDAnnotation: "other-batch",
					},
				},
			}
			nudgePLR.Status.MarkSucceeded(tektonv1.PipelineRunReasonCompleted.String(), "done")
			c := newClient(nc, nudgePLR)

			Expect(ProcessFiringNudgeBatches(ctx, c, nc)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nc.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFailed))
			Expect(updated.Status.ActiveBatches[0].Message).To(ContainSubstring("batch-id"))
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
