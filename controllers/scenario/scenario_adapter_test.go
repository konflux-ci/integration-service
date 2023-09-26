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

package scenario

import (
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta1"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Scenario Adapter", Ordered, func() {
	const (
		DefaultNamespace = "default"
		SampleRepoLink   = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
	)
	var (
		adapter                 *Adapter
		hasApp                  *applicationapiv1alpha1.Application
		integrationTestScenario *v1beta1.IntegrationTestScenario
		invalidScenario         *v1beta1.IntegrationTestScenario
		logger                  helpers.IntegrationLogger
		envNamespace            string = DefaultNamespace
		env                     applicationapiv1alpha1.Environment
	)

	BeforeAll(func() {

		logger = helpers.IntegrationLogger{Logger: ctrl.Log}

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

		integrationTestScenario = &v1beta1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta1.IntegrationTestScenarioSpec{
				Application: "application-sample",
				ResolverRef: v1beta1.ResolverRef{
					Resolver: "git",
					Params: []v1beta1.ResolverParameter{
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
				Environment: v1beta1.TestEnvironment{
					Name: "envname",
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())

		invalidScenario = &v1beta1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-fail",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta1.IntegrationTestScenarioSpec{
				Application: "perpetum-mobile",
				ResolverRef: v1beta1.ResolverRef{
					Resolver: "git",
					Params: []v1beta1.ResolverParameter{
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
				Environment: v1beta1.TestEnvironment{
					Name: "invEnv",
					Type: "POC",
					Configuration: &applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, invalidScenario)).Should(Succeed())

	})

	JustBeforeEach(func() {

		env = applicationapiv1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "envname",
				Namespace: envNamespace,
			},
			Spec: applicationapiv1alpha1.EnvironmentSpec{
				Type:               "POC",
				DisplayName:        "my-environment",
				DeploymentStrategy: applicationapiv1alpha1.DeploymentStrategy_Manual,
				ParentEnvironment:  "",
				Tags:               []string{},
				Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
					Env: []applicationapiv1alpha1.EnvVarPair{
						{
							Name:  "var_name",
							Value: "test",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, &env)).Should(Succeed())

		adapter = NewAdapter(hasApp, integrationTestScenario, logger, k8sClient, ctx)
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, &env)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, invalidScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

	})

	It("can create a new Adapter instance", func() {
		Expect(reflect.TypeOf(NewAdapter(hasApp, integrationTestScenario, logger, k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("can create a new Adapter instance with invalid scenario", func() {
		Expect(reflect.TypeOf(NewAdapter(hasApp, invalidScenario, logger, k8sClient, ctx))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("EnsureCreatedScenarioIsValid without app", func() {
		a := NewAdapter(nil, integrationTestScenario, logger, k8sClient, ctx)

		Eventually(func() bool {
			result, err := a.EnsureCreatedScenarioIsValid()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

	})

	When("environment is in a different namespace than scenario", func() {

		var namespace *corev1.Namespace

		BeforeEach(func() {
			envNamespace = "separatenamespace"

			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: envNamespace,
				},
			}

			Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
		})

		AfterEach(func() {
			envNamespace = DefaultNamespace

			err := k8sClient.Delete(ctx, namespace)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("ensure the scenario status is invalid", func() {

			Eventually(func() bool {
				result, err := adapter.EnsureCreatedScenarioIsValid()
				return !result.CancelRequest && err == nil
			}, time.Second*10).Should(BeTrue())
			Expect(meta.IsStatusConditionFalse(integrationTestScenario.Status.Conditions, gitops.IntegrationTestScenarioValid)).To(BeTrue())
		})

	})

	It("ensures the integrationTestPipelines are created", func() {
		Eventually(func() bool {
			result, err := adapter.EnsureCreatedScenarioIsValid()
			return !result.CancelRequest && err == nil
		}, time.Second*20).Should(BeTrue())
	})

	It("ensures the Scenario status can be marked as invalid", func() {
		SetScenarioIntegrationStatusAsInvalid(invalidScenario, "Test message")
		Expect(invalidScenario).NotTo(BeNil())
		Expect(invalidScenario.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionFalse(invalidScenario.Status.Conditions, gitops.IntegrationTestScenarioValid)).To(BeTrue())
	})

	It("ensures the Scenario status can be marked as valid", func() {
		SetScenarioIntegrationStatusAsValid(integrationTestScenario, "Test message")
		Expect(integrationTestScenario).NotTo(BeNil())
		Expect(integrationTestScenario.Status.Conditions).NotTo(BeNil())
		Expect(meta.IsStatusConditionTrue(integrationTestScenario.Status.Conditions, gitops.IntegrationTestScenarioValid)).To(BeTrue())
	})

})
