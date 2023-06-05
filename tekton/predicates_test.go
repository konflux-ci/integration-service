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
		pipelineRun    *tektonv1beta1.PipelineRun
		newPipelineRun *tektonv1beta1.PipelineRun
	)

	Context("when testing BuildPipelineRunFinishedPredicate", func() {
		instance := tekton.BuildPipelineRunFinishedPredicate()

		BeforeEach(func() {

			pipelineRun = &tektonv1beta1.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: prefix + "-",
					Namespace:    namespace,
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type": "build",
					},
					Annotations: map[string]string{},
				},
				Spec: tektonv1beta1.PipelineRunSpec{},
			}
			newPipelineRun = pipelineRun.DeepCopy()
		})

		It("should ignore creating events", func() {
			contextEvent := event.CreateEvent{
				Object: pipelineRun,
			}
			Expect(instance.Create(contextEvent)).To(BeFalse())
		})
		It("should ignore deleting events", func() {
			contextEvent := event.DeleteEvent{
				Object: pipelineRun,
			}
			Expect(instance.Delete(contextEvent)).To(BeFalse())
		})

		It("should ignore generic events", func() {
			contextEvent := event.GenericEvent{
				Object: pipelineRun,
			}
			Expect(instance.Generic(contextEvent)).To(BeFalse())
		})

		It("should return true when an updated event is received for a succeeded PipelineRun and signed", func() {
			newPipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "True",
			})
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())

			newPipelineRun.Annotations["chains.tekton.dev/signed"] = "true"
			Expect(instance.Update(contextEvent)).To(BeTrue())
			contextEvent.ObjectNew = &tektonv1beta1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("should return true when an updated event is received for a failed PipelineRun and signed", func() {
			// also failed pipelines are signed by tekton chains, test it
			newPipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "False",
			})

			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())

			newPipelineRun.Annotations["chains.tekton.dev/signed"] = "true"
			Expect(instance.Update(contextEvent)).To(BeTrue())
			contextEvent.ObjectNew = &tektonv1beta1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("should return false when an updated event is received for a failed PipelineRun and no signed", func() {
			// also failed pipelines are signed by tekton chains, test it
			newPipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "True",
			})

			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())

			newPipelineRun.Annotations["chains.tekton.dev/signed"] = "false"
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})
	})

	Context("when testing IntegrationPipelineRunPredicate", func() {
		instance := tekton.IntegrationPipelineRunPredicate()

		BeforeEach(func() {

			pipelineRun = &tektonv1beta1.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: prefix + "-",
					Namespace:    namespace,
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type": "test",
					},
				},
				Spec: tektonv1beta1.PipelineRunSpec{},
			}
			newPipelineRun = pipelineRun.DeepCopy()
		})

		It("should ignore create events", func() {
			contextEvent := event.CreateEvent{
				Object: pipelineRun,
			}
			Expect(instance.Create(contextEvent)).To(BeFalse())
		})
		It("should ignore delete events", func() {
			contextEvent := event.DeleteEvent{
				Object: pipelineRun,
			}
			Expect(instance.Delete(contextEvent)).To(BeFalse())
		})

		It("should ignore generic events", func() {
			contextEvent := event.GenericEvent{
				Object: pipelineRun,
			}
			Expect(instance.Generic(contextEvent)).To(BeFalse())
		})

		It("should return true when an updated event is received for a succeeded PipelineRun", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}

			Expect(instance.Update(contextEvent)).To(BeFalse())
			newPipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "True",
			})
			Expect(instance.Update(contextEvent)).To(BeTrue())
			contextEvent.ObjectNew = &tektonv1beta1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("should return true when an updated event is received for a failed PipelineRun", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}

			Expect(instance.Update(contextEvent)).To(BeFalse())
			newPipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "False",
			})
			Expect(instance.Update(contextEvent)).To(BeTrue())
			contextEvent.ObjectNew = &tektonv1beta1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("should return true when an update event is received for a started PipelineRun", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
			newPipelineRun.Status.StartTime = &v1.Time{Time: time.Now()}
			Expect(instance.Update(contextEvent)).To(BeTrue())
			contextEvent.ObjectNew = &tektonv1beta1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})
	})
})
