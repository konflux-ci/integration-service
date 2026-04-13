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
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ComponentGroup helpers", func() {
	It("returns empty names for nil component groups", func() {
		names := helpers.GetComponentGroupNames(nil)
		Expect(names).To(BeEmpty())
	})

	It("returns names for non-empty component groups", func() {
		componentGroups := []v1beta2.ComponentGroup{
			{ObjectMeta: metav1.ObjectMeta{Name: "group-a"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "group-b"}},
		}

		names := helpers.GetComponentGroupNames(&componentGroups)
		Expect(names).To(ConsistOf("group-a", "group-b"))
	})
})
