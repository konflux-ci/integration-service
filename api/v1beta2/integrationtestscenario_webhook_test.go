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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	types "k8s.io/apimachinery/pkg/types"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("IntegrationTestScenario webhook", func() {

	var integrationTestScenario, integrationTestScenarioInvalidGitResolver *IntegrationTestScenario

	BeforeEach(func() {
		integrationTestScenario = &IntegrationTestScenario{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1beta2",
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

	It("should fail to create scenario with long name", func() {
		integrationTestScenario.Name = "this-name-is-too-long-it-has-64-characters-and-we-allow-max-63ch"
		Expect(k8sClient.Create(ctx, integrationTestScenario)).ShouldNot(Succeed())
	})

	It("should fail to create scenario with snapshot parameter set", func() {
		integrationTestScenario.Spec.Params = append(integrationTestScenario.Spec.Params, PipelineParameter{Name: "SNAPSHOT"})
		Expect(k8sClient.Create(ctx, integrationTestScenario)).ShouldNot(Succeed())
	})

	It("should success to create scenario when only url in git reolver params", func() {
		integrationTestScenarioInvalidGitResolver = &IntegrationTestScenario{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "appstudio.redhat.com/v1beta2",
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

		Expect(k8sClient.Create(ctx, integrationTestScenarioInvalidGitResolver)).Should(Succeed())
	})

	It("should set the value of 'Spec.ResovlerRef.ResourceKind' to 'pipeline' if not already set", func() {
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())
		Eventually(func() error {
			appliedScenario := &IntegrationTestScenario{}
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

	It("should not set the value of 'Spec.ResovlerRef.ResourceKind' if it is already set", func() {
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

	It("should fail to create scenario when in git resolver params url+repo", func() {

		integrationTestScenarioInvalidGitResolver.Spec.ResolverRef.Params = append(integrationTestScenarioInvalidGitResolver.Spec.ResolverRef.Params, ResolverParameter{Name: "repo", Value: "my-repository-name"})
		integrationTestScenarioInvalidGitResolver.Spec.ResolverRef.Params = append(integrationTestScenarioInvalidGitResolver.Spec.ResolverRef.Params, ResolverParameter{Name: "org", Value: "my-org-name"})
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
})
