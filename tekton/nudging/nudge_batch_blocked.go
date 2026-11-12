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
	"sort"
	"strings"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcileBatchBlockedCondition sets or clears the BatchBlocked status condition from activeBatches (ADR 0072).
func ReconcileBatchBlockedCondition(nc *v1beta2.NudgeConfig, now metav1.Time) {
	summaries := blockedBatchSummaries(nc.Status.ActiveBatches)
	if len(summaries) == 0 {
		clearNudgeConfigCondition(nc, v1beta2.NudgeConfigConditionBatchBlocked)
		return
	}
	message := strings.Join(summaries, "; ")
	setNudgeConfigCondition(nc, v1beta2.NudgeConfigConditionBatchBlocked, metav1.ConditionTrue, "Blocked", message, now)
}

// SyncBatchBlockedCondition reconciles the BatchBlocked condition against the latest stored activeBatches.
func SyncBatchBlockedCondition(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()
		now := metav1.Now()
		ReconcileBatchBlockedCondition(latest, now)
		if batchBlockedConditionEqual(original, latest) {
			return nil
		}
		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

func blockedBatchSummaries(batches []v1beta2.ActiveBatch) []string {
	summaries := make([]string, 0)
	for i := range batches {
		batch := &batches[i]
		if batch.Phase != v1beta2.BatchPhaseBlocked || len(batch.Failed) == 0 {
			continue
		}
		failed := failedComponentNames(batch)
		sort.Strings(failed)
		summaries = append(summaries, fmt.Sprintf(`target %q blocked: failed source(s) %s`, batch.Target, strings.Join(failed, ", ")))
	}
	sort.Strings(summaries)
	return summaries
}

func batchBlockedConditionEqual(before, after *v1beta2.NudgeConfig) bool {
	oldCond := meta.FindStatusCondition(before.Status.Conditions, v1beta2.NudgeConfigConditionBatchBlocked)
	newCond := meta.FindStatusCondition(after.Status.Conditions, v1beta2.NudgeConfigConditionBatchBlocked)
	if oldCond == nil && newCond == nil {
		return true
	}
	if oldCond == nil || newCond == nil {
		return false
	}
	return oldCond.Status == newCond.Status &&
		oldCond.Reason == newCond.Reason &&
		oldCond.Message == newCond.Message
}
