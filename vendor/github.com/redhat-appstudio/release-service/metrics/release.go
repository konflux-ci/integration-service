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

	ReleaseConcurrentDeploymentsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "release_concurrent_deployments_total",
			Help: "Total number of concurrent release deployment attempts",
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

	ReleaseDeploymentDurationSeconds = prometheus.NewHistogramVec(
		releaseDeploymentDurationSecondsOpts,
		releaseDeploymentDurationSecondsLabels,
	)
	releaseDeploymentDurationSecondsLabels = []string{
		"environment",
		"reason",
		"target",
	}
	releaseDeploymentDurationSecondsOpts = prometheus.HistogramOpts{
		Name:    "release_deployment_duration_seconds",
		Help:    "How long in seconds a Release deployment takes to complete",
		Buckets: []float64{60, 150, 300, 450, 600, 750, 900, 1050, 1200, 1800, 3600},
	}

	ReleaseDurationSeconds = prometheus.NewHistogramVec(
		releaseDurationSecondsOpts,
		releaseDurationSecondsLabels,
	)
	releaseDurationSecondsLabels = []string{
		"deployment_reason",
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
		"deployment_reason",
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
	deploymentReason, postActionsReason, processingReason, releaseReason, target, validationReason string) {
	if startTime == nil || completionTime == nil {
		return
	}

	labels := prometheus.Labels{
		"deployment_reason":   deploymentReason,
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

// RegisterCompletedReleaseDeployment registers a Release deployment as complete, adding a new observation for the
// Release deployment duration and decreasing the number of concurrent deployments. If either the startTime or the
// completionTime parameters are nil, no action will be taken.
func RegisterCompletedReleaseDeployment(startTime, completionTime *metav1.Time, environment, reason, target string) {
	if startTime == nil || completionTime == nil {
		return
	}

	ReleaseDeploymentDurationSeconds.
		With(prometheus.Labels{
			"environment": environment,
			"reason":      reason,
			"target":      target,
		}).
		Observe(completionTime.Sub(startTime.Time).Seconds())
	ReleaseConcurrentDeploymentsTotal.WithLabelValues().Dec()
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

// RegisterNewRelease register a new Release, increasing the number of concurrent releases.
func RegisterNewRelease() {
	ReleaseConcurrentTotal.WithLabelValues().Inc()
}

// RegisterNewReleaseDeployment register a new Release deployment, increasing the number of concurrent deployments.
func RegisterNewReleaseDeployment() {
	ReleaseConcurrentDeploymentsTotal.WithLabelValues().Inc()
}

// RegisterNewReleaseProcessing register a new Release processing, increasing the number of concurrent processings.
func RegisterNewReleaseProcessing() {
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
		ReleaseConcurrentDeploymentsTotal,
		ReleaseConcurrentProcessingsTotal,
		ReleaseConcurrentPostActionsExecutionsTotal,
		ReleaseDeploymentDurationSeconds,
		ReleaseDurationSeconds,
		ReleasePostActionsExecutionDurationSeconds,
		ReleaseProcessingDurationSeconds,
		ReleaseTotal,
	)
}
