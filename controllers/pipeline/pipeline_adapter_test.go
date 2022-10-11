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
	"fmt"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Pipeline Adapter", Ordered, func() {
	var (
		adapter *Adapter

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

		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      testpipelineRunBuild.Name,
				Namespace: "default",
			}, testpipelineRunBuild)
			return err
		}, time.Second*10).ShouldNot(HaveOccurred())

		adapter = NewAdapter(testpipelineRunBuild, hasComp, hasApp, ctrl.Log, k8sClient, ctx)
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))

	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, testpipelineRunBuild)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("can create a new Adapter instance", func() {
		Expect(reflect.TypeOf(NewAdapter(testpipelineRunBuild, hasComp, hasApp, ctrl.Log, k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("ensures the Applicationcomponents can be found ", func() {
		applicationComponents, err := adapter.getAllApplicationComponents(hasApp)
		Expect(err == nil).To(BeTrue())
		Expect(applicationComponents != nil).To(BeTrue())
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
})
