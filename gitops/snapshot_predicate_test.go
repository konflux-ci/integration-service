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
		namespace             = "default"
		applicationName       = "test-application"
		componentName         = "test-component"
		snapshotOldName       = "test-snapshot-old"
		snapshotNewName       = "test-snapshot-new"
		snapshotAnnotationOld = "snapshot-annotation-old"
		snapshotAnnotationNew = "snapshot-annotation-new"
	)

	var (
		hasSnapshotUnknownStatus *applicationapiv1alpha1.Snapshot
		hasSnapshotTrueStatus    *applicationapiv1alpha1.Snapshot
		hasSnapshotAnnotationOld *applicationapiv1alpha1.Snapshot
		hasSnapshotAnnotationNew *applicationapiv1alpha1.Snapshot
		sampleImage              string
	)

	BeforeAll(func() {
		sampleImage = "quay.io/redhat-appstudio/sample-image:latest"

		hasSnapshotUnknownStatus = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotOldName,
				Namespace: namespace,
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel: componentName,
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: applicationName,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           componentName,
						ContainerImage: sampleImage,
					},
				},
			},
		}
		hasSnapshotTrueStatus = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotNewName,
				Namespace: namespace,
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel: componentName,
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: applicationName,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           componentName,
						ContainerImage: sampleImage,
					},
				},
			},
		}

		hasSnapshotAnnotationOld = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotAnnotationOld,
				Namespace: namespace,
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel: componentName,
				},
				Annotations: map[string]string{
					gitops.SnapshotTestsStatusAnnotation: "[{\"scenario\":\"scenario-1\",\"status\":\"EnvironmentProvisionError\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"sompletionTime\":\"2023-07-26T17:57:49+02:00\",\"details\":\"Failed to find deploymentTargetClass with right provisioner for copy of existingEnvironment\"}]",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: applicationName,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           componentName,
						ContainerImage: sampleImage,
					},
				},
			},
		}

		hasSnapshotAnnotationNew = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      snapshotAnnotationNew,
				Namespace: namespace,
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:                     gitops.SnapshotComponentType,
					gitops.SnapshotComponentLabel:                componentName,
					"pac.test.appstudio.openshift.io/event-type": "pull_request",
				},
				Annotations: map[string]string{
					gitops.SnapshotTestsStatusAnnotation: "[{\"scenario\":\"scenario-1\",\"status\":\"TestPassed\",\"startTime\":\"2023-07-26T16:57:49+02:00\",\"completionTime\":\"2023-07-26T17:57:49+02:00\",\"details\": \"test pass\"}]",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: applicationName,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           componentName,
						ContainerImage: sampleImage,
					},
				},
			},
		}
		ctx := context.Background()

		Expect(k8sClient.Create(ctx, hasSnapshotUnknownStatus)).Should(Succeed())
		Expect(k8sClient.Create(ctx, hasSnapshotTrueStatus)).Should(Succeed())
		Expect(k8sClient.Create(ctx, hasSnapshotAnnotationOld)).Should(Succeed())
		Expect(k8sClient.Create(ctx, hasSnapshotAnnotationNew)).Should(Succeed())

		// Set the binding statuses after they are created
		hasSnapshotUnknownStatus.Status.Conditions = []metav1.Condition{
			{
				Type:   gitops.AppStudioTestSucceededCondition,
				Status: metav1.ConditionUnknown,
			},
		}
		hasSnapshotTrueStatus.Status.Conditions = []metav1.Condition{
			{
				Type:   gitops.AppStudioTestSucceededCondition,
				Status: metav1.ConditionTrue,
			},
		}
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasSnapshotUnknownStatus)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasSnapshotTrueStatus)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	Context("when testing IntegrationSnapshotChangePredicate predicate", func() {
		instance := gitops.IntegrationSnapshotChangePredicate()

		It("returns true when the old Snapshot has unknown status and the new one has true status", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: hasSnapshotUnknownStatus,
				ObjectNew: hasSnapshotTrueStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})
		It("returns false when the old Snapshot has true status and the new one has true status", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: hasSnapshotTrueStatus,
				ObjectNew: hasSnapshotTrueStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})
		It("returns false when the Snapshot is deleted", func() {
			contextEvent := event.DeleteEvent{
				Object: hasSnapshotUnknownStatus,
			}
			Expect(instance.Delete(contextEvent)).To(BeFalse())
		})
	})

	Context("when testing IntegrationSnapshotChangePredicate predicate", func() {
		instance := gitops.SnapshotTestAnnotationChangePredicate()

		It("returns true when the test status annotation of Snapshot changed ", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: hasSnapshotAnnotationOld,
				ObjectNew: hasSnapshotAnnotationNew,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("returns false when the test status annotation of Snapshot is not changed ", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: hasSnapshotAnnotationOld,
				ObjectNew: hasSnapshotAnnotationOld,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("returns true when the test status annotation of old Snapshot doesn't exist but exists in new snapshot ", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: hasSnapshotTrueStatus,
				ObjectNew: hasSnapshotAnnotationNew,
			}
			Expect(instance.Update(contextEvent)).To(BeTrue())
		})

		It("returns false when the test status annotation doesn't exist in old and new Snapshot", func() {
			contextEvent := event.UpdateEvent{
				ObjectOld: hasSnapshotTrueStatus,
				ObjectNew: hasSnapshotTrueStatus,
			}
			Expect(instance.Update(contextEvent)).To(BeFalse())
		})

		It("returns false when the Snapshot is deleted", func() {
			contextEvent := event.DeleteEvent{
				Object: hasSnapshotUnknownStatus,
			}
			Expect(instance.Delete(contextEvent)).To(BeFalse())
		})
	})
})
