package tekton_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/tekton"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"
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
					PipelineResults: []tektonv1beta1.PipelineRunResult{
						{
							Name:  "IMAGE_DIGEST",
							Value: *tektonv1beta1.NewArrayOrString("image_digest_value"),
						},
						{
							Name:  "IMAGE_URL",
							Value: *tektonv1beta1.NewArrayOrString("test-image"),
						},
						{
							Name:  "CHAINS-GIT_URL",
							Value: *tektonv1beta1.NewArrayOrString("https://github.com/devfile-samples/devfile-sample-java-springboot-basic"),
						},
						{
							Name:  "CHAINS-GIT_COMMIT",
							Value: *tektonv1beta1.NewArrayOrString("a2ba645d50e471d5f084b"),
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
		klog.Infoln("Got expected git_url")
	})

	It("can get git-url", func() {
		git_url, _ := tekton.GetComponentSourceGitUrl(pipelineRun)
		if git_url != "https://github.com/devfile-samples/devfile-sample-java-springboot-basic" {
			Fail(fmt.Sprintf("Expected git_url is https://github.com/devfile-samples/devfile-sample-java-springboot-basic, but got %s", git_url))
		}
		klog.Infoln("Got expected git_url")
	})

	It("can return err when can't find result for CHAINS-GIT_URL", func() {
		pipelineRun.Status.PipelineResults = []tektonv1beta1.PipelineRunResult{}
		_, err := tekton.GetComponentSourceGitUrl(pipelineRun)
		Expect(err).ToNot(BeNil())
	})

	It("can get git-commit", func() {
		commit, _ := tekton.GetComponentSourceGitCommit(pipelineRun)
		if commit != "a2ba645d50e471d5f084b" {
			Fail(fmt.Sprintf("Expected commit is a2ba645d50e471d5f084b, but got %s", commit))
		}
		klog.Infoln("Got expected commit")
	})

	It("can return err when can't find result CHAINS-GIT_COMMIT", func() {
		pipelineRun.Status.PipelineResults = []tektonv1beta1.PipelineRunResult{}
		_, err := tekton.GetComponentSourceGitCommit(pipelineRun)
		Expect(err).ToNot(BeNil())
	})
})
