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

var _ = Describe("nextBatchWakeDuration", func() {
	It("should return zero when a batch is already due", func() {
		now := time.Now()
		past := metav1.NewTime(now.Add(-time.Minute))
		batches := []v1beta2.ActiveBatch{
			{
				Phase:  v1beta2.BatchPhaseAccumulating,
				FireAt: &past,
			},
		}
		Expect(nudging.NextBatchWakeDuration(batches)).To(Equal(time.Duration(0)))
	})

	It("should return the shortest positive wait among accumulating batches", func() {
		now := time.Now()
		soon := metav1.NewTime(now.Add(2 * time.Minute))
		later := metav1.NewTime(now.Add(10 * time.Minute))
		batches := []v1beta2.ActiveBatch{
			{Phase: v1beta2.BatchPhaseAccumulating, FireAt: &later},
			{Phase: v1beta2.BatchPhaseAccumulating, FireAt: &soon},
			{Phase: v1beta2.BatchPhaseCompleted, FireAt: &soon},
		}
		wake := nudging.NextBatchWakeDuration(batches)
		Expect(wake).To(BeNumerically(">", 0))
		Expect(wake).To(BeNumerically("<", 3*time.Minute))
	})
})
