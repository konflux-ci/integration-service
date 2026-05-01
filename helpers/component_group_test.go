/*
Copyright 2022 Red Hat Inc.

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

package helpers

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	name    = "component"
	version = "v1"
)

var _ = Describe("Utility functions", Ordered, func() {
	Context("Validating component version strings", func() {
		It("Can generate a valid component version string", func() {
			expectedResult := fmt.Sprintf("%s/%s", name, version)
			result := GetComponentVersionString(name, version)
			Expect(result).To(Equal(expectedResult))
		})

		It("Can generate a valid component version log string", func() {
			expectedResult := fmt.Sprintf("%s (version %s)", name, version)
			result := GetComponentVersionLogString(name, version)
			Expect(result).To(Equal(expectedResult))
		})
	})
})
