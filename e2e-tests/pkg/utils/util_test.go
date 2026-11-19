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

package utils_test

import (
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
)

var _ = Describe("GeneratePipelineRunName", func() {

	It("should generate a name with the prefix", func() {
		prefix := "test-plr"
		name := utils.GeneratePipelineRunName(prefix)
		Expect(name).To(HavePrefix(prefix))
	})

	It("should include a hyphen between prefix and random suffix", func() {
		prefix := "myprefix"
		name := utils.GeneratePipelineRunName(prefix)
		Expect(name).To(HavePrefix(prefix + "-"))
	})

	It("should generate a 6-character random suffix", func() {
		prefix := "test"
		name := utils.GeneratePipelineRunName(prefix)
		// Format should be "prefix-XXXXXX" where X is 6 random chars
		suffix := strings.TrimPrefix(name, prefix+"-")
		Expect(suffix).To(HaveLen(6))
	})

	It("should generate unique names on consecutive calls", func() {
		prefix := "test-plr"
		names := make(map[string]bool)
		for i := 0; i < 100; i++ {
			name := utils.GeneratePipelineRunName(prefix)
			Expect(names[name]).To(BeFalse(), "Generated duplicate name: %s", name)
			names[name] = true
		}
	})

	It("should only use lowercase alphanumeric characters in random suffix", func() {
		prefix := "test"
		for i := 0; i < 50; i++ {
			name := utils.GeneratePipelineRunName(prefix)
			suffix := strings.TrimPrefix(name, prefix+"-")
			// Verify suffix matches pattern of lowercase letters and digits
			suffixRegex := regexp.MustCompile("^[a-z0-9]{6}$")
			Expect(suffixRegex.MatchString(suffix)).To(BeTrue(), "Suffix %s does not match expected pattern", suffix)
		}
	})

	It("should handle empty prefix", func() {
		name := utils.GeneratePipelineRunName("")
		// Should start with a hyphen since format is "prefix-random"
		Expect(name).To(HavePrefix("-"))
		// Should have 7 characters total: "-" + 6 random chars
		Expect(name).To(HaveLen(7))
	})

	It("should handle long prefix", func() {
		prefix := "very-long-prefix-for-pipeline-run-name-test"
		name := utils.GeneratePipelineRunName(prefix)
		Expect(name).To(HavePrefix(prefix + "-"))
		suffix := strings.TrimPrefix(name, prefix+"-")
		Expect(suffix).To(HaveLen(6))
	})

	It("should handle prefix with numbers", func() {
		prefix := "test123"
		name := utils.GeneratePipelineRunName(prefix)
		Expect(name).To(HavePrefix(prefix + "-"))
	})

	It("should handle prefix with hyphens", func() {
		prefix := "test-plr-name"
		name := utils.GeneratePipelineRunName(prefix)
		Expect(name).To(HavePrefix(prefix + "-"))
		suffix := strings.TrimPrefix(name, prefix+"-")
		Expect(suffix).To(HaveLen(6))
	})

	DescribeTable("should generate valid names for various prefixes",
		func(prefix string) {
			name := utils.GeneratePipelineRunName(prefix)
			parts := strings.Split(name, "-")
			// Should have at least 2 parts (prefix and random suffix)
			Expect(len(parts)).To(BeNumerically(">=", 2))
			// Last part should be the 6-character random suffix
			Expect(parts[len(parts)-1]).To(HaveLen(6))
		},
		Entry("simple prefix", "plr"),
		Entry("hyphenated prefix", "test-plr"),
		Entry("numeric prefix", "test123"),
		Entry("long prefix", "very-long-prefix-name"),
	)

	It("should be suitable for use as Kubernetes resource name", func() {
		prefix := "prebuilt-plr"
		name := utils.GeneratePipelineRunName(prefix)
		// Kubernetes names must be lowercase, alphanumeric, and hyphens
		matched, err := regexp.MatchString("^[a-z0-9-]+$", name)
		Expect(err).NotTo(HaveOccurred())
		Expect(matched).To(BeTrue(), "Generated name %s is not valid for Kubernetes resources", name)
	})

	It("should generate names within reasonable length for Kubernetes", func() {
		prefix := "test"
		name := utils.GeneratePipelineRunName(prefix)
		// Kubernetes names have max 253 characters, our generated names should be well under
		Expect(len(name)).To(BeNumerically("<", 100))
	})

	It("should consistently format names as prefix-suffix", func() {
		prefix := "myplr"
		name := utils.GeneratePipelineRunName(prefix)
		// Count hyphens - should be at least 1 (between prefix and suffix)
		// More if prefix contains hyphens
		hyphenCount := strings.Count(name, "-")
		minExpected := strings.Count(prefix, "-") + 1
		Expect(hyphenCount).To(Equal(minExpected))
	})

})
