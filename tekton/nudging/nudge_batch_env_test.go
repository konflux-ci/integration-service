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
	"errors"
	"fmt"
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
			Expect(updated.Status.ActiveBatches[0].NudgePipelineRun).To(Equal(BatchedNudgePipelineRunName(updated.Status.ActiveBatches[0].BatchID, 0)))
			Expect(updated.Status.ActiveBatches[0].Accumulated).To(HaveLen(1))
			Expect(updated.Status.ActiveBatches[0].Accumulated[0].BuildPipelineRun).To(Equal(buildPLR.Name))
			Expect(updated.Status.ActiveBatches[0].Accumulated[0].ServiceAccountName).To(Equal(buildPLR.Spec.TaskRunTemplate.ServiceAccountName))
			Expect(updated.Status.ActiveBatches[0].BatchID).To(MatchRegexp(`^` + target + `_\d+$`))
		})

		It("should be idempotent when the same build PipelineRun is recorded twice", func() {
			c := newClient(nudgeConfig, buildPLR)
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())
			Expect(RecordBuildForBatchedNudge(ctx, c, nudgeConfig, target, buildPLR, buildResult)).To(Succeed())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
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

			err := ProcessDueNudgeBatches(ctx, c, nudgeConfig)
			Expect(err).NotTo(HaveOccurred())

			updated := &v1beta2.NudgeConfig{}
			Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
			Expect(updated.Status.ActiveBatches).To(HaveLen(1))
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFailed))
			Expect(updated.Status.ActiveBatches[0].Message).NotTo(BeEmpty())
		})
	})
})
