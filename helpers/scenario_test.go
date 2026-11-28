/*
Copyright 2023 Red Hat Inc.

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/pkg/keys"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
)

var _ = Describe("Gitops functions for managing Snapshots", Ordered, func() {

	var (
		integrationTestScenario *v1beta2.IntegrationTestScenario
	)

	BeforeAll(func() {
		integrationTestScenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "application-sample",
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "git",
					Params: []v1beta2.ResolverParameter{
						{
							Name:  "url",
							Value: "https://github.com/redhat-appstudio/integration-examples.git",
						},
						{
							Name:  "revision",
							Value: "main",
						},
						{
							Name:  "pathInRepo",
							Value: "pipelineruns/integration_pipelinerun_pass.yaml",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())
	})

	It("recognizes the new-style optional label", func() {
		scenario := &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					keys.Test("optional").New: "true",
				},
			},
		}
		Expect(helpers.IsIntegrationTestScenarioOptional(scenario)).To(BeTrue())
	})

	It("recognizes the component service account annotation in either key style", func() {
		Expect(keys.Test("component-service-account").Old).To(Equal(tektonconsts.ComponentServiceAccountAnnotation))
		Expect(helpers.ScenarioUsesComponentServiceAccount(nil)).To(BeFalse())
		Expect(helpers.ScenarioUsesComponentServiceAccount(integrationTestScenario)).To(BeFalse())

		for _, key := range []string{keys.Test("component-service-account").Old, keys.Test("component-service-account").New} {
			enabled := &v1beta2.IntegrationTestScenario{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{key: "true"}},
			}
			Expect(helpers.ScenarioUsesComponentServiceAccount(enabled)).To(BeTrue(), key)

			disabled := &v1beta2.IntegrationTestScenario{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{key: "false"}},
			}
			Expect(helpers.ScenarioUsesComponentServiceAccount(disabled)).To(BeFalse(), key)
		}

		// The new key wins when both are set.
		both := &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
				keys.Test("component-service-account").Old: "true",
				keys.Test("component-service-account").New: "false",
			}},
		}
		Expect(helpers.ScenarioUsesComponentServiceAccount(both)).To(BeFalse())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})
})
