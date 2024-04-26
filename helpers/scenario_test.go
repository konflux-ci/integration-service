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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/redhat-appstudio/integration-service/api/v1beta2"
	"github.com/redhat-appstudio/integration-service/helpers"
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

	AfterAll(func() {
		err := k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	Context("IntegrationTestscenario can be marked as valid and invalid", func() {
		It("ensures the Scenario status can be marked as invalid", func() {
			helpers.SetScenarioIntegrationStatusAsInvalid(integrationTestScenario, "Test message")
			Expect(integrationTestScenario).NotTo(BeNil())
			Expect(helpers.IsScenarioValid(integrationTestScenario)).To(BeFalse())
			Expect(meta.IsStatusConditionFalse(integrationTestScenario.Status.Conditions, helpers.IntegrationTestScenarioValid)).To(BeTrue())
		})

		It("ensures the Scenario status can be marked as valid", func() {
			helpers.SetScenarioIntegrationStatusAsValid(integrationTestScenario, "Test message")
			Expect(integrationTestScenario).NotTo(BeNil())
			Expect(helpers.IsScenarioValid(integrationTestScenario)).To(BeTrue())
			Expect(meta.IsStatusConditionTrue(integrationTestScenario.Status.Conditions, helpers.IntegrationTestScenarioValid)).To(BeTrue())
		})
	})
})
