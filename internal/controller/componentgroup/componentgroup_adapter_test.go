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

package componentgroup

import (
	"bytes"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tonglil/buflogr"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ComponentGroup Adapter", Ordered, func() {
	var (
		adapter        *Adapter
		logger         helpers.IntegrationLogger
		componentGroup *v1beta2.ComponentGroup
	)

	const (
		SampleImage = "quay.io/example/image@sha256:abc123def456"
	)

	// fetchFreshCG retrieves the latest version of componentGroup from the cluster,
	// which is required to have a correct ResourceVersion for optimistic-lock patches.
	fetchFreshCG := func() *v1beta2.ComponentGroup {
		fresh := &v1beta2.ComponentGroup{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(componentGroup), fresh)
		}, time.Second*10).Should(Succeed())
		return fresh
	}

	// setGCL overwrites the GCL status on the cluster and waits for it to be visible.
	setGCL := func(entries []v1beta2.ComponentState) {
		cg := fetchFreshCG()
		cg.Status.GlobalCandidateList = entries
		Expect(k8sClient.Status().Update(ctx, cg)).To(Succeed())
		newRV := cg.ResourceVersion // Updated in-place by the client after a successful write

		// Wait for the informer cache to reflect the exact ResourceVersion written above.
		// Checking only HaveLen is insufficient when entries is empty, because the cache
		// may satisfy HaveLen(0) with a stale pre-update entry, causing fetchFreshCG() to
		// return a stale ResourceVersion and triggering a 409 conflict on the subsequent patch.
		Eventually(func(g Gomega) {
			updated := &v1beta2.ComponentGroup{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(componentGroup), updated)).To(Succeed())
			g.Expect(updated.ResourceVersion).To(Equal(newRV))
			g.Expect(updated.Status.GlobalCandidateList).To(HaveLen(len(entries)))
		}, time.Second*10).Should(Succeed())
	}

	BeforeAll(func() {
		logger = helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&bytes.Buffer{})}

		componentGroup = &v1beta2.ComponentGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-component-group",
				Namespace: "default",
			},
			Spec: v1beta2.ComponentGroupSpec{
				Components: []v1beta2.ComponentReference{
					{
						Name: "comp-a",
						ComponentVersion: v1beta2.ComponentVersionReference{
							Name: "main",
						},
					},
					{
						Name: "comp-b",
						ComponentVersion: v1beta2.ComponentVersionReference{
							Name: "v1",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, componentGroup)).Should(Succeed())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, componentGroup)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("can create a new Adapter instance", func() {
		Expect(reflect.TypeOf(NewAdapter(ctx, componentGroup, logger, loader.NewMockLoader(), k8sClient))).
			To(Equal(reflect.TypeOf(&Adapter{})))
	})

	When("alignGCLWithSpecComponents is called with an empty GCL", func() {
		BeforeEach(func() {
			setGCL([]v1beta2.ComponentState{})
			adapter = NewAdapter(ctx, componentGroup, logger, loader.NewMockLoader(), k8sClient)
		})

		It("adds a GCL entry for every component in spec.components", func() {
			Expect(adapter.alignGCLWithSpecComponents(fetchFreshCG())).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &v1beta2.ComponentGroup{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(componentGroup), updated)).To(Succeed())
				g.Expect(updated.Status.GlobalCandidateList).To(HaveLen(2))
				g.Expect(updated.Status.GlobalCandidateList[0].Name).To(Equal("comp-a"))
				g.Expect(updated.Status.GlobalCandidateList[0].Version).To(Equal("main"))
				g.Expect(updated.Status.GlobalCandidateList[0].LastPromotedImage).To(BeEmpty())
				g.Expect(updated.Status.GlobalCandidateList[1].Name).To(Equal("comp-b"))
				g.Expect(updated.Status.GlobalCandidateList[1].Version).To(Equal("v1"))
				g.Expect(updated.Status.GlobalCandidateList[1].LastPromotedImage).To(BeEmpty())
			}, time.Second*10).Should(Succeed())
		})
	})

	When("alignGCLWithSpecComponents is called with a GCL entry that matches a spec component", func() {
		BeforeEach(func() {
			setGCL([]v1beta2.ComponentState{
				{Name: "comp-a", Version: "main", LastPromotedImage: SampleImage},
			})
			adapter = NewAdapter(ctx, componentGroup, logger, loader.NewMockLoader(), k8sClient)
		})

		It("preserves the existing image data for the matching entry", func() {
			Expect(adapter.alignGCLWithSpecComponents(fetchFreshCG())).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &v1beta2.ComponentGroup{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(componentGroup), updated)).To(Succeed())
				g.Expect(updated.Status.GlobalCandidateList).To(HaveLen(2))

				compA := updated.Status.GlobalCandidateList[0]
				g.Expect(compA.Name).To(Equal("comp-a"))
				g.Expect(compA.LastPromotedImage).To(Equal(SampleImage))

				compB := updated.Status.GlobalCandidateList[1]
				g.Expect(compB.Name).To(Equal("comp-b"))
				g.Expect(compB.LastPromotedImage).To(BeEmpty())
			}, time.Second*10).Should(Succeed())
		})
	})

	When("alignGCLWithSpecComponents is called with a GCL entry for a component no longer in spec", func() {
		BeforeEach(func() {
			setGCL([]v1beta2.ComponentState{
				{Name: "comp-a", Version: "main", LastPromotedImage: SampleImage},
				{Name: "comp-b", Version: "v1"},
				{Name: "comp-c", Version: "main"}, // not in spec.components
			})
			adapter = NewAdapter(ctx, componentGroup, logger, loader.NewMockLoader(), k8sClient)
		})

		It("removes the stale entry from the GCL", func() {
			Expect(adapter.alignGCLWithSpecComponents(fetchFreshCG())).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &v1beta2.ComponentGroup{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(componentGroup), updated)).To(Succeed())
				g.Expect(updated.Status.GlobalCandidateList).To(HaveLen(2))
				names := []string{
					updated.Status.GlobalCandidateList[0].Name,
					updated.Status.GlobalCandidateList[1].Name,
				}
				g.Expect(names).NotTo(ContainElement("comp-c"))
			}, time.Second*10).Should(Succeed())
		})
	})

	When("EnsureGCLAlignedWithSpecComponents is called", func() {
		BeforeEach(func() {
			adapter = NewAdapter(ctx, componentGroup, logger, loader.NewLoader(), k8sClient)
		})

		It("returns ContinueProcessing without error", func() {
			result, err := adapter.EnsureGCLAlignedWithSpecComponents()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())
			Expect(result.RequeueRequest).To(BeFalse())
		})
	})
})
