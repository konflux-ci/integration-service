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
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var _ = Describe("Metrics Integration", Ordered, func() {
	BeforeAll(func() {
		metrics.Registry.Unregister(SnapshotCreatedToPipelineRunStartedSeconds)
		metrics.Registry.Unregister(SnapshotDurationSeconds)
	})

	var (
		SnapshotCreatedToPipelineRunStartedSecondsHeader = inputHeader{
			Name: "integration_svc_snapshot_created_to_pipelinerun_started_seconds",
			Help: "Time duration from the moment the snapshot resource was created till a integration pipelineRun is started in the environment",
		}

		SnapshotDurationSecondsHeader = inputHeader{
			Name: "integration_svc_snapshot_attempt_duration_seconds",
			Help: "Snapshot durations from the moment the Snapshot was created till the Snapshot is marked as finished",
		}
		SnapshotTotalHeader = inputHeader{
			Name: "integration_svc_snapshot_attempt_total",
			Help: "Total number of snapshots processed by the operator",
		}
	)

	Context("When RegisterPipelineRunStarted is called", func() {
		// As we need to share metrics within the Context, we need to use "per Context" '(Before|After)All'
		BeforeAll(func() {
			// Mocking metrics to be able to resent data with each tests. Otherwise, we would have to take previous tests into account.
			//
			// 'Help' can't be overridden due to 'https://github.com/prometheus/client_golang/blob/83d56b1144a0c2eb10d399e7abbae3333bebc463/prometheus/registry.go#L314'
			SnapshotCreatedToPipelineRunStartedSeconds = prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Name:    "integration_svc_snapshot_created_to_pipelinerun_started_seconds",
					Help:    "Time duration from the moment the snapshot resource was created till a integration pipelineRun is started in the environment",
					Buckets: []float64{1, 5, 10, 30},
				},
			)
		})

		AfterAll(func() {
			metrics.Registry.Unregister(SnapshotCreatedToPipelineRunStartedSeconds)
		})

		// Input seconds for duration of operations less or equal to the following buckets of 1, 5, 10 and 30 seconds
		inputSeconds := []float64{1, 3, 8, 15}
		elapsedSeconds := 0.0

		// Refactored after removal of bugged concurrent snapshot metric to check for data being written to histograms
		It("registers the pipeline run start time'.", func() {
			creationTime := metav1.Time{}
			for _, seconds := range inputSeconds {
				startTime := metav1.NewTime(creationTime.Add(time.Second * time.Duration(seconds)))
				elapsedSeconds += seconds
				RegisterPipelineRunStarted(creationTime, &startTime)
			}
			// check that the histograms contain data
			Expect(testutil.CollectAndCount(SnapshotCreatedToPipelineRunStartedSeconds)).To(BeNumerically(">", 0))
		})

		It("registers a new observation for 'integration_svc_snapshot_created_to_pipelinerun_started_seconds' with the elapsed time from the moment"+
			"the snapshot is created to first integration pipelineRun is started.", func() {
			// Defined buckets for SnapshotCreatedToPipelineRunStartedSeconds
			timeBuckets := []string{"1", "5", "10", "30"}
			data := []int{1, 2, 3, 4}
			readerData := createHistogramReader(SnapshotCreatedToPipelineRunStartedSecondsHeader, timeBuckets, data, "", elapsedSeconds, len(inputSeconds))
			Expect(testutil.CollectAndCompare(SnapshotCreatedToPipelineRunStartedSeconds, strings.NewReader(readerData))).To(Succeed())
		})
	})

	Context("When RegisterCompletedSnapshot is called", func() {
		BeforeAll(func() {
			SnapshotCreatedToPipelineRunStartedSeconds = prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Name:    "integration_svc_snapshot_created_to_pipelinerun_started_seconds",
					Help:    "Time duration from the moment the snapshot resource was created till a integration pipelineRun is started in the environment",
					Buckets: []float64{1, 5, 10, 30},
				},
			)
			SnapshotDurationSeconds = prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    "integration_svc_snapshot_attempt_duration_seconds",
					Help:    "Snapshot durations from the moment the Snapshot was created till the Snapshot is marked as finished",
					Buckets: []float64{60, 600, 1800, 3600},
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
			metrics.Registry.MustRegister(SnapshotCreatedToPipelineRunStartedSeconds, SnapshotDurationSeconds, SnapshotTotal)
		})

		AfterAll(func() {
			metrics.Registry.Unregister(SnapshotCreatedToPipelineRunStartedSeconds)
			metrics.Registry.Unregister(SnapshotDurationSeconds)
			metrics.Registry.Unregister(SnapshotTotal)
		})

		// Input seconds for duration of operations less or equal to the following buckets of 60, 600, 1800 and 3600 seconds
		inputSeconds := []float64{30, 500, 1500, 3000}
		elapsedSeconds := 0.0
		labels := fmt.Sprintf(`reason="%s", type="%s",`, "passed", "AppStudioIntegrationStatus")

		It("increments 'SnapshotTotal'", func() {
			startTime := metav1.Time{Time: time.Now()}
			completionTime := startTime
			for _, seconds := range inputSeconds {
				completionTime := metav1.NewTime(completionTime.Add(time.Second * time.Duration(seconds)))
				elapsedSeconds += seconds
				RegisterCompletedSnapshot("AppStudioIntegrationStatus", "passed", startTime, &completionTime)
			}
			readerData := createCounterReader(SnapshotTotalHeader, labels, true, len(inputSeconds))
			Expect(testutil.CollectAndCompare(SnapshotTotal, strings.NewReader(readerData))).To(Succeed())
		})

		It("registers a new observation for 'SnapshotDurationSeconds' with the elapsed time from the moment the Snapshot is created.", func() {
			timeBuckets := []string{"60", "600", "1800", "3600"}
			// For each time bucket how many Snapshot completed below 4 seconds
			data := []int{1, 2, 3, 4}
			readerData := createHistogramReader(SnapshotDurationSecondsHeader, timeBuckets, data, labels, elapsedSeconds, len(inputSeconds))
			Expect(testutil.CollectAndCompare(SnapshotDurationSeconds, strings.NewReader(readerData))).To(Succeed())
		})
	})

	Context("When RegisterInvalidSnapshot", func() {

		It("increments the 'SnapshotTotal' metric.", func() {
			for i := 0; i < 10; i++ {
				RegisterInvalidSnapshot("AppStudioIntegrationStatus", "invalid")
			}
			labels := fmt.Sprintf(`reason="%s", type="%s",`, "invalid", "AppStudioIntegrationStatus")
			readerData := createCounterReader(SnapshotTotalHeader, labels, true, 10.0)
			Expect(testutil.CollectAndCompare(SnapshotTotal.WithLabelValues("AppStudioIntegrationStatus", "invalid"),
				strings.NewReader(readerData))).To(Succeed())
		})
	})

	Context("When RegisterReleaseLatency is called", func() {

		metrics.Registry.Unregister(ReleaseLatencySeconds)

		BeforeAll(func() {
			// Mocking metrics to reset data with each test.
			ReleaseLatencySeconds = prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Name:    "release_latency_seconds",
					Help:    "Latency between integration tests completion and release creation",
					Buckets: []float64{0.05, 0.1, 0.5, 1, 2, 3, 4, 5, 10, 15, 30},
				},
			)
			// Register this metric with the registry
			metrics.Registry.MustRegister(ReleaseLatencySeconds)
		})

		AfterAll(func() {
			metrics.Registry.Unregister(ReleaseLatencySeconds)
		})

		var startTime, completionTime metav1.Time

		BeforeEach(func() {
			// Set completion time to current time and start time to 60 seconds prior.
			completionTime = metav1.Time{Time: time.Now()}
			startTime = metav1.Time{Time: completionTime.Add(-60 * time.Second)}
		})

		It("adds an observation to ReleaseLatencySeconds", func() {
			RegisterReleaseLatency(startTime)

			// Calculate observed latency
			observedLatency := completionTime.Sub(startTime.Time).Seconds()

			// Ensure the observed latency is within ±5ms of 60 seconds
			Expect(observedLatency).To(BeNumerically(">=", 59.995))
			Expect(observedLatency).To(BeNumerically("<=", 60.005))
		})

		It("measures latency for only one release", func() {
			RegisterReleaseLatency(startTime)
			Expect(testutil.CollectAndCount(ReleaseLatencySeconds)).To(Equal(1))
		})
	})
})
