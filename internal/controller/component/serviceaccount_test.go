/*
Copyright 2026 Red Hat Inc.

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

package component

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/tonglil/buflogr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Error paths of the per-component ServiceAccount ensure that the adapter specs cannot reach through
// a live API server: a Get that fails for a reason other than NotFound, an owner the scheme cannot
// resolve, an owner in another namespace, and a malformed owner reference on an existing object.
var _ = Describe("ensureOwnedObject and checkExistingObject", func() {
	var log helpers.IntegrationLogger
	const otherUID = "99999999-8888-7777-6666-555555555555"

	newOwner := func(name, namespace string) *applicationapiv1alpha1.Component {
		return &applicationapiv1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	}

	BeforeEach(func() {
		log = helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&bytes.Buffer{})}
	})

	It("returns a Get error that is not NotFound as it is", func() {
		// A PartialObjectMetadata without a kind cannot be resolved against the scheme, so the
		// client's Get fails before it reaches the cache, with an error that is not NotFound.
		obj := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "its-pipeline-unit-get-error", Namespace: "default"}}
		err := ensureOwnedObject(ctx, k8sClient, log, newOwner("unit-owner", "default"), "Object", obj)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsNotFound(err)).To(BeFalse())
		Expect(err).To(MatchError(ContainSubstring("Kind")))
	})

	It("returns the owner reference error when the owner is in another namespace and creates nothing", func() {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "its-pipeline-unit-cross-namespace", Namespace: "default"}}
		err := ensureOwnedObject(ctx, k8sClient, log, newOwner("unit-owner", "elsewhere"), "ServiceAccount", sa)
		Expect(err).To(MatchError(ContainSubstring("cross-namespace owner references are disallowed")))

		err = k8sClient.Get(ctx, types.NamespacedName{Name: sa.Name, Namespace: "default"}, &corev1.ServiceAccount{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("returns the error when the owner's kind is not in the scheme", func() {
		existing := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
			Name:      "its-pipeline-unit-owned",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: applicationapiv1alpha1.GroupVersion.String(),
				Kind:       "Component",
				Name:       "unit-owner",
				UID:        otherUID,
			}},
		}}
		err := checkExistingObject(k8sClient, &metav1.PartialObjectMetadata{}, existing)
		Expect(err).To(MatchError(ContainSubstring("Kind")))
	})

	It("returns the error when an owner reference carries a malformed apiVersion", func() {
		existing := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
			Name:      "its-pipeline-unit-owned",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "appstudio.redhat.com/v1alpha1/extra",
				Kind:       "Component",
				Name:       "unit-owner",
				UID:        otherUID,
			}},
		}}
		err := checkExistingObject(k8sClient, newOwner("unit-owner", "default"), existing)
		Expect(err).To(MatchError(ContainSubstring("unexpected GroupVersion string")))
	})
})
