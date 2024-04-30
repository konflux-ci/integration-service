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
	"bytes"
	"reflect"
	"time"

	"github.com/redhat-appstudio/integration-service/loader"
	"github.com/tonglil/buflogr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1beta2"
	"github.com/redhat-appstudio/integration-service/gitops"
	"github.com/redhat-appstudio/integration-service/helpers"

	"k8s.io/apimachinery/pkg/api/errors"
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
		integrationTestScenario *v1beta2.IntegrationTestScenario
		invalidScenario         *v1beta2.IntegrationTestScenario
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

		invalidScenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-fail",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "perpetum-mobile",
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

		adapter = NewAdapter(ctx, hasApp, integrationTestScenario, logger, loader.NewMockLoader(), k8sClient)
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
		Expect(reflect.TypeOf(NewAdapter(ctx, hasApp, integrationTestScenario, logger, loader.NewMockLoader(), k8sClient))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("can create a new Adapter instance with invalid scenario", func() {
		Expect(reflect.TypeOf(NewAdapter(ctx, hasApp, invalidScenario, logger, loader.NewMockLoader(), k8sClient))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("EnsureCreatedScenarioIsValid without app", func() {
		a := NewAdapter(ctx, nil, integrationTestScenario, logger, loader.NewMockLoader(), k8sClient)

		Eventually(func() bool {
			result, err := a.EnsureCreatedScenarioIsValid()
			return !result.CancelRequest && err == nil
		}, time.Second*10).Should(BeTrue())

	})

	It("ensures the integrationTestPipelines are created", func() {
		Eventually(func() bool {
			result, err := adapter.EnsureCreatedScenarioIsValid()
			return !result.CancelRequest && err == nil
		}, time.Second*20).Should(BeTrue())
	})

	When("IntegrationTestScenario is deleted while environment resources are still on the cluster", func() {
		var (
			ephemeralEnvironment  *applicationapiv1alpha1.Environment
			deploymentTargetClaim *applicationapiv1alpha1.DeploymentTargetClaim
		)

		BeforeEach(func() {
			deploymentTargetClaim = &applicationapiv1alpha1.DeploymentTargetClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dtcname",
					Namespace: "default",
				},
				Spec: applicationapiv1alpha1.DeploymentTargetClaimSpec{
					DeploymentTargetClassName: "dtcls-name",
					TargetName:                "deploymentTarget",
				},
			}
			Expect(k8sClient.Create(ctx, deploymentTargetClaim)).Should(Succeed())

			ephemeralEnvironment = &applicationapiv1alpha1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ephemeral-env",
					Namespace: "default",
					Labels: map[string]string{
						gitops.SnapshotTestScenarioLabel: integrationTestScenario.Name,
					},
				},
				Spec: applicationapiv1alpha1.EnvironmentSpec{
					Type:               "POC",
					DisplayName:        "my-environment",
					DeploymentStrategy: applicationapiv1alpha1.DeploymentStrategy_Manual,
					ParentEnvironment:  "",
					Tags:               []string{"ephemeral"},
					Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{
							{
								Name:  "var_name",
								Value: "test",
							},
						},
						Target: applicationapiv1alpha1.EnvironmentTarget{
							DeploymentTargetClaim: applicationapiv1alpha1.DeploymentTargetClaimConfig{
								ClaimName: deploymentTargetClaim.Name,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ephemeralEnvironment)).Should(Succeed())
		})

		AfterEach(func() {
			err := k8sClient.Delete(ctx, deploymentTargetClaim)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Delete(ctx, ephemeralEnvironment)
			Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		})

		It("ensures the integrationTestScenario deletion cleans up related environment resources", func() {
			var buf bytes.Buffer
			log := helpers.IntegrationLogger{Logger: buflogr.NewWithBuffer(&buf)}

			deletedIntegrationTestScenario := integrationTestScenario.DeepCopy()
			controllerutil.AddFinalizer(deletedIntegrationTestScenario, helpers.IntegrationTestScenarioFinalizer)
			now := metav1.NewTime(metav1.Now().Add(time.Second * 1))
			deletedIntegrationTestScenario.SetDeletionTimestamp(&now)
			adapter = NewAdapter(ctx, hasApp, deletedIntegrationTestScenario, log, loader.NewMockLoader(), k8sClient)

			Eventually(func() bool {
				result, err := adapter.EnsureDeletedScenarioResourcesAreCleanedUp()
				return !result.CancelRequest && err == nil
			}, time.Second*20).Should(BeTrue())

			expectedLogEntry := "Removed Finalizer from the IntegrationTestScenario"
			Expect(buf.String()).Should(ContainSubstring(expectedLogEntry))

			Expect(controllerutil.ContainsFinalizer(deletedIntegrationTestScenario, helpers.IntegrationTestScenarioFinalizer)).To(BeFalse())
		})
	})
})
