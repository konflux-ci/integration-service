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
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	name    = "component"
	version = "v1"
)

var _ = Describe("Helpers for component groups", func() {

	Context("Validating component version strings", func() {
		It("Can generate a valid component version string", func() {
			expectedResult := fmt.Sprintf("%s/%s", name, version)
			result := helpers.GetComponentVersionString(name, version)
			Expect(result).To(Equal(expectedResult))
		})

		It("Can generate a valid component version log string", func() {
			expectedResult := fmt.Sprintf("%s (version %s)", name, version)
			result := helpers.GetComponentVersionLogString(name, version)
			Expect(result).To(Equal(expectedResult))
		})
	})

	Context("GetComponentGroupNames", func() {

		It("returns an empty slice when componentGroups is nil", func() {
			Expect(helpers.GetComponentGroupNames(nil)).To(BeEmpty())
		})

		It("returns an empty slice when there are no component groups", func() {
			groups := []v1beta2.ComponentGroup{}
			Expect(helpers.GetComponentGroupNames(&groups)).To(BeEmpty())
		})

		It("returns the name of each component group in order", func() {
			groups := []v1beta2.ComponentGroup{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "frontend"},
					Spec:       v1beta2.ComponentGroupSpec{},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "backend"},
					Spec:       v1beta2.ComponentGroupSpec{},
				},
			}
			Expect(helpers.GetComponentGroupNames(&groups)).To(Equal([]string{"frontend", "backend"}))
		})
	})

	Context("GetComponentNamesFromComponentGroup", func() {

		It("returns an empty slice when the component group is nil", func() {
			Expect(helpers.GetComponentNamesFromComponentGroup(nil)).To(BeEmpty())
		})

		It("returns an empty slice when Spec.Components is nil", func() {
			group := &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "my-group"},
				Spec:       v1beta2.ComponentGroupSpec{},
			}
			Expect(helpers.GetComponentNamesFromComponentGroup(group)).To(BeEmpty())
		})

		It("returns an empty slice when Spec.Components is empty", func() {
			group := &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "my-group"},
				Spec:       v1beta2.ComponentGroupSpec{Components: []v1beta2.ComponentReference{}},
			}
			Expect(helpers.GetComponentNamesFromComponentGroup(group)).To(BeEmpty())
		})

		It("returns each component Name from Spec.Components in order", func() {
			group := &v1beta2.ComponentGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "my-group"},
				Spec: v1beta2.ComponentGroupSpec{
					Components: []v1beta2.ComponentReference{
						{Name: "api-service", ComponentVersion: v1beta2.ComponentVersionReference{Name: "main"}},
						{Name: "worker", ComponentVersion: v1beta2.ComponentVersionReference{Name: "release-1"}},
					},
				},
			}
			Expect(helpers.GetComponentNamesFromComponentGroup(group)).To(Equal([]string{"api-service", "worker"}))
		})
	})
})
