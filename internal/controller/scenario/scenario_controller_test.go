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

	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	crwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"
)

var _ = Describe("ScenarioController", Ordered, func() {
	var (
		manager            ctrl.Manager
		scenarioReconciler *Reconciler
		req                ctrl.Request
		scheme             runtime.Scheme
		hasApp             *applicationapiv1alpha1.Application
		hasScenario        *v1beta2.IntegrationTestScenario
		failScenario       *v1beta2.IntegrationTestScenario
	)

	BeforeAll(func() {
		applicationName := "application-sample"

		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		scenarioName := "scenario-sample"

		hasScenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scenarioName,
				Namespace: "default",
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: applicationName,
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
		Expect(k8sClient.Create(ctx, hasScenario)).Should(Succeed())

		failScenario = &v1beta2.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "failscenario",
				Namespace: "default",
			},
			Spec: v1beta2.IntegrationTestScenarioSpec{
				Application: "idontexist",
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
		Expect(k8sClient.Create(ctx, failScenario)).Should(Succeed())

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      hasScenario.Name,
			},
		}

		webhookInstallOptions := &testEnv.WebhookInstallOptions

		klog.Info(webhookInstallOptions.LocalServingHost)
		klog.Info(webhookInstallOptions.LocalServingPort)
		klog.Info(webhookInstallOptions.LocalServingCertDir)

		var err error
		manager, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme: clientsetscheme.Scheme,
			WebhookServer: crwebhook.NewServer(crwebhook.Options{
				CertDir: webhookInstallOptions.LocalServingCertDir,
				Host:    webhookInstallOptions.LocalServingHost,
				Port:    webhookInstallOptions.LocalServingPort,
			}),
			Metrics: server.Options{
				BindAddress: "0", // disables metrics
			},
			LeaderElection: false,
		})
		Expect(err).NotTo(HaveOccurred())

		scenarioReconciler = NewScenarioReconciler(k8sClient, &logf.Log, &scheme)
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, hasApp)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, hasScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Delete(ctx, failScenario)
		Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default"}}
		_ = k8sClient.Delete(ctx, sa)
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default"}}
		_ = k8sClient.Delete(ctx, secret)
		rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default"}}
		_ = k8sClient.Delete(ctx, rb)
	})

	It("can create and return a new Reconciler object", func() {
		Expect(reflect.TypeOf(scenarioReconciler)).To(Equal(reflect.TypeOf(&Reconciler{})))
	})

	It("can Reconcile and creates the integration pipeline ServiceAccount, Secret, and RoleBinding", func() {
		result, err := scenarioReconciler.Reconcile(ctx, req)
		Expect(reflect.TypeOf(result)).To(Equal(reflect.TypeOf(reconcile.Result{})))
		Expect(err).ToNot(HaveOccurred())

		sa := &corev1.ServiceAccount{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name: tektonconsts.DefaultIntegrationPipelineServiceAccount, Namespace: "default",
		}, sa)
		Expect(err).ToNot(HaveOccurred())
		Expect(sa.ImagePullSecrets).To(ContainElement(
			corev1.LocalObjectReference{Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName},
		))

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name: tektonconsts.DefaultIntegrationPipelineImagePullSecretName, Namespace: "default",
		}, secret)
		Expect(err).ToNot(HaveOccurred())
		Expect(secret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))

		Eventually(func() error {
			rb := &rbacv1.RoleBinding{}
			return k8sClient.Get(ctx, types.NamespacedName{
				Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default",
			}, rb)
		}, "5s").Should(Succeed())

		rb := &rbacv1.RoleBinding{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: tektonconsts.DefaultIntegrationPipelineRoleBindingName, Namespace: "default",
		}, rb)).To(Succeed())
		Expect(rb.RoleRef.Name).To(Equal(tektonconsts.DefaultIntegrationPipelineClusterRoleName))
	})

	It("can setup a new controller manager with the given reconciler", func() {
		err := setupControllerWithManager(manager, scenarioReconciler)
		Expect(err).NotTo(HaveOccurred())
	})

	It("can Reconcile when scenario references non-existent application", func() {
		failReq := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      failScenario.Name,
			},
		}
		result, err := scenarioReconciler.Reconcile(ctx, failReq)
		Expect(reflect.TypeOf(result)).To(Equal(reflect.TypeOf(reconcile.Result{})))
		Expect(err).ToNot(HaveOccurred())
	})
})
