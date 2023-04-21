package tekton_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/tekton"

	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
				Labels: map[string]string{
					"pipelines.appstudio.openshift.io/type": "test",
				},
			},
			Spec: tektonv1beta1.PipelineRunSpec{},
		}
		newtestpipelineRun = testpipelineRun.DeepCopy()
	})

	Context("when testing IntegrationOrBuildPipelineRunSucceededPredicate", func() {
		instance := tekton.IntegrationOrBuildPipelineRunFinishedPredicate()

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
			newtestpipelineRun.Labels["pipelines.appstudio.openshift.io/type"] = "build"
			Expect(instance.Update(contextEvent)).To(BeTrue())
			contextEvent.ObjectNew = &tektonv1beta1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("should return true when an updated event is received for a failed PipelineRun", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: testpipelineRun,
				ObjectNew: newtestpipelineRun,
			}

			Expect(instance.Update(contextEvent)).To(BeFalse())
			newtestpipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "False",
			})
			Expect(instance.Update(contextEvent)).To(BeTrue())
			newtestpipelineRun.Labels["pipelines.appstudio.openshift.io/type"] = "build"
			Expect(instance.Update(contextEvent)).To(BeTrue())
			contextEvent.ObjectNew = &tektonv1beta1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})
	})

	Context("when testing IntegrationPipelineRunStartedPredicate", func() {
		instance := tekton.IntegrationPipelineRunStartedPredicate()

		It("should ignore create events", func() {
			contextEvent := event.CreateEvent{
				Object: testpipelineRun,
			}
			Expect(instance.Create(contextEvent)).To(BeFalse())
		})
		It("should ignore delete events", func() {
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

		It("should return true when an update event is received for a started PipelineRun", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: testpipelineRun,
				ObjectNew: newtestpipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
			newtestpipelineRun.Status.StartTime = &v1.Time{Time: time.Now()}
			Expect(instance.Update(contextEvent)).To(BeTrue())
			contextEvent.ObjectNew = &tektonv1beta1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})
	})
})
