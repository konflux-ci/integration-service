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
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/loader"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RecordBuildForBatchedNudge records a successful source build against the target's open batch
// window (ADR-0072). When no batch exists yet, it creates one with fireAt=now+debounce and
// hardDeadline=now+maxWait. When a batch is already Accumulating (or Blocked and this build clears
// failures), it appends to accumulated or replaces the entry for the same source component,
// resets fireAt to now+debounce without moving hardDeadline or createdAt, and leaves batchId unchanged.
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

	// Up to 5 attempts: re-read the NudgeConfig and re-apply the change on every
	// resourceVersion conflict (e.g. another reconcile racing on the same singleton).
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()

		now := metav1.Now()
		debounce, maxWait, _ := latest.Spec.EffectiveBatchPolicy(targetName)
		batchIdx, batch := findOpenBatch(latest.Status.ActiveBatches, targetName)
		if batch == nil {
			batchID := newBatchID(targetName, now)
			newBatch := v1beta2.ActiveBatch{
				Target:       targetName,
				BatchID:      batchID,
				Phase:        v1beta2.BatchPhaseAccumulating,
				CreatedAt:    now,
				HardDeadline: metav1.NewTime(now.Add(maxWait)),
				Accumulated:  []v1beta2.AccumulatedEntry{},
			}
			latest.Status.ActiveBatches = append(latest.Status.ActiveBatches, newBatch)
			batchIdx = len(latest.Status.ActiveBatches) - 1
			batch = &latest.Status.ActiveBatches[batchIdx]
		}

		replaced := false
		for i, entry := range batch.Accumulated {
			if entry.From == buildResult.SourceComponentName {
				batch.Accumulated[i].ImageDigest = buildResult.Digest
				batch.Accumulated[i].BuildPipelineRun = buildPLR.Name
				batch.Accumulated[i].CapturedAt = now
				replaced = true
				break
			}
		}
		if !replaced {
			batch.Accumulated = append(batch.Accumulated, v1beta2.AccumulatedEntry{
				From:             buildResult.SourceComponentName,
				ImageDigest:      buildResult.Digest,
				BuildPipelineRun: buildPLR.Name,
				CapturedAt:       now,
			})
		}
		removeFailedEntriesForSource(batch, buildResult.SourceComponentName)
		if len(batch.Failed) == 0 && batch.Phase == v1beta2.BatchPhaseBlocked {
			batch.Phase = v1beta2.BatchPhaseAccumulating
		}
		fireAt := metav1.NewTime(now.Add(debounce))
		batch.FireAt = &fireAt
		latest.Status.ActiveBatches[batchIdx] = *batch

		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// RecordFailedBuildForBatchedNudge records a failed source build against the target's batch window.
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
		reason = "build failed"
	}

	// Up to 5 attempts: re-read the NudgeConfig and re-apply the change on every
	// resourceVersion conflict (e.g. another reconcile racing on the same singleton).
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()

		now := metav1.Now()
		_, maxWait, failurePolicy := latest.Spec.EffectiveBatchPolicy(targetName)
		batchIdx, batch := findOpenBatch(latest.Status.ActiveBatches, targetName)
		if batch == nil {
			batchID := newBatchID(targetName, now)
			newBatch := v1beta2.ActiveBatch{
				Target:       targetName,
				BatchID:      batchID,
				Phase:        v1beta2.BatchPhaseAccumulating,
				CreatedAt:    now,
				HardDeadline: metav1.NewTime(now.Add(maxWait)),
				Accumulated:  []v1beta2.AccumulatedEntry{},
			}
			latest.Status.ActiveBatches = append(latest.Status.ActiveBatches, newBatch)
			batchIdx = len(latest.Status.ActiveBatches) - 1
			batch = &latest.Status.ActiveBatches[batchIdx]
		}

		replaced := false
		for i, entry := range batch.Failed {
			if entry.From == sourceComponentName {
				batch.Failed[i].BuildPipelineRun = buildPLR.Name
				batch.Failed[i].Reason = reason
				batch.Failed[i].CapturedAt = now
				replaced = true
				break
			}
		}
		if !replaced {
			batch.Failed = append(batch.Failed, v1beta2.FailedEntry{
				From:             sourceComponentName,
				BuildPipelineRun: buildPLR.Name,
				Reason:           reason,
				CapturedAt:       now,
			})
		}
		applyFailurePolicyAfterFailedBuild(batch, failurePolicy)
		latest.Status.ActiveBatches[batchIdx] = *batch

		return c.Status().Patch(ctx, latest, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// ProcessForceFireAction immediately fires the accumulating batch for spec.actions.forceFire.target.
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

		batchIdx, batch := findForceFireBatch(latest.Status.ActiveBatches, action.Target, includePartial)
		if batch == nil {
			return fmt.Errorf("no force-fireable batch for target %q", action.Target)
		}
		if len(batch.Accumulated) == 0 {
			return fmt.Errorf("active batch for target %q has no accumulated builds", action.Target)
		}
		if len(batch.Failed) > 0 && !includePartial {
			return fmt.Errorf("active batch for target %q has failed members and includePartial is false", action.Target)
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
// It returns how long to wait before checking again for the earliest pending FireAt.
func ProcessDueNudgeBatches(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig) (time.Duration, error) {
	var nextWake time.Duration
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()
		now := time.Now()
		changed := false
		nextWake = 0

		for i := range latest.Status.ActiveBatches {
			batch := &latest.Status.ActiveBatches[i]
			if batch.Phase == v1beta2.BatchPhaseFiring {
				if err := fireBatch(ctx, c, latest, batch); err != nil {
					batch.Phase = v1beta2.BatchPhaseFailed
					batch.Message = err.Error()
					changed = true
					continue
				}
				changed = true
				continue
			}
			if batch.Phase != v1beta2.BatchPhaseAccumulating {
				continue
			}
			if !BatchShouldFire(batch, now) {
				if batch.FireAt != nil {
					wait := batch.FireAt.Sub(now)
					if wait > 0 && (nextWake == 0 || wait < nextWake) {
						nextWake = wait
					}
				}
				continue
			}
			_, _, failurePolicy := latest.Spec.EffectiveBatchPolicy(batch.Target)
			if !BatchAllowsScheduledFire(batch, failurePolicy) {
				if len(batch.Failed) > 0 && failurePolicy == v1beta2.FailurePolicyBlock {
					if batch.Phase != v1beta2.BatchPhaseBlocked {
						batch.Phase = v1beta2.BatchPhaseBlocked
						batch.FireAt = nil
						batch.Message = "blocked by failed member build(s)"
						changed = true
					}
				}
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
	return nextWake, err
}

// BatchShouldFire reports whether an accumulating batch is due to fire at now.
func BatchShouldFire(batch *v1beta2.ActiveBatch, now time.Time) bool {
	if len(batch.Accumulated) == 0 {
		return false
	}
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

func applyFailurePolicyAfterFailedBuild(batch *v1beta2.ActiveBatch, failurePolicy v1beta2.FailurePolicyType) {
	if len(batch.Failed) == 0 {
		return
	}
	if failurePolicy == v1beta2.FailurePolicyBlock {
		batch.Phase = v1beta2.BatchPhaseBlocked
		batch.FireAt = nil
		batch.Message = "blocked by failed member build(s)"
	}
}

func removeFailedEntriesForSource(batch *v1beta2.ActiveBatch, sourceComponent string) {
	if len(batch.Failed) == 0 {
		return
	}
	filtered := make([]v1beta2.FailedEntry, 0, len(batch.Failed))
	for _, entry := range batch.Failed {
		if entry.From != sourceComponent {
			filtered = append(filtered, entry)
		}
	}
	batch.Failed = filtered
}

func findAccumulatingBatch(batches []v1beta2.ActiveBatch, target string) (int, *v1beta2.ActiveBatch) {
	for i := range batches {
		if batches[i].Target == target && batches[i].Phase == v1beta2.BatchPhaseAccumulating {
			return i, &batches[i]
		}
	}
	return -1, nil
}

func findOpenBatch(batches []v1beta2.ActiveBatch, target string) (int, *v1beta2.ActiveBatch) {
	for i := range batches {
		if batches[i].Target != target {
			continue
		}
		switch batches[i].Phase {
		case v1beta2.BatchPhaseAccumulating, v1beta2.BatchPhaseBlocked:
			return i, &batches[i]
		}
	}
	return -1, nil
}

func findForceFireBatch(batches []v1beta2.ActiveBatch, target string, includePartial bool) (int, *v1beta2.ActiveBatch) {
	idx, batch := findAccumulatingBatch(batches, target)
	if batch != nil {
		return idx, batch
	}
	for i := range batches {
		if batches[i].Target == target && batches[i].Phase == v1beta2.BatchPhaseBlocked {
			return i, &batches[i]
		}
	}
	return -1, nil
}

func fireBatch(ctx context.Context, c client.Client, nudgeConfig *v1beta2.NudgeConfig, batch *v1beta2.ActiveBatch) error {
	if len(batch.Accumulated) == 0 {
		return fmt.Errorf("cannot fire empty batch %q", batch.BatchID)
	}

	plannedName := batch.NudgePipelineRun
	if plannedName == "" {
		nameAnchorEntry := batch.Accumulated[len(batch.Accumulated)-1]
		nameAnchorPLR := &tektonv1.PipelineRun{}
		if err := c.Get(ctx, types.NamespacedName{Name: nameAnchorEntry.BuildPipelineRun, Namespace: nudgeConfig.Namespace}, nameAnchorPLR); err != nil {
			return fmt.Errorf("loading build PipelineRun %q: %w", nameAnchorEntry.BuildPipelineRun, err)
		}
		plannedName = NudgePipelineRunNameForBatchedTarget(nameAnchorPLR, batch.Target)
	}
	existingNudgePLR := &tektonv1.PipelineRun{}
	getErr := c.Get(ctx, types.NamespacedName{Name: plannedName, Namespace: nudgeConfig.Namespace}, existingNudgePLR)
	if getErr == nil {
		batch.NudgePipelineRun = plannedName
		batch.Phase = v1beta2.BatchPhaseCompleted
		batch.Message = batchFireCompletionMessage(batch.Accumulated, len(latestAccumulatedEntryPerSource(batch.Accumulated)))
		return nil
	}
	if !apierrors.IsNotFound(getErr) {
		return fmt.Errorf("loading nudge PipelineRun %q: %w", plannedName, getErr)
	}

	batch.NudgePipelineRun = plannedName

	buildResults, anchorPLR, anchorSource, simpleBranchName, err := resolveBuildResultsForBatchedFire(ctx, c, nudgeConfig.Namespace, batch.Accumulated)
	if err != nil {
		return err
	}

	targetComponent := &applicationapiv1alpha1.Component{}
	if err := c.Get(ctx, types.NamespacedName{Name: batch.Target, Namespace: nudgeConfig.Namespace}, targetComponent); err != nil {
		return fmt.Errorf("loading target component %q: %w", batch.Target, err)
	}

	// Registry credentials are resolved from the last source in the batch (see
	// resolveBuildResultsForBatchedFire). The nudge PipelineRun updates the target
	// repository; using a source SA is a pragmatic default until target-scoped
	// credentials are wired explicitly.
	saName := anchorPLR.Spec.TaskRunTemplate.ServiceAccountName
	if saName == "" {
		saName = tektonconsts.DefaultPipelineServiceAccount
	}
	objectLoader := loader.NewLoader()
	imageRepoHost, imageRepoUser, imageRepoPwd, err := GetImageRegistryCredentials(ctx, c, objectLoader, anchorSource, saName)
	if err != nil {
		return err
	}

	targets := GetNudgeTargetsGithubApp(ctx, c, objectLoader, []applicationapiv1alpha1.Component{*targetComponent}, imageRepoHost, imageRepoUser, imageRepoPwd)
	if len(targets) == 0 {
		targets = GetNudgeTargetsBasicAuth(ctx, c, objectLoader, []applicationapiv1alpha1.Component{*targetComponent}, imageRepoHost, imageRepoUser, imageRepoPwd)
	}
	if len(targets) == 0 {
		return fmt.Errorf("no credentials resolved for batched nudge target %q", batch.Target)
	}

	if err := CreateNudgePipelineRunWithName(ctx, c, anchorPLR, targets, buildResults, simpleBranchName, plannedName); err != nil {
		return err
	}

	batch.Phase = v1beta2.BatchPhaseCompleted
	batch.Message = batchFireCompletionMessage(batch.Accumulated, len(buildResults))
	return nil
}

// latestAccumulatedEntryPerSource keeps the last accumulated entry per source component
// (in batch order) so repeated builds from the same source collapse to the newest image.
func latestAccumulatedEntryPerSource(entries []v1beta2.AccumulatedEntry) []v1beta2.AccumulatedEntry {
	byFrom := make(map[string]v1beta2.AccumulatedEntry, len(entries))
	order := make([]string, 0, len(entries))
	for _, entry := range entries {
		if _, seen := byFrom[entry.From]; !seen {
			order = append(order, entry.From)
		}
		byFrom[entry.From] = entry
	}
	resolved := make([]v1beta2.AccumulatedEntry, 0, len(order))
	for _, from := range order {
		resolved = append(resolved, byFrom[from])
	}
	return resolved
}

func batchFireCompletionMessage(accumulated []v1beta2.AccumulatedEntry, sourceCount int) string {
	return fmt.Sprintf("fired with %d accumulated build(s) covering %d source component(s)", len(accumulated), sourceCount)
}

func resolveBuildResultsForBatchedFire(
	ctx context.Context,
	c client.Client,
	namespace string,
	accumulated []v1beta2.AccumulatedEntry,
) ([]*NudgeBuildResult, *tektonv1.PipelineRun, *applicationapiv1alpha1.Component, bool, error) {
	entries := latestAccumulatedEntryPerSource(accumulated)
	buildResults := make([]*NudgeBuildResult, 0, len(entries))
	var anchorPLR *tektonv1.PipelineRun
	var anchorSource *applicationapiv1alpha1.Component
	simpleBranchName := false

	for _, entry := range entries {
		buildPLR := &tektonv1.PipelineRun{}
		if err := c.Get(ctx, types.NamespacedName{Name: entry.BuildPipelineRun, Namespace: namespace}, buildPLR); err != nil {
			return nil, nil, nil, false, fmt.Errorf("loading build PipelineRun %q: %w", entry.BuildPipelineRun, err)
		}

		sourceComponent := &applicationapiv1alpha1.Component{}
		if err := c.Get(ctx, types.NamespacedName{Name: entry.From, Namespace: namespace}, sourceComponent); err != nil {
			return nil, nil, nil, false, fmt.Errorf("loading source component %q: %w", entry.From, err)
		}

		buildResult, err := ExtractBuildResultForNudging(buildPLR, sourceComponent)
		if err != nil {
			return nil, nil, nil, false, err
		}
		if entry.ImageDigest != "" {
			buildResult.Digest = entry.ImageDigest
		}
		buildResults = append(buildResults, buildResult)
		anchorPLR = buildPLR
		anchorSource = sourceComponent
		if sourceComponent.Annotations != nil && sourceComponent.Annotations[tektonconsts.NudgeSimpleBranchAnnotation] == "true" {
			simpleBranchName = true
		}
	}

	if anchorPLR == nil || anchorSource == nil {
		return nil, nil, nil, false, fmt.Errorf("no build results resolved for batched fire")
	}
	return buildResults, anchorPLR, anchorSource, simpleBranchName, nil
}

// newBatchID returns batchId as <target>_<createdAt-epoch> per ADR-0072.
func newBatchID(targetName string, createdAt metav1.Time) string {
	return fmt.Sprintf("%s_%d", targetName, createdAt.Unix())
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
