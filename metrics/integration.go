/*
Copyright 2023 Red Hat Inc.

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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	IntegrationSvcResponseSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "integration_svc_response_seconds",
			Help:    "Integration service response time from the moment the buildPipelineRun is completed till the snapshot is marked as in progress status",
			Buckets: []float64{0.5, 1, 2, 3, 4, 5, 6, 7, 10, 15, 30},
		},
	)

	SnapshotCreatedToPipelineRunStartedStaticEnvSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "integration_svc_snapshot_created_to_pipelinerun_with_static_env_started_seconds",
			Help:    "Time duration from the moment the snapshot resource was created till a integration pipelineRun is started in a static environment",
			Buckets: []float64{0.05, 0.1, 0.5, 1, 2, 3, 4, 5, 10, 15, 30},
		},
	)

	SnapshotCreatedToPipelineRunWithEphemeralEnvStartedSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "integration_svc_snapshot_created_to_pipelinerun_with_ephemeral_env_started_seconds",
			Help:    "The duration measures the time elapsed between the creation of a snapshot resource and the initiation of an integration PipelineRun for pipelines operating within an ephemeral environment.",
			Buckets: []float64{0.05, 0.1, 0.5, 1, 2, 3, 4, 5, 10, 15, 30},
		},
	)

	SEBCreatedToReadySeconds = prometheus.NewHistogram(
		sebCreatedToReadySecondsOpts,
	)

	sebCreatedToReadySecondsOpts = prometheus.HistogramOpts{
		Name:    "integration_svc_seb_created_to_ready_seconds",
		Help:    "Time duration from the moment the snapshotEnvironmentBinding was created till the snapshot is deployed to the environment",
		Buckets: []float64{1, 5, 10, 20, 40, 60, 80, 120, 160, 200, 300},
	}

	SEBEphemeralDeploymentsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "integration_svc_seb_ephemeral_deployments_total",
			Help: "Total number of SEB deployments processed by the operator",
		}, []string{"reason"},
	)

	IntegrationPipelineRunTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "integration_svc_integration_pipelinerun_total",
			Help: "Total number of integration PipelineRun created",
		},
	)

	SnapshotConcurrentTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "integration_svc_snapshot_attempt_concurrent_requests",
			Help: "Total number of concurrent snapshot attempts",
		},
	)

	SnapshotDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "integration_svc_snapshot_attempt_duration_seconds",
			Help:    "Snapshot durations from the moment the Snapshot was created till the Snapshot is marked as finished",
			Buckets: []float64{7, 15, 30, 60, 150, 300, 450, 600, 750, 900, 1050},
		},
		[]string{"type", "reason"},
	)

	SnapshotInvalidTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "integration_svc_snapshot_attempt_invalid_total",
			Help: "Number of invalid snapshots",
		},
		[]string{"reason"},
	)

	SnapshotTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "integration_svc_snapshot_attempt_total",
			Help: "Total number of snapshots processed by the operator",
		},
		[]string{"type", "reason"},
	)

	ReleaseLatencySeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "integration_svc_release_latency_seconds",
			Help:    "Latency between integration tests completion and release creation",
			Buckets: []float64{0.05, 0.1, 0.5, 1, 2, 3, 4, 5, 10, 15, 30},
		},
	)
)

func RegisterCompletedSnapshot(conditiontype, reason string, startTime metav1.Time, completionTime *metav1.Time) {
	labels := prometheus.Labels{
		"type":   conditiontype,
		"reason": reason,
	}

	SnapshotConcurrentTotal.Sub(1)
	SnapshotDurationSeconds.With(labels).Observe(completionTime.Sub(startTime.Time).Seconds())
	SnapshotTotal.With(labels).Inc()
}

func RegisterInvalidSnapshot(conditiontype, reason string) {
	SnapshotConcurrentTotal.Dec()
	SnapshotInvalidTotal.With(prometheus.Labels{"reason": reason}).Inc()
	SnapshotTotal.With(prometheus.Labels{
		"type":   conditiontype,
		"reason": reason,
	}).Inc()
}

func RegisterSEBSuccessfulDeployment() {
	SEBEphemeralDeploymentsTotal.With(prometheus.Labels{
		"reason": "successful",
	}).Inc()
}

func RegisterSEBFailedDeployment() {
	SEBEphemeralDeploymentsTotal.With(prometheus.Labels{
		"reason": "failed",
	}).Inc()
}

func RegisterPipelineRunStarted(snapshotCreatedTime metav1.Time, pipelineRunStartTime *metav1.Time) {
	SnapshotCreatedToPipelineRunStartedStaticEnvSeconds.Observe(pipelineRunStartTime.Sub(snapshotCreatedTime.Time).Seconds())
}

func RegisterPipelineRunWithEphemeralEnvStarted(snapshotCreatedTime metav1.Time, pipelineRunStartTime metav1.Time) {
	SnapshotCreatedToPipelineRunWithEphemeralEnvStartedSeconds.Observe(pipelineRunStartTime.Sub(snapshotCreatedTime.Time).Seconds())
}

func RegisterSEBCreatedToReady(sebCreatedTime metav1.Time, sebReadyTime *metav1.Time) {
	SEBCreatedToReadySeconds.
		Observe(sebReadyTime.Sub(sebCreatedTime.Time).Seconds())
}

func RegisterIntegrationResponse(buildPipelineFinishTime metav1.Time, inProgressTime *metav1.Time) {
	IntegrationSvcResponseSeconds.Observe(inProgressTime.Sub(buildPipelineFinishTime.Time).Seconds())
}

func RegisterNewSnapshot() {
	SnapshotConcurrentTotal.Inc()
}

func RegisterNewIntegrationPipelineRun() {
	IntegrationPipelineRunTotal.Inc()
}

func RegisterReleaseLatency(startTime metav1.Time) {
	latency := time.Since(startTime.Time).Seconds()
	ReleaseLatencySeconds.Observe(latency)
}

func init() {
	metrics.Registry.MustRegister(
		SnapshotCreatedToPipelineRunStartedStaticEnvSeconds,
		SnapshotCreatedToPipelineRunWithEphemeralEnvStartedSeconds,
		SEBCreatedToReadySeconds,
		IntegrationSvcResponseSeconds,
		IntegrationPipelineRunTotal,
		SnapshotConcurrentTotal,
		SnapshotDurationSeconds,
		SnapshotInvalidTotal,
		SnapshotTotal,
		ReleaseLatencySeconds,
		SEBEphemeralDeploymentsTotal,
	)
}
