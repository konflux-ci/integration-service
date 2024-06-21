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
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	IntegrationSvcResponseSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "integration_svc_response_seconds",
			Help:    "Integration service response time from the moment the buildPipelineRun is completed till the snapshot is marked as in progress status",
			Buckets: []float64{0.5, 1, 2, 3, 4, 5, 6, 7, 10, 15, 30, 60, 120, 240},
		},
	)

	//This metric should be dropped once SnapshotCreatedToPipelineRunStartedSeconds is merged in prod
	SnapshotCreatedToPipelineRunStartedStaticEnvSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "integration_svc_snapshot_created_to_pipelinerun_with_static_env_started_seconds",
			Help:    "Time duration from the moment the snapshot resource was created till a integration pipelineRun is started in a static environment",
			Buckets: []float64{0.05, 0.1, 0.5, 1, 2, 3, 4, 5, 10, 15, 30},
		},
	)

	SnapshotCreatedToPipelineRunStartedSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "integration_svc_snapshot_created_to_pipelinerun_started_seconds",
			Help:    "Time duration from the moment the snapshot resource was created till a integration pipelineRun is started in the environment",
			Buckets: []float64{0.05, 0.1, 0.5, 1, 2, 3, 4, 5, 10, 15, 30},
		},
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

// IntegrationMetrics represents a collection of metrics to be registered on a
// Prometheus metrics registry for a integration service.
type IntegrationMetrics struct {
	probes []AvailabilityProbe
}

func NewIntegrationMetrics(probes []AvailabilityProbe) *IntegrationMetrics {
	return &IntegrationMetrics{probes: probes}
}

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
	SnapshotTotal.With(prometheus.Labels{
		"type":   conditiontype,
		"reason": reason,
	}).Inc()
}

func RegisterPipelineRunStarted(snapshotCreatedTime metav1.Time, pipelineRunStartTime *metav1.Time) {
	SnapshotCreatedToPipelineRunStartedStaticEnvSeconds.Observe(pipelineRunStartTime.Sub(snapshotCreatedTime.Time).Seconds())
	SnapshotCreatedToPipelineRunStartedSeconds.Observe(pipelineRunStartTime.Sub(snapshotCreatedTime.Time).Seconds())
}

func RegisterIntegrationResponse(duration time.Duration) {
	IntegrationSvcResponseSeconds.Observe(duration.Seconds())
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

func (m *IntegrationMetrics) InitMetrics(registerer prometheus.Registerer) error {
	registerer.MustRegister(
		SnapshotCreatedToPipelineRunStartedStaticEnvSeconds,
		SnapshotCreatedToPipelineRunStartedSeconds,
		IntegrationSvcResponseSeconds,
		IntegrationPipelineRunTotal,
		SnapshotConcurrentTotal,
		SnapshotDurationSeconds,
		SnapshotTotal,
		ReleaseLatencySeconds,
	)
	for _, probe := range m.probes {
		if err := registerer.Register(probe.AvailabilityGauge()); err != nil {
			return fmt.Errorf("failed to register the availability metric: %w", err)
		}
	}

	return nil
}

func (m *IntegrationMetrics) StartAvailabilityProbes(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	log := ctrllog.FromContext(ctx)
	log.Info("starting availability probes")
	go func() {
		for {
			select {
			case <-ctx.Done(): // Shutdown if context is canceled
				log.Info("Shutting down metrics")
				ticker.Stop()
				return
			case <-ticker.C:
				m.checkProbes(ctx)
			}
		}
	}()
}

func (m *IntegrationMetrics) checkProbes(ctx context.Context) {
	for _, probe := range m.probes {
		if probe.CheckAvailability(ctx) {
			probe.AvailabilityGauge().Set(1)
		} else {
			probe.AvailabilityGauge().Set(0)
		}
	}
}

// AvailabilityProbe represents a probe that checks the availability of a certain aspects of the service
type AvailabilityProbe interface {
	CheckAvailability(ctx context.Context) bool
	AvailabilityGauge() prometheus.Gauge
}
