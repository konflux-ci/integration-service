/*
Copyright 2024 Red Hat Inc.

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
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Auto-release annotation evaluation", Ordered, func() {
	var (
		hasSnapshot *applicationapiv1alpha1.Snapshot
	)

	const (
		namespace    = "default"
		snapshotName = "auto-release-snapshot"
	)

	BeforeAll(func() {
		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:        snapshotName,
				Namespace:   namespace,
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: "test-application",
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           "component-sample",
						ContainerImage: "quay.io/redhat-appstudio/sample-image:latest",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).To(Succeed())

		// Ensure it's created and retrievable
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: snapshotName, Namespace: namespace}, hasSnapshot)
		}, time.Second*10).ShouldNot(HaveOccurred())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasSnapshot)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("returns false when the auto-release annotation is missing", func() {
		snapshotCopy := hasSnapshot.DeepCopy()
		autoReleaseAnnotation := ""
		allowed, err := gitops.EvaluateSnapshotAutoReleaseAnnotation(autoReleaseAnnotation, snapshotCopy)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeFalse())
	})

	It("returns true when the auto-release annotation is 'true'", func() {
		snapshotCopy := hasSnapshot.DeepCopy()
		autoReleaseAnnotation := "true"
		allowed, err := gitops.EvaluateSnapshotAutoReleaseAnnotation(autoReleaseAnnotation, snapshotCopy)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeTrue())
	})

	It("returns false when the auto-release annotation is 'false'", func() {
		snapshotCopy := hasSnapshot.DeepCopy()
		autoReleaseAnnotation := "false"
		allowed, err := gitops.EvaluateSnapshotAutoReleaseAnnotation(autoReleaseAnnotation, snapshotCopy)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeFalse())
	})

	It("returns true when the snapshot is older than 1 week", func() {
		snapshotCopy := hasSnapshot.DeepCopy()
		snapshotCopy.CreationTimestamp = metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
		autoReleaseAnnotation := "timestamp(snapshot.metadata.creationTimestamp) < (timestamp(now) - duration('168h'))"
		allowed, err := gitops.EvaluateSnapshotAutoReleaseAnnotation(autoReleaseAnnotation, snapshotCopy)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeTrue())
	})

	It("returns true  when the updated component is 'component-sample'", func() {
		snapshotCopy := hasSnapshot.DeepCopy()
		autoReleaseAnnotation := "updatedComponentIs('component-sample')"
		allowed, err := gitops.EvaluateSnapshotAutoReleaseAnnotation(autoReleaseAnnotation, snapshotCopy)
		Expect(err).NotTo(HaveOccurred())
		Expect(allowed).To(BeTrue())
	})

	It("returns error and false when the auto-release annotation is an invalid CEL expression", func() {
		snapshotCopy := hasSnapshot.DeepCopy()
		autoReleaseAnnotation := "invalid expression$" // syntactically invalid
		allowed, err := gitops.EvaluateSnapshotAutoReleaseAnnotation(autoReleaseAnnotation, snapshotCopy)
		Expect(err).To(HaveOccurred())
		Expect(allowed).To(BeFalse())
	})
})
