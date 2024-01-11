/*
Copyright 2022 Red Hat Inc.

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

package tekton_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-appstudio/integration-service/tekton"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("Predicates", func() {

	const (
		prefix    = "testpipeline"
		namespace = "default"
	)
	var (
		pipelineRun    *tektonv1.PipelineRun
		newPipelineRun *tektonv1.PipelineRun
	)

	Context("when testing BuildPipelineRunSignedAndSucceededPredicate", func() {
		instance := tekton.BuildPipelineRunSignedAndSucceededPredicate()

		BeforeEach(func() {

			pipelineRun = &tektonv1.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: prefix + "-",
					Namespace:    namespace,
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type": "build",
					},
					Annotations: map[string]string{},
				},
				Spec: tektonv1.PipelineRunSpec{},
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
			contextEvent.ObjectNew = &tektonv1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("should return false when an updated event is received for a failed PipelineRun and signed", func() {
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
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("should return true when an updated event is received for a succeeded PipelineRun with failed chains signing", func() {
			newPipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "True",
			})

			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())

			newPipelineRun.Annotations["chains.tekton.dev/signed"] = "failed"
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("should return false when an updated event is received for a succeeded and signed pipelineRun that was annotated with Snapshot", func() {
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
			newPipelineRun.Annotations["appstudio.openshift.io/snapshot"] = "snapshot"
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})
	})

	Context("when testing IntegrationPipelineRunPredicate", func() {
		instance := tekton.IntegrationPipelineRunPredicate()

		BeforeEach(func() {

			pipelineRun = &tektonv1.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: prefix + "-",
					Namespace:    namespace,
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type": "test",
					},
				},
				Spec: tektonv1.PipelineRunSpec{},
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
			contextEvent.ObjectNew = &tektonv1.TaskRun{}
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
			contextEvent.ObjectNew = &tektonv1.TaskRun{}
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
			contextEvent.ObjectNew = &tektonv1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("should return true when an update event is received for a PipelineRun getting deleted", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
			newPipelineRun.DeletionTimestamp = nil
			Expect(instance.Update(contextEvent)).To(BeFalse())
			newPipelineRun.DeletionTimestamp = &v1.Time{Time: time.Now()}
			Expect(instance.Update(contextEvent)).To(BeTrue())
			contextEvent.ObjectNew = &tektonv1.TaskRun{}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})
	})

	Context("when testing BuildPipelineRunCreatedPredicate", func() {
		instance := tekton.BuildPipelineRunCreatedPredicate()

		BeforeEach(func() {

			pipelineRun = &tektonv1.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: prefix + "-",
					Namespace:    namespace,
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type": "build",
					},
					Annotations: map[string]string{},
				},
				Spec: tektonv1.PipelineRunSpec{},
			}
			newPipelineRun = pipelineRun.DeepCopy()
		})

		It("should return true when a build pipeline is created", func() {
			contextEvent := event.CreateEvent{
				Object: pipelineRun,
			}
			Expect(instance.Create(contextEvent)).To(BeTrue())
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

		It("should ignore update events", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: pipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

	})

	Context("when testing BuildPipelineRunFailedPredicate", func() {
		instance := tekton.BuildPipelineRunFailedPredicate()

		BeforeEach(func() {

			pipelineRun = &tektonv1.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: prefix + "-",
					Namespace:    namespace,
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type": "build",
					},
					Annotations: map[string]string{},
				},
				Spec: tektonv1.PipelineRunSpec{},
			}
			newPipelineRun = pipelineRun.DeepCopy()
		})

		It("should ignore creation events", func() {
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

		It("should return false for an update event in which the build PLR has not finished", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("should return true for an update event in which the build PLR failed", func() {
			newPipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "False",
			})
			newPipelineRun.Annotations["chains.tekton.dev/signed"] = "true"
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("should return false for an update event in which the build PLR succeeded", func() {
			newPipelineRun.Status.SetCondition(&apis.Condition{
				Type:   apis.ConditionSucceeded,
				Status: "True",
			})
			newPipelineRun.Annotations["chains.tekton.dev/signed"] = "true"
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})
	})

	Context("when testing BuildPipelineRunDeletingPredicate", func() {
		instance := tekton.BuildPipelineRunDeletingPredicate()

		BeforeEach(func() {

			pipelineRun = &tektonv1.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: prefix + "-",
					Namespace:    namespace,
					Labels: map[string]string{
						"pipelines.appstudio.openshift.io/type": "build",
					},
					Annotations: map[string]string{},
				},
				Spec: tektonv1.PipelineRunSpec{},
			}
			newPipelineRun = pipelineRun.DeepCopy()
		})

		It("should ignore creation events", func() {
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

		It("should return false for an update event in which the build PLR has not been deleted", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("should return true for an update event marking build PLR for deletion", func() {
			newPipelineRun.DeletionTimestamp = &v1.Time{Time: time.Now()}
			contextEvent := event.UpdateEvent{
				ObjectOld: pipelineRun,
				ObjectNew: newPipelineRun,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

	})
})
