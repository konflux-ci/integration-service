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
	"testing"

	konfluxv1beta2 "github.com/konflux-ci/integration-service/api/konflux/v1beta2"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func forceFireLifecycleFixture(t *testing.T) (*v1beta2.NudgeConfig, *tektonv1.PipelineRun, *fake.ClientBuilder) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := konfluxv1beta2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := tektonv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	nc := &v1beta2.NudgeConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "nudge-config", Namespace: "test", Generation: 1},
		Spec:       v1beta2.NudgeConfigSpec{Actions: &v1beta2.Actions{ForceFire: &v1beta2.ForceFireAction{Target: "target"}}},
		Status: v1beta2.NudgeConfigStatus{ActiveBatches: []v1beta2.ActiveBatch{{
			BatchID: "target_123", Target: "target", Phase: v1beta2.BatchPhaseBlocked,
			Accumulated: []v1beta2.AccumulatedEntry{{From: "source", ImageDigest: "sha256:abc", BuildPipelineRun: "pruned-build"}},
			Failed:      []v1beta2.FailedEntry{{From: "failed-source"}},
		}}},
	}
	plr := &tektonv1.PipelineRun{ObjectMeta: metav1.ObjectMeta{
		Name: BatchedNudgePipelineRunName("target_123", 0), Namespace: "test",
		Labels:      map[string]string{tektonconsts.NudgeTypeLabel: tektonconsts.NudgePipelineRunTypeValue},
		Annotations: map[string]string{tektonconsts.NudgeBatchIDAnnotation: "target_123"},
	}}
	return nc, plr, fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1beta2.NudgeConfig{}).WithObjects(nc, plr)
}

func getForceFireConfig(t *testing.T, c client.Client, nc *v1beta2.NudgeConfig) *v1beta2.NudgeConfig {
	t.Helper()
	latest := &v1beta2.NudgeConfig{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(nc), latest); err != nil {
		t.Fatal(err)
	}
	return latest
}

