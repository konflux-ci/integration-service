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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konflux-ci/integration-service/pkg/keys"
)

var _ = Describe("style dispatch and compatibility helpers", func() {
	It("detects old, new, and nil object styles", func() {
		Expect(keys.StyleOf(nil)).To(Equal(keys.StyleOld))
		Expect(keys.Same("custom.example/key")).To(Equal(keys.Pair{Old: "custom.example/key", New: "custom.example/key"}))
		Expect(keys.StyleOf(styledMap("appstudio.redhat.com/v1alpha1", nil, nil))).To(Equal(keys.StyleOld))
		Expect(keys.StyleOf(styledMap("konflux-ci.dev/v1alpha1", nil, nil))).To(Equal(keys.StyleNew))
		Expect(keys.StyleOf(styledMap("", map[string]string{keys.Test("marker").New: "true"}, nil))).To(Equal(keys.StyleNew))
		newFinalizer := styledMap("", nil, nil)
		newFinalizer.Finalizers = []string{keys.Test("component").New}
		Expect(keys.StyleOf(newFinalizer)).To(Equal(keys.StyleNew))
	})

	It("recognizes every owned key family and rejects malformed prefixes", func() {
		ownedKeys := []string{
			keys.AutoRelease().Old,
			keys.ComponentGroupName().Old,
			keys.SnapshotName().Old,
			keys.BuildPipelineRunName().Old,
			keys.BuildComponent().Old,
			keys.PipelineType().Old,
			keys.PAC("sha").Old,
			keys.PAC("sha").New,
			keys.Test("scenario").Old,
			keys.Test("scenario").New,
		}
		for _, key := range ownedKeys {
			_, ok := keys.PairForKey(key)
			Expect(ok).To(BeTrue(), key)
		}
		_, ok := keys.PairForKey(keys.PrefixTestOld + "/nested/key")
		Expect(ok).To(BeFalse())
		_, ok = keys.PairForKey("unowned.example/key")
		Expect(ok).To(BeFalse())
	})

	It("reads owned and raw labels and annotations with new-key precedence", func() {
		value, found := keys.ReadLabel(nil, keys.Test("name").Old)
		Expect(value).To(Equal(""), "nil value")
		Expect(found).To(BeFalse(), "nil found")

		obj := styledMap("", map[string]string{
			keys.Test("name").Old:  "old",
			keys.Test("name").New:  "new",
			"custom.example/label": "raw",
		}, map[string]string{
			keys.Test("status").Old:     "old-status",
			keys.Test("status").New:     "new-status",
			"custom.example/annotation": "raw-annotation",
		})
		value, found = keys.ReadLabel(obj, keys.Test("name").Old)
		Expect(found).To(BeTrue())
		Expect(value).To(Equal("new"))
		value, found = keys.ReadLabel(obj, "custom.example/label")
		Expect(found).To(BeTrue())
		Expect(value).To(Equal("raw"))
		value, found = keys.ReadAnnotation(obj, keys.Test("status").Old)
		Expect(found).To(BeTrue())
		Expect(value).To(Equal("new-status"))
		value, found = keys.ReadAnnotation(obj, "custom.example/annotation")
		Expect(found).To(BeTrue())
		Expect(value).To(Equal("raw-annotation"))
		value, found = keys.ReadAnnotation(nil, keys.Test("status").Old)
		Expect(value).To(Equal(""))
		Expect(found).To(BeFalse())
	})

	It("writes owned and raw annotations without leaving both styles", func() {
		obj := styledMap("konflux-ci.dev/v1alpha1", nil, nil)
		Expect(keys.ApplyAnnotation(obj, keys.Test("status").Old, "passed")).To(Succeed())
		Expect(obj.Annotations).To(HaveKeyWithValue(keys.Test("status").New, "passed"))
		Expect(obj.Annotations).NotTo(HaveKey(keys.Test("status").Old))
		Expect(keys.ApplyAnnotationWith(obj, keys.Test("other").Old, "old", keys.StyleOld)).To(Succeed())
		Expect(obj.Annotations).To(HaveKeyWithValue(keys.Test("other").Old, "old"))
		Expect(obj.Annotations).NotTo(HaveKey(keys.Test("other").New))
		Expect(keys.ApplyAnnotation(obj, "custom.example/raw", "value")).To(Succeed())
		Expect(obj.Annotations).To(HaveKeyWithValue("custom.example/raw", "value"))
		Expect(keys.ApplyAnnotationWith(obj, "custom.example/raw2", "value2", keys.StyleNew)).To(Succeed())
		Expect(obj.Annotations).To(HaveKeyWithValue("custom.example/raw2", "value2"))
	})

	It("reports invalid objects and styles without mutating them", func() {
		Expect(keys.ApplyAnnotation(nil, keys.Test("status").Old, "value")).To(MatchError("object cannot be nil"))
		Expect(keys.ApplyAnnotationWith(nil, keys.Test("status").Old, "value", keys.StyleOld)).To(MatchError("object cannot be nil"))
	})

	It("recognizes, resolves, and enumerates owned finalizers", func() {
		Expect(keys.HasFinalizer(nil, keys.Test("component").Old)).To(BeFalse())
		obj := styledMap("", nil, nil)
		obj.Finalizers = []string{keys.Test("component").New}
		Expect(keys.HasFinalizer(obj, keys.Test("component").Old)).To(BeTrue())
		Expect(keys.HasFinalizer(obj, "custom.example/finalizer")).To(BeFalse())
		Expect(keys.FinalizerNames(keys.Test("component").Old)).To(ConsistOf(keys.Test("component").Old, keys.Test("component").New))
		Expect(keys.FinalizerNames("custom.example/finalizer")).To(Equal([]string{"custom.example/finalizer"}))
		name, err := keys.FinalizerToAdd(obj, keys.Test("component").Old)
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal(keys.Test("component").New))
		name, err = keys.FinalizerToAdd(obj, "custom.example/finalizer")
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal("custom.example/finalizer"))
	})

	It("accepts metav1 objects with nil metadata maps", func() {
		obj := &metav1.PartialObjectMetadata{}
		Expect(keys.ApplyAnnotation(obj, "custom.example/raw", "value")).To(Succeed())
		Expect(obj.GetAnnotations()).To(HaveKeyWithValue("custom.example/raw", "value"))
	})
})
