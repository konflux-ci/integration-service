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
	"strings"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	h "github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const maxBatchNudgePLRRetries = 3

// RecordBuildForBatchedNudge records a successful source build into the active batch for targetName.
func RecordBuildForBatchedNudge(
	ctx context.Context,
	c client.Client,
	nudgeConfig *v1beta2.NudgeConfig,
	targetName string,
	buildPLR *tektonv1.PipelineRun,
	buildResult *NudgeBuildResult,
) error {
	if buildResult == nil {
		return fmt.Errorf("build result is required for batched nudge target %q", targetName)
	}
	return recordSuccessForBatchedNudge(ctx, c, nudgeConfig, targetName, buildResult.SourceComponentName, buildPLR.Name, buildResult.Digest)
}

// RecordFailedBuildForBatchedNudge records a failed source build for a batched nudge target.
func RecordFailedBuildForBatchedNudge(
	ctx context.Context,
	c client.Client,
	nudgeConfig *v1beta2.NudgeConfig,
	targetName string,
	sourceComponentName string,
	buildPLR *tektonv1.PipelineRun,
	reason string,
) error {
	if reason == "" {
		reason = "build pipeline failed"
	}
	sourceRef := ""
	if buildPLR != nil {
		sourceRef = buildPLR.Name
	}
	return recordFailureForBatchedNudge(ctx, c, nudgeConfig, targetName, sourceComponentName, sourceRef, reason)
}

// ProcessForceFireAction immediately fires the batch selected by spec.actions.forceFire.
func ProcessForceFireAction(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig) error {
	if nudgeConfig.Spec.Actions == nil || nudgeConfig.Spec.Actions.ForceFire == nil {
		return nil
	}
	action := nudgeConfig.Spec.Actions.ForceFire
	includePartial := true
	if action.IncludePartial != nil {
		includePartial = *action.IncludePartial
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()

		if isBatchFiringForTarget(latest.Status.ActiveBatches, action.Target) {
			return fmt.Errorf("%w: batch for target %q is already firing", ErrForceFireIgnored, action.Target)
		}

		batchIdx, batch := findForceFireBatch(latest.Status.ActiveBatches, action.Target, includePartial)
		if batch == nil {
			return fmt.Errorf("no force-fireable batch for target %q", action.Target)
		}
		if len(batch.Accumulated) == 0 {
			return fmt.Errorf("active batch for target %q has no accumulated builds", action.Target)
		}
		if err := validateForceFireBatch(batch, includePartial); err != nil {
			return err
		}
		if err := fireBatch(ctx, c, latest, batch); err != nil {
			batch.Phase = v1beta2.BatchPhaseFailed
			batch.Message = err.Error()
			latest.Status.ActiveBatches[batchIdx] = *batch
			if patchErr := c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{})); patchErr != nil {
				return patchErr
			}
			return err
		}
		latest.Status.ActiveBatches[batchIdx] = *batch
		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// SetForceFireRejectedCondition records why a forceFire action could not be applied.
