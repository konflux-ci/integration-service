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

package gitops_test

import (
	"context"
	"github.com/redhat-appstudio/integration-service/gitops"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Predicates", Ordered, func() {

	const (
		namespace       = "default"
		applicationName = "test-application"
		environmentName = "test-environment"
		snapshotName    = "test-snapshot"
	)

	var bindingUnknownStatus, bindingTrueStatus *applicationapiv1alpha1.SnapshotEnvironmentBinding

	BeforeAll(func() {
		bindingUnknownStatus = &applicationapiv1alpha1.SnapshotEnvironmentBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "bindingunknownstatus",
				Namespace:  namespace,
				Generation: 1,
			},
			Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
				Application: applicationName,
				Environment: environmentName,
				Snapshot:    snapshotName,
				Components:  []applicationapiv1alpha1.BindingComponent{},
			},
		}
		bindingTrueStatus = &applicationapiv1alpha1.SnapshotEnvironmentBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "bindingtruestatus",
				Namespace:  namespace,
				Generation: 1,
				Labels:     map[string]string{gitops.SnapshotTestScenarioLabel: "test-scenario"},
			},
			Spec: applicationapiv1alpha1.SnapshotEnvironmentBindingSpec{
				Application: applicationName,
				Environment: environmentName,
				Snapshot:    snapshotName,
				Components:  []applicationapiv1alpha1.BindingComponent{},
			},
		}
		ctx := context.Background()

		Expect(k8sClient.Create(ctx, bindingUnknownStatus)).Should(Succeed())
		Expect(k8sClient.Create(ctx, bindingTrueStatus)).Should(Succeed())

		// Set the binding statuses after they are created
		bindingUnknownStatus.Status.ComponentDeploymentConditions = []metav1.Condition{
			{
				Type:   gitops.BindingDeploymentStatusConditionType,
				Status: metav1.ConditionUnknown,
			},
		}
		bindingTrueStatus.Status.ComponentDeploymentConditions = []metav1.Condition{
			{
				Type:   gitops.BindingDeploymentStatusConditionType,
				Status: metav1.ConditionTrue,
			},
		}
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, bindingUnknownStatus)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, bindingTrueStatus)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	Context("when testing DeploymentFinishedPredicate predicate", func() {
		instance := gitops.DeploymentFinishedForIntegrationBindingPredicate()

		It("returns true when the old SnapshotEnvironmentBinding has unknown status and the new one has true status", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: bindingUnknownStatus,
				ObjectNew: bindingTrueStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})
	})
})
