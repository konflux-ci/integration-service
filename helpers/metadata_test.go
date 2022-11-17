package helpers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/helpers"

	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Helpers for labels and annotation", Ordered, func() {

	var (
		testpipelineLabel *tektonv1beta1.PipelineRun
	)

	BeforeEach(func() {
		testpipelineLabel = &tektonv1beta1.PipelineRun{
			ObjectMeta: v1.ObjectMeta{
				GenerateName: "testpipelineLabel-",
				Namespace:    "default",
			},
		}
		Expect(testpipelineLabel).NotTo(BeNil())
	})

	Context("testing integration service helpers to validate Labels and its value", func() {
		It("HasLabel with an existing label and return true", func() {
			testpipelineLabel.ObjectMeta.Labels = map[string]string{
				"pipelines.appstudio.openshift.io/type": "test",
			}
			Expect(testpipelineLabel.ObjectMeta.Labels["pipelines.appstudio.openshift.io/type"]).
				Should(Equal("test"))
			Expect(helpers.HasLabel(testpipelineLabel, "pipelines.appstudio.openshift.io/type")).
				To(BeTrue())
		})
		It("HasLabel with an non-existent label and return false", func() {
			testpipelineLabel.ObjectMeta.Labels = map[string]string{
				"pipelines.appstudio.openshift.io/type": "test",
			}
			Expect(testpipelineLabel.ObjectMeta.Labels["pipelines.appstudio.openshift.io/not-exist"]).
				To(Equal(""))
			Expect(helpers.HasLabel(testpipelineLabel, "pipelines.appstudio.openshift.io/not-exist")).
				To(BeFalse())
		})
		It("HasLabelvalue with an existing label value and return true", func() {
			testpipelineLabel.ObjectMeta.Labels = map[string]string{
				"pipelines.appstudio.openshift.io/type": "test",
				"PipelinesTypeLabel":                    "integration",
			}
			Expect(testpipelineLabel.ObjectMeta.Labels["PipelinesTypeLabel"]).
				To(Equal("integration"))
			Expect(helpers.HasLabelWithValue(testpipelineLabel, "PipelinesTypeLabel", "integration")).
				To(BeTrue())
		})
		It("HasLabelvalue with an different label value and return false", func() {
			testpipelineLabel.ObjectMeta.Labels = map[string]string{
				"pipelines.appstudio.openshift.io/type": "test",
				"PipelinesTypeLabel":                    "integration",
			}
			Expect(testpipelineLabel.ObjectMeta.Labels["NoLabel"]).
				To(Equal(""))
			Expect(helpers.HasLabelWithValue(testpipelineLabel, "NoLabel", "integration")).
				To(BeFalse())
		})
		It("HasLabelvalue with an No label value and return false", func() {
			testpipelineLabel.ObjectMeta.Labels = map[string]string{
				"pipelines.appstudio.openshift.io/type": "test",
				"PipelinesTypeLabel":                    "integration",
			}
			Expect(testpipelineLabel.ObjectMeta.Labels[""]).
				To(Equal(""))
			Expect(helpers.HasLabelWithValue(testpipelineLabel, "", "integration")).
				To(BeFalse())
		})
		It("HasAnnotation with an existing Annotation and return true", func() {
			testpipelineLabel.ObjectMeta.Annotations = map[string]string{
				"AnnotationKey": "integration",
			}
			Expect(testpipelineLabel.ObjectMeta.Annotations["AnnotationKey"]).
				Should(Equal("integration"))
			Expect(helpers.HasAnnotation(testpipelineLabel, "AnnotationKey")).
				To(BeTrue())
		})
		It("HasAnnotation with an Different Annotation and return false", func() {
			testpipelineLabel.ObjectMeta.Annotations = map[string]string{
				"AnnotationKey": "integration",
			}
			Expect(testpipelineLabel.ObjectMeta.Annotations["NoAnnotationKey"]).
				Should(Equal(""))
			Expect(helpers.HasAnnotation(testpipelineLabel, "NoAnnotationKey")).
				To(BeFalse())
		})
		It("HasAnnotationValue with an existing Annotation and return true", func() {
			testpipelineLabel.ObjectMeta.Annotations = map[string]string{
				"AnnotationKey": "integration",
			}
			Expect(testpipelineLabel.ObjectMeta.Annotations["AnnotationKey"]).
				Should(Equal("integration"))
			Expect(helpers.HasAnnotationWithValue(testpipelineLabel, "AnnotationKey", "integration")).
				To(BeTrue())
		})
		It("HasAnnotation with an Different Annotation and return false", func() {
			testpipelineLabel.ObjectMeta.Annotations = map[string]string{
				"AnnotationKey": "integration",
			}
			Expect(testpipelineLabel.ObjectMeta.Annotations["NoAnnotationKey"]).
				Should(Equal(""))
			Expect(helpers.HasAnnotationWithValue(testpipelineLabel, "NoAnnotationKey", "integration")).
				To(BeFalse())
		})
		It("HasAnnotation with an No Annotation and return false", func() {
			testpipelineLabel.ObjectMeta.Annotations = map[string]string{
				"AnnotationKey": "integration",
			}
			Expect(testpipelineLabel.ObjectMeta.Annotations[""]).
				Should(Equal(""))
			Expect(helpers.HasAnnotationWithValue(testpipelineLabel, "", "integration")).
				To(BeFalse())
		})
		It("CopyLabelsByPrefix with a replacement prefix", func() {
			dest := &tektonv1beta1.PipelineRun{ObjectMeta: v1.ObjectMeta{}}
			testpipelineLabel.ObjectMeta.Labels = map[string]string{
				"test.prefix/foo":         "bar",
				"another.test.prefix/baz": "qux",
			}
			helpers.CopyLabelsByPrefix(&testpipelineLabel.ObjectMeta, &dest.ObjectMeta, "test.prefix", "new.prefix")
			Expect(helpers.HasLabelWithValue(dest, "new.prefix/foo", "bar")).To(BeTrue())
			Expect(helpers.HasLabel(dest, "another.test.prefix/baz")).To(BeFalse())
		})
		It("CopyLabelsByPrefix without a replacement prefix", func() {
			dest := &tektonv1beta1.PipelineRun{ObjectMeta: v1.ObjectMeta{}}
			testpipelineLabel.ObjectMeta.Labels = map[string]string{
				"test.prefix/foo": "bar",
			}
			helpers.CopyLabelsByPrefix(&testpipelineLabel.ObjectMeta, &dest.ObjectMeta, "test.prefix", "test.prefix")
			Expect(helpers.HasLabelWithValue(dest, "test.prefix/foo", "bar")).To(BeTrue())
		})
		It("CopyAnnotationsByPrefix with a replacement prefix", func() {
			dest := &tektonv1beta1.PipelineRun{ObjectMeta: v1.ObjectMeta{}}
			testpipelineLabel.ObjectMeta.Annotations = map[string]string{
				"test.prefix/foo":         "bar",
				"another.test.prefix/baz": "qux",
			}
			helpers.CopyAnnotationsByPrefix(&testpipelineLabel.ObjectMeta, &dest.ObjectMeta, "test.prefix", "new.prefix")
			Expect(helpers.HasAnnotationWithValue(dest, "new.prefix/foo", "bar")).To(BeTrue())
			Expect(helpers.HasAnnotation(dest, "another.test.prefix/baz")).To(BeFalse())
		})
		It("CopyAnnotationsByPrefix without a replacement prefix", func() {
			dest := &tektonv1beta1.PipelineRun{ObjectMeta: v1.ObjectMeta{}}
			testpipelineLabel.ObjectMeta.Annotations = map[string]string{
				"test.prefix/foo": "bar",
			}
			helpers.CopyAnnotationsByPrefix(&testpipelineLabel.ObjectMeta, &dest.ObjectMeta, "test.prefix", "test.prefix")
			Expect(helpers.HasAnnotationWithValue(dest, "test.prefix/foo", "bar")).To(BeTrue())
		})
	})
})
