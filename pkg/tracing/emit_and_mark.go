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

	"github.com/konflux-ci/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EmitAndMarkTimingSpans emits a PipelineRun's timing spans and patches TimingEmittedAnnotation so subsequent reconciles skip re-emission.
func EmitAndMarkTimingSpans(
	ctx context.Context,
	pr *tektonv1.PipelineRun,
	spanContext, failureMsg string,
	cl client.Client,
	refetch func() (*tektonv1.PipelineRun, error),
) (patched *tektonv1.PipelineRun, err error) {
	if _, found := pr.Annotations[TimingEmittedAnnotation]; found {
		return // already marked; nothing to do
	}
	if !EmitTimingSpans(pr, LoadLabelNames(), EmitOptions{SpanContext: spanContext, FailureMsg: failureMsg}) {
		return // emission skipped (noop provider or run not yet complete)
	}
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest, refetchErr := refetch()
		if refetchErr != nil {
			return refetchErr
		}
		mergePatch := client.MergeFrom(latest.DeepCopy())
		_ = metadata.SetAnnotation(&latest.ObjectMeta, TimingEmittedAnnotation, "true")
		if patchErr := cl.Patch(ctx, latest, mergePatch); patchErr != nil {
			return patchErr
		}
		patched = latest
		return nil
	})
	return
}
