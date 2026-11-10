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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/konflux-ci/integration-service/pkg/keys"
)

func styledMap(apiVersion string, labels, annotations map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: apiVersion, Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "sample",
			Namespace:   "default",
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

var _ = Describe("owned metadata style", func() {
	When("reading a key", func() {
		It("should prefer the new key and fall back to the old key", func() {
			both := styledMap("", map[string]string{
				keys.Test("type").Old: "component",
				keys.Test("type").New: "group",
			}, nil)
			oldOnly := styledMap("", map[string]string{keys.Test("type").Old: "component"}, nil)
			newOnly := styledMap("", map[string]string{keys.Test("type").New: "override"}, nil)

			value, found := keys.ReadLabel(both, keys.Test("type").Old)
			Expect(found).To(BeTrue())
			Expect(value).To(Equal("group"))
			value, found = keys.ReadLabel(oldOnly, keys.Test("type").Old)
			Expect(found).To(BeTrue())
			Expect(value).To(Equal("component"))
			value, found = keys.ReadLabel(newOnly, keys.Test("type").Old)
			Expect(found).To(BeTrue())
			Expect(value).To(Equal("override"))
			_, found = keys.ReadLabel(styledMap("", nil, nil), keys.Test("type").Old)
			Expect(found).To(BeFalse())
		})
	})

	When("an object already has a new key", func() {
		It("should stay on the new style even without a konflux API group", func() {
			obj := styledMap("", map[string]string{keys.Test("type").New: "component"}, nil)
			Expect(keys.StyleOf(obj)).To(Equal(keys.StyleNew))
		})
	})

	When("listing either style", func() {
		It("should return old-only and new-only objects", func() {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			oldCM := styledMap("", map[string]string{
				keys.SnapshotName().Old: "snap",
				keys.PipelineType().Old: "test",
			}, nil)
			oldCM.Name = "old"
			newCM := styledMap("", map[string]string{
				keys.SnapshotName().New: "snap",
				keys.PipelineType().New: "test",
			}, nil)
			newCM.Name = "new"
			other := styledMap("", map[string]string{keys.SnapshotName().Old: "other"}, nil)
			other.Name = "other"
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(oldCM, newCM, other).Build()

			list := &corev1.ConfigMapList{}
			err := keys.ListEither(context.Background(), cl, list, "default", []keys.Requirement{
				{Pair: keys.SnapshotName(), Operator: selection.In, Values: []string{"snap"}},
				{Pair: keys.PipelineType(), Operator: selection.In, Values: []string{"test"}},
			})
			Expect(err).NotTo(HaveOccurred())
			names := make([]string, 0, len(list.Items))
			for _, item := range list.Items {
				names = append(names, item.Name)
			}
			Expect(names).To(ConsistOf("old", "new"))
		})

		It("returns a conflicting object only for its new value", func() {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			conflict := styledMap("", map[string]string{
				keys.SnapshotName().Old: "old-snap",
				keys.SnapshotName().New: "new-snap",
			}, nil)
			conflict.Name = "conflict"
			oldOnly := styledMap("", map[string]string{
				keys.SnapshotName().Old: "old-snap",
			}, nil)
			oldOnly.Name = "old-only"
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(conflict, oldOnly).Build()

			oldList := &corev1.ConfigMapList{}
			err := keys.ListEither(context.Background(), cl, oldList, "default", []keys.Requirement{
				{Pair: keys.SnapshotName(), Operator: selection.In, Values: []string{"old-snap"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(oldList.Items).To(HaveLen(1))
			Expect(oldList.Items[0].Name).To(Equal("old-only"))

			newList := &corev1.ConfigMapList{}
			err = keys.ListEither(context.Background(), cl, newList, "default", []keys.Requirement{
				{Pair: keys.SnapshotName(), Operator: selection.In, Values: []string{"new-snap"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(newList.Items).To(HaveLen(1))
			Expect(newList.Items[0].Name).To(Equal("conflict"))
		})
	})

	When("removing a finalizer", func() {
		It("should recognize either finalizer name", func() {
			obj := styledMap("", nil, nil)
			obj.Finalizers = []string{keys.Test("pipelinerun").New}
			Expect(keys.HasFinalizer(obj, keys.Test("pipelinerun").Old)).To(BeTrue())
			name, err := keys.FinalizerToAdd(obj, keys.Test("pipelinerun").Old)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal(keys.Test("pipelinerun").New))
		})
	})
})

var _ = Describe("owned writers", func() {
	It("should not put both styles on one owned value", func() {
		obj := styledMap("konflux-ci.dev/v1alpha1", map[string]string{
			keys.Test("scenario").Old: "its",
		}, nil)
		Expect(keys.SetLabel(obj, keys.Test("scenario"), "its", keys.StyleOf(obj))).To(Succeed())
		Expect(keys.ApplyAnnotationWith(obj, keys.Test("pr-status").Old, "merged", keys.StyleOf(obj))).To(Succeed())
		Expect(obj.Labels).To(HaveKeyWithValue(keys.Test("scenario").New, "its"))
		Expect(obj.Labels).NotTo(HaveKey(keys.Test("scenario").Old))
		Expect(obj.Annotations).To(HaveKeyWithValue(keys.Test("pr-status").New, "merged"))
		Expect(obj.Annotations).NotTo(HaveKey(keys.Test("pr-status").Old))
	})
})
