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

package keys_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/konflux-ci/integration-service/pkg/keys"
)

func namedConfigMap(name string, lbls map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    lbls,
		},
	}
}

func names(list *corev1.ConfigMapList) []string {
	out := make([]string, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, list.Items[i].Name)
	}
	return out
}

var _ = Describe("DualList", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
		c      client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		c = fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			namedConfigMap("old-build", map[string]string{keys.PipelineType.Old: "build", "extra": "keep"}),
			namedConfigMap("new-build", map[string]string{keys.PipelineType.New: "build", "extra": "keep"}),
			namedConfigMap("both-build", map[string]string{
				keys.PipelineType.Old: "build",
				keys.PipelineType.New: "build",
			}),
			namedConfigMap("conflict-old-build-new-test", map[string]string{
				keys.PipelineType.Old: "build",
				keys.PipelineType.New: "test",
			}),
			namedConfigMap("conflict-old-test-new-build", map[string]string{
				keys.PipelineType.Old: "test",
				keys.PipelineType.New: "build",
			}),
			namedConfigMap("old-test", map[string]string{keys.PipelineType.Old: "test"}),
			namedConfigMap("unrelated", map[string]string{"extra": "keep"}),
		).Build()
	})

	When("listing by PipelineType with value build", func() {
		It("should list objects whose effective type is build", func() {
			list := &corev1.ConfigMapList{}
			Expect(keys.DualList(ctx, c, list, "default", keys.PipelineType, []string{"build"})).To(Succeed())
			Expect(names(list)).To(ConsistOf("old-build", "new-build", "both-build", "conflict-old-test-new-build"))
		})

		It("should deduplicate an object that has both keys", func() {
			list := &corev1.ConfigMapList{}
			Expect(keys.DualList(ctx, c, list, "default", keys.PipelineType, []string{"build"})).To(Succeed())
			Expect(names(list)).To(HaveLen(4))
		})
	})

	When("extra label requirements are provided", func() {
		It("should AND extras onto both selectors and not fake an OR", func() {
			extra, err := labels.NewRequirement("extra", selection.Equals, []string{"keep"})
			Expect(err).NotTo(HaveOccurred())

			list := &corev1.ConfigMapList{}
			Expect(keys.DualList(ctx, c, list, "default", keys.PipelineType, []string{"build"}, *extra)).To(Succeed())
			Expect(names(list)).To(ConsistOf("old-build", "new-build"))
		})
	})

	When("the pair has no new key", func() {
		It("should list only objects matching the old key", func() {
			oldOnly := keys.Pair{Old: keys.PipelineType.Old}
			list := &corev1.ConfigMapList{}
			Expect(keys.DualList(ctx, c, list, "default", oldOnly, []string{"build"})).To(Succeed())
			// No new key on the pair, so precedence does not apply and dual-labeled objects match on Old.
			Expect(names(list)).To(ConsistOf("old-build", "both-build", "conflict-old-build-new-test"))
		})
	})

	When("old and new keys conflict", func() {
		It("should exclude an object whose new key disagrees with the requested value", func() {
			list := &corev1.ConfigMapList{}
			Expect(keys.DualList(ctx, c, list, "default", keys.PipelineType, []string{"build"})).To(Succeed())
			Expect(names(list)).To(ConsistOf("old-build", "new-build", "both-build", "conflict-old-test-new-build"))
			Expect(names(list)).NotTo(ContainElement("conflict-old-build-new-test"))
		})

		It("should include an object when only the preferred new key matches", func() {
			list := &corev1.ConfigMapList{}
			Expect(keys.DualList(ctx, c, list, "default", keys.PipelineType, []string{"test"})).To(Succeed())
			Expect(names(list)).To(ConsistOf("old-test", "conflict-old-build-new-test"))
			Expect(names(list)).NotTo(ContainElement("conflict-old-test-new-build"))
		})
	})

	When("the caller list carries pagination metadata", func() {
		It("should clear Continue and related ListMeta on the merged result", func() {
			remaining := int64(7)
			list := &corev1.ConfigMapList{}
			list.SetContinue("stale-token")
			list.SetResourceVersion("12345")
			list.SetRemainingItemCount(&remaining)

			Expect(keys.DualList(ctx, c, list, "default", keys.PipelineType, []string{"build"})).To(Succeed())
			Expect(list.GetContinue()).To(BeEmpty())
			Expect(list.GetResourceVersion()).To(BeEmpty())
			Expect(list.GetRemainingItemCount()).To(BeNil())
			Expect(names(list)).To(ConsistOf("old-build", "new-build", "both-build", "conflict-old-test-new-build"))
		})
	})
})
