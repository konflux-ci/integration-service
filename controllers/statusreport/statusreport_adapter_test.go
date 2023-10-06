/*
Copyright 2023.

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

package statusreport

import (
	"context"
	"fmt"
	"os"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/loader"
	"github.com/redhat-appstudio/integration-service/status"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockStatusAdapter struct {
	Reporter          *MockStatusReporter
	GetReportersError error
}

type MockStatusReporter struct {
	Called            bool
	ReportStatusError error
}

func (r *MockStatusReporter) ReportStatus(client.Client, context.Context, *tektonv1beta1.PipelineRun) error {
	r.Called = true
	return r.ReportStatusError
}

func (r *MockStatusReporter) ReportStatusForSnapshot(client.Client, context.Context, *helpers.IntegrationLogger, *applicationapiv1alpha1.Snapshot) error {
	r.Called = true
	r.ReportStatusError = nil
	return r.ReportStatusError
}

func (a *MockStatusAdapter) GetReporters(object client.Object) ([]status.Reporter, error) {
	return []status.Reporter{a.Reporter}, a.GetReportersError
}

var _ = Describe("Snapshot Adapter", Ordered, func() {
	var (
		adapter        *Adapter
		logger         helpers.IntegrationLogger
		statusAdapter  *MockStatusAdapter
		statusReporter *MockStatusReporter

		hasApp      *applicationapiv1alpha1.Application
		hasSnapshot *applicationapiv1alpha1.Snapshot
	)
	const (
		SampleRepoLink  = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		sample_image    = "quay.io/redhat-appstudio/sample-image"
		sample_revision = "random-value"
	)

	BeforeAll(func() {
		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "application-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                         "component",
					gitops.SnapshotComponentLabel:                    "component-sample",
					"build.appstudio.redhat.com/pipeline":            "enterprise-contract",
					gitops.PipelineAsCodeEventTypeLabel:              "pull_request",
					"pac.test.appstudio.openshift.io/url-org":        "testorg",
					"pac.test.appstudio.openshift.io/url-repository": "testrepo",
					"pac.test.appstudio.openshift.io/sha":            "testsha",
					gitops.PipelineAsCodeGitProviderLabel:            gitops.PipelineAsCodeGitHubProviderType,
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation:   "123",
					"build.appstudio.redhat.com/commit_sha":         "6c65b2fcaea3e1a0a92476c8b5dc89e92a85f025",
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
					gitops.SnapshotTestsStatusAnnotation:            "[{\"scenario\":\"scenario-1\",\"status\":\"EnvironmentProvisionError\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"lastUpdateTime\":\"2023-08-26T17:57:49+02:00\",\"details\":\"Failed to find deploymentTargetClass with right provisioner for copy of existingEnvironment\"}]",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           "component-sample",
						ContainerImage: sample_image,
						Source: applicationapiv1alpha1.ComponentSource{
							ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
								GitSource: &applicationapiv1alpha1.GitSource{
									Revision: sample_revision,
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		// enable feature flag for testing
		err := os.Setenv(FeatureFlagStatusReprotingEnabled, "yes")
		Expect(err).To(BeNil())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

		_ = os.Unsetenv(FeatureFlagStatusReprotingEnabled)
	})

	When("adapter is created", func() {
		It("can create a new Adapter instance", func() {
			Expect(reflect.TypeOf(NewAdapter(hasSnapshot, hasApp, logger, loader.NewMockLoader(), k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
		})

		It("ensures the statusResport is called", func() {
			adapter = NewAdapter(hasSnapshot, hasApp, logger, loader.NewMockLoader(), k8sClient, ctx)
			statusReporter = &MockStatusReporter{}
			statusAdapter = &MockStatusAdapter{Reporter: statusReporter}
			adapter.status = statusAdapter
			adapter.context = loader.GetMockedContext(ctx, []loader.MockData{
				{
					ContextKey: loader.ApplicationContextKey,
					Resource:   hasApp,
				},
				{
					ContextKey: loader.SnapshotContextKey,
					Resource:   hasSnapshot,
				},
			})
			result, err := adapter.EnsureSnapshotTestStatusReported()
			fmt.Fprintf(GinkgoWriter, "-------err: %v\n", err)
			fmt.Fprintf(GinkgoWriter, "-------result: %v\n", result)
			Expect(!result.CancelRequest && err == nil).To(BeTrue())
		})
	})

})
