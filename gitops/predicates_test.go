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

	var bindingMissingStatus, bindingFalseStatus, bindingTrueStatus, bindingDeploymentFailedStatus *applicationapiv1alpha1.SnapshotEnvironmentBinding

	BeforeAll(func() {
		bindingMissingStatus = &applicationapiv1alpha1.SnapshotEnvironmentBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "bindingmissingstatus",
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
		bindingFalseStatus = &applicationapiv1alpha1.SnapshotEnvironmentBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "bindingfalsestatus",
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
		bindingDeploymentFailedStatus = &applicationapiv1alpha1.SnapshotEnvironmentBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "bindingdeploymentfailedstatus",
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

		Expect(k8sClient.Create(ctx, bindingMissingStatus)).Should(Succeed())
		Expect(k8sClient.Create(ctx, bindingFalseStatus)).Should(Succeed())
		Expect(k8sClient.Create(ctx, bindingTrueStatus)).Should(Succeed())
		Expect(k8sClient.Create(ctx, bindingDeploymentFailedStatus)).Should(Succeed())

		// Set the binding statuses after they are created
		bindingFalseStatus.Status.ComponentDeploymentConditions = []metav1.Condition{
			{
				Type:   gitops.BindingDeploymentStatusConditionType,
				Status: metav1.ConditionFalse,
			},
		}
		bindingTrueStatus.Status.ComponentDeploymentConditions = []metav1.Condition{
			{
				Type:   gitops.BindingDeploymentStatusConditionType,
				Status: metav1.ConditionTrue,
			},
		}
		bindingDeploymentFailedStatus.Status.BindingConditions = []metav1.Condition{
			{
				Type:   gitops.BindingErrorOccurredStatusConditionType,
				Status: metav1.ConditionTrue,
			},
		}
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, bindingMissingStatus)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, bindingFalseStatus)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, bindingTrueStatus)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	Context("when testing DeploymentSucceededPredicate predicate", func() {
		instance := gitops.DeploymentSucceededForIntegrationBindingPredicate()

		It("returns true when the .Status.ComponentDeploymentConditions field of old SEB is set to false and that of new SEB is set to true", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: bindingFalseStatus,
				ObjectNew: bindingTrueStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("returns true when the .Status.ComponentDeploymentConditions field of old SEB is unset and that of new SEB is set to true", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: bindingMissingStatus,
				ObjectNew: bindingTrueStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("returns false when the SEB with succeeded deployment is created", func() {
			contextEvent := event.CreateEvent{
				Object: bindingTrueStatus,
			}
			Expect(instance.Create(contextEvent)).To(BeFalse())
		})

		It("returns false when the SEB with succeeded deployment is deleted", func() {
			contextEvent := event.DeleteEvent{
				Object: bindingTrueStatus,
			}
			Expect(instance.Delete(contextEvent)).To(BeFalse())
		})

		It("returns false when the SEB with succeeded deployment encounters a generic event", func() {
			contextEvent := event.GenericEvent{
				Object: bindingTrueStatus,
			}
			Expect(instance.Generic(contextEvent)).To(BeFalse())
		})
	})
	Context("when testing DeploymentFailedPredicate predicate", func() {
		instance := gitops.DeploymentFailedForIntegrationBindingPredicate()
		It("returns true when the .Status.BindingConditions.ErrorOccured field of old SEB is set to false and that of new SEB is set to true", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: bindingFalseStatus,
				ObjectNew: bindingDeploymentFailedStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("returns true when the .Status.BindingConditions.ErrorOccured field of old SEB is unset and that of new SEB is set to true", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: bindingMissingStatus,
				ObjectNew: bindingDeploymentFailedStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("returns false when the SEB with failed deployment is created", func() {
			contextEvent := event.CreateEvent{
				Object: bindingDeploymentFailedStatus,
			}
			Expect(instance.Create(contextEvent)).To(BeFalse())
		})

		It("returns false when the SEB with failed deployment is deleted", func() {
			contextEvent := event.DeleteEvent{
				Object: bindingDeploymentFailedStatus,
			}
			Expect(instance.Delete(contextEvent)).To(BeFalse())
		})

		It("returns false when the SEB with failed deployment encounters a generic event", func() {
			contextEvent := event.GenericEvent{
				Object: bindingDeploymentFailedStatus,
			}
			Expect(instance.Generic(contextEvent)).To(BeFalse())
		})
	})
	Context("when testing IntegrationSnapshotEnvironmentBindingPredicate predicate", func() {
		instance := gitops.IntegrationSnapshotEnvironmentBindingPredicate()
		It("returns true when the successfully deployed SEB has the SnapshotTestScenarioLabel set", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: bindingMissingStatus,
				ObjectNew: bindingTrueStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("returns true when the SEB with failed deployment has the SnapshotTestScenarioLabel set", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: bindingMissingStatus,
				ObjectNew: bindingDeploymentFailedStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("returns false when the successfully deployed SEB has the SnapshotTestScenarioLabel not set", func() {
			bindingTrueStatus.ObjectMeta.Labels = map[string]string{}

			contextEvent := event.UpdateEvent{
				ObjectOld: bindingMissingStatus,
				ObjectNew: bindingTrueStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("returns false when the SEB with SnapshotTestScenarioLabel is created", func() {
			bindingMissingStatus.ObjectMeta.Labels = map[string]string{gitops.SnapshotTestScenarioLabel: "test-scenario"}

			contextEvent := event.CreateEvent{
				Object: bindingMissingStatus,
			}
			Expect(instance.Create(contextEvent)).To(BeFalse())
		})
		It("returns false when the SEB with SnapshotTestScenarioLabel is deleted", func() {
			bindingMissingStatus.ObjectMeta.Labels = map[string]string{gitops.SnapshotTestScenarioLabel: "test-scenario"}

			contextEvent := event.DeleteEvent{
				Object: bindingMissingStatus,
			}
			Expect(instance.Delete(contextEvent)).To(BeFalse())
		})
		It("returns false when the SEB with SnapshotTestScenarioLabel encounters a generic event", func() {
			bindingMissingStatus.ObjectMeta.Labels = map[string]string{gitops.SnapshotTestScenarioLabel: "test-scenario"}

			contextEvent := event.GenericEvent{
				Object: bindingMissingStatus,
			}
			Expect(instance.Generic(contextEvent)).To(BeFalse())
		})
	})
})