func TestForceFireLifecycleDefaultPartialAndRecovery(t *testing.T) {
	nc, _, builder := forceFireLifecycleFixture(t)
	fail := true
	c := builder.WithInterceptorFuncs(interceptor.Funcs{
		SubResourcePatch: func(ctx context.Context, c client.Client, resource string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			if resource == "status" && fail {
				return apierrors.NewServiceUnavailable("status unavailable")
			}
			return c.SubResource(resource).Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	if _, err := ProcessForceFireActionResult(context.Background(), c, nc); err == nil {
		t.Fatal("expected API failure")
	}
	latest := getForceFireConfig(t, c, nc)
	if latest.Spec.Actions == nil || latest.Status.ActiveBatches[0].Phase != v1beta2.BatchPhaseBlocked {
		t.Fatal("API failure lost action or changed persisted batch")
	}
	fail = false
	result, err := ProcessForceFireActionResult(context.Background(), c, latest)
	if err != nil || result.Disposition != ForceFireApplied {
		t.Fatalf("recovery: %+v, %v", result, err)
	}
	latest = getForceFireConfig(t, c, nc)
	if latest.Status.ActiveBatches[0].Phase != v1beta2.BatchPhaseFiring {
		t.Fatal("default includePartial did not recover blocked batch")
	}
	var runs tektonv1.PipelineRunList
	if err := c.List(context.Background(), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs.Items) != 1 {
		t.Fatalf("expected one deterministic PipelineRun, got %d", len(runs.Items))
	}
}

func TestForceFireLifecycleUncertainStatusDoesNotFireNextBatch(t *testing.T) {
	nc, _, builder := forceFireLifecycleFixture(t)
	uncertain := true
	c := builder.WithInterceptorFuncs(interceptor.Funcs{
		SubResourcePatch: func(ctx context.Context, c client.Client, resource string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			if err := c.SubResource(resource).Patch(ctx, obj, patch, opts...); err != nil {
				return err
			}
			if uncertain {
				uncertain = false
				return apierrors.NewTimeoutError("response lost", 0)
			}
			return nil
		},
	}).Build()
	if _, err := ProcessForceFireActionResult(context.Background(), c, nc); err == nil {
		t.Fatal("expected uncertain status response")
	}
	latest := getForceFireConfig(t, c, nc)
	latest.Status.ActiveBatches[0].Phase = v1beta2.BatchPhaseCompleted
	next := *latest.Status.ActiveBatches[0].DeepCopy()
	next.BatchID = "target_456"
	next.Phase = v1beta2.BatchPhaseAccumulating
	latest.Status.ActiveBatches = append(latest.Status.ActiveBatches, next)
	if err := c.Status().Update(context.Background(), latest); err != nil {
		t.Fatal(err)
	}
	result, err := ProcessForceFireActionResult(context.Background(), c, latest)
	if err != nil || result.Disposition != ForceFireApplied {
		t.Fatalf("replay: %+v, %v", result, err)
	}
	latest = getForceFireConfig(t, c, nc)
	if latest.Status.ActiveBatches[1].Phase != v1beta2.BatchPhaseAccumulating {
		t.Fatal("replay fired next batch")
	}
	// Completion TTL may remove the claimed batch before consumption succeeds.
	latest.Status.ActiveBatches = latest.Status.ActiveBatches[1:]
	if err := c.Status().Update(context.Background(), latest); err != nil {
		t.Fatal(err)
	}
	if result, err = ProcessForceFireActionResult(context.Background(), c, latest); err != nil || result.Disposition != ForceFireApplied {
		t.Fatalf("removed batch replay: %+v, %v", result, err)
	}
}

func TestForceFireLifecycleOperationalErrorRetainsBatch(t *testing.T) {
	nc, _, builder := forceFireLifecycleFixture(t)
	c := builder.WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*tektonv1.PipelineRun); ok {
				return apierrors.NewServiceUnavailable("PLR lookup unavailable")
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()
	result, err := ProcessForceFireActionResult(context.Background(), c, nc)
	if err == nil || result.Disposition != "" {
		t.Fatalf("operational failure became terminal outcome: %+v, %v", result, err)
	}
	latest := getForceFireConfig(t, c, nc)
	if latest.Spec.Actions == nil || latest.Status.ActiveBatches[0].Phase != v1beta2.BatchPhaseBlocked {
		t.Fatal("operational error lost action or batch")
	}
}

func TestForceFireLifecycleRejectedReceiptDoesNotChangeOutcome(t *testing.T) {
	nc, _, builder := forceFireLifecycleFixture(t)
	no := false
	nc.Spec.Actions.ForceFire.IncludePartial = &no
	c := builder.Build()
	result, err := ProcessForceFireActionResult(context.Background(), c, nc)
	if err != nil || result.Disposition != ForceFireRejected {
		t.Fatalf("expected rejection: %+v, %v", result, err)
	}
	latest := getForceFireConfig(t, c, nc)
	latest.Status.ActiveBatches[0].Failed = nil
	if err := c.Status().Update(context.Background(), latest); err != nil {
		t.Fatal(err)
	}
	again, err := ProcessForceFireActionResult(context.Background(), c, latest)
	if err != nil || again != result {
		t.Fatalf("rejection changed on retry: %+v, %v", again, err)
	}
}

func TestForceFireLifecycleReplacementSurvivesClearConflict(t *testing.T) {
	for _, sameValue := range []bool{false, true} {
		t.Run(fmt.Sprintf("same-value-%v", sameValue), func(t *testing.T) {
			nc, _, builder := forceFireLifecycleFixture(t)
			replaced := false
			c := builder.WithInterceptorFuncs(interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					if !replaced {
						replaced = true
						latest := getForceFireConfig(t, c, nc)
						latest.Generation++
						if !sameValue {
							latest.Spec.Actions.ForceFire.Target = "replacement"
						}
						if err := c.Update(ctx, latest); err != nil {
							return err
						}
					}
					return c.Patch(ctx, obj, patch, opts...)
				},
			}).Build()
			if err := ClearNudgeConfigActions(context.Background(), c, nc); err != nil {
				t.Fatal(err)
			}
			latest := getForceFireConfig(t, c, nc)
			if latest.Spec.Actions == nil {
				t.Fatal("cleared replacement action")
			}
			result, err := ProcessForceFireActionResult(context.Background(), c, nc)
			if err != nil || result.Disposition != ForceFireSuperseded {
				t.Fatalf("stale request processed: %+v, %v", result, err)
			}
		})
	}
}

func TestForceFireLifecycleReceiptWriteFailureHasNoEffects(t *testing.T) {
	nc, _, builder := forceFireLifecycleFixture(t)
	lookups := 0
	c := builder.WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			return apierrors.NewServiceUnavailable("receipt unavailable")
		},
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*tektonv1.PipelineRun); ok {
				lookups++
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()
	if _, err := ProcessForceFireActionResult(context.Background(), c, nc); err == nil {
		t.Fatal("expected receipt persistence failure")
	}
	if lookups != 0 {
		t.Fatal("attempted PLR effect before durably claiming request")
	}
}

