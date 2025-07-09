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

package v1beta2

import (
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Snapshot webhook", Ordered, func() {

	var (
		snapshot           *applicationapiv1alpha1.Snapshot
		hasApp             *applicationapiv1alpha1.Application
		originalComponents []applicationapiv1alpha1.SnapshotComponent
	)

	BeforeAll(func() {
		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-application",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "test-application",
				Description: "This is a test application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		originalComponents = []applicationapiv1alpha1.SnapshotComponent{
			{
				Name:           "component-1",
				ContainerImage: "registry.io/image1:v1.0.0",
			},
			{
				Name:           "component-2",
				ContainerImage: "registry.io/image2:v1.0.0",
			},
		}
	})

	BeforeEach(func() {
		snapshot = &applicationapiv1alpha1.Snapshot{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-snapshot",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "test-application",
				DisplayName: "Test Snapshot",
				Components:  originalComponents,
			},
		}
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, snapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("should successfully create a snapshot with components", func() {
		Expect(k8sClient.Create(ctx, snapshot)).Should(Succeed())

		// Verify the snapshot was created with the correct components
		createdSnapshot := &applicationapiv1alpha1.Snapshot{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-snapshot",
				Namespace: "default",
			}, createdSnapshot)
		}).Should(Succeed())

		Expect(createdSnapshot.Spec.Components).To(Equal(originalComponents))
	})

	It("should reject updates to the components field", func() {
		// Create the snapshot first
		Expect(k8sClient.Create(ctx, snapshot)).Should(Succeed())

		// Try to update the components field
		updatedSnapshot := &applicationapiv1alpha1.Snapshot{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-snapshot",
				Namespace: "default",
			}, updatedSnapshot)
		}).Should(Succeed())

		// Modify the components
		updatedSnapshot.Spec.Components = append(updatedSnapshot.Spec.Components, applicationapiv1alpha1.SnapshotComponent{
			Name:           "component-3",
			ContainerImage: "registry.io/image3:v1.0.0",
		})

		// This update should fail due to webhook validation
		Expect(k8sClient.Update(ctx, updatedSnapshot)).ShouldNot(Succeed())
	})

	It("should reject modifications to existing components", func() {
		// Create the snapshot first
		Expect(k8sClient.Create(ctx, snapshot)).Should(Succeed())

		// Try to modify an existing component
		updatedSnapshot := &applicationapiv1alpha1.Snapshot{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-snapshot",
				Namespace: "default",
			}, updatedSnapshot)
		}).Should(Succeed())

		// Modify the container image of the first component
		updatedSnapshot.Spec.Components[0].ContainerImage = "registry.io/image1:v2.0.0"

		// This update should fail due to webhook validation
		Expect(k8sClient.Update(ctx, updatedSnapshot)).ShouldNot(Succeed())
	})

	It("should reject removal of components", func() {
		// Create the snapshot first
		Expect(k8sClient.Create(ctx, snapshot)).Should(Succeed())

		// Try to remove a component
		updatedSnapshot := &applicationapiv1alpha1.Snapshot{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-snapshot",
				Namespace: "default",
			}, updatedSnapshot)
		}).Should(Succeed())

		// Remove the last component
		updatedSnapshot.Spec.Components = updatedSnapshot.Spec.Components[:len(updatedSnapshot.Spec.Components)-1]

		// This update should fail due to webhook validation
		Expect(k8sClient.Update(ctx, updatedSnapshot)).ShouldNot(Succeed())
	})

	It("should handle invalid object types gracefully", func() {
		validator := &SnapshotCustomValidator{}

		// Test ValidateCreate with wrong object type
		_, err := validator.ValidateCreate(ctx, hasApp)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected a Snapshot object"))

		// Test ValidateUpdate with wrong oldObj type
		_, err = validator.ValidateUpdate(ctx, hasApp, snapshot)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected a Snapshot object for oldObj"))

		// Test ValidateUpdate with wrong newObj type
		_, err = validator.ValidateUpdate(ctx, snapshot, hasApp)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected a Snapshot object for newObj"))

		// Test ValidateDelete with wrong object type
		_, err = validator.ValidateDelete(ctx, hasApp)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected a Snapshot object"))
	})

	It("should directly test validator methods without cluster", func() {
		validator := &SnapshotCustomValidator{}

		// Test ValidateCreate directly
		_, err := validator.ValidateCreate(ctx, snapshot)
		Expect(err).NotTo(HaveOccurred())

		// Test ValidateUpdate with identical components (should succeed)
		snapshot2 := snapshot.DeepCopy()
		_, err = validator.ValidateUpdate(ctx, snapshot, snapshot2)
		Expect(err).NotTo(HaveOccurred())

		// Test ValidateUpdate with different components (should fail)
		snapshot3 := snapshot.DeepCopy()
		snapshot3.Spec.Components[0].ContainerImage = "registry.io/image1:v2.0.0"
		_, err = validator.ValidateUpdate(ctx, snapshot, snapshot3)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("components field is immutable"))

		// Test ValidateDelete directly
		_, err = validator.ValidateDelete(ctx, snapshot)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should allow updates to other fields while preserving components", func() {
		// Create the snapshot first
		Expect(k8sClient.Create(ctx, snapshot)).Should(Succeed())

		// Update non-components fields
		updatedSnapshot := &applicationapiv1alpha1.Snapshot{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-snapshot",
				Namespace: "default",
			}, updatedSnapshot)
		}).Should(Succeed())

		// Modify display name and description (but keep components unchanged)
		updatedSnapshot.Spec.DisplayName = "Updated Test Snapshot"
		updatedSnapshot.Spec.DisplayDescription = "Updated description"

		// This update should succeed as components are not modified
		Expect(k8sClient.Update(ctx, updatedSnapshot)).Should(Succeed())

		// Verify the changes were applied
		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-snapshot",
				Namespace: "default",
			}, updatedSnapshot)
			if err != nil {
				return err
			}
			if updatedSnapshot.Spec.DisplayName != "Updated Test Snapshot" {
				return errors.NewBadRequest("DisplayName was not updated")
			}
			return nil
		}).Should(Succeed())

		// Verify components remained unchanged
		Expect(updatedSnapshot.Spec.Components).To(Equal(originalComponents))
	})
})
