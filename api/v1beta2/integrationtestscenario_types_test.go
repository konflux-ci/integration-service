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

package v1beta2

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("IntegrationTestScenario type", func() {

	var integrationTestScenario *IntegrationTestScenario

	BeforeEach(func() {
		integrationTestScenario = &IntegrationTestScenario{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1beta1",
				Kind:       "IntegrationTestScenario",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "integrationtestscenario",
				Namespace: "default",
			},
			Spec: IntegrationTestScenarioSpec{
				Application: "application-sample",
				Params: []PipelineParameter{
					{
						Name:  "pipeline-param-name",
						Value: "pipeline-param-value",
					},
				},
				Contexts: []TestContext{
					{
						Name:        "test-ctx",
						Description: "test-ctx-description",
					},
				},
				ResolverRef: ResolverRef{
					Resolver: "git",
					Params: []ResolverParameter{
						{
							Name:  "url",
							Value: "https://url",
						},
						{
							Name:  "revision",
							Value: "main",
						},
						{
							Name:  "pathInRepo",
							Value: "pipeline/helloworld.yaml",
						},
					},
				},
			},
		}
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})
})
