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

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/helpers"
	"github.com/redhat-appstudio/integration-service/status"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MockStatusAdapter struct {
	Reporter          *MockStatusReporter
	GetReportersError error
}

type MockStatusReporter struct {
	Called            bool
	ReportStatusError error
}

func (r *MockStatusReporter) ReportStatus(context.Context) error {
	r.Called = true
	return r.ReportStatusError
}

func (a *MockStatusAdapter) GetReporters(ctx context.Context, pipelineRun *tektonv1beta1.PipelineRun) ([]status.Reporter, error) {
	return []status.Reporter{a.Reporter}, a.GetReportersError
}

var _ = Describe("Pipeline Adapter", Ordered, func() {
	var (
		adapter        *Adapter
		statusAdapter  *MockStatusAdapter
		statusReporter *MockStatusReporter

		testpipelineRunBuild     *tektonv1beta1.PipelineRun
		testpipelineRunComponent *tektonv1beta1.PipelineRun
		hasComp                  *applicationapiv1alpha1.Component
		hasApp                   *applicationapiv1alpha1.Application
	)
	const (
		SampleRepoLink = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
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

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-sample",
				Application:    "application-sample",
				ContainerImage: "",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: SampleRepoLink,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())
	})

	BeforeEach(func() {
		testpipelineRunBuild = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-build-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type":  "build",
					"pipelines.openshift.io/used-by":         "build-cloud",
					"pipelines.openshift.io/runtime":         "nodejs",
					"pipelines.openshift.io/strategy":        "s2i",
					"build.appstudio.openshift.io/component": "component-sample",
				},
				Annotations: map[string]string{
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
				},
			},
			Spec: tektonv1beta1.PipelineRunSpec{
				PipelineRef: &tektonv1beta1.PipelineRef{
					Name:   "build-pipeline-pass",
					Bundle: "quay.io/kpavic/test-bundle:build-pipeline-pass",
				},
				Params: []tektonv1beta1.Param{
					{
						Name: "output-image",
						Value: tektonv1beta1.ArrayOrString{
							Type:      "string",
							StringVal: "quay.io/redhat-appstudio/sample-image",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, testpipelineRunBuild)).Should(Succeed())

		testpipelineRunBuild.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				TaskRuns: map[string]*tektonv1beta1.PipelineRunTaskRunStatus{
					"index1": &tektonv1beta1.PipelineRunTaskRunStatus{
						PipelineTaskName: "build-container",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "IMAGE_DIGEST",
										Value: "image_digest_value",
									},
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, testpipelineRunBuild)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      testpipelineRunBuild.Name,
				Namespace: "default",
			}, testpipelineRunBuild)
			return err == nil && len(testpipelineRunBuild.Status.TaskRuns) > 0
		}, time.Second*10).Should(BeTrue())

		adapter = NewAdapter(testpipelineRunBuild, hasComp, hasApp, ctrl.Log, k8sClient, ctx)
		statusReporter = &MockStatusReporter{}
		statusAdapter = &MockStatusAdapter{Reporter: statusReporter}
		adapter.status = statusAdapter
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, testpipelineRunBuild)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	It("can create a new Adapter instance", func() {
		Expect(reflect.TypeOf(NewAdapter(testpipelineRunBuild, hasComp, hasApp, ctrl.Log, k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("ensures the Applicationcomponents can be found ", func() {
		applicationComponents, err := adapter.getAllApplicationComponents(hasApp)
		Expect(err == nil).To(BeTrue())
		Expect(applicationComponents != nil).To(BeTrue())
	})

	It("ensures the Imagepullspec from pipelinerun and prepare applicationSnapshot can be created", func() {
		imageDigest, err := adapter.getImagePullSpecFromPipelineRun(testpipelineRunBuild)
		Expect(err == nil).To(BeTrue())
		Expect(imageDigest != "").To(BeTrue())
		applicationSnapshot, err := adapter.prepareApplicationSnapshot(hasApp, hasComp, imageDigest)
		Expect(err == nil).To(BeTrue())
		Expect(applicationSnapshot != nil).To(BeTrue())
	})

	It("ensures the global component list unchanged and compositeSnapshot shouldn't be created ", func() {
		expectedApplicationSnapshot, err := adapter.prepareApplicationSnapshotForPipelineRun(testpipelineRunBuild, hasComp, hasApp)
		Expect(err == nil).To(BeTrue())
		Expect(expectedApplicationSnapshot != nil).To(BeTrue())

		integrationTestScenarios, err := helpers.GetRequiredIntegrationTestScenariosForApplication(k8sClient, ctx, hasApp)
		Expect(err == nil).To(BeTrue())

		integrationPipelineRuns, err := adapter.getAllPipelineRunsForApplicationSnapshot(expectedApplicationSnapshot, integrationTestScenarios)
		Expect(err == nil).To(BeTrue())
		Expect(expectedApplicationSnapshot != nil).To(BeTrue())

		allIntegrationPipelineRunsPassed, err := adapter.determineIfAllIntegrationPipelinesPassed(integrationPipelineRuns)
		Expect(err == nil).To(BeTrue())
		Expect(allIntegrationPipelineRunsPassed).To(BeTrue())

		// check if the global component list changed in the meantime and create a composite snapshot if it did.
		compositeApplicationSnapshot, err := adapter.createCompositeSnapshotsIfConflictExists(hasApp, hasComp, expectedApplicationSnapshot)
		Expect(err == nil).To(BeTrue())
		Expect(compositeApplicationSnapshot == nil).To(BeTrue())
	})

	It("ensures allApplicationSnapshot exists and can be found ", func() {
		Eventually(func() bool {
			result, err := adapter.EnsureApplicationSnapshotExists()
			fmt.Fprintf(GinkgoWriter, "Err: %v\n", err)
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())
	})

	It("ensures ApplicationSnapshot passed all tests", func() {
		testpipelineRunComponent = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-component-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelinesascode.tekton.dev/url-org":         "redhat-appstudio",
					"pipelinesascode.tekton.dev/original-prname": "build-service-on-push",
					"pipelinesascode.tekton.dev/url-repository":  "build-service",
					"pipelinesascode.tekton.dev/repository":      "build-service-pac",
					"test.appstudio.openshift.io/snapshot":       "snapshot-sample",
				},
				Annotations: map[string]string{
					"pipelinesascode.tekton.dev/on-target-branch": "[main]",
				},
			},
			Spec: tektonv1beta1.PipelineRunSpec{
				PipelineRef: &tektonv1beta1.PipelineRef{
					Name:   "component-pipeline-pass",
					Bundle: "quay.io/kpavic/test-bundle:component-pipeline-pass",
				},
			},
		}
		Expect(k8sClient.Create(ctx, testpipelineRunComponent)).Should(Succeed())
		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      testpipelineRunComponent.Name,
				Namespace: "default",
			}, testpipelineRunComponent)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		adapter = NewAdapter(testpipelineRunComponent, hasComp, hasApp, ctrl.Log, k8sClient, ctx)
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

		Eventually(func() bool {
			result, err := adapter.EnsureApplicationSnapshotPassedAllTests()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())
	})

	It("ensures status is reported for integration PipelineRuns", func() {
		adapter.pipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-status-sample",
				Namespace: "default",
				Labels: map[string]string{
					"test.appstudio.openshift.io/application":    "test-application",
					"test.appstudio.openshift.io/component":      "devfile-sample-go-basic",
					"test.appstudio.openshift.io/snapshot":       "test-application-s8tnj",
					"test.appstudio.openshift.io/scenario":       "example-pass",
					"pipelinesascode.tekton.dev/state":           "started",
					"pipelinesascode.tekton.dev/sender":          "foo",
					"pipelinesascode.tekton.dev/check-run-id":    "9058825284",
					"pipelinesascode.tekton.dev/branch":          "main",
					"pipelinesascode.tekton.dev/url-org":         "devfile-sample",
					"pipelinesascode.tekton.dev/original-prname": "devfile-sample-go-basic-on-pull-request",
					"pipelinesascode.tekton.dev/url-repository":  "devfile-sample-go-basic",
					"pipelinesascode.tekton.dev/repository":      "devfile-sample-go-basic",
					"pipelinesascode.tekton.dev/sha":             "12a4a35ccd08194595179815e4646c3a6c08bb77",
					"pipelinesascode.tekton.dev/git-provider":    "github",
					"pipelinesascode.tekton.dev/event-type":      "pull_request",
					"pipelines.appstudio.openshift.io/type":      "test",
				},
				Annotations: map[string]string{
					"pipelinesascode.tekton.dev/on-target-branch": "[main,master]",
					"pipelinesascode.tekton.dev/repo-url":         "https://github.com/devfile-samples/devfile-sample-go-basic",
					"pipelinesascode.tekton.dev/sha-title":        "Appstudio update devfile-sample-go-basic",
					"pipelinesascode.tekton.dev/git-auth-secret":  "pac-gitauth-zjib",
					"pipelinesascode.tekton.dev/pull-request":     "16",
					"pipelinesascode.tekton.dev/on-event":         "[pull_request]",
					"pipelinesascode.tekton.dev/installation-id":  "30353543",
				},
			},
			Spec: tektonv1beta1.PipelineRunSpec{
				PipelineRef: &tektonv1beta1.PipelineRef{
					Name:   "component-pipeline-pass",
					Bundle: "quay.io/kpavic/test-bundle:component-pipeline-pass",
				},
			},
		}

		Eventually(func() bool {
			result, err := adapter.EnsureStatusReported()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		Expect(statusReporter.Called).To(BeTrue())

		statusAdapter.GetReportersError = errors.New("GetReportersError")

		Eventually(func() bool {
			result, err := adapter.EnsureStatusReported()
			return result.RequeueRequest && err != nil && err.Error() == "GetReportersError"
		}, time.Second*10).Should(BeTrue())

		statusAdapter.GetReportersError = nil
		statusReporter.ReportStatusError = errors.New("ReportStatusError")

		Eventually(func() bool {
			result, err := adapter.EnsureStatusReported()
			return result.RequeueRequest && err != nil && err.Error() == "ReportStatusError"
		}, time.Second*10).Should(BeTrue())
	})

})
