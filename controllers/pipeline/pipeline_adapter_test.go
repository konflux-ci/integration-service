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
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	integrationv1alpha1 "github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck/v1beta1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/status"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tonglil/buflogr"
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
		hasSnapshot              *applicationapiv1alpha1.Snapshot
		integrationTestScenario  *integrationv1alpha1.IntegrationTestScenario
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

		sampleImage := "quay.io/redhat-appstudio/sample-image"

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      "component",
					gitops.SnapshotComponentLabel: hasComp.Name,
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation: "123",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           hasComp.Name,
						ContainerImage: sampleImage,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		integrationTestScenario = &integrationv1alpha1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: integrationv1alpha1.IntegrationTestScenarioSpec{
				Application: hasApp.Name,
				Bundle:      "quay.io/redhat-appstudio/example-tekton-bundle:component-pipeline-pass",
				Pipeline:    "component-pipeline-pass",
				Environment: integrationv1alpha1.TestEnvironment{
					Name: "envname",
					Type: "POC",
					Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())
	})

	BeforeEach(func() {
		testpipelineRunBuild = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-build-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type": "build",
					"pipelines.openshift.io/used-by":        "build-cloud",
					"pipelines.openshift.io/runtime":        "nodejs",
					"pipelines.openshift.io/strategy":       "s2i",
					"appstudio.openshift.io/component":      "component-sample",
					"pipelinesascode.tekton.dev/event-type": "pull_request",
				},
				Annotations: map[string]string{
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
					"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
					"foo": "bar",
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
					"index1": {
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
			Status: v1beta1.Status{
				Conditions: v1beta1.Conditions{
					apis.Condition{
						Reason: "Completed",
						Status: "True",
						Type:   apis.ConditionSucceeded,
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

		testpipelineRunComponent = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-component-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type":           "test",
					"pac.test.appstudio.openshift.io/url-org":         "redhat-appstudio",
					"pac.test.appstudio.openshift.io/original-prname": "build-service-on-push",
					"pac.test.appstudio.openshift.io/url-repository":  "build-service",
					"pac.test.appstudio.openshift.io/repository":      "build-service-pac",
					"appstudio.openshift.io/snapshot":                 hasSnapshot.Name,
					"test.appstudio.openshift.io/scenario":            integrationTestScenario.Name,
				},
				Annotations: map[string]string{
					"pac.test.appstudio.openshift.io/on-target-branch": "[main]",
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

		testpipelineRunComponent.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				TaskRuns: map[string]*tektonv1beta1.PipelineRunTaskRunStatus{
					"index1": &tektonv1beta1.PipelineRunTaskRunStatus{
						PipelineTaskName: "task-failure",
						Status: &tektonv1beta1.TaskRunStatus{
							TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
								TaskRunResults: []tektonv1beta1.TaskRunResult{
									{
										Name:  "HACBS_TEST_OUTPUT",
										Value: *tektonv1beta1.NewArrayOrString("{\"result\":\"SUCCESS\",\"timestamp\":\"1665405317\",\"failures\":0,\"successes\":1}"),
									},
								},
							},
						},
					},
				},
			},
			Status: v1beta1.Status{
				Conditions: v1beta1.Conditions{
					apis.Condition{
						Reason: "Completed",
						Status: "True",
						Type:   apis.ConditionSucceeded,
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, testpipelineRunComponent)).Should(Succeed())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      testpipelineRunComponent.Name,
				Namespace: "default",
			}, testpipelineRunComponent)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		adapter = NewAdapter(testpipelineRunBuild, hasComp, hasApp, ctrl.Log, k8sClient, ctx)
		statusReporter = &MockStatusReporter{}
		statusAdapter = &MockStatusAdapter{Reporter: statusReporter}
		adapter.status = statusAdapter
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, testpipelineRunBuild)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, testpipelineRunComponent)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
	})

	It("can create a new Adapter instance", func() {
		Expect(reflect.TypeOf(NewAdapter(testpipelineRunBuild, hasComp, hasApp, ctrl.Log, k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("ensures the Application Components can be found ", func() {
		applicationComponents, err := adapter.getAllApplicationComponents(hasApp)
		Expect(err == nil).To(BeTrue())
		Expect(applicationComponents != nil).To(BeTrue())
	})

	It("ensures the Imagepullspec from pipelinerun and prepare snapshot can be created", func() {
		imagePullSpec, err := adapter.getImagePullSpecFromPipelineRun(testpipelineRunBuild)
		Expect(err == nil).To(BeTrue())
		Expect(imagePullSpec != "").To(BeTrue())

		snapshot, err := adapter.prepareSnapshot(hasApp, hasComp, imagePullSpec)
		Expect(err == nil).To(BeTrue())
		Expect(snapshot != nil).To(BeTrue())

		fetchedPullSpec, err := adapter.getImagePullSpecFromSnapshotComponent(snapshot, hasComp)
		Expect(err == nil).To(BeTrue())
		Expect(fetchedPullSpec != "").To(BeTrue())
		Expect(fetchedPullSpec == imagePullSpec).To(BeTrue())
	})

	It("ensures the global component list unchanged and compositeSnapshot shouldn't be created ", func() {
		expectedSnapshot, err := adapter.prepareSnapshotForPipelineRun(testpipelineRunBuild, hasComp, hasApp)
		Expect(err == nil).To(BeTrue())
		Expect(expectedSnapshot != nil).To(BeTrue())

		// Check if the global component list changed in the meantime and create a composite snapshot if it did.
		compositeSnapshot, err := adapter.createCompositeSnapshotsIfConflictExists(hasApp, hasComp, expectedSnapshot)
		Expect(err == nil).To(BeTrue())
		Expect(compositeSnapshot == nil).To(BeTrue())
	})

	It("ensures the global component list is changed and compositeSnapshot should be created", func() {
		createdSnapshot, err := adapter.getSnapshotFromPipelineRun(testpipelineRunComponent)
		Expect(err).To(BeNil())
		Expect(createdSnapshot).ToNot(BeNil())

		// A new component is added to the application in the meantime, changing the global component list.
		hasCompNew := &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample-2",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-sample-2",
				Application:    hasApp.Name,
				ContainerImage: "quay.io/redhat-appstudio/sample-image:new-label",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: SampleRepoLink,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasCompNew)).Should(Succeed())

		Eventually(func() bool {
			applicationComponents, err := adapter.getAllApplicationComponents(hasApp)
			return err == nil && len(*applicationComponents) > 1
		}, time.Second*10).Should(BeTrue())

		// Check if the global component list changed in the meantime and create a composite snapshot if it did.
		compositeSnapshot, err := adapter.createCompositeSnapshotsIfConflictExists(hasApp, hasComp, createdSnapshot)
		fmt.Fprintf(GinkgoWriter, "compositeSnapshot: %v\n", compositeSnapshot.Name)
		Expect(err == nil).To(BeTrue())
		Expect(compositeSnapshot != nil).To(BeTrue())

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      compositeSnapshot.Name,
				Namespace: compositeSnapshot.Namespace,
			}, compositeSnapshot)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		// Check if the composite snapshot that was already created above was correctly detected and returned.
		existingCompositeSnapshot, err := adapter.createCompositeSnapshotsIfConflictExists(hasApp, hasComp, createdSnapshot)
		Expect(err == nil).To(BeTrue())
		Expect(existingCompositeSnapshot != nil).To(BeTrue())
		Expect(existingCompositeSnapshot.Name == compositeSnapshot.Name).To(BeTrue())
	})

	It("ensures pipelines as code labels and annotations are propagated to the snapshot", func() {
		snapshot, err := adapter.prepareSnapshotForPipelineRun(testpipelineRunBuild, hasComp, hasApp)
		Expect(err).To(BeNil())
		Expect(snapshot).ToNot(BeNil())
		annotation, found := snapshot.GetAnnotations()["pac.test.appstudio.openshift.io/on-target-branch"]
		Expect(found).To(BeTrue())
		Expect(annotation).To(Equal("[main,master]"))
		label, found := snapshot.GetLabels()["pac.test.appstudio.openshift.io/event-type"]
		Expect(found).To(BeTrue())
		Expect(label).To(Equal("pull_request"))
	})

	It("ensures non-pipelines as code labels and annotations are NOT propagated to the snapshot", func() {
		snapshot, err := adapter.prepareSnapshotForPipelineRun(testpipelineRunBuild, hasComp, hasApp)
		Expect(err).To(BeNil())
		Expect(snapshot).ToNot(BeNil())

		// non-PaC labels are not copied
		_, found := testpipelineRunBuild.GetLabels()["pipelines.appstudio.openshift.io/type"]
		Expect(found).To(BeTrue())
		_, found = snapshot.GetLabels()["pipelines.appstudio.openshift.io/type"]
		Expect(found).To(BeFalse())

		// non-PaC annotations are not copied
		_, found = testpipelineRunBuild.GetAnnotations()["foo"]
		Expect(found).To(BeTrue())
		_, found = snapshot.GetAnnotations()["foo"]
		Expect(found).To(BeFalse())
	})

	It("ensures Snapshot exists and can be found ", func() {
		Eventually(func() bool {
			result, err := adapter.EnsureSnapshotExists()
			fmt.Fprintf(GinkgoWriter, "Err: %v\n", err)
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())
	})

	It("ensures snapshot creation is skipped when there is no component defined ", func() {
		adapter = NewAdapter(testpipelineRunBuild, nil, hasApp, ctrl.Log, k8sClient, ctx)
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

		Eventually(func() bool {
			result, err := adapter.EnsureSnapshotExists()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())
	})

	When("Snapshot already exists", func() {
		BeforeEach(func() {
			adapter = NewAdapter(testpipelineRunBuild, hasComp, hasApp, ctrl.Log, k8sClient, ctx)
			Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

			snapshot, err := adapter.prepareSnapshotForPipelineRun(adapter.pipelineRun, adapter.component, adapter.application)
			Expect(err).To(BeNil())

			err = adapter.client.Create(adapter.context, snapshot)
			Expect(err).To(BeNil())
		})

		It("ensures snapshot creation is skipped when snapshot already exists", func() {
			Eventually(func() bool {
				result, err := adapter.EnsureSnapshotExists()
				return !result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())
		})
	})

	It("ensures Snapshot passed all tests", func() {
		adapter = NewAdapter(testpipelineRunComponent, hasComp, hasApp, ctrl.Log, k8sClient, ctx)
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

		Eventually(func() bool {
			result, err := adapter.EnsureSnapshotPassedAllTests()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

		integrationTestScenarios, err := helpers.GetRequiredIntegrationTestScenariosForApplication(k8sClient, ctx, hasApp)
		Expect(err == nil).To(BeTrue())
		Expect(len(*integrationTestScenarios) > 0).To(BeTrue())

		integrationPipelineRuns, err := adapter.getAllPipelineRunsForSnapshot(hasSnapshot, integrationTestScenarios)
		Expect(err == nil).To(BeTrue())
		Expect(len(*integrationPipelineRuns) > 0).To(BeTrue())

		allIntegrationPipelineRunsPassed, err := adapter.determineIfAllIntegrationPipelinesPassed(integrationPipelineRuns)
		Expect(err == nil).To(BeTrue())
		Expect(allIntegrationPipelineRunsPassed).To(BeTrue())
	})

	It("ensures status is reported for integration PipelineRuns", func() {
		adapter.pipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-status-sample",
				Namespace: "default",
				Labels: map[string]string{
					"appstudio.openshift.io/application":              "test-application",
					"appstudio.openshift.io/component":                "devfile-sample-go-basic",
					"appstudio.openshift.io/snapshot":                 "test-application-s8tnj",
					"test.appstudio.openshift.io/scenario":            "example-pass",
					"pac.test.appstudio.openshift.io/state":           "started",
					"pac.test.appstudio.openshift.io/sender":          "foo",
					"pac.test.appstudio.openshift.io/check-run-id":    "9058825284",
					"pac.test.appstudio.openshift.io/branch":          "main",
					"pac.test.appstudio.openshift.io/url-org":         "devfile-sample",
					"pac.test.appstudio.openshift.io/original-prname": "devfile-sample-go-basic-on-pull-request",
					"pac.test.appstudio.openshift.io/url-repository":  "devfile-sample-go-basic",
					"pac.test.appstudio.openshift.io/repository":      "devfile-sample-go-basic",
					"pac.test.appstudio.openshift.io/sha":             "12a4a35ccd08194595179815e4646c3a6c08bb77",
					"pac.test.appstudio.openshift.io/git-provider":    "github",
					"pac.test.appstudio.openshift.io/event-type":      "pull_request",
					"pipelines.appstudio.openshift.io/type":           "test",
				},
				Annotations: map[string]string{
					"pac.test.appstudio.openshift.io/on-target-branch": "[main,master]",
					"pac.test.appstudio.openshift.io/repo-url":         "https://github.com/devfile-samples/devfile-sample-go-basic",
					"pac.test.appstudio.openshift.io/sha-title":        "Appstudio update devfile-sample-go-basic",
					"pac.test.appstudio.openshift.io/git-auth-secret":  "pac-gitauth-zjib",
					"pac.test.appstudio.openshift.io/pull-request":     "16",
					"pac.test.appstudio.openshift.io/on-event":         "[pull_request]",
					"pac.test.appstudio.openshift.io/installation-id":  "30353543",
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

	When("multiple succesfull build pipeline runs exists for the same component", func() {

		var (
			testpipelineRunBuild2 *tektonv1beta1.PipelineRun
		)

		BeforeAll(func() {
			testpipelineRunBuild2 = &tektonv1beta1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipelinerun-build-sample-2",
					Namespace: "default",
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type": "build",
						"pipelines.openshift.io/used-by":        "build-cloud",
						"pipelines.openshift.io/runtime":        "nodejs",
						"pipelines.openshift.io/strategy":       "s2i",
						"appstudio.openshift.io/component":      "component-sample",
						"pipelinesascode.tekton.dev/event-type": "pull_request",
					},
					Annotations: map[string]string{
						"appstudio.redhat.com/updateComponentOnSuccess": "false",
						"pipelinesascode.tekton.dev/on-target-branch":   "[main,master]",
						"foo": "bar",
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
			Expect(k8sClient.Create(ctx, testpipelineRunBuild2)).Should(Succeed())

			testpipelineRunBuild2.Status = tektonv1beta1.PipelineRunStatus{
				PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
					TaskRuns: map[string]*tektonv1beta1.PipelineRunTaskRunStatus{
						"index1": {
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
				Status: v1beta1.Status{
					Conditions: v1beta1.Conditions{
						apis.Condition{
							Reason: "Completed",
							Status: "True",
							Type:   apis.ConditionSucceeded,
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, testpipelineRunBuild2)).Should(Succeed())
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testpipelineRunBuild2.Name,
					Namespace: "default",
				}, testpipelineRunBuild2)
				if err != nil {
					return err
				}
				if !helpers.HasPipelineRunSucceeded(testpipelineRunBuild2) {
					return fmt.Errorf("Pipeline is not marked as succeeded yet")
				}
				return err
			}, time.Second*10).ShouldNot(HaveOccurred())
		})

		AfterAll(func() {
			err := k8sClient.Delete(ctx, testpipelineRunBuild2)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
		})

		It("isLatestSucceededPipelineRun reports second pipeline as the latest pipeline", func() {
			// make sure the seocnd pipeline started as second
			testpipelineRunBuild2.CreationTimestamp.Time = testpipelineRunBuild2.CreationTimestamp.Add(2 * time.Hour)
			adapter = NewAdapter(testpipelineRunBuild2, hasComp, hasApp, ctrl.Log, k8sClient, ctx)
			isLatest, err := adapter.isLatestSucceededPipelineRun()
			Expect(err).To(BeNil())
			Expect(isLatest).To(BeTrue())
		})

		It("isLatestSucceededPipelineRun doesn't report first pipeline as the latest pipeline", func() {
			// make sure the first pipeline started as first
			testpipelineRunBuild.CreationTimestamp.Time = testpipelineRunBuild.CreationTimestamp.Add(-2 * time.Hour)
			adapter = NewAdapter(testpipelineRunBuild, hasComp, hasApp, ctrl.Log, k8sClient, ctx)
			isLatest, err := adapter.isLatestSucceededPipelineRun()
			Expect(err).To(BeNil())
			Expect(isLatest).To(BeFalse())
		})

		It("ensure that EnsureSnapshotExists doesn't create snapshot for previous pipeline run", func() {
			var buf bytes.Buffer
			var log logr.Logger = buflogr.NewWithBuffer(&buf)

			// make sure the first pipeline started as first
			testpipelineRunBuild.CreationTimestamp.Time = testpipelineRunBuild.CreationTimestamp.Add(-2 * time.Hour)
			adapter = NewAdapter(testpipelineRunBuild, hasComp, hasApp, log, k8sClient, ctx)
			Eventually(func() bool {
				result, err := adapter.EnsureSnapshotExists()
				return !result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())

			expectedLogEntry := "INFO The pipelineRun pipelinerun-build-sample is not the latest succeded pipelineRun for component component-sample will not create a new Snapshot."
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))
		})
	})
})
