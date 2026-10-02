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
})
