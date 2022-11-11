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

func (r *MockStatusReporter) ReportStatus(context.Context, *tektonv1beta1.PipelineRun) error {
	r.Called = true
	return r.ReportStatusError
}

func (a *MockStatusAdapter) GetReporters(pipelineRun *tektonv1beta1.PipelineRun) ([]status.Reporter, error) {
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
					"pipelinesascode.tekton.dev/event-type":  "pull_request",
				},
				Annotations: map[string]string{
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
					"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
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
										Value: *tektonv1beta1.NewArrayOrString("image_digest_value"),
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

	It("ensures the Imagepullspec from pipelinerun and prepare snapshot can be created", func() {
		imageDigest, err := adapter.getImagePullSpecFromPipelineRun(testpipelineRunBuild)
		Expect(err == nil).To(BeTrue())
		Expect(imageDigest != "").To(BeTrue())
		snapshot, err := adapter.prepareSnapshot(hasApp, hasComp, imageDigest)
		Expect(err == nil).To(BeTrue())
		Expect(snapshot != nil).To(BeTrue())
	})

	It("ensures the global component list unchanged and compositeSnapshot shouldn't be created ", func() {
		expectedSnapshot, err := adapter.prepareSnapshotForPipelineRun(testpipelineRunBuild, hasComp, hasApp)
		Expect(err == nil).To(BeTrue())
		Expect(expectedSnapshot != nil).To(BeTrue())

		integrationTestScenarios, err := helpers.GetRequiredIntegrationTestScenariosForApplication(k8sClient, ctx, hasApp)
		Expect(err == nil).To(BeTrue())

		integrationPipelineRuns, err := adapter.getAllPipelineRunsForSnapshot(expectedSnapshot, integrationTestScenarios)
		Expect(err == nil).To(BeTrue())
		Expect(expectedSnapshot != nil).To(BeTrue())

		allIntegrationPipelineRunsPassed, err := adapter.determineIfAllIntegrationPipelinesPassed(integrationPipelineRuns)
		Expect(err == nil).To(BeTrue())
		Expect(allIntegrationPipelineRunsPassed).To(BeTrue())

		// check if the global component list changed in the meantime and create a composite snapshot if it did.
		compositeSnapshot, err := adapter.createCompositeSnapshotsIfConflictExists(hasApp, hasComp, expectedSnapshot)
		Expect(err == nil).To(BeTrue())
		Expect(compositeSnapshot == nil).To(BeTrue())
	})

	It("ensures pipelines as code labels and annotations are propagated to the snapshot", func() {
		snapshot, err := adapter.prepareSnapshotForPipelineRun(testpipelineRunBuild, hasComp, hasApp)
		Expect(err).To(BeNil())
		Expect(snapshot).ToNot(BeNil())
		annotation, found := snapshot.GetAnnotations()["test.appstudio.openshift.io/on-target-branch"]
		Expect(found).To(BeTrue())
		Expect(annotation).To(Equal("[main,master]"))
		label, found := snapshot.GetLabels()["test.appstudio.openshift.io/event-type"]
		Expect(found).To(BeTrue())
		Expect(label).To(Equal("pull_request"))
	})

	It("ensures allSnapshot exists and can be found ", func() {
		Eventually(func() bool {
			result, err := adapter.EnsureSnapshotExists()
			fmt.Fprintf(GinkgoWriter, "Err: %v\n", err)
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())
	})

	It("ensures Snapshot passed all tests", func() {
		testpipelineRunComponent = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-component-sample",
				Namespace: "default",
				Labels: map[string]string{
					"test.appstudio.openshift.io/url-org":         "redhat-appstudio",
					"test.appstudio.openshift.io/original-prname": "build-service-on-push",
					"test.appstudio.openshift.io/url-repository":  "build-service",
					"test.appstudio.openshift.io/repository":      "build-service-pac",
					"test.appstudio.openshift.io/snapshot":        "snapshot-sample",
				},
				Annotations: map[string]string{
					"test.appstudio.openshift.io/on-target-branch": "[main]",
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
			result, err := adapter.EnsureSnapshotPassedAllTests()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())
	})

	It("ensures status is reported for integration PipelineRuns", func() {
		adapter.pipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-status-sample",
				Namespace: "default",
				Labels: map[string]string{
					"test.appstudio.openshift.io/application":     "test-application",
					"test.appstudio.openshift.io/component":       "devfile-sample-go-basic",
					"test.appstudio.openshift.io/snapshot":        "test-application-s8tnj",
					"test.appstudio.openshift.io/scenario":        "example-pass",
					"test.appstudio.openshift.io/state":           "started",
					"test.appstudio.openshift.io/sender":          "foo",
					"test.appstudio.openshift.io/check-run-id":    "9058825284",
					"test.appstudio.openshift.io/branch":          "main",
					"test.appstudio.openshift.io/url-org":         "devfile-sample",
					"test.appstudio.openshift.io/original-prname": "devfile-sample-go-basic-on-pull-request",
					"test.appstudio.openshift.io/url-repository":  "devfile-sample-go-basic",
					"test.appstudio.openshift.io/repository":      "devfile-sample-go-basic",
					"test.appstudio.openshift.io/sha":             "12a4a35ccd08194595179815e4646c3a6c08bb77",
					"test.appstudio.openshift.io/git-provider":    "github",
					"test.appstudio.openshift.io/event-type":      "pull_request",
					"pipelines.appstudio.openshift.io/type":       "test",
				},
				Annotations: map[string]string{
					"test.appstudio.openshift.io/on-target-branch": "[main,master]",
					"test.appstudio.openshift.io/repo-url":         "https://github.com/devfile-samples/devfile-sample-go-basic",
					"test.appstudio.openshift.io/sha-title":        "Appstudio update devfile-sample-go-basic",
					"test.appstudio.openshift.io/git-auth-secret":  "pac-gitauth-zjib",
					"test.appstudio.openshift.io/pull-request":     "16",
					"test.appstudio.openshift.io/on-event":         "[pull_request]",
					"test.appstudio.openshift.io/installation-id":  "30353543",
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

	It("ensures status is not reported for integration PipelineRuns derived from optional IntegrationTestScenarios", func() {
		adapter.pipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-status-sample",
				Namespace: "default",
				Labels: map[string]string{
					"test.appstudio.openshift.io/optional":  "true",
					"pipelines.appstudio.openshift.io/type": "test",
				},
			},
		}

		Eventually(func() bool {
			result, err := adapter.EnsureStatusReported()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())
		Expect(statusReporter.Called).To(BeFalse())
	})

})