func TestForceFireLifecycleEmptyEligibleIsProcessedWithoutPipelineRun(t *testing.T) {
	nc, plr, builder := forceFireLifecycleFixture(t)
	nc.Status.ActiveBatches[0].Accumulated = nil
	c := builder.Build()
	if err := c.Delete(context.Background(), plr); err != nil {
		t.Fatal(err)
	}
	result, err := ProcessForceFireActionResult(context.Background(), c, nc)
	if err != nil || result.Disposition != ForceFireApplied {
		t.Fatalf("empty accepted action: %+v, %v", result, err)
	}
	latest := getForceFireConfig(t, c, nc)
	if latest.Status.ActiveBatches[0].Phase != v1beta2.BatchPhaseFailed {
		t.Fatal("empty batch should fail")
	}
	var runs tektonv1.PipelineRunList
	if err := c.List(context.Background(), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs.Items) != 0 {
		t.Fatal("created a PipelineRun for empty batch")
	}
	if err := ClearNudgeConfigActions(context.Background(), c, nc); err != nil {
		t.Fatal(err)
	}
	if getForceFireConfig(t, c, nc).Spec.Actions != nil {
		t.Fatal("processed empty action was not consumed")
	}
}

func TestForceFireLifecycleRevalidationRejectionIsDurable(t *testing.T) {
	nc, _, builder := forceFireLifecycleFixture(t)
	no := false
	nc.Spec.Actions.ForceFire.IncludePartial = &no
	nc.Status.ActiveBatches[0].Failed = nil
	failStatus := true
	c := builder.WithInterceptorFuncs(interceptor.Funcs{
		SubResourcePatch: func(ctx context.Context, c client.Client, resource string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			if failStatus {
				return apierrors.NewServiceUnavailable("status unavailable")
			}
			return c.SubResource(resource).Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	if _, err := ProcessForceFireActionResult(context.Background(), c, nc); err == nil {
		t.Fatal("expected status failure")
	}
	failStatus = false
	latest := getForceFireConfig(t, c, nc)
	latest.Status.ActiveBatches[0].Failed = []v1beta2.FailedEntry{{From: "new-failure"}}
	if err := c.Status().Update(context.Background(), latest); err != nil {
		t.Fatal(err)
	}
	result, err := ProcessForceFireActionResult(context.Background(), c, latest)
	if err != nil || result.Disposition != ForceFireRejected {
		t.Fatalf("expected revalidation rejection: %+v, %v", result, err)
	}
	latest = getForceFireConfig(t, c, nc)
	latest.Status.ActiveBatches[0].Failed = nil
	if err := c.Status().Update(context.Background(), latest); err != nil {
		t.Fatal(err)
	}
	again, err := ProcessForceFireActionResult(context.Background(), c, latest)
	if err != nil || again != result {
		t.Fatalf("revalidation rejection changed on retry: %+v, %v", again, err)
	}
}
