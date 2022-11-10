/*
Copyright 2022.

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

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("IntegrationTestScenario type", func() {

	var integrationTestScenario *IntegrationTestScenario

	BeforeEach(func() {
		integrationTestScenario = &IntegrationTestScenario{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "IntegrationTestScenario",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "integrationtestscenario",
				Namespace: "default",
			},
			Spec: IntegrationTestScenarioSpec{
				Application: "application-sample",
				Pipeline:    "component-pipeline-pass",
				Bundle:      "quay.io/kpavic/test-bundle:build-pipeline-pass",
				Params: []PipelineParameter{
					{
						Name:  "pipeline-param-name",
						Value: "pipeline-param-value",
					},
				},
				Environment: TestEnvironment{
					Name: "environment-name",
					Type: "POC",
					Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{
							{
								Name:  "env-config-name",
								Value: "env-config-value",
							},
						},
					},
				},
				Contexts: []TestContext{
					{
						Name:        "test-ctx",
						Description: "test-ctx-description",
					},
				},
			},
		}
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("should be able to create integrationTestScenario", func() {
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      integrationTestScenario.Name,
				Namespace: integrationTestScenario.Namespace,
			}, integrationTestScenario)
			return err == nil
		}, timeout).Should(BeTrue())
		Expect(integrationTestScenario.Spec.Application).To(Equal("application-sample"))
	})
})
