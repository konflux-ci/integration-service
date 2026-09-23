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
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RecordBuildForBatchedNudge appends a successful source build to the target's active batch window.
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

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &v1beta2.NudgeConfig{}
		if err := c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: nudgeConfig.Namespace}, latest); err != nil {
			return err
		}
		original := latest.DeepCopy()

		now := metav1.Now()
		debounce, maxWait, _ := latest.Spec.EffectiveBatchPolicy(targetName)
		batchIdx, batch := findAccumulatingBatch(latest.Status.ActiveBatches, targetName)
		if batch == nil {
			batchID := fmt.Sprintf("%s-%d", targetName, now.UnixNano())
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

		for _, entry := range batch.Accumulated {
			if entry.BuildPipelineRun == buildPLR.Name {
				return nil
			}
		}

		batch.Accumulated = append(batch.Accumulated, v1beta2.AccumulatedEntry{
			From:             buildResult.SourceComponentName,
			ImageDigest:      buildResult.Digest,
			BuildPipelineRun: buildPLR.Name,
			CapturedAt:       now,
		})
		fireAt := metav1.NewTime(now.Add(debounce))
		batch.FireAt = &fireAt
		latest.Status.ActiveBatches[batchIdx] = *batch

		return c.Status().Patch(ctx, latest, client.MergeFrom(original))
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
		if err := fireBatch(ctx, c, latest, batch); err != nil {
			batch.Phase = v1beta2.BatchPhaseFailed
			batch.Message = err.Error()
			latest.Status.ActiveBatches[batchIdx] = *batch
			if patchErr := c.Status().Patch(ctx, latest, client.MergeFrom(original)); patchErr != nil {
				return patchErr
			}
			return err
		}
		latest.Status.ActiveBatches[batchIdx] = *batch
		return c.Status().Patch(ctx, latest, client.MergeFrom(original))
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
		return c.Patch(ctx, latest, client.MergeFrom(original))
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
		return c.Status().Patch(ctx, latest, client.MergeFrom(original))
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

func findAccumulatingBatch(batches []v1beta2.ActiveBatch, target string) (int, *v1beta2.ActiveBatch) {
	for i := range batches {
		if batches[i].Target == target && batches[i].Phase == v1beta2.BatchPhaseAccumulating {
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
	if !includePartial {
		return -1, nil
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
	lastEntry := batch.Accumulated[len(batch.Accumulated)-1]

	buildPLR := &tektonv1.PipelineRun{}
	if err := c.Get(ctx, types.NamespacedName{Name: lastEntry.BuildPipelineRun, Namespace: nudgeConfig.Namespace}, buildPLR); err != nil {
		return fmt.Errorf("loading build PipelineRun %q: %w", lastEntry.BuildPipelineRun, err)
	}

	sourceComponent := &applicationapiv1alpha1.Component{}
	if err := c.Get(ctx, types.NamespacedName{Name: lastEntry.From, Namespace: nudgeConfig.Namespace}, sourceComponent); err != nil {
		return fmt.Errorf("loading source component %q: %w", lastEntry.From, err)
	}

	targetComponent := &applicationapiv1alpha1.Component{}
	if err := c.Get(ctx, types.NamespacedName{Name: batch.Target, Namespace: nudgeConfig.Namespace}, targetComponent); err != nil {
		return fmt.Errorf("loading target component %q: %w", batch.Target, err)
	}

	buildResult, err := ExtractBuildResultForNudging(buildPLR, sourceComponent)
	if err != nil {
		return err
	}

	simpleBranchName := sourceComponent.Annotations != nil && sourceComponent.Annotations[tektonconsts.NudgeSimpleBranchAnnotation] == "true"
	saName := buildPLR.Spec.TaskRunTemplate.ServiceAccountName
	if saName == "" {
		saName = tektonconsts.DefaultPipelineServiceAccount
	}
	imageRepoHost, imageRepoUser, imageRepoPwd, err := GetImageRegistryCredentials(ctx, c, sourceComponent, saName)
	if err != nil {
		return err
	}

	targets := GetNudgeTargetsGithubApp(ctx, c, []applicationapiv1alpha1.Component{*targetComponent}, imageRepoHost, imageRepoUser, imageRepoPwd)
	if len(targets) == 0 {
		targets = GetNudgeTargetsBasicAuth(ctx, c, []applicationapiv1alpha1.Component{*targetComponent}, imageRepoHost, imageRepoUser, imageRepoPwd)
	}
	if len(targets) == 0 {
		return fmt.Errorf("no credentials resolved for batched nudge target %q", batch.Target)
	}

	batch.Phase = v1beta2.BatchPhaseFiring
	if err := CreateNudgePipelineRun(ctx, c, buildPLR, targets, buildResult, simpleBranchName); err != nil {
		return err
	}

	batch.Phase = v1beta2.BatchPhaseCompleted
	batch.Message = fmt.Sprintf("fired with %d accumulated build(s)", len(batch.Accumulated))
	return nil
}

// NextBatchWakeDuration returns the time until the next accumulating batch should be checked.
func NextBatchWakeDuration(batches []v1beta2.ActiveBatch) time.Duration {
	now := time.Now()
	var next time.Duration
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
