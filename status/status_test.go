/*
Copyright 2022 Red Hat Inc.

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

package status_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/status"

	"github.com/go-logr/logr"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockReporter struct{}

func (r *MockReporter) ReportStatusForSnapshot(client.Client, context.Context, *helpers.IntegrationLogger, *applicationapiv1alpha1.Snapshot) error {
	return nil
}

var _ = Describe("Status Adapter", func() {

	var pipelineRun *tektonv1.PipelineRun

	BeforeEach(func() {
		pipelineRun = &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"pac.test.appstudio.openshift.io/git-provider": "github",
				},
			},
		}
	})

	It("can get reporters from a PipelineRun", func() {
		adapter := status.NewAdapter(logr.Discard(), nil, status.WithGitHubReporter(&MockReporter{}))
		reporters, err := adapter.GetReporters(pipelineRun)
		Expect(err).To(BeNil())
		Expect(reporters).To(HaveLen(1))
	})
})
