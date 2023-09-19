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

package integrationpipeline

import (
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	klog "k8s.io/klog/v2"
)

var _ = Describe("Integration PipelineController", func() {
	var (
		manager                ctrl.Manager
		pipelineReconciler     *Reconciler
		scheme                 runtime.Scheme
		req                    ctrl.Request
		successfulTaskRun      *tektonv1beta1.TaskRun
		integrationPipelineRun *tektonv1beta1.PipelineRun
		hasApp                 *applicationapiv1alpha1.Application
		hasComp                *applicationapiv1alpha1.Component
		hasSnapshot            *applicationapiv1alpha1.Snapshot
	)
	const (
		applicationName = "application-sample"
		sample_image    = "quay.io/redhat-appstudio/sample-image"
		sample_revision = "random-value"
		SampleRepoLink  = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
	)

	BeforeEach(func() {

		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
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
				ComponentName: "component-sample",
				Application:   applicationName,
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

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:              "component",
					gitops.SnapshotComponentLabel:         "component-sample",
					"build.appstudio.redhat.com/pipeline": "enterprise-contract",
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation:   "123",
					"build.appstudio.redhat.com/commit_sha":         "6c65b2fcaea3e1a0a92476c8b5dc89e92a85f025",
					"appstudio.redhat.com/updateComponentOnSuccess": "false",
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

		successfulTaskRun = &tektonv1beta1.TaskRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-taskrun-pass",
				Namespace: "default",
			},
			Spec: tektonv1beta1.TaskRunSpec{
				TaskRef: &tektonv1beta1.TaskRef{
					Name:   "test-taskrun-pass",
					Bundle: "quay.io/redhat-appstudio/example-tekton-bundle:test",
				},
			},
		}

		Expect(k8sClient.Create(ctx, successfulTaskRun)).Should(Succeed())

		now := time.Now()
		successfulTaskRun.Status = tektonv1beta1.TaskRunStatus{
			TaskRunStatusFields: tektonv1beta1.TaskRunStatusFields{
				StartTime:      &metav1.Time{Time: now},
				CompletionTime: &metav1.Time{Time: now.Add(5 * time.Minute)},
				TaskRunResults: []tektonv1beta1.TaskRunResult{
					{
						Name: "TEST_OUTPUT",
						Value: *tektonv1beta1.NewStructuredValues(`{
											"result": "SUCCESS",
											"timestamp": "1665405318",
											"failures": 0,
											"successes": 10,
											"warnings": 0
										}`),
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, successfulTaskRun)).Should(Succeed())

		integrationPipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type": "test",
					"pipelines.openshift.io/used-by":        "build-cloud",
					"pipelines.openshift.io/runtime":        "nodejs",
					"pipelines.openshift.io/strategy":       "s2i",
					"appstudio.openshift.io/component":      "component-sample",
					"appstudio.openshift.io/application":    applicationName,
					gitops.SnapshotLabel:                    hasSnapshot.Name,
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
						Value: tektonv1beta1.ParamValue{
							Type:      tektonv1beta1.ParamTypeString,
							StringVal: "quay.io/redhat-appstudio/sample-image",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationPipelineRun)).Should(Succeed())

		integrationPipelineRun.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				ChildReferences: []tektonv1beta1.ChildStatusReference{
					{
						Name:             successfulTaskRun.Name,
						PipelineTaskName: "task1",
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, integrationPipelineRun)).Should(Succeed())

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      integrationPipelineRun.Name,
			},
		}

		webhookInstallOptions := &testEnv.WebhookInstallOptions

		klog.Info(webhookInstallOptions.LocalServingHost)
		klog.Info(webhookInstallOptions.LocalServingPort)
		klog.Info(webhookInstallOptions.LocalServingCertDir)

		var err error
		manager, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             clientsetscheme.Scheme,
			Host:               webhookInstallOptions.LocalServingHost,
			Port:               webhookInstallOptions.LocalServingPort,
			CertDir:            webhookInstallOptions.LocalServingCertDir,
			MetricsBindAddress: "0", // this disables metrics
			LeaderElection:     false,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(err).To(BeNil())

		pipelineReconciler = NewIntegrationReconciler(k8sClient, &logf.Log, &scheme)
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationPipelineRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, successfulTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("can create and return a new Reconciler object", func() {
		Expect(reflect.TypeOf(pipelineReconciler)).To(Equal(reflect.TypeOf(&Reconciler{})))
	})

	It("can fail when Reconcile fails to prepare the adapter when pipeline is not found", func() {
		Expect(k8sClient.Delete(ctx, integrationPipelineRun)).Should(Succeed())
		Eventually(func() error {
			_, err := pipelineReconciler.Reconcile(ctx, req)
			return err
		}).Should(BeNil())
	})

	It("can Reconcile function prepare the adapter and return the result of the reconcile handling operation", func() {
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "non-existent",
				Namespace: "default",
			},
		}
		result, err := pipelineReconciler.Reconcile(ctx, req)
		Expect(reflect.TypeOf(result)).To(Equal(reflect.TypeOf(reconcile.Result{})))
		Expect(err).To(BeNil())
	})

	It("can setup the cache by adding a new index field to search for ReleasePlanAdmissions", func() {
		err := setupCache(manager)
		Expect(err).ToNot(HaveOccurred())
	})

	It("can setup a new controller manager with the given reconciler", func() {
		err := setupControllerWithManager(manager, pipelineReconciler)
		Expect(err).NotTo(HaveOccurred())
	})

	It("can setup a new Controller manager and start it", func() {
		err := SetupController(manager, &ctrl.Log)
		Expect(err).To(BeNil())
		go func() {
			defer GinkgoRecover()
			err = manager.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
		}()
	})

	When("pipelinerun has no component", func() {

		var (
			integrationPipelineRunNoComponent *tektonv1beta1.PipelineRun
			reqNoComponent                    ctrl.Request
		)

		BeforeEach(func() {
			integrationPipelineRunNoComponent = &tektonv1beta1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pipelinerun-sample-no-component",
					Namespace: "default",
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type": "test",
						"pipelines.openshift.io/used-by":        "build-cloud",
						"pipelines.openshift.io/runtime":        "nodejs",
						"pipelines.openshift.io/strategy":       "s2i",
						"appstudio.openshift.io/application":    applicationName,
						gitops.SnapshotLabel:                    hasSnapshot.Name,
					},
					Annotations: map[string]string{
						"appstudio.redhat.com/updateComponentOnSuccess": "false",
					},
				},
				Spec: tektonv1beta1.PipelineRunSpec{
					PipelineRef: &tektonv1beta1.PipelineRef{
						Name:   "component-pipeline-pass",
						Bundle: "quay.io/kpavic/test-bundle:component-pipeline-pass",
					},
				},
			}
			Expect(k8sClient.Create(ctx, integrationPipelineRunNoComponent)).Should(Succeed())

			reqNoComponent = ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      integrationPipelineRunNoComponent.Name,
				},
			}
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, integrationPipelineRunNoComponent)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("reconcile with application taken from pipelinerun (integration pipeline)", func() {
			result, err := pipelineReconciler.Reconcile(ctx, reqNoComponent)
			Expect(reflect.TypeOf(result)).To(Equal(reflect.TypeOf(reconcile.Result{})))
			Expect(err).To(BeNil())
		})

	})
})
