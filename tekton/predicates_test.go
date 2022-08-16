package tekton_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tekton "github.com/redhat-appstudio/integration-service/tekton"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"

	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("Predicates", func() {

	const (
		prefix                  = "testpipeline"
		namespace               = "default"
		PipelineTypeIntegration = "integration"
	)
	var (
		testpipelineRun    *tektonv1beta1.PipelineRun
		newtestpipelineRun *tektonv1beta1.PipelineRun
	)
	BeforeEach(func() {

		testpipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: v1.ObjectMeta{
				GenerateName: prefix + "-",
				Namespace:    namespace,
			},
			Spec: tektonv1beta1.PipelineRunSpec{
				Params: []tektonv1beta1.Param{
					{
						Name: "output-image",
						Value: tektonv1beta1.ArrayOrString{
							StringVal: "test-image",
						},
					},
				},
			},
			Status: tektonv1beta1.PipelineRunStatus{
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
			},
		}
		newtestpipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: v1.ObjectMeta{
				GenerateName: prefix + "-",
				Namespace:    namespace,
			},
			Spec: tektonv1beta1.PipelineRunSpec{},
		}

		testpipelineRun.ObjectMeta.Labels = map[string]string{
			"pipelines.appstudio.openshift.io/type": "build",
			"PipelinesTypeLabel1":                   PipelineTypeIntegration,
		}
		newtestpipelineRun.ObjectMeta.Labels = map[string]string{
			"pipelines.appstudio.openshift.io/type": "build",
			"PipelinesTypeLabel1":                   PipelineTypeIntegration,
		}
		testpipelineRun.ObjectMeta.Annotations = map[string]string{"foo": "bar"}
		newtestpipelineRun.ObjectMeta.Annotations = map[string]string{"foo": "bar"}
	})

	Context("when testing  IntegrationOrBuildPipelineRunSucceededPredicat epredicate", func() {
		instance := tekton.IntegrationOrBuildPipelineRunSucceededPredicate()

		It("should ignore creating events", func() {
			contextEvent := event.CreateEvent{
				Object: testpipelineRun,
			}
			Expect(instance.Create(contextEvent)).To(BeFalse())
		})
		It("should ignore deleting events", func() {
			contextEvent := event.DeleteEvent{
				Object: testpipelineRun,
			}
			Expect(instance.Delete(contextEvent)).To(BeFalse())
		})

		It("should ignore generic events", func() {
			contextEvent := event.GenericEvent{
				Object: testpipelineRun,
			}
			Expect(instance.Generic(contextEvent)).To(BeFalse())
		})

		It("should return true when an updated event is received for a succeeded PipelineRun", func() {

			newtestpipelineRun.ObjectMeta.Labels = map[string]string{
				"pipelines.appstudio.openshift.io/type": "test",
				"PipelinesLabel1":                       "label2",
			}
			newtestpipelineRun.Spec.ServiceAccountName = "test-service-account"

			contextEvent := event.UpdateEvent{
				ObjectOld: testpipelineRun,
				ObjectNew: newtestpipelineRun,
			}

			Expect(instance.Update(contextEvent)).To(BeFalse())
			newtestpipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "True",
			})
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("get output-image", func() {
			image, _ := tekton.GetOutputImage(testpipelineRun)
			if image != "test-image" {
				Fail(fmt.Sprintf("Expected image is test-image, bug got %s", image))
			}
			klog.Infoln("Got expected image")
		})
		It("get output-image-digest", func() {
			image_digest, _ := tekton.GetOutputImageDigest(testpipelineRun)
			if image_digest != "image_digest_value" {
				Fail(fmt.Sprintf("Expected image_digest is image_digest_value, bug got %s", image_digest))
			}
			klog.Infoln("Got expected image_digest")
		})
	})
})
