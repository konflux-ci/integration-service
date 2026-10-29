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

var _ = Describe("referenceBuildPLRForBatch and buildBatchedRenovatePayload", func() {
	const namespace = "ref-batch-ns"

	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(v1beta2.AddToScheme(scheme)).To(Succeed())
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())
		Expect(applicationapiv1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	It("returns the first existing build PipelineRun from accumulated entries", func() {
		buildPLR := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "build-plr-1", Namespace: namespace},
			Spec: tektonv1.PipelineRunSpec{
				TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{ServiceAccountName: "build-sa"},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(buildPLR).Build()
		batch := &v1beta2.ActiveBatch{
			Accumulated: []v1beta2.AccumulatedEntry{
				{BuildPipelineRun: "missing-plr", ServiceAccountName: "ignored-sa"},
				{BuildPipelineRun: buildPLR.Name, ServiceAccountName: "ignored-sa"},
			},
		}

		reference, err := referenceBuildPLRForBatch(ctx, c, namespace, batch)
		Expect(err).NotTo(HaveOccurred())
		Expect(reference.Name).To(Equal(buildPLR.Name))
		Expect(reference.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("build-sa"))
	})

	It("synthesizes a reference PipelineRun using the first non-empty service account", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		batch := &v1beta2.ActiveBatch{
			Accumulated: []v1beta2.AccumulatedEntry{
				{BuildPipelineRun: "missing-a", ServiceAccountName: ""},
				{BuildPipelineRun: "missing-b", ServiceAccountName: "custom-sa"},
			},
		}

		reference, err := referenceBuildPLRForBatch(ctx, c, namespace, batch)
		Expect(err).NotTo(HaveOccurred())
		Expect(reference.Name).To(BeEmpty())
		Expect(reference.Spec.TaskRunTemplate.ServiceAccountName).To(Equal("custom-sa"))
	})

	It("falls back to the default pipeline service account when none is recorded", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		batch := &v1beta2.ActiveBatch{
			Accumulated: []v1beta2.AccumulatedEntry{
				{BuildPipelineRun: "missing-plr"},
			},
		}

		reference, err := referenceBuildPLRForBatch(ctx, c, namespace, batch)
		Expect(err).NotTo(HaveOccurred())
		Expect(reference.Spec.TaskRunTemplate.ServiceAccountName).To(Equal(tektonconsts.DefaultPipelineServiceAccount))
	})

	It("returns an error from buildBatchedRenovatePayload when the target component is missing", func() {
		now := metav1.Now()
		nudgeConfig := &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{Name: v1beta2.NudgeConfigSingletonName, Namespace: namespace},
		}
		batch := &v1beta2.ActiveBatch{
			Target: "missing-target",
			Accumulated: []v1beta2.AccumulatedEntry{
				{From: "source", BuildPipelineRun: "missing-build", CapturedAt: now},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nudgeConfig).Build()

		_, _, _, err := buildBatchedRenovatePayload(ctx, c, nudgeConfig, batch)
		Expect(err).To(MatchError(ContainSubstring(`loading target component "missing-target"`)))
	})

	It("marks a batch failed when force-fire cannot resolve the target component", func() {
		now := metav1.Now()
		nudgeConfig := &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{Name: v1beta2.NudgeConfigSingletonName, Namespace: namespace},
			Spec: v1beta2.NudgeConfigSpec{
				Actions: &v1beta2.Actions{
					ForceFire: &v1beta2.ForceFireAction{Target: "missing-target"},
				},
			},
			Status: v1beta2.NudgeConfigStatus{
				ActiveBatches: []v1beta2.ActiveBatch{{
					Target:       "missing-target",
					BatchID:      "force-fail",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: "source", ImageDigest: "sha256:abc", BuildPipelineRun: "build-plr", CapturedAt: now},
					},
				}},
			},
		}
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&v1beta2.NudgeConfig{}).
			WithObjects(nudgeConfig).
			Build()

		err := ProcessForceFireAction(ctx, c, nudgeConfig)
		Expect(err).To(HaveOccurred())

		updated := &v1beta2.NudgeConfig{}
		Expect(c.Get(ctx, types.NamespacedName{Name: nudgeConfig.Name, Namespace: namespace}, updated)).To(Succeed())
		Expect(updated.Status.ActiveBatches[0].Phase).To(Equal(v1beta2.BatchPhaseFailed))
		Expect(updated.Status.ActiveBatches[0].Message).NotTo(BeEmpty())
	})

	It("returns status patch errors when marking a force-fired batch as failed", func() {
		now := metav1.Now()
		nudgeConfig := &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{Name: v1beta2.NudgeConfigSingletonName, Namespace: namespace},
			Spec: v1beta2.NudgeConfigSpec{
				Actions: &v1beta2.Actions{
					ForceFire: &v1beta2.ForceFireAction{Target: "missing-target"},
				},
			},
			Status: v1beta2.NudgeConfigStatus{
				ActiveBatches: []v1beta2.ActiveBatch{{
					Target:       "missing-target",
					BatchID:      "force-fail-patch",
					Phase:        v1beta2.BatchPhaseAccumulating,
					CreatedAt:    now,
					HardDeadline: metav1.NewTime(now.Add(time.Hour)),
					Accumulated: []v1beta2.AccumulatedEntry{
						{From: "source", ImageDigest: "sha256:abc", BuildPipelineRun: "build-plr", CapturedAt: now},
					},
				}},
			},
		}
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&v1beta2.NudgeConfig{}).
			WithObjects(nudgeConfig).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourcePatch: func(ctx context.Context, cl client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
					if subResourceName == "status" {
						if nc, ok := obj.(*v1beta2.NudgeConfig); ok && len(nc.Status.ActiveBatches) > 0 &&
							nc.Status.ActiveBatches[0].Phase == v1beta2.BatchPhaseFailed {
							return fmt.Errorf("status patch blocked")
						}
					}
					return cl.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
				},
			}).Build()

		err := ProcessForceFireAction(ctx, c, nudgeConfig)
		Expect(err).To(MatchError(ContainSubstring("status patch blocked")))
	})

	It("returns a wrapped error when loading a reference build PLR fails unexpectedly", func() {
		now := metav1.Now()
		batch := &v1beta2.ActiveBatch{
			Accumulated: []v1beta2.AccumulatedEntry{
				{BuildPipelineRun: "bad-build-plr", CapturedAt: now},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*tektonv1.PipelineRun); ok && key.Name == "bad-build-plr" {
					return fmt.Errorf("apiserver unavailable")
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).Build()

		_, err := referenceBuildPLRForBatch(ctx, c, namespace, batch)
		Expect(err).To(MatchError(ContainSubstring(`loading reference build PipelineRun "bad-build-plr"`)))
	})

	It("returns referenceBuildPLRForBatch errors from buildBatchedRenovatePayload", func() {
		now := metav1.Now()
		nudgeConfig := &v1beta2.NudgeConfig{
			ObjectMeta: metav1.ObjectMeta{Name: v1beta2.NudgeConfigSingletonName, Namespace: namespace},
		}
		batch := &v1beta2.ActiveBatch{
			Target: "nudge-target",
			Accumulated: []v1beta2.AccumulatedEntry{
				{From: "nudge-source", BuildPipelineRun: "bad-build-plr", CapturedAt: now},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nudgeConfig).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*tektonv1.PipelineRun); ok && key.Name == "bad-build-plr" {
					return fmt.Errorf("apiserver unavailable")
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).Build()

		_, _, _, err := buildBatchedRenovatePayload(ctx, c, nudgeConfig, batch)
		Expect(err).To(MatchError(ContainSubstring(`loading reference build PipelineRun "bad-build-plr"`)))
	})
})
