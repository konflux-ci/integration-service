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
	"github.com/konflux-ci/integration-service/helpers"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Helpers for the per-component integration pipeline ServiceAccount", func() {

	Context("ComponentIntegrationPipelineServiceAccountName", func() {
		It("prefixes the Component name with its-pipeline-", func() {
			Expect(helpers.ComponentIntegrationPipelineServiceAccountName("my-component")).To(Equal("its-pipeline-my-component"))
		})

		It("uses the shared prefix constant", func() {
			Expect(helpers.ComponentIntegrationPipelineServiceAccountName("x")).To(HavePrefix(tektonconsts.ComponentIntegrationPipelineServiceAccountPrefix))
		})
	})
})
