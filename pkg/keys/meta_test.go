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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konflux-ci/integration-service/pkg/keys"
)

func configMap(labels, annotations map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "sample",
			Namespace:   "default",
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

var _ = Describe("read either", func() {
	When("getting a label", func() {
		DescribeTable("should prefer new and fall back to old",
			func(labels map[string]string, wantValue string, wantFound bool) {
				value, found := keys.GetLabel(configMap(labels, nil), keys.PipelineType)
				Expect(found).To(Equal(wantFound))
				Expect(value).To(Equal(wantValue))
			},
			Entry("old only", map[string]string{keys.PipelineType.Old: "build"}, "build", true),
			Entry("new only", map[string]string{keys.PipelineType.New: "build"}, "build", true),
			Entry("both present prefers new", map[string]string{
				keys.PipelineType.Old: "test",
				keys.PipelineType.New: "build",
			}, "build", true),
			Entry("neither", map[string]string{"unrelated": "x"}, "", false),
			Entry("nil labels", nil, "", false),
		)
	})

	When("getting an annotation", func() {
		It("should prefer new and fall back to old", func() {
			pair := keys.Pair{Old: "old.example/ann", New: "new.example/ann"}
			obj := configMap(nil, map[string]string{
				pair.Old: "old-value",
				pair.New: "new-value",
			})
			value, found := keys.GetAnnotation(obj, pair)
			Expect(found).To(BeTrue())
			Expect(value).To(Equal("new-value"))
		})
	})

	When("checking a label value", func() {
		DescribeTable("should match via HasLabelValue for old or new keys",
			func(labels map[string]string, value string, want bool) {
				Expect(keys.HasLabelValue(configMap(labels, nil), keys.PipelineType, value)).To(Equal(want))
			},
			Entry("old build", map[string]string{keys.PipelineType.Old: "build"}, "build", true),
			Entry("new build", map[string]string{keys.PipelineType.New: "build"}, "build", true),
			Entry("wrong value", map[string]string{keys.PipelineType.Old: "test"}, "build", false),
			Entry("missing", nil, "build", false),
		)
	})

	When("detecting new-model keys", func() {
		DescribeTable("should report HasNew only when a new key is present",
			func(labels map[string]string, want bool) {
				Expect(keys.HasNew(configMap(labels, nil), keys.PipelineType, keys.BuildComponent)).To(Equal(want))
			},
			Entry("old type only", map[string]string{keys.PipelineType.Old: "build"}, false),
			Entry("old component only, no application (ComponentGroup path)", map[string]string{
				keys.PipelineType.Old:   "build",
				keys.BuildComponent.Old: "component-sample",
			}, false),
			Entry("new type", map[string]string{keys.PipelineType.New: "build"}, true),
			Entry("new component", map[string]string{keys.BuildComponent.New: "component-sample"}, true),
		)

		It("should see a new-style annotation", func() {
			pair := keys.Pair{Old: "old.example/ann", New: "new.example/ann"}
			obj := configMap(nil, map[string]string{pair.New: "x"})
			Expect(keys.HasNew(obj, pair)).To(BeTrue())
		})
	})
})

var _ = Describe("write either", func() {
	When("setting a label with StyleOld", func() {
		It("should write only the old key", func() {
			obj := configMap(nil, nil)
			Expect(keys.SetLabel(obj, keys.PipelineType, "build", keys.StyleOld)).To(Succeed())
			Expect(obj.Labels).To(HaveKeyWithValue(keys.PipelineType.Old, "build"))
			Expect(obj.Labels).NotTo(HaveKey(keys.PipelineType.New))
		})
	})

	When("setting a label with StyleNew", func() {
		It("should write only the new key", func() {
			obj := configMap(nil, nil)
			Expect(keys.SetLabel(obj, keys.PipelineType, "build", keys.StyleNew)).To(Succeed())
			Expect(obj.Labels).To(HaveKeyWithValue(keys.PipelineType.New, "build"))
			Expect(obj.Labels).NotTo(HaveKey(keys.PipelineType.Old))
		})

		It("should clear a previously set old key", func() {
			obj := configMap(map[string]string{keys.PipelineType.Old: "build"}, nil)
			Expect(keys.SetLabel(obj, keys.PipelineType, "build", keys.StyleNew)).To(Succeed())
			Expect(obj.Labels).To(HaveKeyWithValue(keys.PipelineType.New, "build"))
			Expect(obj.Labels).NotTo(HaveKey(keys.PipelineType.Old))
		})

		It("should error when the pair has no new key", func() {
			pair := keys.Pair{Old: "appstudio.openshift.io/application"}
			obj := configMap(nil, nil)
			err := keys.SetLabel(obj, pair, "app", keys.StyleNew)
			Expect(err).To(HaveOccurred())
			Expect(obj.Labels).To(BeEmpty())
		})
	})

	When("setting an annotation with StyleOld", func() {
		It("should write only the old key", func() {
			pair := keys.Pair{Old: "old.example/ann", New: "new.example/ann"}
			obj := configMap(nil, nil)
			Expect(keys.SetAnnotation(obj, pair, "v", keys.StyleOld)).To(Succeed())
			Expect(obj.Annotations).To(HaveKeyWithValue(pair.Old, "v"))
			Expect(obj.Annotations).NotTo(HaveKey(pair.New))
		})
	})

	When("setting with an invalid style", func() {
		It("should not mutate labels for an out-of-range style", func() {
			obj := configMap(map[string]string{"keep": "me"}, nil)
			err := keys.SetLabel(obj, keys.PipelineType, "build", keys.Style(99))
			Expect(err).To(MatchError(ContainSubstring("invalid style")))
			Expect(obj.Labels).To(Equal(map[string]string{"keep": "me"}))
		})

		It("should not mutate annotations for a negative style", func() {
			pair := keys.Pair{Old: "old.example/ann", New: "new.example/ann"}
			obj := configMap(nil, map[string]string{"keep": "me"})
			err := keys.SetAnnotation(obj, pair, "v", keys.Style(-1))
			Expect(err).To(MatchError(ContainSubstring("invalid style")))
			Expect(obj.Annotations).To(Equal(map[string]string{"keep": "me"}))
		})
	})
})
