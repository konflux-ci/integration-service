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
	"fmt"
	"time"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	konfluxv1beta2 "github.com/konflux-ci/integration-service/api/konflux/v1beta2"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	types "k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("IntegrationTestScenario webhook", Ordered, func() {

	var (
		integrationTestScenario                   *v1beta2.IntegrationTestScenario
		integrationTestScenarioInvalidGitResolver *v1beta2.IntegrationTestScenario
		hasApp                                    *applicationapiv1alpha1.Application
		hasComponentGroup                         *konfluxv1beta2.ComponentGroup
	)

	BeforeAll(func() {
		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "application-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasComponentGroup = &konfluxv1beta2.ComponentGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "componentgroup-sample",
				Namespace: "default",
			},
			Spec: konfluxv1beta2.ComponentGroupSpec{
				Components: []konfluxv1beta2.ComponentReference{
					{
						Name: "component-a",
						ComponentVersion: konfluxv1beta2.ComponentVersionReference{
							Name: "main",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComponentGroup)).Should(Succeed())
	})

	BeforeEach(func() {
		integrationTestScenario = &v1beta2.IntegrationTestScenario{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1beta2",
				Kind:       "IntegrationTestScenario",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "integrationtestscenario",
				Namespace: "default",
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "application-sample",
				Params: []v1beta2.PipelineParameter{
					{
						Name:  "pipeline-param-name",
						Value: "pipeline-param-value",
					},
				},
				Contexts: []v1beta2.TestContext{
					{
						Name:        "test-ctx",
						Description: "test-ctx-description",
					},
				},
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "git",
					Params: []v1beta2.ResolverParameter{
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

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasComponentGroup)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("should fail to create scenario with long name", func() {
		integrationTestScenario.Name = "this-name-is-too-long-it-has-64-characters-and-we-allow-max-63ch"
		Expect(k8sClient.Create(ctx, integrationTestScenario)).ShouldNot(Succeed())
	})

	It("should fail to create scenario with snapshot parameter set", func() {
		integrationTestScenario.Spec.Params = append(integrationTestScenario.Spec.Params, v1beta2.PipelineParameter{Name: "SNAPSHOT"})
		Expect(k8sClient.Create(ctx, integrationTestScenario)).ShouldNot(Succeed())
	})

	It("should success to create scenario when only url in git resolver params", func() {
		integrationTestScenarioInvalidGitResolver = &v1beta2.IntegrationTestScenario{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1beta2",
				Kind:       "IntegrationTestScenario",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "integrationtestscenario",
				Namespace: "default",
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "application-sample",
				Params: []v1beta2.PipelineParameter{
					{
						Name:  "pipeline-param-name",
						Value: "pipeline-param-value",
					},
				},
				Contexts: []v1beta2.TestContext{
					{
						Name:        "test-ctx",
						Description: "test-ctx-description",
					},
				},
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "git",
					Params: []v1beta2.ResolverParameter{
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

		Expect(k8sClient.Create(ctx, integrationTestScenarioInvalidGitResolver)).Should(Succeed())
	})

	It("should set the value of 'Spec.ResolverRef.ResourceKind' to 'pipeline' if not already set", func() {
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())
		Eventually(func() error {
			appliedScenario := &v1beta2.IntegrationTestScenario{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "integrationtestscenario",
				Namespace: "default",
			}, appliedScenario)
			if err != nil {
				return err
			}
			rType := appliedScenario.Spec.ResolverRef.ResourceKind
			if rType != "pipeline" {
				return fmt.Errorf("ResourceKind should be 'pipeline', is '%s'", rType)
			}
			return nil
		}, time.Second*10).Should(Succeed())
	})

	It("should not set the value of 'Spec.ResolverRef.ResourceKind' if it is already set", func() {
		integrationTestScenario.Spec.ResolverRef.ResourceKind = "pipelinerun"
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())
		Eventually(func() error {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "integrationtestscenario",
				Namespace: "default",
			}, integrationTestScenario)
			if err != nil {
				return err
			}
			rType := integrationTestScenario.Spec.ResolverRef.ResourceKind
			if rType != "pipelinerun" {
				return fmt.Errorf("ResourceKind should be 'pipelinerun', is '%s'", rType)
			}
			return nil
		}, time.Second*10).Should(Succeed())
	})

	It("should succeed to create scenario with bundle resolver and pipeline params", func() {
		bundleScenario := &v1beta2.IntegrationTestScenario{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1beta2",
				Kind:       "IntegrationTestScenario",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bundle-scenario",
				Namespace: "default",
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "application-sample",
				Params: []v1beta2.PipelineParameter{
					{
						Name:  "POLICY_CONFIGURATION",
						Value: "some-policy",
					},
				},
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "bundles",
					Params: []v1beta2.ResolverParameter{
						{
							Name:  "bundle",
							Value: "quay.io/redhat-appstudio/example-tekton-bundle:integration-pipeline-pass",
						},
						{
							Name:  "name",
							Value: "integration-pipeline-pass",
						},
						{
							Name:  "kind",
							Value: "pipeline",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bundleScenario)).Should(Succeed())
		err := k8sClient.Delete(ctx, bundleScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("should succeed to create scenario with bundle resolver and no pipeline params", func() {
		bundleScenario := &v1beta2.IntegrationTestScenario{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1beta2",
				Kind:       "IntegrationTestScenario",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bundle-scenario-no-params",
				Namespace: "default",
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "application-sample",
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "bundles",
					Params: []v1beta2.ResolverParameter{
						{
							Name:  "bundle",
							Value: "quay.io/redhat-appstudio/example-tekton-bundle:integration-pipeline-pass",
						},
						{
							Name:  "name",
							Value: "integration-pipeline-pass",
						},
						{
							Name:  "kind",
							Value: "pipeline",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, bundleScenario)).Should(Succeed())
		err := k8sClient.Delete(ctx, bundleScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	It("should fail to create scenario when in git resolver params url+repo", func() {

		integrationTestScenarioInvalidGitResolver.Spec.ResolverRef.Params = append(integrationTestScenarioInvalidGitResolver.Spec.ResolverRef.Params, v1beta2.ResolverParameter{Name: "repo", Value: "my-repository-name"})
		integrationTestScenarioInvalidGitResolver.Spec.ResolverRef.Params = append(integrationTestScenarioInvalidGitResolver.Spec.ResolverRef.Params, v1beta2.ResolverParameter{Name: "org", Value: "my-org-name"})
		Expect(k8sClient.Create(ctx, integrationTestScenarioInvalidGitResolver)).ShouldNot(Succeed())
	})

	It("should return nil when string contains neither leading nor trailing whitespace", func() {
		testString := "this is a test string"
		err := validateNoWhitespace("testString", testString)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return an error when string contains leading whitespace", func() {
		testString := " this is a test string"
		err := validateNoWhitespace("testString", testString)
		Expect(err).To(HaveOccurred())
	})

	It("should return an error when string contains trailing whitespace", func() {
		testString := "this is a test string "
		err := validateNoWhitespace("testString", testString)
		Expect(err).To(HaveOccurred())
	})

	It("should return nil when provided with a valid url", func() {
		url := "https://github.com/konflux-ci/integration-examples"
		err := validateUrl("serverURL", url)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return an error when provided with an invalid url", func() {
		url := "konflux-ci/integration-examples"
		err := validateUrl("serverURL", url)
		Expect(err).To(HaveOccurred())
	})

	It("should return an error when provided with a url without 'https://'", func() {
		url := "http://github.com/konflux-ci/integration-examples"
		err := validateUrl("serverURL", url)
		Expect(err).To(HaveOccurred())
	})

	It("should return an error when url contains whitespace", func() {
		url := " https://github.com/konflux-ci/integration-examples "
		err := validateUrl("serverURL", url)
		Expect(err).To(HaveOccurred())
	})

	It("should return nil when token is a valid k8s secret name", func() {
		token := "my-secret"
		err := validateToken(token)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return nil when token is a valid k8s secret name", func() {
		token := "My_Secret"
		err := validateToken(token)
		Expect(err).To(HaveOccurred())
	})

	It("should fail validation when both application and componentGroup are specified", func() {
		scenario := &v1beta2.IntegrationTestScenario{
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application:    "test-app",
				ComponentGroup: "test-cg",
			},
		}
		err := validateOwnerField(scenario)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one of 'application' or 'componentGroup' must be specified, not both"))
	})

	It("should fail validation when neither application nor componentGroup is specified", func() {
		scenario := &v1beta2.IntegrationTestScenario{
			Spec: v1beta2.IntegrationTestScenarioSpec{},
		}
		err := validateOwnerField(scenario)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("exactly one of 'application' or 'componentGroup' must be specified"))
	})

	It("should pass validation when only application is specified", func() {
		scenario := &v1beta2.IntegrationTestScenario{
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "test-app",
			},
		}
		err := validateOwnerField(scenario)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should pass validation when only componentGroup is specified", func() {
		scenario := &v1beta2.IntegrationTestScenario{
			Spec: v1beta2.IntegrationTestScenarioSpec{
				ComponentGroup: "test-cg",
			},
		}
		err := validateOwnerField(scenario)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("validation helpers", func() {
		It("should validate IntegrationTestScenario names", func() {
			scenario := validIntegrationTestScenarioForValidation()
			Expect(validateScenarioName(scenario)).To(Succeed())

			scenario.Name = "Invalid_Name"
			Expect(validateScenarioName(scenario)).NotTo(Succeed())
		})

		It("should reject user supplied SNAPSHOT params", func() {
			scenario := validIntegrationTestScenarioForValidation()
			Expect(validateNoUserSuppliedSnapshotParam(scenario)).To(Succeed())

			scenario.Spec.Params = append(scenario.Spec.Params, v1beta2.PipelineParameter{Name: "SNAPSHOT"})
			Expect(validateNoUserSuppliedSnapshotParam(scenario)).NotTo(Succeed())
		})

		It("should validate git resolver url params", func() {
			scenario := validIntegrationTestScenarioForValidation()
			Expect(validateGitResolverParams(scenario)).To(Succeed())

			scenario.Spec.ResolverRef.Params = []v1beta2.ResolverParameter{{Name: "url", Value: "http://github.com/konflux-ci/integration-service"}}
			Expect(validateGitResolverParams(scenario)).NotTo(Succeed())
		})

		It("should validate git resolver repo/org params", func() {
			scenario := validIntegrationTestScenarioForValidation()
			scenario.Spec.ResolverRef.Params = []v1beta2.ResolverParameter{
				{Name: "repo", Value: "integration-service"},
				{Name: "org", Value: "konflux-ci"},
				{Name: "serverURL", Value: "https://github.com"},
				{Name: "token", Value: "github-token"},
			}
			Expect(validateGitResolverParams(scenario)).To(Succeed())

			scenario.Spec.ResolverRef.Params = []v1beta2.ResolverParameter{{Name: "repo", Value: "integration-service"}}
			Expect(validateGitResolverParams(scenario)).NotTo(Succeed())
		})

		It("should reject conflicting git resolver params", func() {
			scenario := validIntegrationTestScenarioForValidation()
			scenario.Spec.ResolverRef.Params = append(scenario.Spec.ResolverRef.Params,
				v1beta2.ResolverParameter{Name: "repo", Value: "integration-service"},
				v1beta2.ResolverParameter{Name: "org", Value: "konflux-ci"},
			)

			Expect(validateGitResolverParams(scenario)).NotTo(Succeed())
		})

		It("should reject invalid git resolver serverURL and token params", func() {
			scenario := validIntegrationTestScenarioForValidation()
			scenario.Spec.ResolverRef.Params = []v1beta2.ResolverParameter{
				{Name: "repo", Value: "integration-service"},
				{Name: "org", Value: "konflux-ci"},
				{Name: "serverURL", Value: "http://github.com"},
			}
			Expect(validateGitResolverParams(scenario)).NotTo(Succeed())

			scenario.Spec.ResolverRef.Params = append(scenario.Spec.ResolverRef.Params[:2], v1beta2.ResolverParameter{Name: "token", Value: "Invalid_Token"})
			Expect(validateGitResolverParams(scenario)).NotTo(Succeed())
		})

		It("should reject leading or trailing whitespace in git resolver params", func() {
			scenario := validIntegrationTestScenarioForValidation()
			scenario.Spec.ResolverRef.Params = append(scenario.Spec.ResolverRef.Params,
				v1beta2.ResolverParameter{Name: "revision", Value: " main"},
			)

			Expect(validateGitResolverParams(scenario)).NotTo(Succeed())
			Expect(validateUrl("url", " https://github.com/konflux-ci/integration-service ")).NotTo(Succeed())
		})

		It("should skip git resolver validation for non-git resolvers", func() {
			scenario := validIntegrationTestScenarioForValidation()
			scenario.Spec.ResolverRef.Resolver = "bundles"
			scenario.Spec.ResolverRef.Params = []v1beta2.ResolverParameter{{Name: "url", Value: "http://example.com"}}

			Expect(validateGitResolverParams(scenario)).To(Succeed())
		})

		It("should call validation helpers from ValidateCreate", func() {
			validator := &IntegrationTestScenarioCustomValidator{}
			scenario := validIntegrationTestScenarioForValidation()

			warnings, err := validator.ValidateCreate(ctx, scenario)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())

			scenario = validIntegrationTestScenarioForValidation()
			scenario.Spec.Params = append(scenario.Spec.Params, v1beta2.PipelineParameter{Name: "SNAPSHOT"})
			_, err = validator.ValidateCreate(ctx, scenario)
			Expect(err).To(HaveOccurred())

			scenario = validIntegrationTestScenarioForValidation()
			scenario.OwnerReferences = nil
			_, err = validator.ValidateCreate(ctx, scenario)
			Expect(err).To(HaveOccurred())
		})

		It("should call validation helpers from ValidateUpdate", func() {
			validator := &IntegrationTestScenarioCustomValidator{}
			oldScenario := validIntegrationTestScenarioForValidation()
			newScenario := validIntegrationTestScenarioForValidation()

			warnings, err := validator.ValidateUpdate(ctx, oldScenario, newScenario)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())

			newScenario = validIntegrationTestScenarioForValidation()
			newScenario.Spec.Params = append(newScenario.Spec.Params, v1beta2.PipelineParameter{Name: "SNAPSHOT"})
			_, err = validator.ValidateUpdate(ctx, oldScenario, newScenario)
			Expect(err).To(HaveOccurred())

			_, err = validator.ValidateUpdate(ctx, oldScenario, &konfluxv1beta2.ComponentGroup{})
			Expect(err).To(HaveOccurred())
		})
	})

	It("should create scenario with componentGroup successfully", func() {
		cgScenario := &v1beta2.IntegrationTestScenario{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1beta2",
				Kind:       "IntegrationTestScenario",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "integrationtestscenario-cg",
				Namespace: "default",
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				ComponentGroup: "componentgroup-sample",
				ResolverRef: v1beta2.ResolverRef{
					Resolver: "git",
					Params: []v1beta2.ResolverParameter{
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
		Expect(k8sClient.Create(ctx, cgScenario)).Should(Succeed())

		// Verify owner reference was set to ComponentGroup
		Eventually(func() error {
			appliedScenario := &v1beta2.IntegrationTestScenario{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "integrationtestscenario-cg",
				Namespace: "default",
			}, appliedScenario)
			if err != nil {
				return err
			}
			if len(appliedScenario.OwnerReferences) == 0 {
				return fmt.Errorf("expected owner reference to be set")
			}
			if appliedScenario.OwnerReferences[0].Kind != "ComponentGroup" {
				return fmt.Errorf("expected owner kind to be ComponentGroup, got %s", appliedScenario.OwnerReferences[0].Kind)
			}
			return nil
		}, time.Second*10).Should(Succeed())

		// Cleanup
		err := k8sClient.Delete(ctx, cgScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})
})

func validIntegrationTestScenarioForValidation() *v1beta2.IntegrationTestScenario {
	return &v1beta2.IntegrationTestScenario{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "integrationtestscenario",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "appstudio.redhat.com/v1alpha1",
					Kind:       "Application",
					Name:       "application-sample",
					UID:        types.UID("application-sample-uid"),
				},
			},
		},
		Spec: v1beta2.IntegrationTestScenarioSpec{
			Application: "application-sample",
			Params: []v1beta2.PipelineParameter{
				{Name: "pipeline-param-name", Value: "pipeline-param-value"},
			},
			ResolverRef: v1beta2.ResolverRef{
				Resolver: "git",
				Params: []v1beta2.ResolverParameter{
					{Name: "url", Value: "https://github.com/konflux-ci/integration-service"},
					{Name: "revision", Value: "main"},
					{Name: "pathInRepo", Value: "pipelines/integration.yaml"},
				},
			},
		},
	}
}
