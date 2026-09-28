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

package nudgeconfig

import (
	"time"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	nudging "github.com/konflux-ci/integration-service/tekton/nudging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("NudgeConfig reconciler", func() {
	var (
		scheme     *runtime.Scheme
		reconciler *Reconciler
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(v1beta2.AddToScheme(scheme)).To(Succeed())
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
		reconciler = &Reconciler{
			Log: logf.Log,
		}
	})

	newClient := func(objects ...client.Object) client.Client {
		builder := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1beta2.NudgeConfig{})
		if len(objects) > 0 {
			builder = builder.WithObjects(objects...)
		}
		return builder.Build()
	}

	Context("When the NudgeConfig does not exist", func() {
		It("should complete without error", func() {
			reconciler.Client = newClient()
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: v1beta2.NudgeConfigSingletonName, Namespace: "missing-ns"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("When an accumulating batch is not yet due", func() {
		It("should requeue after the minimum batch interval", func() {
			now := metav1.Now()
			futureFire := metav1.NewTime(now.Add(30 * time.Minute))
			nudgeConfig := &v1beta2.NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v1beta2.NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: v1beta2.NudgeConfigSpec{},
				Status: v1beta2.NudgeConfigStatus{
					ActiveBatches: []v1beta2.ActiveBatch{
						{
							Target:       "bundle",
							BatchID:      "batch-1",
							Phase:        v1beta2.BatchPhaseAccumulating,
							CreatedAt:    now,
							FireAt:       &futureFire,
							HardDeadline: metav1.NewTime(now.Add(time.Hour)),
							Accumulated: []v1beta2.AccumulatedEntry{
								{From: "a", ImageDigest: "sha256:1", BuildPipelineRun: "plr-1", CapturedAt: now},
							},
						},
					},
				},
			}
			reconciler.Client = newClient(nudgeConfig)

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(nudgeConfig)})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">=", minBatchRequeue))
		})
	})

	Context("When there are no pending batches or actions", func() {
		It("should complete without requeue", func() {
			nudgeConfig := &v1beta2.NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v1beta2.NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: v1beta2.NudgeConfigSpec{},
			}
			reconciler.Client = newClient(nudgeConfig)

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(nudgeConfig)})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("When forceFire succeeds", func() {
		It("should clear spec.actions after processing", func() {
			now := metav1.Now()
			buildPLR := &tektonv1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "force-fire-build", Namespace: "default", UID: types.UID("force-fire-uid")},
			}
			target := "bundle"
			plrName := nudging.NudgePipelineRunNameForBatchedTarget(buildPLR, target)
			nudgeConfig := &v1beta2.NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v1beta2.NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: v1beta2.NudgeConfigSpec{
					Actions: &v1beta2.Actions{
						ForceFire: &v1beta2.ForceFireAction{Target: target},
					},
				},
				Status: v1beta2.NudgeConfigStatus{
					ActiveBatches: []v1beta2.ActiveBatch{
						{
							Target:       target,
							BatchID:      "force-fire-batch",
							Phase:        v1beta2.BatchPhaseAccumulating,
							CreatedAt:    now,
							HardDeadline: metav1.NewTime(now.Add(time.Hour)),
							Accumulated: []v1beta2.AccumulatedEntry{
								{From: "source", ImageDigest: "sha256:abc", BuildPipelineRun: buildPLR.Name, CapturedAt: now},
							},
						},
					},
				},
			}
			existingPLR := &tektonv1.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: plrName, Namespace: "default"}}
			reconciler.Client = newClient(nudgeConfig, buildPLR, existingPLR)

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(nudgeConfig)})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			updated := &v1beta2.NudgeConfig{}
			Expect(reconciler.Client.Get(ctx, client.ObjectKeyFromObject(nudgeConfig), updated)).To(Succeed())
			Expect(updated.Spec.Actions).To(BeNil())
			Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseCompleted))
		})
	})

	Context("When a forceFire action cannot be satisfied", func() {
		It("should return an error and leave the action in spec", func() {
			nudgeConfig := &v1beta2.NudgeConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v1beta2.NudgeConfigSingletonName,
					Namespace: "default",
				},
				Spec: v1beta2.NudgeConfigSpec{
					Actions: &v1beta2.Actions{
						ForceFire: &v1beta2.ForceFireAction{Target: "missing-target"},
					},
				},
			}
			reconciler.Client = newClient(nudgeConfig)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(nudgeConfig)})
			Expect(err).To(MatchError(ContainSubstring("no force-fireable batch")))

			updated := &v1beta2.NudgeConfig{}
			Expect(reconciler.Client.Get(ctx, client.ObjectKeyFromObject(nudgeConfig), updated)).To(Succeed())
			Expect(updated.Spec.Actions).NotTo(BeNil())
		})
	})
})

var _ = Describe("nudgeBatchStatusChanged predicate", func() {
	var predicate nudgeBatchStatusChanged

	BeforeEach(func() {
		predicate = nudgeBatchStatusChanged{}
	})

	It("should react to active batch status changes", func() {
		oldNC := &v1beta2.NudgeConfig{Status: v1beta2.NudgeConfigStatus{}}
		newNC := &v1beta2.NudgeConfig{
			Status: v1beta2.NudgeConfigStatus{
				ActiveBatches: []v1beta2.ActiveBatch{{BatchID: "b1", Target: "t", Phase: v1beta2.BatchPhaseAccumulating}},
			},
		}
		Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldNC, ObjectNew: newNC})).To(BeTrue())
	})

	It("should react when spec.actions is added", func() {
		oldNC := &v1beta2.NudgeConfig{Spec: v1beta2.NudgeConfigSpec{}}
		newNC := &v1beta2.NudgeConfig{
			Spec: v1beta2.NudgeConfigSpec{
				Actions: &v1beta2.Actions{ForceFire: &v1beta2.ForceFireAction{Target: "t"}},
			},
		}
		Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldNC, ObjectNew: newNC})).To(BeTrue())
	})

	It("should allow create events", func() {
		Expect(predicate.Create(event.CreateEvent{})).To(BeTrue())
	})

	It("should ignore delete events", func() {
		Expect(predicate.Delete(event.DeleteEvent{})).To(BeFalse())
	})

	It("should allow generic events", func() {
		Expect(predicate.Generic(event.GenericEvent{})).To(BeTrue())
	})

	It("should allow updates when either object is nil", func() {
		nc := &v1beta2.NudgeConfig{}
		Expect(predicate.Update(event.UpdateEvent{ObjectNew: nc})).To(BeTrue())
		Expect(predicate.Update(event.UpdateEvent{ObjectOld: nc})).To(BeTrue())
	})

	It("should ignore unrelated spec updates", func() {
		oldNC := &v1beta2.NudgeConfig{
			Spec: v1beta2.NudgeConfigSpec{
				Nudges: []v1beta2.NudgeRelationship{{From: "a", To: "b"}},
			},
		}
		newNC := oldNC.DeepCopy()
		newNC.Spec.Nudges[0].To = "c"
		Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldNC, ObjectNew: newNC})).To(BeFalse())
	})
})
