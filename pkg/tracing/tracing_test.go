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
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
	"go.opentelemetry.io/otel/trace/noop"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"

	"github.com/konflux-ci/integration-service/pkg/tracing"
)

func snapshotTracingEnv(keys ...string) map[string]string {
	saved := map[string]string{}
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			saved[k] = v
		}
	}
	return saved
}

func restoreTracingEnv(keys []string, saved map[string]string) {
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
	for k, v := range saved {
		_ = os.Setenv(k, v)
	}
}

func resetGlobalTracerProviderOnCleanup() {
	prev := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(noop.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	DeferCleanup(func() {
		otel.SetTracerProvider(prev)
		otel.SetTextMapPropagator(prevProp)
	})
}

var _ = Describe("ResultEnum", func() {
	DescribeTable("maps PipelineRun Succeeded condition to the semconv enum",
		func(status corev1.ConditionStatus, reason, want string) {
			cond := &apis.Condition{Status: status, Reason: reason}
			Expect(tracing.ResultEnum(cond).Value.AsString()).To(Equal(want))
		},
		Entry("success",
			corev1.ConditionTrue, tektonv1.PipelineRunReasonSuccessful.String(),
			semconv.CICDPipelineResultSuccess.Value.AsString()),
		Entry("completed is also success",
			corev1.ConditionTrue, tektonv1.PipelineRunReasonCompleted.String(),
			semconv.CICDPipelineResultSuccess.Value.AsString()),
		Entry("failure",
			corev1.ConditionFalse, tektonv1.PipelineRunReasonFailed.String(),
			semconv.CICDPipelineResultFailure.Value.AsString()),
		Entry("timeout",
			corev1.ConditionFalse, tektonv1.PipelineRunReasonTimedOut.String(),
			semconv.CICDPipelineResultTimeout.Value.AsString()),
		Entry("cancelled",
			corev1.ConditionFalse, tektonv1.PipelineRunReasonCancelled.String(),
			semconv.CICDPipelineResultCancellation.Value.AsString()),
		Entry("cancelled-running-finally",
			corev1.ConditionFalse, tektonv1.PipelineRunReasonCancelledRunningFinally.String(),
			semconv.CICDPipelineResultCancellation.Value.AsString()),
		Entry("validation failure falls back to error",
			corev1.ConditionFalse, tektonv1.PipelineRunReasonFailedValidation.String(),
			semconv.CICDPipelineResultError.Value.AsString()),
		Entry("unknown reason falls back to error",
			corev1.ConditionFalse, "SomeFutureTektonReason",
			semconv.CICDPipelineResultError.Value.AsString()),
	)
})

var _ = Describe("TruncateResultMessage", func() {
	It("passes through a short message unchanged", func() {
		Expect(tracing.TruncateResultMessage("short")).To(Equal("short"))
	})

	It("passes through a message exactly at the limit unchanged", func() {
		msg := strings.Repeat("a", tracing.MaxResultMessageLen)
		Expect(tracing.TruncateResultMessage(msg)).To(Equal(msg))
	})

	It("truncates and suffixes a message over the limit", func() {
		msg := strings.Repeat("a", tracing.MaxResultMessageLen+50)
		got := tracing.TruncateResultMessage(msg)
		Expect(len(got)).To(BeNumerically("<=", tracing.MaxResultMessageLen))
		Expect(got).To(HaveSuffix(tracing.TruncatedSuffix))
	})

	It("preserves UTF-8 boundaries when truncating", func() {
		prefix := strings.Repeat("a", tracing.MaxResultMessageLen-len(tracing.TruncatedSuffix)-2)
		msg := prefix + "世" + strings.Repeat("b", 100)
		got := tracing.TruncateResultMessage(msg)
		head := strings.TrimSuffix(got, tracing.TruncatedSuffix)
		Expect(head).NotTo(Equal(got))
		for _, r := range head {
			Expect(r).NotTo(Equal('�'))
		}
	})
})

var _ = Describe("LoadLabelNames", func() {
	envKeys := []string{
		tracing.EnvTracingLabelAction,
		tracing.EnvTracingLabelApplication,
		tracing.EnvTracingLabelComponent,
	}

	snapshotEnv := func() map[string]string {
		saved := map[string]string{}
		for _, k := range envKeys {
			if v, ok := os.LookupEnv(k); ok {
				saved[k] = v
			}
		}
		return saved
	}

	restoreEnv := func(saved map[string]string) {
		for _, k := range envKeys {
			_ = os.Unsetenv(k)
		}
		for k, v := range saved {
			_ = os.Setenv(k, v)
		}
	}

	It("returns the delivery.tekton.dev/* defaults when env vars are unset", func() {
		saved := snapshotEnv()
		DeferCleanup(restoreEnv, saved)
		for _, k := range envKeys {
			_ = os.Unsetenv(k)
		}
		got := tracing.LoadLabelNames()
		Expect(got.Action).To(Equal(tracing.DefaultTracingLabelAction))
		Expect(got.Application).To(Equal(tracing.DefaultTracingLabelApplication))
		Expect(got.Component).To(Equal(tracing.DefaultTracingLabelComponent))
	})

	It("treats an explicit empty-string as disabling the attribute", func() {
		saved := snapshotEnv()
		DeferCleanup(restoreEnv, saved)
		for _, k := range envKeys {
			_ = os.Setenv(k, "")
		}
		got := tracing.LoadLabelNames()
		Expect(got.Action).To(Equal(""))
		Expect(got.Application).To(Equal(""))
		Expect(got.Component).To(Equal(""))
	})
})

var _ = Describe("New (TracerProvider lifecycle)", func() {
	envKeys := []string{
		tracing.EnvOTLPEndpoint,
		tracing.EnvTracesSampler,
		tracing.EnvTracesSamplerArg,
	}

	It("disables sampling when the OTLP endpoint env var is unset", func() {
		saved := snapshotTracingEnv(envKeys...)
		DeferCleanup(restoreTracingEnv, envKeys, saved)
		_ = os.Unsetenv(tracing.EnvOTLPEndpoint)
		resetGlobalTracerProviderOnCleanup()

		tp, err := tracing.New(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(tp).NotTo(BeNil())

		_, isSDK := otel.GetTracerProvider().(*sdktrace.TracerProvider)
		Expect(isSDK).To(BeFalse())

		_, span := otel.Tracer("no-endpoint-probe").Start(context.Background(), "probe")
		defer span.End()
		Expect(span.SpanContext().IsSampled()).To(BeFalse())

		Expect(tp.Shutdown(context.Background())).To(Succeed())
	})

	It("installs an SDK TracerProvider and W3C TraceContext propagator when the endpoint is well-formed", func() {
		saved := snapshotTracingEnv(envKeys...)
		DeferCleanup(restoreTracingEnv, envKeys, saved)
		// otlptracegrpc dials lazily; an unreachable endpoint still yields a usable provider.
		_ = os.Setenv(tracing.EnvOTLPEndpoint, "http://localhost:4317")
		resetGlobalTracerProviderOnCleanup()

		tp, err := tracing.New(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(tp).NotTo(BeNil())

		_, isSDK := otel.GetTracerProvider().(*sdktrace.TracerProvider)
		Expect(isSDK).To(BeTrue())
		_, isW3C := otel.GetTextMapPropagator().(propagation.TraceContext)
		Expect(isW3C).To(BeTrue())

		Expect(tp.Shutdown(context.Background())).To(Succeed())
	})
})

var _ = Describe("TracerProvider.Shutdown", func() {
	envKeys := []string{tracing.EnvOTLPEndpoint}

	It("returns nil for a noop-backed provider", func() {
		saved := snapshotTracingEnv(envKeys...)
		DeferCleanup(restoreTracingEnv, envKeys, saved)
		_ = os.Unsetenv(tracing.EnvOTLPEndpoint)
		resetGlobalTracerProviderOnCleanup()

		tp, err := tracing.New(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(tp.Shutdown(context.Background())).To(Succeed())
	})

	It("is idempotent for an SDK-backed provider", func() {
		saved := snapshotTracingEnv(envKeys...)
		DeferCleanup(restoreTracingEnv, envKeys, saved)
		_ = os.Setenv(tracing.EnvOTLPEndpoint, "http://localhost:4317")
		resetGlobalTracerProviderOnCleanup()

		tp, err := tracing.New(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(tp.Shutdown(context.Background())).To(Succeed())
		// A second call must not panic or surface an error.
		Expect(tp.Shutdown(context.Background())).To(Succeed())
	})
})

var _ = Describe("samplerFromEnv (verified through New)", func() {
	envKeys := []string{
		tracing.EnvOTLPEndpoint,
		tracing.EnvTracesSampler,
		tracing.EnvTracesSamplerArg,
	}

	DescribeTable("the OTEL_TRACES_SAMPLER env var drives the sampling decision",
		func(samplerEnv, samplerArg string, wantSampled bool) {
			saved := snapshotTracingEnv(envKeys...)
			DeferCleanup(restoreTracingEnv, envKeys, saved)
			resetGlobalTracerProviderOnCleanup()

			_ = os.Setenv(tracing.EnvOTLPEndpoint, "http://localhost:4317")
			if samplerEnv == "" {
				_ = os.Unsetenv(tracing.EnvTracesSampler)
			} else {
				_ = os.Setenv(tracing.EnvTracesSampler, samplerEnv)
			}
			if samplerArg == "" {
				_ = os.Unsetenv(tracing.EnvTracesSamplerArg)
			} else {
				_ = os.Setenv(tracing.EnvTracesSamplerArg, samplerArg)
			}

			tp, err := tracing.New(context.Background())
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = tp.Shutdown(context.Background()) })

			_, span := otel.Tracer("samplerFromEnv-probe").Start(context.Background(), "probe")
			defer span.End()
			Expect(span.SpanContext().IsSampled()).To(Equal(wantSampled))
		},
		Entry("always_off blocks sampling", "always_off", "", false),
		Entry("always_on samples", "always_on", "", true),
		Entry("traceidratio with arg=1.0 samples every trace", "traceidratio", "1.0", true),
		Entry("traceidratio with arg=0 samples nothing", "traceidratio", "0", false),
		Entry("parentbased_always_off without parent context defaults to off", "parentbased_always_off", "", false),
		Entry("parentbased_traceidratio with arg=1.0 samples root spans", "parentbased_traceidratio", "1.0", true),
		Entry("unset env defaults to parent-based-always-on (samples root spans)", "", "", true),
		Entry("unrecognized value falls back to the default sampler", "some-future-otel-keyword", "", true),
	)
})
