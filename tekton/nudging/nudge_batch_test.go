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

package nudging_test

import (
	"time"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	nudging "github.com/konflux-ci/integration-service/tekton/nudging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("pruneCompletedBatches (slice helper)", func() {
	It("removes a completed batch whose CreatedAt exceeds the 24h TTL", func() {
		now := time.Now()
		old := metav1.NewTime(now.Add(-25 * time.Hour))
		batches := []v1beta2.ActiveBatch{
			{BatchID: "old-done", Phase: v1beta2.BatchPhaseCompleted, CreatedAt: old},
			{BatchID: "active", Phase: v1beta2.BatchPhaseAccumulating, CreatedAt: metav1.NewTime(now)},
		}
		result := nudging.FilterExpiredCompletedBatches(batches, now)
		Expect(result).To(HaveLen(1))
		Expect(result[0].BatchID).To(Equal("active"))
	})

	It("keeps a completed batch that is still within the 24h TTL", func() {
		now := time.Now()
		recent := metav1.NewTime(now.Add(-23 * time.Hour))
		batches := []v1beta2.ActiveBatch{
			{BatchID: "recent-done", Phase: v1beta2.BatchPhaseCompleted, CreatedAt: recent},
		}
		result := nudging.FilterExpiredCompletedBatches(batches, now)
		Expect(result).To(HaveLen(1))
	})

	It("keeps non-Completed batches regardless of age", func() {
		now := time.Now()
		old := metav1.NewTime(now.Add(-48 * time.Hour))
		batches := []v1beta2.ActiveBatch{
			{BatchID: "old-failed", Phase: v1beta2.BatchPhaseFailed, CreatedAt: old},
			{BatchID: "old-blocked", Phase: v1beta2.BatchPhaseBlocked, CreatedAt: old},
		}
		result := nudging.FilterExpiredCompletedBatches(batches, now)
		Expect(result).To(HaveLen(2))
	})
})

var _ = Describe("HasFiringNudgeBatches", func() {
	It("returns true when at least one batch is in the Firing phase", func() {
		batches := []v1beta2.ActiveBatch{
			{Target: "a", Phase: v1beta2.BatchPhaseAccumulating},
			{Target: "b", Phase: v1beta2.BatchPhaseFiring},
		}
		Expect(nudging.HasFiringNudgeBatches(batches)).To(BeTrue())
	})

	It("returns false when no batch is in the Firing phase", func() {
		batches := []v1beta2.ActiveBatch{
			{Target: "a", Phase: v1beta2.BatchPhaseAccumulating},
			{Target: "b", Phase: v1beta2.BatchPhaseCompleted},
		}
		Expect(nudging.HasFiringNudgeBatches(batches)).To(BeFalse())
	})
})

var _ = Describe("batchShouldFire", func() {
	It("fires when FireAt has elapsed", func() {
		now := time.Now()
		fireAt := metav1.NewTime(now.Add(-time.Minute))
		deadline := metav1.NewTime(now.Add(time.Hour))

		batch := &v1beta2.ActiveBatch{
			FireAt:       &fireAt,
			HardDeadline: deadline,
			Accumulated: []v1beta2.AccumulatedEntry{
				{From: "a", ImageDigest: "sha256:abc", BuildPipelineRun: "plr-1", CapturedAt: metav1.Now()},
			},
		}
		Expect(nudging.BatchShouldFire(batch, now)).To(BeTrue())
	})

	It("does not fire when there are no accumulated builds", func() {
		now := time.Now()
		fireAt := metav1.NewTime(now.Add(-time.Minute))
		batch := &v1beta2.ActiveBatch{
			FireAt:       &fireAt,
			HardDeadline: metav1.NewTime(now.Add(time.Hour)),
		}
		Expect(nudging.BatchShouldFire(batch, now)).To(BeFalse())
	})

	It("does not fire when debounce and deadline are still in the future", func() {
		now := time.Now()
		future := metav1.NewTime(now.Add(time.Hour))
		batch := &v1beta2.ActiveBatch{
			FireAt:       &future,
			HardDeadline: future,
			Accumulated: []v1beta2.AccumulatedEntry{
				{From: "a", ImageDigest: "sha256:abc", BuildPipelineRun: "plr-1", CapturedAt: metav1.Now()},
			},
		}
		Expect(nudging.BatchShouldFire(batch, now)).To(BeFalse())
	})

	It("fires when hard deadline has elapsed even if FireAt is in the future", func() {
		now := time.Now()
		futureFire := metav1.NewTime(now.Add(time.Hour))
		batch := &v1beta2.ActiveBatch{
			FireAt:       &futureFire,
			HardDeadline: metav1.NewTime(now.Add(-time.Minute)),
			Accumulated: []v1beta2.AccumulatedEntry{
				{From: "a", ImageDigest: "sha256:abc", BuildPipelineRun: "plr-1", CapturedAt: metav1.Now()},
			},
		}
		Expect(nudging.BatchShouldFire(batch, now)).To(BeTrue())
	})
})

