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

package helpers_test

import (
	"context"
	"strings"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("CreateWarningEvent", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		component *applicationapiv1alpha1.Component
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(applicationapiv1alpha1.AddToScheme(scheme)).To(Succeed())
		component = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "my-component", Namespace: "tenant", UID: "uid-1"},
		}
	})

	It("creates a Warning event that references the object", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		Expect(helpers.CreateWarningEvent(ctx, c, component, "TestReason", "something went wrong")).To(Succeed())

		events := &corev1.EventList{}
		Expect(c.List(ctx, events, client.InNamespace("tenant"))).To(Succeed())
		Expect(events.Items).To(HaveLen(1))
		event := events.Items[0]
		Expect(event.Type).To(Equal(corev1.EventTypeWarning))
		Expect(event.Reason).To(Equal("TestReason"))
		Expect(event.Message).To(Equal("something went wrong"))
		Expect(event.Source.Component).To(Equal("integration-service"))
		Expect(event.InvolvedObject.Kind).To(Equal("Component"))
		Expect(event.InvolvedObject.APIVersion).To(Equal("appstudio.redhat.com/v1alpha1"))
		Expect(event.InvolvedObject.Name).To(Equal("my-component"))
		Expect(event.InvolvedObject.Namespace).To(Equal("tenant"))
		Expect(string(event.InvolvedObject.UID)).To(Equal("uid-1"))
	})

	It("truncates long messages", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		Expect(helpers.CreateWarningEvent(ctx, c, component, "TestReason", strings.Repeat("x", 5000))).To(Succeed())

		events := &corev1.EventList{}
		Expect(c.List(ctx, events, client.InNamespace("tenant"))).To(Succeed())
		Expect(events.Items).To(HaveLen(1))
		Expect(events.Items[0].Message).To(HaveLen(1024))
		Expect(events.Items[0].Message).To(HaveSuffix("..."))
	})

	It("returns an error when the object kind is not registered in the scheme", func() {
		c := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
		Expect(helpers.CreateWarningEvent(ctx, c, component, "TestReason", "msg")).NotTo(Succeed())
	})
})