func SetForceFireRejectedCondition(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig, message string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()
		now := metav1.Now()
		setNudgeConfigCondition(latest, v1beta2.NudgeConfigConditionForceFireRejected, metav1.ConditionTrue, "Rejected", message, now)
		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// SetForceFireIgnoredCondition records why a forceFire action was ignored.
func SetForceFireIgnoredCondition(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig, message string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()
		now := metav1.Now()
		setNudgeConfigCondition(latest, v1beta2.NudgeConfigConditionForceFireIgnored, metav1.ConditionTrue, "Ignored", message, now)
		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// ClearForceFireIgnoredCondition removes the transient ForceFireIgnored condition.
func ClearForceFireIgnoredCondition(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		if !clearNudgeConfigCondition(latest, v1beta2.NudgeConfigConditionForceFireIgnored) {
			return nil
		}
		original := latest.DeepCopy()
		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// ClearForceFireRejectedCondition removes the transient ForceFireRejected condition.
func ClearForceFireRejectedCondition(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		if !clearNudgeConfigCondition(latest, v1beta2.NudgeConfigConditionForceFireRejected) {
			return nil
		}
		original := latest.DeepCopy()
		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// ClearNudgeConfigActions removes processed one-shot actions from spec using a merge patch.
func ClearNudgeConfigActions(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		if latest.Spec.Actions == nil {
			return nil
		}
		original := latest.DeepCopy()
		latest.Spec.Actions = nil
		return c.Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// ProcessDueNudgeBatches fires batches whose debounce timer or hard deadline has elapsed.
func ProcessDueNudgeBatches(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()
		now := time.Now()
		changed := false

		for i := range latest.Status.ActiveBatches {
			batch := &latest.Status.ActiveBatches[i]
			if batch.Phase != v1beta2.BatchPhaseAccumulating {
				continue
			}
			if !batchTimerExpired(batch, now) {
				continue
			}
			if len(batch.Accumulated) == 0 {
				batch.Phase = v1beta2.BatchPhaseFailed
				batch.Message = "timer expired with no successful builds to nudge"
				changed = true
				continue
			}
			if isBatchFiringForTarget(latest.Status.ActiveBatches, batch.Target) {
				continue
			}
			if err := fireBatch(ctx, c, latest, batch); err != nil {
				batch.Phase = v1beta2.BatchPhaseFailed
				batch.Message = err.Error()
				changed = true
				continue
			}
			changed = true
		}

		if !changed {
			return nil
		}
		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// ProcessFiringNudgeBatches advances batches in Firing phase based on nudge PipelineRun completion.
func ProcessFiringNudgeBatches(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()
		changed := false

		for i := range latest.Status.ActiveBatches {
			batch := &latest.Status.ActiveBatches[i]
			if batch.Phase != v1beta2.BatchPhaseFiring || batch.NudgePipelineRun == "" {
				continue
			}
			plr := &tektonv1.PipelineRun{}
			if err := c.Get(ctx, types.NamespacedName{Name: batch.NudgePipelineRun, Namespace: nudgeConfig.Namespace}, plr); err != nil {
				return fmt.Errorf("loading nudge PipelineRun %q for batch %q: %w", batch.NudgePipelineRun, batch.BatchID, err)
			}
			if plr.Status.CompletionTime == nil {
				continue
			}
			if h.HasPipelineRunSucceeded(plr) {
				batch.Phase = v1beta2.BatchPhaseCompleted
				batch.Message = fmt.Sprintf("nudge PipelineRun %s succeeded", batch.NudgePipelineRun)
				changed = true
				continue
			}
			if batch.RetryCount >= maxBatchNudgePLRRetries {
				batch.Phase = v1beta2.BatchPhaseFailed
				batch.Message = fmt.Sprintf("nudge PipelineRun %s failed after %d retries", batch.NudgePipelineRun, maxBatchNudgePLRRetries)
				changed = true
				continue
			}
			batch.RetryCount++
			if err := retryFiringBatch(ctx, c, latest, batch); err != nil {
				batch.Phase = v1beta2.BatchPhaseFailed
				batch.Message = err.Error()
			}
			changed = true
		}

		if !changed {
			return nil
		}
		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// HasFiringNudgeBatches reports whether any batch is waiting on a nudge PipelineRun.
func HasFiringNudgeBatches(batches []v1beta2.ActiveBatch) bool {
	for i := range batches {
		if batches[i].Phase == v1beta2.BatchPhaseFiring {
			return true
		}
	}
	return false
}

// BatchShouldFire reports whether an accumulating batch is due to fire at now.
func BatchShouldFire(batch *v1beta2.ActiveBatch, now time.Time) bool {
	if len(batch.Accumulated) == 0 {
		return false
	}
	return batchTimerExpired(batch, now)
}

func batchTimerExpired(batch *v1beta2.ActiveBatch, now time.Time) bool {
	if !batch.HardDeadline.IsZero() && !now.Before(batch.HardDeadline.Time) {
		return true
	}
	if batch.FireAt != nil && !now.Before(batch.FireAt.Time) {
		return true
	}
	return false
}

// BatchAllowsScheduledFire reports whether a due batch may fire on the debounce/deadline schedule.
func BatchAllowsScheduledFire(batch *v1beta2.ActiveBatch, failurePolicy v1beta2.FailurePolicyType) bool {
	if len(batch.Failed) == 0 {
		return true
	}
	if failurePolicy == v1beta2.FailurePolicyProceedWithPartial {
		return len(batch.Accumulated) > 0
	}
	return false
}

// NextBatchWakeDuration returns the time until the next accumulating batch should be checked.
func NextBatchWakeDuration(batches []v1beta2.ActiveBatch) time.Duration {
	now := time.Now()
	var next time.Duration
	for i := range batches {
		if batches[i].Phase == v1beta2.BatchPhaseFiring {
			return time.Nanosecond
		}
	}
	for i := range batches {
		batch := &batches[i]
		if batch.Phase != v1beta2.BatchPhaseAccumulating || batch.FireAt == nil {
			continue
		}
		wait := batch.FireAt.Sub(now)
		if wait <= 0 {
			return 0
		}
		if next == 0 || wait < next {
			next = wait
		}
	}
	return next
}

func findAccumulatingBatch(batches []v1beta2.ActiveBatch, target string) (int, *v1beta2.ActiveBatch) {
	for i := range batches {
		if batches[i].Target == target && batches[i].Phase == v1beta2.BatchPhaseAccumulating {
			return i, &batches[i]
		}
	}
	return -1, nil
}

// findForceFireBatch locates the batch that should respond to spec.actions.forceFire for target.
// Whether partial/failed batches may fire is decided by validateForceFireBatch, not here.
func findForceFireBatch(batches []v1beta2.ActiveBatch, target string, includePartial bool) (int, *v1beta2.ActiveBatch) {
	_ = includePartial
	idx, batch := findAccumulatingBatch(batches, target)
	if batch != nil {
		return idx, batch
	}
	for i := range batches {
		if batches[i].Target != target {
			continue
		}
		switch batches[i].Phase {
		case v1beta2.BatchPhaseBlocked, v1beta2.BatchPhaseFailed:
			return i, &batches[i]
		}
	}
	return -1, nil
}

func findOrCreateAcceptingBatch(batches []v1beta2.ActiveBatch, target string, now metav1.Time, maxWait time.Duration) (int, *v1beta2.ActiveBatch, []v1beta2.ActiveBatch) {
	for i := range batches {
		if batches[i].Target == target &&
			(batches[i].Phase == v1beta2.BatchPhaseAccumulating || batches[i].Phase == v1beta2.BatchPhaseBlocked) {
			return i, &batches[i], batches
		}
	}

	batchID := newBatchID(target, now)
	newBatch := v1beta2.ActiveBatch{
		Target:       target,
		BatchID:      batchID,
		Phase:        v1beta2.BatchPhaseAccumulating,
		CreatedAt:    now,
		HardDeadline: metav1.NewTime(now.Add(maxWait)),
		Accumulated:  []v1beta2.AccumulatedEntry{},
	}
	batches = append(batches, newBatch)
	return len(batches) - 1, &batches[len(batches)-1], batches
}

func isBatchFiringForTarget(batches []v1beta2.ActiveBatch, target string) bool {
	for i := range batches {
		if batches[i].Target == target && batches[i].Phase == v1beta2.BatchPhaseFiring {
			return true
		}
	}
	return false
}

func newBatchID(target string, createdAt metav1.Time) string {
	return fmt.Sprintf("%s_%d", target, createdAt.Unix())
}

func upsertAccumulatedEntry(batch *v1beta2.ActiveBatch, entry v1beta2.AccumulatedEntry) {
	for i := range batch.Accumulated {
		if batch.Accumulated[i].From == entry.From {
			batch.Accumulated[i] = entry
			return
		}
	}
	batch.Accumulated = append(batch.Accumulated, entry)
}

func removeAccumulatedComponent(batch *v1beta2.ActiveBatch, from string) {
	filtered := make([]v1beta2.AccumulatedEntry, 0, len(batch.Accumulated))
	for _, entry := range batch.Accumulated {
		if entry.From != from {
			filtered = append(filtered, entry)
		}
	}
	batch.Accumulated = filtered
}

func upsertFailedEntry(batch *v1beta2.ActiveBatch, entry v1beta2.FailedEntry) {
	for i := range batch.Failed {
		if batch.Failed[i].From == entry.From {
			batch.Failed[i] = entry
			return
		}
	}
	batch.Failed = append(batch.Failed, entry)
}

func removeFailedComponent(batch *v1beta2.ActiveBatch, from string) {
	filtered := make([]v1beta2.FailedEntry, 0, len(batch.Failed))
	for _, entry := range batch.Failed {
		if entry.From != from {
			filtered = append(filtered, entry)
		}
	}
	batch.Failed = filtered
}

func scheduleBatchFire(batch *v1beta2.ActiveBatch, now metav1.Time, debounce time.Duration) {
	if batch.Phase == v1beta2.BatchPhaseBlocked {
		return
	}
	if len(batch.Accumulated) == 0 {
		batch.FireAt = nil
		return
	}
	if !now.Time.Before(batch.HardDeadline.Time) {
		batch.FireAt = &metav1.Time{Time: now.Time}
		return
	}
	fireAt := metav1.NewTime(now.Add(debounce))
	batch.FireAt = &fireAt
}

func validateForceFireBatch(batch *v1beta2.ActiveBatch, includePartial bool) error {
	if includePartial || len(batch.Failed) == 0 {
		return nil
	}
	return fmt.Errorf("forceFire rejected: includePartial=false but failed components remain: %s", strings.Join(failedComponentNames(batch), ", "))
}

func failedComponentNames(batch *v1beta2.ActiveBatch) []string {
	names := make([]string, 0, len(batch.Failed))
	for _, entry := range batch.Failed {
		names = append(names, entry.From)
	}
	return names
}

func recordSuccessForBatchedNudge(
	ctx context.Context,
	c client.Client,
	nudgeConfig *v1beta2.NudgeConfig,
	targetName, sourceComponent, sourceRef, digest string,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()

		now := metav1.Now()
		debounce, maxWait, _ := latest.Spec.EffectiveBatchPolicy(targetName)
		batchIdx, _, batches := findOrCreateAcceptingBatch(latest.Status.ActiveBatches, targetName, now, maxWait)
		latest.Status.ActiveBatches = batches
		batch := &latest.Status.ActiveBatches[batchIdx]

		for _, entry := range batch.Accumulated {
			if entry.From == sourceComponent &&
				entry.ImageDigest == digest &&
				entry.BuildPipelineRun == sourceRef {
				return nil
			}
		}

		upsertAccumulatedEntry(batch, v1beta2.AccumulatedEntry{
			From:             sourceComponent,
			ImageDigest:      digest,
			BuildPipelineRun: sourceRef,
			CapturedAt:       now,
		})
		removeFailedComponent(batch, sourceComponent)

		if batch.Phase == v1beta2.BatchPhaseBlocked && len(batch.Failed) == 0 {
			batch.Phase = v1beta2.BatchPhaseAccumulating
		}
		scheduleBatchFire(batch, now, debounce)

		latest.Status.ActiveBatches[batchIdx] = *batch
		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

func recordFailureForBatchedNudge(
	ctx context.Context,
	c client.Client,
	nudgeConfig *v1beta2.NudgeConfig,
	targetName, sourceComponent, sourceRef, reason string,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()

		now := metav1.Now()
		_, maxWait, failurePolicy := latest.Spec.EffectiveBatchPolicy(targetName)
		batchIdx, _, batches := findOrCreateAcceptingBatch(latest.Status.ActiveBatches, targetName, now, maxWait)
		latest.Status.ActiveBatches = batches
		batch := &latest.Status.ActiveBatches[batchIdx]

		for _, entry := range batch.Failed {
			if entry.From == sourceComponent &&
				entry.BuildPipelineRun == sourceRef &&
				entry.Reason == reason {
				return nil
			}
		}

		removeAccumulatedComponent(batch, sourceComponent)
		upsertFailedEntry(batch, v1beta2.FailedEntry{
			From:             sourceComponent,
			BuildPipelineRun: sourceRef,
			Reason:           reason,
			CapturedAt:       now,
		})

		if failurePolicy == v1beta2.FailurePolicyBlock {
			batch.Phase = v1beta2.BatchPhaseBlocked
			batch.FireAt = nil
		}

		latest.Status.ActiveBatches[batchIdx] = *batch
		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

func fireBatch(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig, batch *v1beta2.ActiveBatch) error {
	if len(batch.Accumulated) == 0 {
		return fmt.Errorf("cannot fire empty batch %q", batch.BatchID)
	}

	plannedName := batchPipelineRunName(batch.BatchID, batch.RetryCount)
	existingPLR := &tektonv1.PipelineRun{}
	if err := c.Get(ctx, types.NamespacedName{Name: plannedName, Namespace: nudgeConfig.Namespace}, existingPLR); err == nil {
		batch.NudgePipelineRun = plannedName
		batch.Phase = v1beta2.BatchPhaseCompleted
		batch.Message = fmt.Sprintf("fired with %d accumulated build(s)", len(batch.Accumulated))
		return nil
	}

	referencePLR, target, renovateEntries, err := buildBatchedRenovatePayload(ctx, c, nudgeConfig, batch)
	if err != nil {
		return err
	}

	batch.Phase = v1beta2.BatchPhaseFiring
	plrName, err := CreateBatchedNudgePipelineRun(ctx, c, referencePLR, target, renovateEntries, batch.BatchID, batch.RetryCount)
	if err != nil {
		return err
	}

	batch.NudgePipelineRun = plrName
	batch.Message = fmt.Sprintf("fired with %d accumulated build(s)", len(batch.Accumulated))
	return nil
}

func retryFiringBatch(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig, batch *v1beta2.ActiveBatch) error {
	referencePLR, target, renovateEntries, err := buildBatchedRenovatePayload(ctx, c, nudgeConfig, batch)
	if err != nil {
		return err
	}
	plrName, err := CreateBatchedNudgePipelineRun(ctx, c, referencePLR, target, renovateEntries, batch.BatchID, batch.RetryCount)
	if err != nil {
		return err
	}
	batch.NudgePipelineRun = plrName
	batch.Message = fmt.Sprintf("retry %d for batch with %d accumulated build(s)", batch.RetryCount, len(batch.Accumulated))
	return nil
}

func buildBatchedRenovatePayload(
	ctx context.Context,
	c client.Client,
	nudgeConfig *v1beta2.NudgeConfig,
	batch *v1beta2.ActiveBatch,
) (*tektonv1.PipelineRun, NudgeTarget, []BatchedRenovateEntry, error) {
	firstEntry := batch.Accumulated[0]
	// Any surviving source build PLR supplies the TaskRun service account; when all were pruned,
	// accumulated digests remain authoritative and the default pipeline SA is used.
	referencePLR, err := resolveReferenceBuildPLR(ctx, c, nudgeConfig.Namespace, batch.Accumulated)
	if err != nil {
		return nil, NudgeTarget{}, nil, err
	}

	targetComponent := &applicationapiv1alpha1.Component{}
	if err := c.Get(ctx, types.NamespacedName{Name: batch.Target, Namespace: nudgeConfig.Namespace}, targetComponent); err != nil {
		return nil, NudgeTarget{}, nil, fmt.Errorf("loading target component %q: %w", batch.Target, err)
	}

	var renovateEntries []BatchedRenovateEntry
	objectLoader := loader.NewLoader()
	for _, accumulated := range batch.Accumulated {
		sourceComponent := &applicationapiv1alpha1.Component{}
		if err := c.Get(ctx, types.NamespacedName{Name: accumulated.From, Namespace: nudgeConfig.Namespace}, sourceComponent); err != nil {
			return nil, NudgeTarget{}, nil, fmt.Errorf("loading source component %q: %w", accumulated.From, err)
		}
		buildResult, err := buildResultForAccumulatedEntry(ctx, c, nudgeConfig.Namespace, accumulated, sourceComponent, referencePLR)
		if err != nil {
			return nil, NudgeTarget{}, nil, err
		}
		renovateEntries = append(renovateEntries, BatchedRenovateEntry{BuildResult: buildResult})
	}

	saName := referencePLR.Spec.TaskRunTemplate.ServiceAccountName
	if saName == "" {
		saName = tektonconsts.DefaultPipelineServiceAccount
	}
	firstSource := &applicationapiv1alpha1.Component{}
	if err := c.Get(ctx, types.NamespacedName{Name: firstEntry.From, Namespace: nudgeConfig.Namespace}, firstSource); err != nil {
		return nil, NudgeTarget{}, nil, fmt.Errorf("loading source component %q: %w", firstEntry.From, err)
	}
	imageRepoHost, imageRepoUser, imageRepoPwd, err := GetImageRegistryCredentials(ctx, c, objectLoader, firstSource, saName)
	if err != nil {
		return nil, NudgeTarget{}, nil, err
	}

	targets := GetNudgeTargetsGithubApp(ctx, c, objectLoader, []applicationapiv1alpha1.Component{*targetComponent}, imageRepoHost, imageRepoUser, imageRepoPwd)
	if len(targets) == 0 {
		targets = GetNudgeTargetsBasicAuth(ctx, c, objectLoader, []applicationapiv1alpha1.Component{*targetComponent}, imageRepoHost, imageRepoUser, imageRepoPwd)
	}
	if len(targets) == 0 {
		return nil, NudgeTarget{}, nil, fmt.Errorf("no credentials resolved for batched nudge target %q", batch.Target)
	}
	return referencePLR, targets[0], renovateEntries, nil
}

func resolveReferenceBuildPLR(
	ctx context.Context,
	c client.Client,
	namespace string,
	accumulated []v1beta2.AccumulatedEntry,
) (*tektonv1.PipelineRun, error) {
	for _, entry := range accumulated {
		if entry.BuildPipelineRun == "" {
			continue
		}
		plr := &tektonv1.PipelineRun{}
		err := c.Get(ctx, types.NamespacedName{Name: entry.BuildPipelineRun, Namespace: namespace}, plr)
		if err == nil {
			return plr, nil
		}
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("loading reference build PipelineRun %q: %w", entry.BuildPipelineRun, err)
		}
	}
	return &tektonv1.PipelineRun{ObjectMeta: metav1.ObjectMeta{Namespace: namespace}}, nil
}

func setNudgeConfigCondition(nc *v1beta2.NudgeConfig, conditionType string, status metav1.ConditionStatus, reason, message string, now metav1.Time) {
	for i := range nc.Status.Conditions {
		if nc.Status.Conditions[i].Type == conditionType {
			nc.Status.Conditions[i].Status = status
			nc.Status.Conditions[i].Reason = reason
			nc.Status.Conditions[i].Message = message
			nc.Status.Conditions[i].LastTransitionTime = now
			return
		}
	}
	nc.Status.Conditions = append(nc.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}

func clearNudgeConfigCondition(nc *v1beta2.NudgeConfig, conditionType string) bool {
	filtered := make([]metav1.Condition, 0, len(nc.Status.Conditions))
	removed := false
	for _, cond := range nc.Status.Conditions {
		if cond.Type == conditionType {
			removed = true
			continue
		}
		filtered = append(filtered, cond)
	}
	nc.Status.Conditions = filtered
	return removed
}
