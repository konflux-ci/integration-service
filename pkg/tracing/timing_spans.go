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

package tracing

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

func CtxFromSpanContext(jsonCarrier string) (context.Context, bool) {
	if jsonCarrier == "" {
		return context.Background(), false
	}
	var carrier map[string]string
	if err := json.Unmarshal([]byte(jsonCarrier), &carrier); err != nil {
		setupLog.Info("ignoring malformed span context annotation", "error", err)
		return context.Background(), false
	}
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.MapCarrier(carrier))
	sc := trace.SpanContextFromContext(ctx)
	return ctx, sc.IsValid()
}

func buildCommonAttributes(pr *tektonv1.PipelineRun, labels LabelNames) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 6)
	attrs = append(attrs,
		NamespaceKey.String(pr.Namespace),
		PipelineRunKey.String(pr.Name),
		DeliveryPipelineRunUIDKey.String(string(pr.UID)),
	)
	prLabels := pr.GetLabels()
	for _, m := range []struct {
		labelName string
		key       attribute.Key
	}{
		{labels.Action, semconv.CICDPipelineActionNameKey},
		{labels.Application, DeliveryApplicationKey},
		{labels.Component, DeliveryComponentKey},
	} {
		if m.labelName == "" {
			continue
		}
		if v := prLabels[m.labelName]; v != "" {
			attrs = append(attrs, m.key.String(v))
		}
	}
	return attrs
}

func buildExecuteAttributes(pr *tektonv1.PipelineRun) []attribute.KeyValue {
	cond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if cond == nil {
		return nil
	}
	attrs := []attribute.KeyValue{ResultEnum(cond)}
	if cond.Status == corev1.ConditionFalse && cond.Message != "" {
		attrs = append(attrs, DeliveryResultMessageKey.String(TruncateResultMessage(cond.Message)))
	}
	return attrs
}

func EmitWaitDuration(ctx context.Context, pr *tektonv1.PipelineRun, labels LabelNames) {
	if pr.Status.StartTime == nil {
		return
	}
	start := pr.CreationTimestamp.Time
	end := pr.Status.StartTime.Time
	if end.Before(start) {
		return
	}

	_, span := otel.Tracer(TracerName).Start(ctx, SpanWaitDuration,
		trace.WithTimestamp(start),
		trace.WithAttributes(buildCommonAttributes(pr, labels)...),
	)
	span.End(trace.WithTimestamp(end))
}

func EmitExecuteDuration(ctx context.Context, pr *tektonv1.PipelineRun, labels LabelNames) {
	if pr.Status.StartTime == nil || pr.Status.CompletionTime == nil {
		return
	}
	start := pr.Status.StartTime.Time
	end := pr.Status.CompletionTime.Time
	if end.Before(start) {
		return
	}

	common := buildCommonAttributes(pr, labels)
	extra := buildExecuteAttributes(pr)
	attrs := make([]attribute.KeyValue, 0, len(common)+len(extra))
	attrs = append(attrs, common...)
	attrs = append(attrs, extra...)

	_, span := otel.Tracer(TracerName).Start(ctx, SpanExecuteDuration,
		trace.WithTimestamp(start),
		trace.WithAttributes(attrs...),
	)
	span.End(trace.WithTimestamp(end))
}

// EmitTimingSpans returns true only after both spans are emitted.
func EmitTimingSpans(pr *tektonv1.PipelineRun, labels LabelNames, spanContext string) bool {
	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); !ok {
		return false
	}

	if pr.Status.StartTime == nil || pr.Status.CompletionTime == nil {
		return false
	}

	parentCtx, ok := CtxFromSpanContext(spanContext)
	if !ok {
		parentCtx = context.Background()
	}

	EmitWaitDuration(parentCtx, pr, labels)
	EmitExecuteDuration(parentCtx, pr, labels)

	return true
}
