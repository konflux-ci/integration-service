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

package loader

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
)

var _ = Describe("Loader", Ordered, func() {
	var (
		hasSnapshot       *applicationapiv1alpha1.Snapshot
		hasApp            *applicationapiv1alpha1.Application
		hasComp           *applicationapiv1alpha1.Component
		successfulTaskRun *tektonv1beta1.TaskRun
		testPipelineRun   *tektonv1beta1.PipelineRun
		sample_image      string
		sample_revision   string
	)

	const (
		SampleRepoLink  = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
		applicationName = "application-sample"
		snapshotName    = "snapshot-sample"
	)

	BeforeAll(func() {
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
				ComponentName:  "component-sample",
				Application:    applicationName,
				ContainerImage: "",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: SampleRepoLink,
						},
					},
				},
			},
			Status: applicationapiv1alpha1.ComponentStatus{
				LastBuiltCommit: "",
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())
	})

	BeforeEach(func() {
		sample_image = "quay.io/redhat-appstudio/sample-image"
		sample_revision = "random-value"

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotName,
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      "component",
					gitops.SnapshotComponentLabel: "component-sample",
				},
				Annotations: map[string]string{
					gitops.PipelineAsCodeInstallationIDAnnotation: "123",
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
						Name: "HACBS_TEST_OUTPUT",
						Value: *tektonv1beta1.NewArrayOrString(`{
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

		testPipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pipelinerun-sample",
				Namespace: "default",
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type": "build",
					"pipelines.openshift.io/used-by":        "build-cloud",
					"pipelines.openshift.io/runtime":        "nodejs",
					"pipelines.openshift.io/strategy":       "s2i",
					"appstudio.openshift.io/component":      "component-sample",
					"appstudio.openshift.io/application":    applicationName,
					"appstudio.openshift.io/snapshot":       snapshotName,
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
		Expect(k8sClient.Create(ctx, testPipelineRun)).Should(Succeed())

		testPipelineRun.Status = tektonv1beta1.PipelineRunStatus{
			PipelineRunStatusFields: tektonv1beta1.PipelineRunStatusFields{
				ChildReferences: []tektonv1beta1.ChildStatusReference{
					{
						Name:             successfulTaskRun.Name,
						PipelineTaskName: "task1",
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, testPipelineRun)).Should(Succeed())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, testPipelineRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, successfulTaskRun)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("ensures environments can be found", func() {
		environments, err := GetAllEnvironments(k8sClient, ctx, hasApp)
		Expect(err).To(BeNil())
		Expect(environments).NotTo(BeNil())
	})

	It("ensures all Releases exists when HACBSTests succeeded", func() {
		Expect(k8sClient).NotTo(BeNil())
		Expect(ctx).NotTo(BeNil())
		Expect(hasSnapshot).NotTo(BeNil())
		gitops.MarkSnapshotAsPassed(k8sClient, ctx, hasSnapshot, "test passed")
		Expect(gitops.HaveAppStudioTestsSucceeded(hasSnapshot)).To(BeTrue())

		// Normally we would Ensure that releases exist here, but that requires
		// importing the snapshot package which causes an import cycle

		releases, err := GetReleasesWithSnapshot(k8sClient, ctx, hasSnapshot)
		Expect(err).To(BeNil())
		Expect(releases).NotTo(BeNil())
		for _, release := range *releases {
			Expect(k8sClient.Delete(ctx, &release)).Should(Succeed())
		}
	})

	It("ensures the Application Components can be found ", func() {
		applicationComponents, err := GetAllApplicationComponents(k8sClient, ctx, hasApp)
		Expect(err).To(BeNil())
		Expect(applicationComponents).NotTo(BeNil())
	})

	It("ensures we can get an Application from a Snapshot ", func() {
		app, err := GetApplicationFromSnapshot(k8sClient, ctx, hasSnapshot)
		Expect(err).To(BeNil())
		Expect(app).NotTo(BeNil())
		Expect(app.ObjectMeta).To(Equal(hasApp.ObjectMeta))
	})

	It("ensures we can get a Component from a Snapshot ", func() {
		comp, err := GetComponentFromSnapshot(k8sClient, ctx, hasSnapshot)
		Expect(err).To(BeNil())
		Expect(comp).NotTo(BeNil())
		Expect(comp.ObjectMeta).To(Equal(hasComp.ObjectMeta))
	})

	It("ensures we can get a Component from a Pipeline Run ", func() {
		comp, err := GetComponentFromPipelineRun(k8sClient, ctx, testPipelineRun)
		Expect(err).To(BeNil())
		Expect(comp).NotTo(BeNil())
		Expect(comp.ObjectMeta).To(Equal(hasComp.ObjectMeta))
	})

	It("ensures we can get the application from the Pipeline Run", func() {
		app, err := GetApplicationFromPipelineRun(k8sClient, ctx, testPipelineRun)
		Expect(err).To(BeNil())
		Expect(app).NotTo(BeNil())
		Expect(app.ObjectMeta).To(Equal(hasApp.ObjectMeta))
	})

	It("ensures we can get the environment from the Pipeline Run", func() {
		env, err := GetApplicationFromPipelineRun(k8sClient, ctx, testPipelineRun)
		Expect(err).To(BeNil())
		Expect(env).NotTo(BeNil())
	})

	It("ensures we can get the Application from a Component", func() {
		app, err := GetApplicationFromComponent(k8sClient, ctx, hasComp)
		Expect(err).To(BeNil())
		Expect(app).NotTo(BeNil())
		Expect(app.ObjectMeta).To(Equal(hasApp.ObjectMeta))
	})

	It("ensures we can get the Snapshot from a Pipeline Run", func() {
		snapshot, err := GetSnapshotFromPipelineRun(k8sClient, ctx, testPipelineRun)
		Expect(err).To(BeNil())
		Expect(snapshot).NotTo(BeNil())
		Expect(snapshot.ObjectMeta).To(Equal(hasSnapshot.ObjectMeta))
	})
})
