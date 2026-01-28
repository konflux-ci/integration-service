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
	hourlyBuckets = []float64{5, 10, 30, 60, 90, 180, 300, 450, 900, 1800, 3600}
	dailyBuckets  = []float64{60, 300, 600, 1800, 3600, 7200, 14400, 21600, 28800, 36000, 43200, 57600, 72000, 86400}

	ReleaseConcurrentTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "release_concurrent_total",
			Help: "Total number of concurrent release attempts",
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
		"type",
	}
	releasePreProcessingDurationSecondsOpts = prometheus.HistogramOpts{
		Name:    "release_pre_processing_duration_seconds",
		Help:    "How long in seconds a Release takes to start processing",
		Buckets: hourlyBuckets,
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
		Buckets: hourlyBuckets,
	}

	ReleaseDurationSeconds = prometheus.NewHistogramVec(
		releaseDurationSecondsOpts,
		releaseDurationSecondsLabels,
	)
	// Prometheus fails if these are not in alphabetical order
	releaseDurationSecondsLabels = []string{
		"final_pipeline_processing_reason",
		"managed_collectors_pipeline_processing_reason",
		"managed_pipeline_processing_reason",
		"release_reason",
		"target",
		"tenant_collectors_pipeline_processing_reason",
		"tenant_pipeline_processing_reason",
		"validation_reason",
	}
	releaseDurationSecondsOpts = prometheus.HistogramOpts{
		Name:    "release_duration_seconds",
		Help:    "How long in seconds a Release takes to complete",
		Buckets: dailyBuckets,
	}

	ReleaseProcessingDurationSeconds = prometheus.NewHistogramVec(
		releaseProcessingDurationSecondsOpts,
		releaseProcessingDurationSecondsLabels,
	)
	releaseProcessingDurationSecondsLabels = []string{
		"reason",
		"target",
		"type",
	}
	releaseProcessingDurationSecondsOpts = prometheus.HistogramOpts{
		Name:    "release_processing_duration_seconds",
		Help:    "How long in seconds a Release processing takes to complete",
		Buckets: dailyBuckets,
	}

	ReleaseTotal = prometheus.NewCounterVec(
		releaseTotalOpts,
		releaseTotalLabels,
	)
	// Prometheus fails if these are not in alphabetical order
	releaseTotalLabels = []string{
		"final_pipeline_processing_reason",
		"managed_collectors_pipeline_processing_reason",
		"managed_pipeline_processing_reason",
		"release_reason",
		"target",
		"tenant_collectors_pipeline_processing_reason",
		"tenant_pipeline_processing_reason",
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
	tenantCollectorsProcessingReason, tenantProcessingReason, managedCollectorsProcessingReason, managedProcessingReason,
	finalProcessingReason, releaseReason, target, validationReason string) {
	if startTime == nil || completionTime == nil {
		return
	}

	// the label sequence here does not need to be alphabetical, as it is only assigning
	// the data to the label, so changed to a logical order as the pipelines are executed
	labels := prometheus.Labels{
		"final_pipeline_processing_reason":              finalProcessingReason,
		"managed_collectors_pipeline_processing_reason": managedCollectorsProcessingReason,
		"managed_pipeline_processing_reason":            managedProcessingReason,
		"release_reason":                                releaseReason,
		"target":                                        target,
		"tenant_collectors_pipeline_processing_reason":  tenantCollectorsProcessingReason,
		"tenant_pipeline_processing_reason":             tenantProcessingReason,
		"validation_reason":                             validationReason,
	}
	ReleaseConcurrentTotal.WithLabelValues().Dec()
	ReleaseDurationSeconds.
		With(labels).
		Observe(completionTime.Sub(startTime.Time).Seconds())
	ReleaseTotal.With(labels).Inc()
}

// RegisterCompletedReleasePipelineProcessing registers a Release pipeline processing as complete, adding a
// new observation for the Release processing duration with the specific type and decreasing the number of
// concurent processings. If either the startTime or the completionTime parameters are nil, no action will be taken.
func RegisterCompletedReleasePipelineProcessing(startTime, completionTime *metav1.Time, reason, target, pipelineType string) {
	if startTime == nil || completionTime == nil {
		return
	}

	ReleaseProcessingDurationSeconds.
		With(prometheus.Labels{
			"reason": reason,
			"target": target,
			"type":   pipelineType,
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

// RegisterNewReleasePipelineProcessing registers a new Release Pipeline processing, adding a
// new observation for the Release start pipeline processing duration and increasing the number of
// concurrent processings. If either the startTime or the processingStartTime are nil, no action will be taken.
func RegisterNewReleasePipelineProcessing(startTime, processingStartTime *metav1.Time, reason, target, pipelineType string) {
	if startTime == nil || processingStartTime == nil {
		return
	}

	ReleasePreProcessingDurationSeconds.
		With(prometheus.Labels{
			"reason": reason,
			"target": target,
			"type":   pipelineType,
		}).
		Observe(processingStartTime.Sub(startTime.Time).Seconds())

	ReleaseConcurrentProcessingsTotal.WithLabelValues().Inc()
}

func init() {
	metrics.Registry.MustRegister(
		ReleaseConcurrentTotal,
		ReleaseConcurrentProcessingsTotal,
		ReleasePreProcessingDurationSeconds,
		ReleaseValidationDurationSeconds,
		ReleaseDurationSeconds,
		ReleaseProcessingDurationSeconds,
		ReleaseTotal,
	)
}
