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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/envtest"

	ctrl "sigs.k8s.io/controller-runtime"
)

type inputHeader struct {
	Name string
	Help string
}

func TestMetricsIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Integration Test Suite")
}

var (
	testEnv *envtest.Environment
	ctx     context.Context
	cancel  context.CancelFunc
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.TODO())
	testEnv = &envtest.Environment{}
	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sManager, _ := ctrl.NewManager(cfg, ctrl.Options{
		MetricsBindAddress: ":8081",
		LeaderElection:     false,
	})

	go func() {
		defer GinkgoRecover()
		Expect(k8sManager.Start(ctx)).To(Succeed())
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// createCounterReader creates a prometheus counter type string with the given parameters to be used as input data
// for 'strings.NewReader' in Prometheus 'client_golang' function 'testutil.CollectAndCompare'.
func createCounterReader(header inputHeader, labels string, renderHeader bool, count int) string {
	readerData := ""
	if renderHeader {
		readerData = fmt.Sprintf("# HELP %s %s\n# TYPE %s counter\n", header.Name, header.Help, header.Name)
	}
	readerData += fmt.Sprintf("%s{%s} %d\n", header.Name, labels, count)

	return readerData
}

// createHistogramReader creates a prometheus histogram type string with the given parameters to be used as input data
// for 'strings.NewReader' in Prometheus 'client_golang' function 'testutil.CollectAndCompare'.
func createHistogramReader(header inputHeader, timeBuckets []string, bucketsData []int, labels string, sum float64, count int) string {
	readerData := fmt.Sprintf("# HELP %s %s\n# TYPE %s histogram\n", header.Name, header.Help, header.Name)
	for i, bucket := range timeBuckets {
		readerData += fmt.Sprintf("%s_bucket{%sle=\"%s\"} %d\n", header.Name, labels, bucket, bucketsData[i])
	}

	if labels != "" {
		readerData += fmt.Sprintf("%s_sum{%s} %f\n\n", header.Name, labels, sum)
		readerData += fmt.Sprintf("%s_count{%s} %d\n", header.Name, labels, count)
	} else {
		readerData += fmt.Sprintf("%s_sum %f\n%s_count %d\n", header.Name, sum, header.Name, count)
	}

	return readerData
}
