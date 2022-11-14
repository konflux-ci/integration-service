package tekton_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/tekton"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"
	"knative.dev/pkg/apis"
)

var _ = Describe("Utils", func() {

	var pipelineRun *tektonv1beta1.PipelineRun

	BeforeEach(func() {

		pipelineRun = &tektonv1beta1.PipelineRun{
			ObjectMeta: v1.ObjectMeta{},
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
			},
		}
	})

	It("can get output-image", func() {
		image, _ := tekton.GetOutputImage(pipelineRun)
		if image != "test-image" {
			Fail(fmt.Sprintf("Expected image is test-image, but got %s", image))
		}
		klog.Infoln("Got expected image")
	})

	It("can get output-image-digest", func() {
		image_digest, _ := tekton.GetOutputImageDigest(pipelineRun)
		if image_digest != "image_digest_value" {
			Fail(fmt.Sprintf("Expected image_digest is image_digest_value, but got %s", image_digest))
		}
		klog.Infoln("Got expected image_digest")
	})

	It("can detect if a PipelineRun has succeeded", func() {
		Expect(tekton.HasPipelineRunSucceeded(pipelineRun)).To(BeFalse())
		pipelineRun.Status.SetCondition(&apis.Condition{
			Type:   apis.ConditionSucceeded,
			Status: "True",
		})
		Expect(tekton.HasPipelineRunSucceeded(pipelineRun)).To(BeTrue())
		Expect(tekton.HasPipelineRunSucceeded(&tektonv1beta1.TaskRun{})).To(BeFalse())
	})

})
