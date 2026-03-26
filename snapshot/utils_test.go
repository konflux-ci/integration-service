package snapshot

import (
	"fmt"
	"unicode"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	name    = "component"
	version = "v1"
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

	Context("Validating component version strings", func() {
		It("Can generate a valid component version string", func() {
			expectedResult := fmt.Sprintf("%s/%s", name, version)
			result := getComponentVersionString(name, version)
			Expect(result).To(Equal(expectedResult))
		})

		It("Can generate a valid component version log string", func() {
			expectedResult := fmt.Sprintf("%s (version %s)", name, version)
			result := getComponentVersionLogString(name, version)
			Expect(result).To(Equal(expectedResult))
		})
	})
})
