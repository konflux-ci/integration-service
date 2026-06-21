/*
Copyright 2023 Red Hat Inc.

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

package scenario

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var _ = Describe("Predicates", func() {

	var scenario *v1beta2.IntegrationTestScenario

	BeforeEach(func() {
		scenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-scenario",
				Namespace: "default",
			},
		}
	})

	Context("when testing ScenarioCreatedPredicate", func() {
		instance := ScenarioCreatedPredicate()

		It("should return true for create events", func() {
			Expect(instance.Create(event.CreateEvent{
				Object: scenario,
			})).To(BeTrue())
		})

		It("should return false for delete events", func() {
			Expect(instance.Delete(event.DeleteEvent{
				Object: scenario,
			})).To(BeFalse())
		})

		It("should return false for generic events", func() {
			Expect(instance.Generic(event.GenericEvent{
				Object: scenario,
			})).To(BeFalse())
		})

		It("should return false for update events", func() {
			Expect(instance.Update(event.UpdateEvent{
				ObjectOld: scenario,
				ObjectNew: scenario,
			})).To(BeFalse())
		})
	})
})
