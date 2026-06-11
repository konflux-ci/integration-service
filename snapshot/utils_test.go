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

package snapshot

import (
	"unicode"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Utility functions", Ordered, func() {
	Context("Validating generateRandomSuffix", func() {
		It("Can generate a suffix", func() {
			suffix, err := generateRandomSuffix()
			Expect(err).NotTo(HaveOccurred())
			Expect(suffix).To(HaveLen(2))
			Expect(unicode.IsDigit(rune(suffix[0])) || unicode.IsLower(rune(suffix[0]))).To(BeTrue())
			Expect(unicode.IsDigit(rune(suffix[1])) || unicode.IsLower(rune(suffix[1]))).To(BeTrue())
		})
	})
})
