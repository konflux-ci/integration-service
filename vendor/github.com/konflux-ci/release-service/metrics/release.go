/*
Copyright 2022.

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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ReleaseConcurrentTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "release_concurrent_total",
			Help: "Total number of concurrent release attempts",
		},
		[]string{},
	)

	ReleaseConcurrentPostActionsExecutionsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "release_concurrent_post_actions_executions_total",
			Help: "Total number of concurrent release post actions executions attempts",
		},
		[]string{},
	)

	ReleaseConcurrentProcessingsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "release_concurrent_processings_total",
			Help: "Total number of concurrent release processing attempts",
		},
		[]string{},
	)

	ReleasePreProcessingDurationSeconds = prometheus.NewHistogramVec(
		releasePreProcessingDurationSecondsOpts,
		releasePreProcessingDurationSecondsLabels,
	)
	releasePreProcessingDurationSecondsLabels = []string{
		"reason",
		"target",
	}
	releasePreProcessingDurationSecondsOpts = prometheus.HistogramOpts{
		Name:    "release_pre_processing_duration_seconds",
		Help:    "How long in seconds a Release takes to start processing",
		Buckets: []float64{5, 10, 15, 30, 45, 60, 90, 120, 180, 240, 300},
	}

	ReleaseValidationDurationSeconds = prometheus.NewHistogramVec(
		releaseValidationDurationSecondsOpts,
		releaseValidationDurationSecondsLabels,
	)
	releaseValidationDurationSecondsLabels = []string{
		"reason",
		"target",
	}
	releaseValidationDurationSecondsOpts = prometheus.HistogramOpts{
		Name:    "release_validation_duration_seconds",
		Help:    "How long in seconds a Release takes to validate",
		Buckets: []float64{5, 10, 15, 30, 45, 60, 90, 120, 180, 240, 300},
	}

	ReleaseDurationSeconds = prometheus.NewHistogramVec(
		releaseDurationSecondsOpts,
		releaseDurationSecondsLabels,
	)
	releaseDurationSecondsLabels = []string{
		"post_actions_reason",
		"processing_reason",
		"release_reason",
		"target",
		"validation_reason",
	}
	releaseDurationSecondsOpts = prometheus.HistogramOpts{
		Name:    "release_duration_seconds",
		Help:    "How long in seconds a Release takes to complete",
		Buckets: []float64{60, 150, 300, 450, 600, 750, 900, 1050, 1200, 1800, 3600},
	}

	ReleasePostActionsExecutionDurationSeconds = prometheus.NewHistogramVec(
		releasePostActionsExecutionDurationSecondsOpts,
		releasePostActionsExecutionDurationSecondsLabels,
	)
	releasePostActionsExecutionDurationSecondsLabels = []string{
		"reason",
	}
	releasePostActionsExecutionDurationSecondsOpts = prometheus.HistogramOpts{
		Name:    "release_post_actions_execution_duration_seconds",
		Help:    "How long in seconds Release post-actions take to complete",
		Buckets: []float64{60, 150, 300, 450, 600, 750, 900, 1050, 1200, 1800, 3600},
	}

	ReleaseProcessingDurationSeconds = prometheus.NewHistogramVec(
		releaseProcessingDurationSecondsOpts,
		releaseProcessingDurationSecondsLabels,
	)
	releaseProcessingDurationSecondsLabels = []string{
		"reason",
		"target",
	}
	releaseProcessingDurationSecondsOpts = prometheus.HistogramOpts{
		Name:    "release_processing_duration_seconds",
		Help:    "How long in seconds a Release processing takes to complete",
		Buckets: []float64{60, 150, 300, 450, 600, 750, 900, 1050, 1200, 1800, 3600},
	}

	ReleaseTotal = prometheus.NewCounterVec(
		releaseTotalOpts,
		releaseTotalLabels,
	)
	releaseTotalLabels = []string{
		"post_actions_reason",
		"processing_reason",
		"release_reason",
		"target",
		"validation_reason",
	}
	releaseTotalOpts = prometheus.CounterOpts{
		Name: "release_total",
		Help: "Total number of releases reconciled by the operator",
	}
)

// RegisterCompletedRelease registers a Release as complete, decreasing the number of concurrent releases, adding a new
// observation for the Release duration and increasing the total number of releases. If either the startTime or the
// completionTime parameters are nil, no action will be taken.
func RegisterCompletedRelease(startTime, completionTime *metav1.Time,
	postActionsReason, processingReason, releaseReason, target, validationReason string) {
	if startTime == nil || completionTime == nil {
		return
	}

	labels := prometheus.Labels{
		"post_actions_reason": postActionsReason,
		"processing_reason":   processingReason,
		"release_reason":      releaseReason,
		"target":              target,
		"validation_reason":   validationReason,
	}
	ReleaseConcurrentTotal.WithLabelValues().Dec()
	ReleaseDurationSeconds.
		With(labels).
		Observe(completionTime.Sub(startTime.Time).Seconds())
	ReleaseTotal.With(labels).Inc()
}

// RegisterCompletedReleasePostActionsExecuted registers a Release post-actions execution as complete, adding a new
// observation for the Release post-actions execution duration and decreasing the number of concurrent executions.
// If either the startTime or the completionTime parameters are nil, no action will be taken.
func RegisterCompletedReleasePostActionsExecuted(startTime, completionTime *metav1.Time, reason string) {
	if startTime == nil || completionTime == nil {
		return
	}

	ReleasePostActionsExecutionDurationSeconds.
		With(prometheus.Labels{
			"reason": reason,
		}).
		Observe(completionTime.Sub(startTime.Time).Seconds())
	ReleaseConcurrentPostActionsExecutionsTotal.WithLabelValues().Dec()
}

// RegisterCompletedReleaseProcessing registers a Release processing as complete, adding a new observation for the
// Release processing duration and decreasing the number of concurrent processings. If either the startTime or the
// completionTime parameters are nil, no action will be taken.
func RegisterCompletedReleaseProcessing(startTime, completionTime *metav1.Time, reason, target string) {
	if startTime == nil || completionTime == nil {
		return
	}

	ReleaseProcessingDurationSeconds.
		With(prometheus.Labels{
			"reason": reason,
			"target": target,
		}).
		Observe(completionTime.Sub(startTime.Time).Seconds())
	ReleaseConcurrentProcessingsTotal.WithLabelValues().Dec()
}

// RegisterValidatedRelease registers a Release as validated, adding a new observation for the
// Release validated seconds. If either the startTime or the validationTime are nil,
// no action will be taken.
func RegisterValidatedRelease(startTime, validationTime *metav1.Time, reason, target string) {
	if validationTime == nil || startTime == nil {
		return
	}

	ReleaseValidationDurationSeconds.
		With(prometheus.Labels{
			"reason": reason,
			"target": target,
		}).
		Observe(validationTime.Sub(startTime.Time).Seconds())
}

// RegisterNewRelease register a new Release, increasing the number of concurrent releases.
func RegisterNewRelease() {
	ReleaseConcurrentTotal.WithLabelValues().Inc()
}

// RegisterNewReleaseProcessing registers a new Release processing, adding a new observation for the
// Release start processing duration and increasing the number of concurrent processings. If either the
// startTime or the processingStartTime are nil, no action will be taken.
func RegisterNewReleaseProcessing(startTime, processingStartTime *metav1.Time, reason, target string) {
	if startTime == nil || processingStartTime == nil {
		return
	}

	ReleasePreProcessingDurationSeconds.
		With(prometheus.Labels{
			"reason": reason,
			"target": target,
		}).
		Observe(processingStartTime.Sub(startTime.Time).Seconds())

	ReleaseConcurrentProcessingsTotal.WithLabelValues().Inc()
}

// RegisterNewReleasePostActionsExecution register a new Release post-actions execution, increasing the number of
// concurrent executions.
func RegisterNewReleasePostActionsExecution() {
	ReleaseConcurrentPostActionsExecutionsTotal.WithLabelValues().Inc()
}

func init() {
	metrics.Registry.MustRegister(
		ReleaseConcurrentTotal,
		ReleaseConcurrentProcessingsTotal,
		ReleaseConcurrentPostActionsExecutionsTotal,
		ReleasePreProcessingDurationSeconds,
		ReleaseValidationDurationSeconds,
		ReleaseDurationSeconds,
		ReleasePostActionsExecutionDurationSeconds,
		ReleaseProcessingDurationSeconds,
		ReleaseTotal,
	)
}
