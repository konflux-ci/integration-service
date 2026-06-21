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

	"github.com/konflux-ci/integration-service/loader"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/helpers"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Scenario Adapter", Ordered, func() {
	var (
		adapter                 *Adapter
		hasApp                  *applicationapiv1alpha1.Application
		integrationTestScenario *v1beta2.IntegrationTestScenario
		logger                  helpers.IntegrationLogger
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
						{Name: "url", Value: "https://github.com/redhat-appstudio/integration-examples.git"},
						{Name: "revision", Value: "main"},
						{Name: "pathInRepo", Value: "pipelineruns/integration_pipelinerun_pass.yaml"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, integrationTestScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
	})

	JustBeforeEach(func() {
		adapter = NewAdapter(ctx, integrationTestScenario, logger, loader.NewMockLoader(), k8sClient)
		Expect(reflect.TypeOf(adapter)).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	It("can create a new Adapter instance", func() {
		Expect(reflect.TypeOf(NewAdapter(ctx, integrationTestScenario, logger, loader.NewMockLoader(), k8sClient))).To(Equal(reflect.TypeOf(&Adapter{})))
	})

	When("ServiceAccount, Secret, and RoleBinding do not exist", func() {
		AfterAll(func() {
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, sa)
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, secret)
			rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, rb)
		})

		It("creates the ServiceAccount, Secret, and RoleBinding", func() {
			result, err := adapter.EnsureIntegrationPipelineServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			Eventually(func(g Gomega) {
				sa := &corev1.ServiceAccount{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default",
				}, sa)).To(Succeed())
				g.Expect(sa.Name).To(Equal(tektonconsts.DefaultIntegrationPipelineServiceAccount))
				g.Expect(sa.ImagePullSecrets).To(ContainElement(
					corev1.LocalObjectReference{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName},
				))
			}, "5s").Should(Succeed())

			Eventually(func(g Gomega) {
				secret := &corev1.Secret{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default",
				}, secret)).To(Succeed())
				g.Expect(secret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
			}, "5s").Should(Succeed())

			Eventually(func(g Gomega) {
				rb := &rbacv1.RoleBinding{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default",
				}, rb)).To(Succeed())
				g.Expect(rb.RoleRef.Name).To(Equal(tektonconsts.DefaultIntegrationPipelineClusterRoleName))
				g.Expect(rb.RoleRef.Kind).To(Equal("ClusterRole"))
				g.Expect(rb.Subjects).To(HaveLen(1))
				g.Expect(rb.Subjects[0].Name).To(Equal(tektonconsts.DefaultIntegrationPipelineServiceAccount))
			}, "5s").Should(Succeed())
		})
	})

	When("ServiceAccount exists but Secret does not", func() {
		BeforeAll(func() {
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, sa)).Should(Succeed())
		})

		AfterAll(func() {
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, sa)
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, secret)
			rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, rb)
		})

		It("creates the Secret and links it to the existing ServiceAccount", func() {
			result, err := adapter.EnsureIntegrationPipelineServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			Eventually(func(g Gomega) {
				sa := &corev1.ServiceAccount{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default",
				}, sa)).To(Succeed())
				g.Expect(sa.ImagePullSecrets).To(ContainElement(
					corev1.LocalObjectReference{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName},
				))
			}, "5s").Should(Succeed())

			Eventually(func(g Gomega) {
				secret := &corev1.Secret{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default",
				}, secret)).To(Succeed())
				g.Expect(secret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))
			}, "5s").Should(Succeed())
		})
	})

	When("ServiceAccount and Secret exist but Secret is not linked", func() {
		BeforeAll(func() {
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default",
				},
			}
			Expect(k8sClient.Create(ctx, sa)).Should(Succeed())

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default",
				},
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
		})

		AfterAll(func() {
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, sa)
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, secret)
			rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, rb)
		})

		It("links the Secret to the ServiceAccount", func() {
			result, err := adapter.EnsureIntegrationPipelineServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			Eventually(func(g Gomega) {
				sa := &corev1.ServiceAccount{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default",
				}, sa)).To(Succeed())
				g.Expect(sa.ImagePullSecrets).To(ContainElement(
					corev1.LocalObjectReference{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName},
				))
			}, "5s").Should(Succeed())
		})
	})

	When("everything is already configured", func() {
		BeforeAll(func() {
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default",
				},
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName},
				},
			}
			Expect(k8sClient.Create(ctx, sa)).Should(Succeed())

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default",
				},
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			rb := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default",
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "ClusterRole",
					Name:     tektonconsts.DefaultIntegrationPipelineClusterRoleName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Name:      tektonconsts.DefaultIntegrationPipelineServiceAccount,
						Namespace: "default",
					},
				},
			}
			Expect(k8sClient.Create(ctx, rb)).Should(Succeed())
		})

		AfterAll(func() {
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, sa)
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, secret)
			rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, rb)
		})

		It("is idempotent and does not error", func() {
			result, err := adapter.EnsureIntegrationPipelineServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			result, err = adapter.EnsureIntegrationPipelineServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())
		})
	})

	When("ServiceAccount and Secret exist and are linked but RoleBinding is missing", func() {
		BeforeAll(func() {
			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default",
				},
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName},
				},
			}
			Expect(k8sClient.Create(ctx, sa)).Should(Succeed())

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default",
				},
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{
					corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
		})

		AfterAll(func() {
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, sa)
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, secret)
			rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default"}}
			_ = k8sClient.Delete(ctx, rb)
		})

		It("creates the RoleBinding", func() {
			result, err := adapter.EnsureIntegrationPipelineServiceAccountCreated()
			Expect(err).ToNot(HaveOccurred())
			Expect(result.CancelRequest).To(BeFalse())

			Eventually(func(g Gomega) {
				rb := &rbacv1.RoleBinding{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default",
				}, rb)).To(Succeed())
				g.Expect(rb.RoleRef.Name).To(Equal(tektonconsts.DefaultIntegrationPipelineClusterRoleName))
				g.Expect(rb.RoleRef.Kind).To(Equal("ClusterRole"))
			}, "5s").Should(Succeed())
		})
	})
})
