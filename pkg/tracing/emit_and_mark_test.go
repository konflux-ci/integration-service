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

package tracing_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace/noop"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/konflux-ci/integration-service/pkg/tracing"
)

var _ = Describe("EmitAndMarkTimingSpans", func() {
	var (
		ctx    context.Context
		pr     *tektonv1.PipelineRun
		cl     client.Client
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(tektonv1.AddToScheme(scheme)).To(Succeed())

		start := time.Now().Add(-time.Minute)
		end := time.Now()
		pr = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pr-sample",
				Namespace: "default",
			},
			Status: tektonv1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1.PipelineRunStatusFields{
					StartTime:      &metav1.Time{Time: start},
					CompletionTime: &metav1.Time{Time: end},
				},
			},
		}
		cl = fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr.DeepCopy()).Build()
	})

	refetchFrom := func(c client.Client) func() (*tektonv1.PipelineRun, error) {
		return func() (*tektonv1.PipelineRun, error) {
			latest := &tektonv1.PipelineRun{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(pr), latest); err != nil {
				return nil, err
			}
			return latest, nil
		}
	}

	It("returns nil patched when the annotation is already present", func() {
		pr.Annotations = map[string]string{tracing.TimingEmittedAnnotation: "true"}

		patched, err := tracing.EmitAndMarkTimingSpans(ctx, pr, "", "", cl, refetchFrom(cl))

		Expect(err).NotTo(HaveOccurred())
		Expect(patched).To(BeNil())
	})

	It("returns nil patched when the global tracer provider is the noop", func() {
		prev := otel.GetTracerProvider()
		otel.SetTracerProvider(noop.NewTracerProvider())
		DeferCleanup(func() { otel.SetTracerProvider(prev) })

		patched, err := tracing.EmitAndMarkTimingSpans(ctx, pr, "", "", cl, refetchFrom(cl))

		Expect(err).NotTo(HaveOccurred())
		Expect(patched).To(BeNil())
	})

	It("returns nil patched when start or completion time is missing", func() {
		prev := otel.GetTracerProvider()
		otel.SetTracerProvider(sdktrace.NewTracerProvider())
		DeferCleanup(func() { otel.SetTracerProvider(prev) })

		pr.Status.StartTime = nil
		pr.Status.CompletionTime = nil

		patched, err := tracing.EmitAndMarkTimingSpans(ctx, pr, "", "", cl, refetchFrom(cl))

		Expect(err).NotTo(HaveOccurred())
		Expect(patched).To(BeNil())
	})

	It("emits spans and patches the timingEmitted annotation when the run has completed", func() {
		prev := otel.GetTracerProvider()
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		DeferCleanup(func() {
			otel.SetTracerProvider(prev)
			_ = tp.Shutdown(ctx)
		})

		patched, err := tracing.EmitAndMarkTimingSpans(ctx, pr, "", "", cl, refetchFrom(cl))

		Expect(err).NotTo(HaveOccurred())
		Expect(patched).NotTo(BeNil())
		Expect(patched.GetAnnotations()).To(HaveKeyWithValue(tracing.TimingEmittedAnnotation, "true"))

		stored := &tektonv1.PipelineRun{}
		Expect(cl.Get(ctx, client.ObjectKeyFromObject(pr), stored)).To(Succeed())
		Expect(stored.GetAnnotations()).To(HaveKeyWithValue(tracing.TimingEmittedAnnotation, "true"))
	})

	It("surfaces the error when the refetch callback fails", func() {
		prev := otel.GetTracerProvider()
		otel.SetTracerProvider(sdktrace.NewTracerProvider())
		DeferCleanup(func() { otel.SetTracerProvider(prev) })

		boom := errors.New("refetch failed")
		patched, err := tracing.EmitAndMarkTimingSpans(ctx, pr, "", "", cl, func() (*tektonv1.PipelineRun, error) {
			return nil, boom
		})

		Expect(err).To(MatchError(boom))
		Expect(patched).To(BeNil())
	})

	It("surfaces the patch error and leaves the timingEmitted annotation unset so a future reconcile retries", func() {
		prev := otel.GetTracerProvider()
		otel.SetTracerProvider(sdktrace.NewTracerProvider())
		DeferCleanup(func() { otel.SetTracerProvider(prev) })

		boom := errors.New("patch failed")
		failingCl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pr.DeepCopy()).
			WithInterceptorFuncs(interceptor.Funcs{
				Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
					return boom
				},
			}).Build()

		patched, err := tracing.EmitAndMarkTimingSpans(ctx, pr, "", "", failingCl, refetchFrom(failingCl))

		Expect(err).To(MatchError(boom))
		Expect(patched).To(BeNil())

		stored := &tektonv1.PipelineRun{}
		Expect(failingCl.Get(ctx, client.ObjectKeyFromObject(pr), stored)).To(Succeed())
		Expect(stored.GetAnnotations()).NotTo(HaveKey(tracing.TimingEmittedAnnotation))
	})
})