var _ = Describe("batchAllowsScheduledFire", func() {
	It("allows fire when there are no failed members", func() {
		batch := &v1beta2.ActiveBatch{
			Accumulated: []v1beta2.AccumulatedEntry{{From: "a"}},
		}
		Expect(nudging.BatchAllowsScheduledFire(batch, v1beta2.FailurePolicyBlock)).To(BeTrue())
	})

	It("blocks scheduled fire when failure policy is Block and failures exist", func() {
		batch := &v1beta2.ActiveBatch{
			Accumulated: []v1beta2.AccumulatedEntry{{From: "a"}},
			Failed:      []v1beta2.FailedEntry{{From: "b", BuildPipelineRun: "plr", Reason: "fail", CapturedAt: metav1.Now()}},
		}
		Expect(nudging.BatchAllowsScheduledFire(batch, v1beta2.FailurePolicyBlock)).To(BeFalse())
	})

	It("allows partial scheduled fire when failure policy is ProceedWithPartial", func() {
		batch := &v1beta2.ActiveBatch{
			Accumulated: []v1beta2.AccumulatedEntry{{From: "a"}},
			Failed:      []v1beta2.FailedEntry{{From: "b", BuildPipelineRun: "plr", Reason: "fail", CapturedAt: metav1.Now()}},
		}
		Expect(nudging.BatchAllowsScheduledFire(batch, v1beta2.FailurePolicyProceedWithPartial)).To(BeTrue())
	})
})

var _ = Describe("nextBatchWakeDuration", func() {
	It("should return zero when a batch is already due", func() {
		now := time.Now()
		past := metav1.NewTime(now.Add(-time.Minute))
		batches := []v1beta2.ActiveBatch{
			{
				Phase:       v1beta2.BatchPhaseAccumulating,
				FireAt:      &past,
				Accumulated: []v1beta2.AccumulatedEntry{{From: "a"}},
			},
		}
		Expect(nudging.NextBatchWakeDuration(batches)).To(Equal(time.Duration(0)))
	})

	It("should return immediately when a batch is in Firing phase", func() {
		batches := []v1beta2.ActiveBatch{
			{Phase: v1beta2.BatchPhaseFiring},
		}
		Expect(nudging.NextBatchWakeDuration(batches)).To(Equal(time.Nanosecond))
	})

	It("should return the shortest positive wait among accumulating batches", func() {
		now := time.Now()
		soon := metav1.NewTime(now.Add(2 * time.Minute))
		later := metav1.NewTime(now.Add(10 * time.Minute))
		batches := []v1beta2.ActiveBatch{
			{Phase: v1beta2.BatchPhaseAccumulating, FireAt: &later, Accumulated: []v1beta2.AccumulatedEntry{{From: "a"}}},
			{Phase: v1beta2.BatchPhaseAccumulating, FireAt: &soon, Accumulated: []v1beta2.AccumulatedEntry{{From: "b"}}},
			{Phase: v1beta2.BatchPhaseCompleted, FireAt: &soon},
		}
		wake := nudging.NextBatchWakeDuration(batches)
		Expect(wake).To(BeNumerically(">", 0))
		Expect(wake).To(BeNumerically("<", 3*time.Minute))
	})

	It("should wake at hardDeadline when it is earlier than fireAt", func() {
		now := time.Now()
		laterFire := metav1.NewTime(now.Add(2 * time.Hour))
		soonDeadline := metav1.NewTime(now.Add(10 * time.Minute))
		batches := []v1beta2.ActiveBatch{
			{
				Phase:        v1beta2.BatchPhaseAccumulating,
				FireAt:       &laterFire,
				HardDeadline: soonDeadline,
				Accumulated:  []v1beta2.AccumulatedEntry{{From: "a"}},
			},
		}
		wake := nudging.NextBatchWakeDuration(batches)
		Expect(wake).To(BeNumerically(">", 0))
		Expect(wake).To(BeNumerically("<", 15*time.Minute))
	})
})
